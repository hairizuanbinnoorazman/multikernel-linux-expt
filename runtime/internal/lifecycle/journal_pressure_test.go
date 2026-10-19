package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
)

func TestCreateJournalPressureRefusesBeforeBackendMutation(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Each operation owns up to one MiB of terminal-record capacity. The actual
	// history stays small: the next create must respect other owners' promises.
	var intents []state.JournalEntry
	for i := 0; i < 63; i++ {
		intent := state.JournalEntry{OperationID: fmt.Sprintf("pending-%d", i), SandboxID: fmt.Sprintf("pending-%d", i), Generation: "generation", Method: "StartSandbox", Phase: "intent"}
		if err = store.Append(intent); err != nil {
			t.Fatal(err)
		}
		intents = append(intents, intent)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := &fake{states: map[string]string{}}
	service := New(store, backend, Artifacts{})
	before := store.Snapshot()
	if _, failure := service.Create(context.Background(), config("box-a", 8, 7001), "create-under-pressure"); failure == nil || failure.Code != "INTERNAL" {
		t.Fatalf("unfunded create result: %+v", failure)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("unfunded create reached external backend: %v", backend.calls)
	}
	after := store.Snapshot()
	if before.Sequence != after.Sequence || len(after.Sandboxes) != 0 || len(after.Results) != 0 {
		t.Fatal("unfunded create changed durable ownership or replay state")
	}
	for _, intent := range intents {
		intent.Phase = "complete"
		if err = store.Append(intent); err != nil {
			t.Fatal(err)
		}
	}
	if _, failure := service.Create(context.Background(), config("box-a", 8, 7001), "create-after-pressure"); failure != nil {
		t.Fatalf("completed owners did not release capacity: %+v", failure)
	}
	if len(backend.calls) == 0 {
		t.Fatal("healthy create never reached backend")
	}
	entries, err := store.JournalEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Incomplete(entries)) != 0 {
		t.Fatal("healthy create lost terminal journal evidence")
	}
}
