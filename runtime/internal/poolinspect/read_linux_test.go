//go:build linux

package poolinspect

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestKernelMetadataReadRefusesUnsafeObjectsWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("bounded"), 0600); err != nil {
		t.Fatal(err)
	}
	uid := uint32(os.Geteuid())
	ctx := context.Background()
	data, err := readBounded(ctx, plain, 7, uid)
	if err != nil || string(data) != "bounded" {
		t.Fatalf("valid read=%q %v", data, err)
	}
	if _, err = readBounded(ctx, plain, 6, uid); err != ErrKernelPoolUnavailable {
		t.Fatalf("size limit: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(plain, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err = unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, dir, filepath.Join(dir, "missing")} {
		if _, err = readBounded(ctx, path, 32, uid); err != ErrKernelPoolUnavailable {
			t.Fatalf("unsafe object accepted: %v", err)
		}
	}
	hard := filepath.Join(dir, "hard")
	if err = os.Link(plain, hard); err != nil {
		t.Fatal(err)
	}
	if _, err = readBounded(ctx, plain, 32, uid); err != ErrKernelPoolUnavailable {
		t.Fatalf("hard link accepted: %v", err)
	}
	if err = os.Remove(hard); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(plain, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err = readBounded(ctx, plain, 32, uid); err != ErrKernelPoolUnavailable {
		t.Fatalf("writable metadata accepted: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = readBounded(canceled, filepath.Join(dir, "missing"), 32, uid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
