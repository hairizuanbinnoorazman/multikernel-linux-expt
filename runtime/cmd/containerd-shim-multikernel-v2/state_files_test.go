//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

func TestLoadOrCreateTokenReusesExactSafeIdentity(t *testing.T) {
	directory := t.TempDir()
	first, encoded, err := loadOrCreateToken(directory)
	if err != nil || len(first) != 32 || len(encoded) != 64 {
		t.Fatalf("first token = %x %q, %v", first, encoded, err)
	}
	second, repeated, err := loadOrCreateToken(directory)
	if err != nil || !reflect.DeepEqual(second, first) || repeated != encoded {
		t.Fatalf("repeated token = %x %q, %v", second, repeated, err)
	}
	info, err := os.Lstat(filepath.Join(directory, "token"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 65 {
		t.Fatalf("token metadata = %+v, %v", info, err)
	}
}

func TestLoadOrCreateTokenFromHeldDirectory(t *testing.T) {
	directoryPath := privateTestDirectory(t)
	directory, err := safefile.OpenDirectory(directoryPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	first, encoded, err := loadOrCreateTokenFromDirectory(directory)
	if err != nil || len(first) != 32 || len(encoded) != 64 {
		t.Fatalf("first held token = %x %q, %v", first, encoded, err)
	}
	second, repeated, err := loadOrCreateTokenFromDirectory(directory)
	if err != nil || !reflect.DeepEqual(second, first) || repeated != encoded {
		t.Fatalf("repeated held token = %x %q, %v", second, repeated, err)
	}
}

func TestLoadOrCreateTokenRejectsMalformedAndSymlinkState(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(string) error
	}{
		{"malformed", func(path string) error { return os.WriteFile(path, []byte("short\n"), 0600) }},
		{"permissive", func(path string) error { return os.WriteFile(path, []byte(strings.Repeat("a", 64)+"\n"), 0644) }},
		{"symlink", func(path string) error { return os.Symlink("missing", path) }},
		{"hardlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
				return err
			}
			return os.Link(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := test.setup(filepath.Join(directory, "token")); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadOrCreateToken(directory); err == nil {
				t.Fatal("unsafe existing token accepted")
			}
		})
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		directory := t.TempDir()
		realRuntime := filepath.Join(directory, "real")
		if err := os.Mkdir(realRuntime, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realRuntime, "token"), []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		linkedRuntime := filepath.Join(directory, "linked")
		if err := os.Symlink(realRuntime, linkedRuntime); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrCreateToken(linkedRuntime); err == nil {
			t.Fatal("token beneath a symlinked ancestor was accepted")
		}
	})

	t.Run("symlinked empty ancestor does not redirect creation", func(t *testing.T) {
		directory := t.TempDir()
		realRuntime := filepath.Join(directory, "real-empty")
		if err := os.Mkdir(realRuntime, 0700); err != nil {
			t.Fatal(err)
		}
		linkedRuntime := filepath.Join(directory, "linked-empty")
		if err := os.Symlink(realRuntime, linkedRuntime); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrCreateToken(linkedRuntime); err == nil {
			t.Fatal("token creation beneath a symlinked ancestor was accepted")
		}
		if _, err := os.Stat(filepath.Join(realRuntime, "token")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("redirected token was created: %v", err)
		}
	})
}

func TestAtomicStatePublicationIsDescriptorAnchoredAndPrivate(t *testing.T) {
	t.Run("normal replacement", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "state.json")
		if err := atomicWriteFile(path, []byte("one"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(path, []byte("two"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		if err != nil || statErr != nil || string(data) != "two" || info.Mode().Perm() != 0600 {
			t.Fatalf("published state = %q mode=%v read=%v stat=%v", data, info.Mode(), err, statErr)
		}
		temporary, err := filepath.Glob(filepath.Join(directory, ".state.json.*"))
		if err != nil || len(temporary) != 0 {
			t.Fatalf("temporary files = %v, %v", temporary, err)
		}
	})

	t.Run("symlinked parent", func(t *testing.T) {
		directory := t.TempDir()
		realParent := filepath.Join(directory, "real")
		if err := os.Mkdir(realParent, 0700); err != nil {
			t.Fatal(err)
		}
		linkedParent := filepath.Join(directory, "linked")
		if err := os.Symlink(realParent, linkedParent); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(filepath.Join(linkedParent, "state.json"), []byte("hostile"), 0600); err == nil {
			t.Fatal("atomic publication followed a symlinked parent")
		}
		if _, err := os.Stat(filepath.Join(realParent, "state.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("symlink target was modified: %v", err)
		}
	})

	t.Run("writable parent", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0777); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(directory, 0700)
		if err := atomicWriteFile(filepath.Join(directory, "state.json"), []byte("hostile"), 0600); err == nil {
			t.Fatal("atomic publication accepted a group/world-writable parent")
		}
	})
}
