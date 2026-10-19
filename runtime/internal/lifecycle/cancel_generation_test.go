package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
)

func TestCancelCreateCannotTeardownReusedSandboxGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, retire string
		reopen       bool
	}{
		{"delete", "delete", false}, {"cancel", "cancel", false},
		{"delete-reopen", "delete", true}, {"cancel-reopen", "cancel", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			store, err := state.Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			backend := &fake{states: map[string]string{}}
			service := New(store, backend, Artifacts{})
			defer func() { store.Close() }()
			candidate := config("box-a", 8, 7001)
			old, failure := service.Create(context.Background(), candidate, "old-create")
			if failure != nil {
				t.Fatal(failure)
			}
			if tc.retire == "delete" {
				if _, failure = service.Delete(context.Background(), candidate.ID, old.Sandbox.Generation, "old-delete"); failure != nil {
					t.Fatal(failure)
				}
			} else {
				if safe, failure := service.CancelCreate(context.Background(), candidate, "old-create"); failure != nil || !safe {
					t.Fatalf("retire: %v %+v", safe, failure)
				}
			}
			current, failure := service.Create(context.Background(), candidate, "new-create")
			if failure != nil {
				t.Fatal(failure)
			}
			if current.Sandbox.Generation == old.Sandbox.Generation {
				t.Fatal("fixture reused generation")
			}
			if tc.reopen {
				if err = store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = state.Open(directory)
				if err != nil {
					t.Fatal(err)
				}
				service = New(store, backend, Artifacts{})
				if err = service.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			snapshotBytes, err := os.ReadFile(filepath.Join(directory, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			before := store.Snapshot()
			entries, err := store.JournalEntries()
			if err != nil {
				t.Fatal(err)
			}
			calls := len(backend.calls)
			// Retain exact file bytes as well as decoded ownership and backend effects.
			journal, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if safe, failure := service.CancelCreate(context.Background(), candidate, "old-create"); safe || failure == nil || failure.Code != "IDEMPOTENCY_CONFLICT" {
				t.Errorf("stale cancellation = %v %+v; expected ownership refusal", safe, failure)
			}
			if actual := store.Snapshot(); !reflect.DeepEqual(actual, before) {
				t.Error("stale cancellation changed durable snapshot")
			}
			after, err := store.JournalEntries()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, entries) {
				t.Error("stale cancellation changed journal")
			}
			actual, err := os.ReadFile(filepath.Join(directory, "journal.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(journal) {
				t.Error("stale cancellation changed journal bytes")
			}
			actualSnapshot, err := os.ReadFile(filepath.Join(directory, "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(actualSnapshot) != string(snapshotBytes) {
				t.Error("stale cancellation changed snapshot bytes")
			}
			if len(backend.calls) != calls {
				t.Error("stale cancellation touched backend")
			}
			if got, ok := service.Get(candidate.ID); !ok || !reflect.DeepEqual(got, current.Sandbox) {
				t.Error("replacement generation was changed")
			}
		})
	}
}
