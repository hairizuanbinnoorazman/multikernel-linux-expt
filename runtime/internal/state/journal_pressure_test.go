package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Seed valid completed history at the production 64-MiB boundary. This fixture
// does not change a store limit or mutate its private accounting fields.
func writeJournalPressure(t *testing.T, dir string, target int64) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var written int64
	for sequence := uint64(1); written < target; sequence += 2 {
		intent := JournalEntry{Sequence: sequence, At: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), OperationID: fmt.Sprintf("history-%d", sequence), SandboxID: "history", Generation: "history", Method: "DeleteSandbox", Phase: "intent", IdempotencyKey: "history", Fingerprint: "history"}
		first, _ := json.Marshal(intent)
		first = append(first, '\n')
		complete := intent
		complete.Sequence++
		complete.Phase = "complete"
		complete.Error = &protocol.Error{Code: "BACKEND_FAILURE", Message: ""}
		last, _ := json.Marshal(complete)
		room := target - written - int64(len(first)) - int64(len(last)+1)
		if room < 0 {
			t.Fatal("pressure fixture cannot fit final pair")
		}
		padding := int64(maxJournalEntryBytes - len(last) - 1)
		if room < padding {
			padding = room
		}
		complete.Error.Message = strings.Repeat("x", int(padding))
		last, _ = json.Marshal(complete)
		last = append(last, '\n')
		for _, line := range [][]byte{first, last} {
			n, e := f.Write(line)
			if e != nil || n != len(line) {
				t.Fatalf("seed history: %d %v", n, e)
			}
			written += int64(n)
		}
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
}

func pressureIntent(id string) JournalEntry {
	return JournalEntry{OperationID: id, SandboxID: "box", Generation: "generation", Method: "StartSandbox", Phase: "intent", IdempotencyKey: id, Fingerprint: id}
}

func TestJournalPressureRefusesUnfundedIntent(t *testing.T) {
	dir := t.TempDir()
	target := int64(maxJournalBytes - maxJournalEntryBytes/2)
	writeJournalPressure(t, dir, target)
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	before := store.Snapshot().Sequence
	if err = store.Append(pressureIntent("unfunded")); err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("intent without completion space admitted: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != target || store.Snapshot().Sequence != before {
		t.Fatal("refused intent changed journal or sequence")
	}
}

func TestJournalPressureProtectsPendingCompletionsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	writeJournalPressure(t, dir, int64(maxJournalBytes-2*maxJournalEntryBytes-4096))
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, b := pressureIntent("pending-a"), pressureIntent("pending-b")
	for _, entry := range []JournalEntry{a, b} {
		if err = store.Append(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range []JournalEntry{a, b} {
		if err = store.Append(pressureIntent(fmt.Sprintf("steal-%d", i))); err == nil || !strings.Contains(err.Error(), "capacity") {
			t.Fatalf("new intent stole pending capacity after restart: %v", err)
		}
		entry.Phase = "complete"
		entry.Error = &protocol.Error{Code: "BACKEND_FAILURE", Message: ""}
		data, _ := json.Marshal(entry)
		entry.Error.Message = strings.Repeat("x", maxJournalEntryBytes-len(data)-256)
		if err = store.Append(entry); err != nil {
			t.Fatalf("funded completion failed: %v", err)
		}
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	entries, err := reopened.JournalEntries()
	if err != nil {
		t.Fatal(err)
	}
	if len(Incomplete(entries)) != 0 {
		t.Fatal("funded outcomes were lost after restart")
	}
	info, err := os.Stat(filepath.Join(dir, "journal.jsonl"))
	if err != nil || info.Size() > maxJournalBytes {
		t.Fatalf("journal exceeded its hard bound: %v %v", info, err)
	}
}

func TestJournalAppendRejectsReservationTheft(t *testing.T) {
	for _, field := range []string{"sandbox", "generation", "method", "key", "fingerprint", "orphan", "embedded-identity", "phase", "duplicate-intent", "duplicate-completion"} {
		t.Run(field, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			intent := pressureIntent("pending")
			if err = store.Append(intent); err != nil {
				t.Fatal(err)
			}
			invalid := intent
			invalid.Phase = "complete"
			switch field {
			case "sandbox":
				invalid.SandboxID = "other"
			case "generation":
				invalid.Generation = "other"
			case "method":
				invalid.Method = "DeleteSandbox"
			case "key":
				invalid.IdempotencyKey = "other"
			case "fingerprint":
				invalid.Fingerprint = "other"
			case "orphan":
				invalid.OperationID = "other"
			case "embedded-identity":
				invalid.Sandbox = &protocol.Sandbox{ID: "other"}
			case "phase":
				invalid.Phase = "unknown"
			case "duplicate-intent":
				invalid.Phase = "intent"
			case "duplicate-completion":
				if err = store.Append(invalid); err != nil {
					t.Fatal(err)
				}
			}
			before := store.Snapshot().Sequence
			if err = store.Append(invalid); err == nil {
				t.Fatal("invalid operation spent a pending completion reservation")
			}
			if store.Snapshot().Sequence != before {
				t.Fatal("rejected append changed sequence")
			}
			if field != "duplicate-completion" {
				completion := intent
				completion.Phase = "complete"
				if err = store.Append(completion); err != nil {
					t.Fatalf("valid owner lost reservation: %v", err)
				}
			}
			if err = store.Append(intent); err == nil {
				t.Fatal("completed operation identity reused")
			}
		})
	}
}

func TestJournalLegacyOvercommitStillPermitsOwnedCompletion(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	// An older writer could admit more promises than the journal can fund. Read
	// that valid history without silently dropping it or preventing useful repair.
	var intents []JournalEntry
	for i := 0; i < 65; i++ {
		intent := pressureIntent(fmt.Sprintf("legacy-%d", i))
		intent.Sequence = uint64(i + 1)
		data, _ := json.Marshal(intent)
		data = append(data, '\n')
		if _, err = f.Write(data); err != nil {
			t.Fatal(err)
		}
		intents = append(intents, intent)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Append(pressureIntent("new-while-overcommitted")); err == nil {
		t.Fatal("legacy overcommit admitted new work")
	}
	for _, intent := range intents {
		intent.Phase = "complete"
		if err = store.Append(intent); err != nil {
			t.Fatalf("legacy owned completion refused: %v", err)
		}
	}
	if err = store.Append(pressureIntent("new-after-recovery")); err != nil {
		t.Fatalf("recovery did not release pressure: %v", err)
	}
}
