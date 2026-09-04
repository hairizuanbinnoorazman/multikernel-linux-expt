package state

import (
	"errors"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestSnapshotRenameSynchronizesDirectory(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls := 0
	store.syncDir = func(path string) error {
		calls++
		if path != store.dir {
			t.Fatalf("synced directory %q, want %q", path, store.dir)
		}
		return nil
	}
	if err := store.SetSandbox(protocol.Sandbox{ID: "box", State: "CREATED"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("directory sync calls = %d, want 1", calls)
	}
}

func TestDirectorySyncFailureIsPropagated(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	want := errors.New("injected directory sync failure")
	store.syncDir = func(string) error { return want }
	if err := store.SetSandbox(protocol.Sandbox{ID: "box", State: "CREATED"}); !errors.Is(err, want) {
		t.Fatalf("SetSandbox() error = %v, want %v", err, want)
	}
}

func TestRemoveSandboxPersistsRemoval(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetSandbox(protocol.Sandbox{ID: "box", State: "ALLOCATING"}); err != nil {
		t.Fatal(err)
	}
	if err = store.RemoveSandbox("box"); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Sandbox("box"); ok {
		t.Fatal("sandbox remains after removal")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, ok := reopened.Sandbox("box"); ok {
		t.Fatal("removed sandbox reappeared after reopen")
	}
}
