//go:build linux

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
int main(void) {
  signal(SIGTERM, stop); signal(SIGINT, stop);
  puts("MKNBD_SERVER_READY image=test image_id=test generation=test size=67108864 port=4061"); fflush(stdout);
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
	counters, err := backend.Stop(context.Background(), value)
	if err != nil {
		t.Fatal(err)
	}
	if counters.Reads != 2 || counters.ReadBytes != 8192 || counters.Writes != 3 || counters.WrittenBytes != 12288 || counters.Flushes != 4 {
		t.Fatalf("counters = %+v", counters)
	}
	observed, err = backend.Observe(context.Background(), value)
	if err != nil || observed.Active {
		t.Fatalf("post-stop observation = %+v, %v", observed, err)
	}
	result, err := backend.OfflineCheck(context.Background(), value)
	if err != nil || len(result) != len("e2fsck-clean-sha256:")+64 {
		t.Fatalf("offline check = %q, %v", result, err)
	}
}
