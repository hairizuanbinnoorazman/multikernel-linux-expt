package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCredentialIsConsumedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("ab", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := consumeAgentToken(path)
	if err != nil || !bytes.Equal(key, bytes.Repeat([]byte{0xab}, 32)) {
		t.Fatal("credential consumption failed", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("credential remains on disk", err)
	}
	if _, err := consumeAgentToken(path); err == nil {
		t.Fatal("credential reused")
	}
}

func TestAgentCredentialRejectsUnsafeFiles(t *testing.T) {
	for _, fixture := range []string{"readable", "symlink", "hardlink", "malformed", "oversized"} {
		t.Run(fixture, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "token")
			value := []byte(strings.Repeat("ab", 32) + "\n")
			if fixture == "malformed" {
				value[0] = 'z'
			}
			if fixture == "oversized" {
				value = append(value, 'x')
			}
			if err := os.WriteFile(path, value, 0600); err != nil {
				t.Fatal(err)
			}
			switch fixture {
			case "readable":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := consumeAgentToken(path); err == nil {
				t.Fatal("unsafe credential accepted")
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatal("rejected input was removed", err)
			}
		})
	}
}
