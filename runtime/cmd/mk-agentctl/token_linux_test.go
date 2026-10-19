package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadControlTokenPrivateFileRetained(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "token")
	value := strings.Repeat("a7", 32) + "\n"
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		key, err := readControlToken(path)
		if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{0xa7}, 32)) {
			t.Fatalf("read %d failed: %v", attempt, err)
		}
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != value {
		t.Fatal("harness credential changed or consumed", err)
	}
}

func TestReadControlTokenRefusesUnsafeInputs(t *testing.T) {
	cases := []string{"relative", "shared-directory", "shared-file", "symlink-file", "symlink-directory", "hardlink", "fifo", "oversize", "missing-newline", "bad-hex", "empty", "directory"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "token")
			value := strings.Repeat("a7", 32) + "\n"
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "relative":
				path = "relative-token"
			case "shared-directory":
				must(os.Chmod(directory, 0755))
			case "shared-file":
				must(os.Chmod(path, 0640))
			case "symlink-file":
				target := filepath.Join(directory, "real")
				must(os.Rename(path, target))
				must(os.Symlink(target, path))
			case "symlink-directory":
				alias := filepath.Join(t.TempDir(), "alias")
				must(os.Symlink(directory, alias))
				path = filepath.Join(alias, "token")
			case "hardlink":
				must(os.Link(path, filepath.Join(directory, "second")))
			case "fifo":
				must(os.Remove(path))
				must(unix.Mkfifo(path, 0600))
			case "oversize":
				must(os.WriteFile(path, []byte(value+"extra"), 0600))
			case "missing-newline":
				must(os.WriteFile(path, []byte(value[:64]), 0600))
			case "bad-hex":
				must(os.WriteFile(path, []byte(strings.Repeat("z", 64)+"\n"), 0600))
			case "empty":
				must(os.WriteFile(path, nil, 0600))
			case "directory":
				must(os.Remove(path))
				must(os.Mkdir(path, 0600))
			}
			done := make(chan error, 1)
			go func() {
				key, err := readControlToken(path)
				if len(key) != 0 {
					done <- nil
					return
				}
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("unsafe credential accepted")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("unsafe credential blocked the reader")
			}
		})
	}
}
