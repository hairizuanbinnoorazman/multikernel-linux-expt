package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePersistsStrictPrivateState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "storage")
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	value := Export{SandboxID: "box", SandboxGeneration: sandboxGeneration, ExportGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: "ACTIVE"}
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.Get("box", sandboxGeneration); !ok || got.ExportGeneration != value.ExportGeneration {
		t.Fatalf("reopened export = %+v, %v", got, ok)
	}
	info, err := os.Stat(filepath.Join(directory, "state.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state mode = %v, %v", info.Mode(), err)
	}
}

func TestStoreRejectsSymlinkAndUnknownOrDuplicateState(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(parent, "linked")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(linked); err == nil {
		t.Fatal("symlinked state directory accepted")
	}
	for name, body := range map[string]string{
		"unknown":   `{"version":1,"exports":{},"extra":true}`,
		"duplicate": `{"version":1,"version":1,"exports":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "state.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(directory); err == nil {
				t.Fatal("malformed state accepted")
			}
		})
	}
}
