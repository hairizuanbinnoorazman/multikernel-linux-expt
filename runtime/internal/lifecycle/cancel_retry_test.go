package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCanceledRetryCannotReappearAfterFailedEarlierAttempt(t *testing.T) {
	points := []string{"before-intent", "after-intent", "before-external-mutation", "after-external-mutation", "after-observation", "before-snapshot", "after-snapshot", "before-completion", "after-completion"}
	for _, mode := range []string{"explicit-cancel", "context-cancel"} {
		for _, point := range points {
			t.Run(mode+"/"+point, func(t *testing.T) {
				dir := t.TempDir()
				store, err := state.Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				backend := &fake{states: map[string]string{}, fail: "create"}
				service := New(store, backend, Artifacts{})
				candidate := config("box-a", 8, 7001)
				if _, failure := service.Create(context.Background(), candidate, "retry-key"); failure == nil || failure.Code != "BACKEND_FAILURE" {
					t.Fatalf("first failure control: %+v", failure)
				}
				backend.fail = ""
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				injected := false
				service.SetFaultInjector(func(observed string) error {
					if observed == point && !injected {
						injected = true
						if mode == "context-cancel" {
							cancel()
						}
						return errors.New("synthetic interrupted retry")
					}
					return nil
				})
				if _, failure := service.Create(ctx, candidate, "retry-key"); failure == nil || !injected {
					t.Fatal("retry was not interrupted at the selected boundary")
				}
				service.SetFaultInjector(nil)
				if mode == "explicit-cancel" {
					if safe, failure := service.CancelCreate(context.Background(), candidate, "retry-key"); failure != nil || !safe {
						t.Fatalf("cancellation failed: %v %+v", safe, failure)
					}
				}
				entries, err := store.JournalEntries()
				if err != nil {
					t.Fatal(err)
				}
				if len(state.Incomplete(entries)) != 0 {
					t.Error("canceled retry left a pending create intent")
				}
				if _, exists := service.Get(candidate.ID); exists {
					t.Error("canceled retry retained durable ownership")
				}
				if actual, err := backend.Observe(context.Background(), candidate.ID); err != nil || actual != "ABSENT" {
					t.Fatalf("canceled backend = %s %v", actual, err)
				}
				if err = store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = state.Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				restarted := New(store, backend, Artifacts{})
				calls := len(backend.calls)
				if err = restarted.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
				if _, exists := restarted.Get(candidate.ID); exists {
					t.Error("canceled retry was allocated again by reconciliation")
				}
				if len(backend.calls) != calls {
					t.Error("restart mutated backend after confirmed cancellation")
				}
				result, ok := store.Result("retry-key")
				if !ok || result.Error == nil || result.Error.Code != "ABORTED" {
					t.Error("cancellation tombstone overwritten by forward recovery")
				}
				if _, failure := restarted.Create(context.Background(), candidate, "retry-key"); failure == nil || failure.Code != "ABORTED" {
					t.Fatal("delayed retry ignored cancellation")
				}
			})
		}
	}
}

func TestReconcileRefusesLegacyCanceledIncompleteIntentBeforeEffects(t *testing.T) {
	for _, mode := range []string{"absent-owner", "durable-owner", "conflicting-fingerprint"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			store, err := state.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			backend := &fake{states: map[string]string{}}
			service := New(store, backend, Artifacts{})
			candidate := config("box-a", 8, 7001)
			service.SetFaultInjector(func(point string) error {
				if point == "after-intent" {
					return errors.New("synthetic old writer")
				}
				return nil
			})
			if _, failure := service.Create(context.Background(), candidate, "legacy-key"); failure == nil {
				t.Fatal("legacy pending-intent fixture failed")
			}
			entries, err := store.JournalEntries()
			if err != nil || len(entries) != 1 {
				t.Fatalf("legacy intent: %v", err)
			}
			intent := entries[0]
			if mode == "durable-owner" {
				if err = store.SetSandbox(*intent.Sandbox); err != nil {
					t.Fatal(err)
				}
				backend.states[candidate.ID] = "CREATED"
			}
			fingerprint := intent.Fingerprint
			if mode == "conflicting-fingerprint" {
				fingerprint = "different-fingerprint"
			}
			if err = store.Tombstone("legacy-key", fingerprint, &protocol.Error{Code: "ABORTED", Message: "synthetic prior cancellation", OperationID: intent.OperationID}); err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = state.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			before := store.Snapshot()
			snapshot, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			journal, _ := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
			backend.calls = nil
			backend.observeErr = errors.New("backend observation was reached")
			restarted := New(store, backend, Artifacts{})
			err = restarted.Reconcile(context.Background())
			if err == nil || !strings.Contains(err.Error(), "OPERATOR_ACTION: canceled operation has an incomplete journal intent") {
				t.Fatalf("contradictory legacy cancellation was not refused before observation: %v", err)
			}
			if len(backend.calls) != 0 || !reflect.DeepEqual(before, store.Snapshot()) {
				t.Fatal("legacy refusal mutated backend or in-memory ownership")
			}
			afterSnapshot, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			afterJournal, _ := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
			if string(snapshot) != string(afterSnapshot) || string(journal) != string(afterJournal) {
				t.Fatal("legacy refusal rewrote cancellation evidence")
			}
		})
	}
}
