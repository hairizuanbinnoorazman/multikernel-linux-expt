package safefile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDirectoryCreatesPrivatelyWithoutFollowingSymlinks(t *testing.T) {
	base := t.TempDir()
	directory, err := OpenDirectory(filepath.Join(base, "one", "two"), true)
	if err != nil {
		t.Fatal(err)
	}
	if err = directory.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(base, "one", "two"))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("created directory mode = %v, %v", info.Mode(), err)
	}
	target := filepath.Join(base, "target")
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "linked")
	if err = os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenDirectory(filepath.Join(linked, "redirected"), true); err == nil {
		t.Fatal("symlinked ancestor was followed")
	}
	if _, err = os.Stat(filepath.Join(target, "redirected")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target was mutated: %v", err)
	}
}

func TestPrivateReadAndReplaceAreDescriptorAnchored(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "state")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(original, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	moved := original + ".moved"
	if err = os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err = directory.Replace("state.json", []byte("owned\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, found, err := directory.ReadPrivate("state.json", 64)
	if err != nil || !found || !bytes.Equal(data, []byte("owned\n")) {
		t.Fatalf("anchored read = %q, %v, %v", data, found, err)
	}
	if _, err = os.Stat(filepath.Join(original, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement pathname was mutated: %v", err)
	}
	if data, err = os.ReadFile(filepath.Join(moved, "state.json")); err != nil || !bytes.Equal(data, []byte("owned\n")) {
		t.Fatalf("original inode contents = %q, %v", data, err)
	}
}

func TestReadPrivateRejectsUnsafeFiles(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	target := filepath.Join(base, "target")
	if err = os.WriteFile(target, []byte("value"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(target, filepath.Join(base, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = directory.ReadPrivate("target", 64); err == nil {
		t.Fatal("multiply linked file was accepted")
	}
	if err = os.WriteFile(filepath.Join(base, "permissive"), []byte("value"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = directory.ReadPrivate("permissive", 64); err == nil {
		t.Fatal("permissive file was accepted")
	}
	if err = os.WriteFile(filepath.Join(base, "oversize"), bytes.Repeat([]byte("x"), 65), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = directory.ReadPrivate("oversize", 64); err == nil {
		t.Fatal("oversized file was accepted")
	}
}
