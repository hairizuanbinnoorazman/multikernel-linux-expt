//go:build linux

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func makeExt4(t *testing.T, sparse bool) PreparedImage {
	t.Helper()
	if _, err := exec.LookPath("mke2fs"); err != nil {
		t.Skip("mke2fs is unavailable")
	}
	path := filepath.Join(t.TempDir(), "root.ext4")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if sparse {
		err = file.Truncate(64 << 20)
	} else {
		err = file.Close()
		if err == nil {
			err = exec.Command("fallocate", "-l", strconv.Itoa(64<<20), path).Run()
		}
		file = nil
	}
	if file != nil {
		err = errors.Join(err, file.Close())
	}
	if err != nil {
		t.Fatal(err)
	}
	uuid := "11111111-2222-4333-8444-555555555555"
	if output, err := exec.Command("mke2fs", "-q", "-F", "-t", "ext4", "-E", "nodiscard", "-N", "4096", "-U", uuid, path).CombinedOutput(); err != nil {
		t.Fatalf("mke2fs: %v: %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	return PreparedImage{Path: path, ImageID: "test-image", FilesystemUUID: uuid,
		SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: hex.EncodeToString(digest[:])}
}

func TestLinuxBackendInspectsExt4IdentityQuotaAndCleanState(t *testing.T) {
	image := makeExt4(t, false)
	backend := &LinuxBackend{RequiredUID: os.Getuid()}
	if err := backend.Inspect(context.Background(), image); err != nil {
		t.Fatal(err)
	}
	wrong := image
	wrong.FilesystemUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if err := backend.Inspect(context.Background(), wrong); err == nil {
		t.Fatal("wrong UUID accepted")
	}
	wrong = image
	wrong.InodeLimit++
	if err := backend.Inspect(context.Background(), wrong); err == nil {
		t.Fatal("wrong inode capacity accepted")
	}

	file, err := os.OpenFile(image.Path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	state := make([]byte, 2)
	if _, err = file.ReadAt(state, ext4SuperblockOffset+0x3a); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint16(state, binary.LittleEndian.Uint16(state)&^1)
	if _, err = file.WriteAt(state, ext4SuperblockOffset+0x3a); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = backend.Inspect(context.Background(), image); err == nil {
		t.Fatal("dirty filesystem state accepted")
	}
}

func TestLinuxBackendRejectsSparseAndMultiplyLinkedImages(t *testing.T) {
	backend := &LinuxBackend{RequiredUID: os.Getuid()}
	sparse := makeExt4(t, true)
	if err := backend.Inspect(context.Background(), sparse); err == nil {
		t.Fatal("sparse backing image accepted")
	}
	image := makeExt4(t, false)
	if err := os.Link(image.Path, image.Path+".other"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Inspect(context.Background(), image); err == nil {
		t.Fatal("multiply-linked backing image accepted")
	}
}

func TestLinuxBackendOperationsRejectPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &LinuxBackend{RuntimeDir: filepath.Join(t.TempDir(), "runtime")}
	for name, operation := range map[string]func() error{
		"inspect": func() error { return backend.Inspect(ctx, PreparedImage{}) },
		"start":   func() error { return backend.Start(ctx, Export{}) },
		"observe": func() error { _, err := backend.Observe(ctx, Export{}); return err },
		"stop":    func() error { _, err := backend.Stop(ctx, Export{}); return err },
		"check":   func() error { _, err := backend.OfflineCheck(ctx, Export{}); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("pre-cancelled operation error = %v", err)
			}
		})
	}
	if _, err := os.Stat(backend.RuntimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-cancelled backend created runtime state: %v", err)
	}
}

type cancelAfterChecks struct {
	checks atomic.Int32
	after  int32
	done   chan struct{}
	once   sync.Once
}

func (c *cancelAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterChecks) Done() <-chan struct{}       { return c.done }
func (c *cancelAfterChecks) Value(any) any               { return nil }
func (c *cancelAfterChecks) Err() error {
	if c.checks.Add(1) >= c.after {
		c.once.Do(func() { close(c.done) })
		return context.Canceled
	}
	return nil
}

func TestLinuxBackendInspectionChecksCancellationBetweenChunks(t *testing.T) {
	image := makeExt4(t, false)
	ctx := &cancelAfterChecks{after: 4, done: make(chan struct{})}
	backend := &LinuxBackend{RequiredUID: os.Getuid()}
	if err := backend.Inspect(ctx, image); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-inspection cancellation error = %v", err)
	}
	if checks := ctx.checks.Load(); checks != ctx.after {
		t.Fatalf("context checks = %d, want %d", checks, ctx.after)
	}
}

func TestLinuxBackendProcessIdentityGracefulStopAndOfflineCheck(t *testing.T) {
	image := makeExt4(t, false)
	directory := t.TempDir()
	binaryPath := filepath.Join(directory, "server")
	sourcePath := filepath.Join(directory, "server.c")
	source := `#include <signal.h>
#include <stdio.h>
#include <unistd.h>
static volatile sig_atomic_t done;
static void stop(int signal_number) { (void)signal_number; done = 1; }
int main(int argc, char **argv) {
  if (argc != 6) return 2;
  signal(SIGTERM, stop); signal(SIGINT, stop);
  printf("MKNBD_SERVER_READY image=%s image_id=%s generation=%s size=67108864 port=%s\n", argv[2], argv[4], argv[5], argv[3]); fflush(stdout);
  while (!done) pause();
  puts("MKNBD_SERVER_CLOSED reads=2 read_bytes=8192 writes=3 write_bytes=12288 flushes=4"); fflush(stdout);
  return 0;
}

`
	if err := os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cc", "-O2", "-Wall", "-Wextra", "-Werror", sourcePath, "-o", binaryPath).CombinedOutput(); err != nil {
		t.Skipf("C compiler unavailable: %v: %s", err, output)
	}
	backend := &LinuxBackend{Binary: binaryPath, RuntimeDir: filepath.Join(directory, "run"), RequiredUID: os.Getuid(), ReadyTimeout: time.Second, StopTimeout: 20 * time.Millisecond}
	value := Export{SandboxID: "box", SandboxGeneration: sandboxGeneration,
		ExportGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PreparedImage: image, State: "ACTIVE"}
	if err := backend.Start(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	observed, err := backend.Observe(context.Background(), value)
	if err != nil || !observed.Active || observed.Generation != value.ExportGeneration {
		t.Fatalf("observation = %+v, %v", observed, err)
	}
	// A fresh backend has no in-memory child handle. It must adopt and stop the
	// exact durable process record, as happens after mkruntimed restarts.
	recovered := &LinuxBackend{Binary: binaryPath, RuntimeDir: backend.RuntimeDir, RequiredUID: os.Getuid(), ReadyTimeout: time.Second, StopTimeout: 20 * time.Millisecond}
	observed, err = recovered.Observe(context.Background(), value)
	if err != nil || !observed.Active || observed.Generation != value.ExportGeneration {
		t.Fatalf("recovered observation = %+v, %v", observed, err)
	}
	counters, err := recovered.Stop(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if counters.Reads != 2 || counters.ReadBytes != 8192 || counters.Writes != 3 || counters.WrittenBytes != 12288 || counters.Flushes != 4 {
		t.Fatalf("counters = %+v", counters)
	}
	observed, err = recovered.Observe(context.Background(), value)
	if err != nil || observed.Active || observed.Counters != counters {
		t.Fatalf("post-stop observation = %+v, %v", observed, err)
	}
	result, err := backend.OfflineCheck(context.Background(), value)
	if err != nil || len(result) != len("e2fsck-clean-sha256:")+64 {
		t.Fatalf("offline check = %q, %v", result, err)
	}
}

func validBackendLease(path string) Export {
	return Export{
		SandboxID: "box", SandboxGeneration: sandboxGeneration,
		ExportGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: "ACTIVE",
		PreparedImage: PreparedImage{
			Path: path, ImageID: "image", FilesystemUUID: "11111111-2222-4333-8444-555555555555",
			SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061,
			SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
}

func TestProcessRecordIsPrivateStableAndExactBeforeUse(t *testing.T) {
	directory := t.TempDir()
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	record := processRecord{Version: 1, PID: os.Getpid(), StartTime: start, Path: value.Path,
		Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "record.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if observed, err := readRecord(path, value); err != nil || observed != record {
		t.Fatalf("record = %+v, %v", observed, err)
	}
	conflict := value
	conflict.ExportGeneration = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err = readRecord(path, conflict); err == nil {
		t.Fatal("process record was accepted for a conflicting lease")
	}
	if err = os.Link(path, path+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err = readRecord(path, value); err == nil {
		t.Fatal("hard-linked process record was accepted")
	}
}

func TestCounterEvidenceRequiresExactReadyAndCanonicalTerminalClose(t *testing.T) {
	directory := t.TempDir()
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	path := filepath.Join(directory, "server.log")
	closed := []byte("MKNBD_SERVER_CLOSED reads=2 read_bytes=8192 writes=3 write_bytes=12288 flushes=4\n")
	write := func(content []byte) {
		t.Helper()
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(append(readyMarker(value), closed...))
	if counters, err := parseCounters(path, value); err != nil || counters != (Counters{Reads: 2, ReadBytes: 8192, Writes: 3, WrittenBytes: 12288, Flushes: 4}) {
		t.Fatalf("counters = %+v, %v", counters, err)
	}
	conflict := value
	conflict.ImageID = "other"
	if _, err := parseCounters(path, conflict); err == nil {
		t.Fatal("counter log for another export was accepted")
	}
	write(append(append(readyMarker(value), closed...), []byte("trailing output\n")...))
	if _, err := parseCounters(path, value); err == nil {
		t.Fatal("non-terminal close record was accepted")
	}
	write(closed)
	if _, err := parseCounters(path, value); err == nil {
		t.Fatal("close record without exact readiness evidence was accepted")
	}
}

func TestBackendLeaseValidationPrecedesPathDerivation(t *testing.T) {
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	value.SandboxGeneration = "short"
	backend := &LinuxBackend{RuntimeDir: t.TempDir()}
	if _, err := backend.Observe(context.Background(), value); err == nil {
		t.Fatal("malformed lease reached backend path derivation")
	}
}

func TestMissingEphemeralRuntimeDirectoryObservesExportAbsent(t *testing.T) {
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	backend := &LinuxBackend{RuntimeDir: filepath.Join(t.TempDir(), "missing")}
	observed, err := backend.Observe(context.Background(), value)
	if err != nil || observed != (Observation{}) {
		t.Fatalf("missing runtime observation = %+v, %v", observed, err)
	}
}

func TestManagedBackendSignalsBeforeStopTimeout(t *testing.T) {
	directory := t.TempDir()
	binaryPath := filepath.Join(directory, "server")
	sourcePath := filepath.Join(directory, "server.c")
	source := `#include <signal.h>
#include <stdio.h>
#include <unistd.h>
static volatile sig_atomic_t done;
static void stop(int signal_number) { (void)signal_number; done = 1; }
int main(int argc, char **argv) {
  if (argc != 6) return 2;
  signal(SIGTERM, stop);
  printf("MKNBD_SERVER_READY image=%s image_id=%s generation=%s size=67108864 port=%s\n", argv[2], argv[4], argv[5], argv[3]); fflush(stdout);
  while (!done) pause();
  puts("MKNBD_SERVER_CLOSED reads=0 read_bytes=0 writes=0 write_bytes=0 flushes=0"); fflush(stdout);
  return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cc", "-O2", "-Wall", "-Wextra", "-Werror", sourcePath, "-o", binaryPath).CombinedOutput(); err != nil {
		t.Skipf("C compiler unavailable: %v: %s", err, output)
	}
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	backend := &LinuxBackend{Binary: binaryPath, RuntimeDir: filepath.Join(directory, "run"),
		RequiredUID: os.Getuid(), ReadyTimeout: time.Second, StopTimeout: 3 * time.Second}
	if err := backend.Start(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := backend.Stop(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("managed stop waited before signaling: %v", elapsed)
	}
}
