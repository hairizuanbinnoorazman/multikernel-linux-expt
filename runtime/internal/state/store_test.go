package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestOpenRejectsUnsafeOrMalformedSnapshot(t *testing.T) {
	for name, install := range map[string]func(string) error{
		"unknown field": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":1,"sandboxes":{},"results":{},"extra":true}`), 0600)
		},
		"duplicate field": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":1,"version":1,"sandboxes":{},"results":{}}`), 0600)
		},
		"unsupported version": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":2,"sandboxes":{},"results":{}}`), 0600)
		},
		"permissive": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":1,"sandboxes":{},"results":{}}`), 0644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := install(filepath.Join(directory, "state.json")); err != nil {
				t.Fatal(err)
			}
			if store, err := Open(directory); err == nil {
				_ = store.Close()
				t.Fatal("unsafe or malformed snapshot was accepted")
			}
		})
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.WriteFile(target, []byte(`{"version":1,"sandboxes":{},"results":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(parent, "state")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, filepath.Join(directory, "state.json")); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(directory); err == nil {
		_ = store.Close()
		t.Fatal("hard-linked snapshot was accepted")
	}
}

func TestOpenRejectsMalformedJournal(t *testing.T) {
	valid := `{"sequence":1,"at":"2026-09-10T00:00:00Z","operation_id":"op","idempotency_key":"key","fingerprint":"fingerprint","sandbox_id":"box","method":"CreateSandbox","phase":"intent"}`
	gap := strings.Replace(valid, `"sequence":1`, `"sequence":3`, 1)
	for name, content := range map[string]string{
		"truncated":      valid,
		"unknown field":  strings.TrimSuffix(valid, "}") + `,"extra":true}` + "\n",
		"duplicate":      strings.TrimSuffix(valid, "}") + `,"phase":"complete"}` + "\n",
		"non-increasing": valid + "\n" + valid + "\n",
		"sequence gap":   valid + "\n" + gap + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "journal.jsonl"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if store, err := Open(directory); err == nil {
				_ = store.Close()
				t.Fatal("malformed journal was accepted")
			}
		})
	}
}

func TestOpenRejectsSnapshotSequenceAheadOfJournal(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "state.json"),
		[]byte(`{"version":1,"sequence":1,"sandboxes":{},"results":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(directory); err == nil {
		_ = store.Close()
		t.Fatal("snapshot sequence ahead of an empty journal was accepted")
	}
}

func TestStoreRejectsDirectoryAndJournalReplacement(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "state")
		store, err := Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if err = os.Rename(directory, directory+".moved"); err != nil {
			t.Fatal(err)
		}
		if err = os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err = store.SetSandbox(protocol.Sandbox{ID: "box", State: "CREATED"}); err == nil || !strings.Contains(err.Error(), "identity changed") {
			t.Fatalf("directory replacement error = %v", err)
		}
		if _, found := store.Sandbox("box"); found {
			t.Fatal("failed persistence left in-memory sandbox ownership")
		}
		if entries, readErr := os.ReadDir(directory); readErr != nil || len(entries) != 0 {
			t.Fatalf("replacement directory was mutated: %v, %v", entries, readErr)
		}
	})
	t.Run("journal", func(t *testing.T) {
		directory := t.TempDir()
		store, err := Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		journal := filepath.Join(directory, "journal.jsonl")
		if err = os.Rename(journal, journal+".moved"); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(journal, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err = store.Append(JournalEntry{OperationID: "op"}); err == nil || !strings.Contains(err.Error(), "pathname identity changed") {
			t.Fatalf("journal replacement error = %v", err)
		}
		if info, statErr := os.Stat(journal); statErr != nil || info.Size() != 0 {
			t.Fatalf("replacement journal was mutated: %v, %v", info, statErr)
		}
	})
}

func TestJournalCapacityIsBounded(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.journal.Truncate(maxJournalBytes); err != nil {
		t.Fatal(err)
	}
	if err = store.Append(JournalEntry{OperationID: "op"}); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("journal capacity error = %v", err)
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

func TestCreateTombstoneAndFinalRemovalPersistAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := protocol.Sandbox{ID: "box", State: "CREATED"}
	if err = store.SetSandbox(sandbox); err != nil {
		t.Fatal(err)
	}
	failure := &protocol.Error{Code: "ABORTED", Message: "canceled", Retryable: false}
	if err = store.Tombstone("create-key", "fingerprint", failure); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.Sandbox("box"); !exists {
		t.Fatal("tombstone prematurely removed sandbox ownership")
	}
	if result, ok := store.Result("create-key"); !ok || result.Error == nil || result.Error.Code != "ABORTED" {
		t.Fatalf("tombstone result = %+v, %v", result, ok)
	}
	if err = store.AbortCreate("box", "create-key", "fingerprint", failure); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, exists := reopened.Sandbox("box"); exists {
		t.Fatal("aborted sandbox ownership reappeared after reopen")
	}
	if result, ok := reopened.Result("create-key"); !ok || result.Fingerprint != "fingerprint" || result.Error == nil || result.Error.Code != "ABORTED" {
		t.Fatalf("reopened tombstone = %+v, %v", result, ok)
	}
}
