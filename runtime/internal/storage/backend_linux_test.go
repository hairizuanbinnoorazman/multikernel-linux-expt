//go:build linux

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

func makeExt4(t *testing.T, sparse bool) PreparedImage {
	t.Helper()
	if _, err := exec.LookPath("mke2fs"); err != nil {
		t.Skip("mke2fs is unavailable")
	}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "root.ext4")
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
	identity, err := backend.Inspect(context.Background(), image)
	if err != nil || identity.Device == 0 || identity.Inode == 0 {
		t.Fatal(err)
	}
	wrong := image
	wrong.FilesystemUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if _, err := backend.Inspect(context.Background(), wrong); err == nil {
		t.Fatal("wrong UUID accepted")
	}
	wrong = image
	wrong.InodeLimit++
	if _, err := backend.Inspect(context.Background(), wrong); err == nil {
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
	if _, err = backend.Inspect(context.Background(), image); err == nil {
		t.Fatal("dirty filesystem state accepted")
	}
}

func TestLinuxBackendRejectsSparseAndMultiplyLinkedImages(t *testing.T) {
	backend := &LinuxBackend{RequiredUID: os.Getuid()}
	sparse := makeExt4(t, true)
	if _, err := backend.Inspect(context.Background(), sparse); err == nil {
		t.Fatal("sparse backing image accepted")
	}
	image := makeExt4(t, false)
	if err := os.Link(image.Path, image.Path+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Inspect(context.Background(), image); err == nil {
		t.Fatal("multiply-linked backing image accepted")
	}
}

func TestLinuxBackendRejectsImageParentReplacementAfterOpen(t *testing.T) {
	for _, name := range []string{"inspect", "start", "offline-check"} {
		t.Run(name, func(t *testing.T) {
			image := makeExt4(t, false)
			inspector := &LinuxBackend{RequiredUID: os.Getuid()}
			identity, err := inspector.Inspect(context.Background(), image)
			if err != nil {
				t.Fatal(err)
			}
			value := validBackendLease(image.Path)
			value.PreparedImage, value.ImageIdentity = image, identity
			parent := filepath.Dir(image.Path)
			moved := parent + ".held"
			var once sync.Once
			backend := &LinuxBackend{RequiredUID: os.Getuid(), Binary: "/bin/false", CheckBinary: "/bin/true",
				RuntimeDir: filepath.Join(t.TempDir(), "run")}
			backend.afterImageOpen = func() {
				once.Do(func() {
					if err := os.Rename(parent, moved); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(parent, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(image.Path, []byte("replacement"), 0600); err != nil {
						t.Fatal(err)
					}
				})
			}
			switch name {
			case "inspect":
				_, err = backend.Inspect(context.Background(), image)
			case "start":
				err = backend.Start(context.Background(), value)
			case "offline-check":
				_, err = backend.OfflineCheck(context.Background(), value)
			}
			if err == nil {
				t.Fatal("image parent replacement was accepted")
			}
			if data, readErr := os.ReadFile(image.Path); readErr != nil || string(data) != "replacement" {
				t.Fatalf("replacement image changed: %q, %v", data, readErr)
			}
			if err = os.Remove(image.Path); err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(parent); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(moved, parent); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLinuxBackendRejectsServerExecutableReplacementAfterOpen(t *testing.T) {
	image := makeExt4(t, false)
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(directory, "server")
	sourcePath := filepath.Join(directory, "server.c")
	if err := os.WriteFile(sourcePath, []byte("#include <unistd.h>\nint main(void) { for (;;) pause(); }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("cc", "-O2", "-Wall", "-Wextra", "-Werror", sourcePath, "-o", binaryPath).CombinedOutput(); err != nil {
		t.Skipf("C compiler unavailable: %v: %s", err, output)
	}
	if err := os.Chmod(binaryPath, 0755); err != nil {
		t.Fatal(err)
	}
	backend := &LinuxBackend{Binary: binaryPath, RuntimeDir: filepath.Join(directory, "run"), RequiredUID: os.Getuid()}
	identity, err := backend.Inspect(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	value := validBackendLease(image.Path)
	value.PreparedImage, value.ImageIdentity = image, identity
	replacement := []byte("replacement-server")
	backend.afterBinaryOpen = func() {
		if renameErr := os.Rename(binaryPath, binaryPath+".held"); renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(binaryPath, replacement, 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	err = backend.Start(context.Background(), value)
	if err == nil || !strings.Contains(err.Error(), "executable pathname changed") {
		t.Fatalf("server executable replacement error = %v", err)
	}
	if data, readErr := os.ReadFile(binaryPath); readErr != nil || !bytes.Equal(data, replacement) {
		t.Fatalf("replacement executable changed: %q, %v", data, readErr)
	}
	entries, readErr := os.ReadDir(backend.RuntimeDir)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("replacement failure artifacts = %v, %v", entries, readErr)
	}
}

func TestLinuxBackendOperationsRejectPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &LinuxBackend{RuntimeDir: filepath.Join(t.TempDir(), "runtime")}
	for name, operation := range map[string]func() error{
		"inspect": func() error { _, err := backend.Inspect(ctx, PreparedImage{}); return err },
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
	if _, err := backend.Inspect(ctx, image); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-inspection cancellation error = %v", err)
	}
	if checks := ctx.checks.Load(); checks != ctx.after {
		t.Fatalf("context checks = %d, want %d", checks, ctx.after)
	}
}

func imageLockAvailable(t *testing.T, path string) bool {
	t.Helper()
	descriptor, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(descriptor)
	if err = unix.Flock(descriptor, unix.LOCK_EX|unix.LOCK_NB); err == nil {
		_ = unix.Flock(descriptor, unix.LOCK_UN)
		return true
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return false
	}
	t.Fatalf("probe image lock: %v", err)
	return false
}

func TestLinuxBackendProcessIdentityGracefulStopAndOfflineCheck(t *testing.T) {
	image := makeExt4(t, false)
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
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
	if err := os.Chmod(binaryPath, 0755); err != nil {
		t.Fatal(err)
	}
	checkerSource, err := exec.LookPath("e2fsck")
	if err != nil {
		t.Skip("e2fsck is unavailable")
	}
	checkerData, err := os.ReadFile(checkerSource)
	if err != nil {
		t.Fatal(err)
	}
	checkerPath := filepath.Join(directory, "e2fsck")
	if err = os.WriteFile(checkerPath, checkerData, 0755); err != nil {
		t.Fatal(err)
	}
	backend := &LinuxBackend{Binary: binaryPath, CheckBinary: checkerPath, RuntimeDir: filepath.Join(directory, "run"), RequiredUID: os.Getuid(), ReadyTimeout: time.Second, StopTimeout: 20 * time.Millisecond}
	value := Export{SandboxID: "box", SandboxGeneration: sandboxGeneration,
		ExportGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PreparedImage: image, State: "ACTIVE"}
	identity, inspectErr := backend.Inspect(context.Background(), image)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	value.ImageIdentity = identity
	if err := backend.Start(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if imageLockAvailable(t, image.Path) {
		t.Fatal("server did not retain the inspection lock through inherited fd 3")
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
	recordPath, _ := backend.paths(value)
	recordData, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var conflictingRecord processRecord
	if err = json.Unmarshal(recordData, &conflictingRecord); err != nil {
		t.Fatal(err)
	}
	conflictingRecord.BinaryInode++
	conflictingData, err := json.Marshal(conflictingRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(recordPath, append(conflictingData, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = recovered.Observe(context.Background(), value); err == nil || !strings.Contains(err.Error(), "executable conflicts") {
		t.Fatalf("conflicting executable observation error = %v", err)
	}
	if data, readErr := os.ReadFile(recordPath); readErr != nil || !bytes.Equal(data, append(conflictingData, '\n')) {
		t.Fatalf("conflicting process record was not preserved: %q, %v", data, readErr)
	}
	if err = os.WriteFile(recordPath, recordData, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(binaryPath, binaryPath+".launched"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(binaryPath, []byte("replacement-server"), 0600); err != nil {
		t.Fatal(err)
	}
	observed, err = recovered.Observe(context.Background(), value)
	if err != nil || !observed.Active || observed.Generation != value.ExportGeneration {
		t.Fatalf("public executable replacement observation = %+v, %v", observed, err)
	}
	movedRuntime := backend.RuntimeDir + ".original"
	if err = os.Rename(backend.RuntimeDir, movedRuntime); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(backend.RuntimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	replacementMarker := filepath.Join(backend.RuntimeDir, "replacement")
	if err = os.WriteFile(replacementMarker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = recovered.Observe(context.Background(), value); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replacement runtime observation error = %v", err)
	}
	if data, readErr := os.ReadFile(replacementMarker); readErr != nil || string(data) != "preserve" {
		t.Fatalf("replacement runtime directory changed: %q, %v", data, readErr)
	}
	if err = os.Remove(replacementMarker); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(backend.RuntimeDir); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(movedRuntime, backend.RuntimeDir); err != nil {
		t.Fatal(err)
	}
	counters, err := recovered.Stop(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if counters.Reads != 2 || counters.ReadBytes != 8192 || counters.Writes != 3 || counters.WrittenBytes != 12288 || counters.Flushes != 4 {
		t.Fatalf("counters = %+v", counters)
	}
	if !imageLockAvailable(t, image.Path) {
		t.Fatal("server image lock remained held after exact process stop")
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

func TestStorageStartProtectsExistingArtifactsAndCleansOwnFailures(t *testing.T) {
	image := makeExt4(t, false)
	inspector := &LinuxBackend{RequiredUID: os.Getuid()}
	identity, err := inspector.Inspect(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	value := validBackendLease(image.Path)
	value.PreparedImage, value.ImageIdentity = image, identity
	server := executableScript(t, "exit 0")
	t.Run("unsafe stale log", func(t *testing.T) {
		backend := &LinuxBackend{Binary: server, RuntimeDir: filepath.Join(t.TempDir(), "run"), RequiredUID: os.Getuid()}
		if err := os.Mkdir(backend.RuntimeDir, 0700); err != nil {
			t.Fatal(err)
		}
		_, logPath := backend.paths(value)
		target := filepath.Join(backend.RuntimeDir, "target")
		if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, logPath); err != nil {
			t.Fatal(err)
		}
		if err := backend.Start(t.Context(), value); err == nil {
			t.Fatal("unsafe stale log was replaced")
		}
		if data, err := os.ReadFile(target); err != nil || string(data) != "preserve" {
			t.Fatalf("stale-log target changed: %q, %v", data, err)
		}
	})

	t.Run("record collision preserves prior log", func(t *testing.T) {
		backend := &LinuxBackend{Binary: server, RuntimeDir: filepath.Join(t.TempDir(), "run"), RequiredUID: os.Getuid()}
		if err := os.Mkdir(backend.RuntimeDir, 0700); err != nil {
			t.Fatal(err)
		}
		recordPath, logPath := backend.paths(value)
		if err := os.WriteFile(recordPath, []byte("prior-record"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(logPath, []byte("prior-log"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := backend.Start(t.Context(), value); err == nil {
			t.Fatal("existing process record was overwritten")
		}
		if data, err := os.ReadFile(logPath); err != nil || string(data) != "prior-log" {
			t.Fatalf("record collision changed prior log: %q, %v", data, err)
		}
	})

	t.Run("command start failure leaves no artifacts", func(t *testing.T) {
		serverDirectory := t.TempDir()
		if err := os.Chmod(serverDirectory, 0700); err != nil {
			t.Fatal(err)
		}
		backend := &LinuxBackend{Binary: filepath.Join(serverDirectory, "missing-server"), RuntimeDir: filepath.Join(t.TempDir(), "run"), RequiredUID: os.Getuid()}
		if err := backend.Start(t.Context(), value); err == nil {
			t.Fatal("missing storage server was started")
		}
		entries, err := os.ReadDir(backend.RuntimeDir)
		if (err != nil && !errors.Is(err, os.ErrNotExist)) || len(entries) != 0 {
			t.Fatalf("failed start artifacts = %v, %v", entries, err)
		}
	})
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
		ImageIdentity: ImageIdentity{Device: 1, Inode: 2},
	}
}

func executableScript(t *testing.T, body string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "command")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOfflineCheckIsBoundedAndHashesCombinedEvidence(t *testing.T) {
	image := makeExt4(t, false)
	inspector := &LinuxBackend{RequiredUID: os.Getuid()}
	identity, err := inspector.Inspect(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	value := validBackendLease(image.Path)
	value.PreparedImage = image
	value.ImageIdentity = identity
	t.Run("rejects checker replacement", func(t *testing.T) {
		checker := executableScript(t, "printf original-evidence")
		replacement := []byte("#!/bin/sh\nexit 0\n")
		backend := &LinuxBackend{CheckBinary: checker, CheckTimeout: time.Second, RequiredUID: os.Getuid()}
		backend.afterCheckOpen = func() {
			if renameErr := os.Rename(checker, checker+".held"); renameErr != nil {
				t.Fatal(renameErr)
			}
			if writeErr := os.WriteFile(checker, replacement, 0700); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		if _, err := backend.OfflineCheck(context.Background(), value); err == nil || !strings.Contains(err.Error(), "executable pathname changed") {
			t.Fatalf("checker replacement error = %v", err)
		}
		if data, readErr := os.ReadFile(checker); readErr != nil || !bytes.Equal(data, replacement) {
			t.Fatalf("replacement checker changed: %q, %v", data, readErr)
		}
	})
	t.Run("retains exclusive lock", func(t *testing.T) {
		control := t.TempDir()
		started := filepath.Join(control, "started")
		release := filepath.Join(control, "release")
		body := fmt.Sprintf("printf started >%q\nwhile [ ! -e %q ]; do sleep 0.01; done", started, release)
		backend := &LinuxBackend{CheckBinary: executableScript(t, body), CheckTimeout: time.Second, RequiredUID: os.Getuid()}
		result := make(chan error, 1)
		go func() {
			_, checkErr := backend.OfflineCheck(context.Background(), value)
			result <- checkErr
		}()
		defer os.WriteFile(release, []byte("release"), 0600)
		deadline := time.Now().Add(time.Second)
		for {
			if _, statErr := os.Stat(started); statErr == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("offline checker did not start")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if imageLockAvailable(t, image.Path) {
			t.Fatal("offline checker did not retain the exclusive image lock")
		}
		if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if !imageLockAvailable(t, image.Path) {
			t.Fatal("offline image lock remained held after checker exit")
		}
	})
	t.Run("deadline kills descendants", func(t *testing.T) {
		backend := &LinuxBackend{CheckBinary: executableScript(t, "sleep 60 & wait"), CheckTimeout: 50 * time.Millisecond, RequiredUID: os.Getuid()}
		started := time.Now()
		if _, err := backend.OfflineCheck(context.Background(), value); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("offline deadline error = %v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("offline cancellation took %s", elapsed)
		}
	})

	t.Run("output overflow", func(t *testing.T) {
		backend := &LinuxBackend{CheckBinary: executableScript(t, "head -c 1048577 /dev/zero"), CheckTimeout: time.Second, RequiredUID: os.Getuid()}
		if _, err := backend.OfflineCheck(context.Background(), value); err == nil || !strings.Contains(err.Error(), "output exceeded") {
			t.Fatalf("offline overflow error = %v", err)
		}
	})

	t.Run("stderr evidence", func(t *testing.T) {
		backend := &LinuxBackend{CheckBinary: executableScript(t, "printf 'offline-evidence\\n' >&2"), CheckTimeout: time.Second, RequiredUID: os.Getuid()}
		result, err := backend.OfflineCheck(context.Background(), value)
		if err != nil {
			t.Fatal(err)
		}
		want := sha256.Sum256([]byte("offline-evidence\n"))
		if result != "e2fsck-clean-sha256:"+hex.EncodeToString(want[:]) {
			t.Fatalf("offline evidence = %q", result)
		}
	})

	t.Run("non-clean diagnostic is not disclosed", func(t *testing.T) {
		backend := &LinuxBackend{CheckBinary: executableScript(t, "printf 'sensitive-checker-detail\\n' >&2; exit 4"), CheckTimeout: time.Second, RequiredUID: os.Getuid()}
		_, err := backend.OfflineCheck(context.Background(), value)
		if err == nil || strings.Contains(err.Error(), "sensitive-checker-detail") {
			t.Fatalf("offline non-clean error = %v", err)
		}
	})
}

func TestProcessRecordIsPrivateStableAndExactBeforeUse(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	record := processRecord{Version: 3, PID: os.Getpid(), StartTime: start, Path: value.Path,
		Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration,
		ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode,
		BinaryDevice: 3, BinaryInode: 4}
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
	conflict = value
	conflict.ImageIdentity = ImageIdentity{Device: 9, Inode: 10}
	if _, err = readRecord(path, conflict); err == nil {
		t.Fatal("process record was accepted for a conflicting image inode")
	}
	if err = os.Link(path, path+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err = readRecord(path, value); err == nil {
		t.Fatal("hard-linked process record was accepted")
	}
}

func TestProcessRecordReadRecoversIdentityBoundQuarantine(t *testing.T) {
	directoryPath := t.TempDir()
	if err := os.Chmod(directoryPath, 0700); err != nil {
		t.Fatal(err)
	}
	value := Export{PreparedImage: PreparedImage{Path: "/srv/multikernel/root.ext4", ImageID: "image", Port: 4061},
		ImageIdentity: ImageIdentity{Device: 1, Inode: 2}, ExportGeneration: "0123456789abcdef0123456789abcdef"}
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	record := processRecord{Version: 3, PID: os.Getpid(), StartTime: start, Path: value.Path,
		Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration,
		ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode,
		BinaryDevice: 3, BinaryInode: 4}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	name := "record.json"
	path := filepath.Join(directoryPath, name)
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := safefile.OpenDirectory(directoryPath, false)
	if err != nil {
		t.Fatal(err)
	}
	_, found, identity, err := directory.ReadPrivateIdentity(name, 4096)
	if err != nil || !found {
		t.Fatalf("record identity = %+v, %v, %v", identity, found, err)
	}
	if err = directory.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(name))
	quarantine := filepath.Join(directoryPath, ".mklinux-remove-"+hex.EncodeToString(digest[:8])+"-"+
		fmt.Sprintf("%016x-%016x", identity.Device, identity.Inode))
	if err = os.Rename(path, quarantine); err != nil {
		t.Fatal(err)
	}
	observed, err := readRecord(path, value)
	if err != nil || observed != record {
		t.Fatalf("quarantined process record = %+v, %v", observed, err)
	}
}

func TestCounterEvidenceRequiresExactReadyAndCanonicalTerminalClose(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
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
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
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
	if err := os.Chmod(binaryPath, 0755); err != nil {
		t.Fatal(err)
	}
	backend := &LinuxBackend{Binary: binaryPath, RuntimeDir: filepath.Join(directory, "run"),
		RequiredUID: os.Getuid(), ReadyTimeout: time.Second, StopTimeout: 3 * time.Second}
	image := makeExt4(t, false)
	identity, err := backend.Inspect(context.Background(), image)
	if err != nil {
		t.Fatal(err)
	}
	value := validBackendLease(image.Path)
	value.PreparedImage, value.ImageIdentity = image, identity
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
