package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRejectsUntrustedDurableState(t *testing.T) {
	for name, content := range map[string]string{
		"unknown field":       `{"version":1,"endpoints":{},"extra":true}`,
		"duplicate field":     `{"version":1,"version":1,"endpoints":{}}`,
		"unsupported version": `{"version":2,"endpoints":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "state.json"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(directory); err == nil {
				t.Fatal("untrusted state accepted")
			}
		})
	}
}

func TestStoreRejectsSymlinkAndPermissiveState(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte(`{"version":1,"endpoints":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(directory, "state.json")
	if err := os.Symlink(target, state); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(directory); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("symlink state error=%v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte(`{"version":1,"endpoints":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(directory); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("permissive state error=%v", err)
	}
}
