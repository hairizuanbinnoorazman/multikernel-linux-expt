package safefile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestOpenOwnedDirectoryPreservesSafeSharedMode(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0750); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenOwnedDirectory(base)
	if err != nil {
		t.Fatal(err)
	}
	if err = directory.Close(); err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(base); statErr != nil || info.Mode().Perm() != 0750 {
		t.Fatalf("owned directory mode = %v, %v", info, statErr)
	}
	if err = os.Chmod(base, 0770); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenOwnedDirectory(base); err == nil {
		t.Fatal("group-writable owned directory was accepted")
	}
}

func TestIdentityConditionedRemovalPreservesRacedReplacementAndRecoversQuarantine(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if _, err = directory.PublishExclusive("record", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	_, present, expected, err := directory.ReadPrivateIdentity("record", 4096)
	if err != nil || !present {
		t.Fatalf("read published identity = %v, %v", present, err)
	}
	replaced := false
	removed, err := directory.removeIfIdentityWithHook("record", expected, func() {
		replaced = true
		if renameErr := os.Rename(filepath.Join(base, "record"), filepath.Join(base, "record.original")); renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(filepath.Join(base, "record"), []byte("replacement"), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if !replaced || removed || err == nil {
		t.Fatalf("raced removal = replaced:%v removed:%v err:%v", replaced, removed, err)
	}
	if value, readErr := os.ReadFile(filepath.Join(base, "record")); readErr != nil || string(value) != "replacement" {
		t.Fatalf("replacement file = %q, %v", value, readErr)
	}
	if value, readErr := os.ReadFile(filepath.Join(base, "record.original")); readErr != nil || string(value) != "original" {
		t.Fatalf("original file = %q, %v", value, readErr)
	}
	if err = os.Remove(filepath.Join(base, "record")); err != nil {
		t.Fatal(err)
	}
	quarantine := filepath.Join(base, removalQuarantine("record", expected))
	if err = os.Rename(filepath.Join(base, "record.original"), quarantine); err != nil {
		t.Fatal(err)
	}
	data, present, quarantined, recoveredIdentity, err := directory.ReadPrivateOrQuarantineIdentity("record", 4096)
	if err != nil || !present || !quarantined || string(data) != "original" || !SameObject(recoveredIdentity, expected) {
		t.Fatalf("quarantine recovery read = %q present:%v quarantined:%v identity:%+v err:%v", data, present, quarantined, recoveredIdentity, err)
	}
	removed, err = directory.RemoveIfIdentity("record", expected)
	if err != nil || !removed {
		t.Fatalf("quarantine recovery = %v, %v", removed, err)
	}
	if _, err = os.Stat(quarantine); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovered quarantine remains: %v", err)
	}
}

func TestCaptureRegularIdentityAllowsPublicModeButRejectsReplacement(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	path := filepath.Join(base, "address")
	if err = os.WriteFile(path, []byte("unix:///owned"), 0644); err != nil {
		t.Fatal(err)
	}
	expected, err := directory.CaptureRegularIdentity("address", 0644, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if removed, removeErr := directory.RemoveIfIdentity("address", expected); removed || removeErr == nil {
		t.Fatalf("replacement cleanup = removed:%v error:%v", removed, removeErr)
	}
	if value, readErr := os.ReadFile(path); readErr != nil || string(value) != "replacement" {
		t.Fatalf("replacement = %q, %v", value, readErr)
	}
}

func TestPublicRegularAndSymlinkPublicationRetainExactIdentity(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	created, regular, err := directory.PublishExclusiveRegularIdentity("resolv.conf", []byte("nameserver 192.0.2.53\n"), 0644)
	if err != nil || !created {
		t.Fatalf("publish regular = %v, %v", created, err)
	}
	data, opened, err := directory.ReadRegularIdentity("resolv.conf", 0644, 4096)
	if err != nil || string(data) != "nameserver 192.0.2.53\n" || !SameObject(opened, regular) {
		t.Fatalf("read regular = %q %+v, %v", data, opened, err)
	}
	if removed, err := directory.RemoveIfIdentity("resolv.conf", regular); err != nil || !removed {
		t.Fatalf("remove regular = %v, %v", removed, err)
	}
	created, symlink, err := directory.PublishExclusiveSymlinkIdentity("resolv.conf", "../run/resolv.conf", 4096)
	if err != nil || !created {
		t.Fatalf("publish symlink = %v, %v", created, err)
	}
	target, opened, err := directory.CaptureSymlinkIdentity("resolv.conf", 4096)
	if err != nil || target != "../run/resolv.conf" || !SameObject(opened, symlink) {
		t.Fatalf("capture symlink = %q %+v, %v", target, opened, err)
	}
	if removed, err := directory.RemoveIfIdentity("resolv.conf", symlink); err != nil || !removed {
		t.Fatalf("remove symlink = %v, %v", removed, err)
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

func TestExclusivePublicationAndRemoval(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	created, err := directory.PublishExclusive("record", []byte("first"), 0600)
	if err != nil || !created {
		t.Fatalf("first publication = %v, %v", created, err)
	}
	created, err = directory.PublishExclusive("record", []byte("second"), 0600)
	if err != nil || created {
		t.Fatalf("conflicting publication = %v, %v", created, err)
	}
	if data, readErr := os.ReadFile(filepath.Join(base, "record")); readErr != nil || string(data) != "first" {
		t.Fatalf("published record = %q, %v", data, readErr)
	}
	removed, err := directory.Remove("record")
	if err != nil || !removed {
		t.Fatalf("first removal = %v, %v", removed, err)
	}
	removed, err = directory.Remove("record")
	if err != nil || removed {
		t.Fatalf("idempotent removal = %v, %v", removed, err)
	}
}

func TestReplaceIdentityPublishesExactSuccessor(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	first, err := directory.ReplaceIdentity("state", []byte("first\n"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := directory.ReplaceIdentity("state", []byte("second\n"), 0600, &first)
	if err != nil {
		t.Fatal(err)
	}
	data, found, observed, err := directory.ReadPrivateIdentity("state", 64)
	if err != nil || !found || string(data) != "second\n" || !SameObject(observed, second) || SameObject(first, second) {
		t.Fatalf("replacement data=%q found=%v observed=%+v first=%+v second=%+v err=%v", data, found, observed, first, second, err)
	}
}

func TestReplaceIdentityPreservesRacedSubstituteAndOriginal(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(base, false)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	original, err := directory.ReplaceIdentity("state", []byte("original\n"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "original-held")
	var hookErr error
	_, err = directory.replaceIdentityWithHook("state", []byte("ours\n"), 0600, &original, func() {
		if hookErr = os.Rename(filepath.Join(base, "state"), moved); hookErr == nil {
			hookErr = os.WriteFile(filepath.Join(base, "state"), []byte("substitute\n"), 0600)
		}
	})
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if err == nil || !strings.Contains(err.Error(), "changed during exchange") {
		t.Fatalf("raced replacement error = %v", err)
	}
	if data, readErr := os.ReadFile(filepath.Join(base, "state")); readErr != nil || string(data) != "substitute\n" {
		t.Fatalf("public substitute = %q, %v", data, readErr)
	}
	if data, readErr := os.ReadFile(moved); readErr != nil || string(data) != "original\n" {
		t.Fatalf("displaced original = %q, %v", data, readErr)
	}
}
