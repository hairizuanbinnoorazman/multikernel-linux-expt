package state

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalStatusTracksPendingCompletionReopenAndCloseWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(status string, seq, pending uint64) {
		t.Helper()
		before, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		r, f := s.JournalStatus(t.Context())
		if f != nil || r.Validate() != nil || r.Status != status || r.Sequence != seq || r.PendingCompletions != pending {
			t.Fatal(r, f)
		}
		after, err := os.ReadFile(filepath.Join(dir, "journal.jsonl"))
		if err != nil || string(before) != string(after) {
			t.Fatal("inspection changed journal")
		}
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "private") {
			t.Fatal("journal payload leaked")
		}
	}
	check("idle", 0, 0)
	e := JournalEntry{OperationID: "private-operation", SandboxID: "private-sandbox", Generation: "generation", Method: "DeleteSandbox", Phase: "intent", IdempotencyKey: "private-key", Fingerprint: "private-credential"}
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	check("pending", 1, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	check("failed", 1, 1)
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	check("pending", 1, 1)
	e.Phase = "complete"
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	check("idle", 2, 0)
}

func TestJournalStatusExposesObservedIdentityFailureWithoutReadingOrRepair(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(dir, "journal.jsonl")
	held := path + ".held"
	if err := os.Rename(path, held); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("private-replacement-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	// This is explicitly an in-memory view. Before a mutation detects the path
	// change it must not claim to have attested filesystem health.
	r, f := s.JournalStatus(t.Context())
	if f != nil || r.Status != "idle" {
		t.Fatal(r, f)
	}
	err = s.Append(JournalEntry{OperationID: "new", SandboxID: "box", Phase: "intent"})
	if err == nil {
		t.Fatal("replaced journal accepted")
	}
	r, f = s.JournalStatus(t.Context())
	if f != nil || r.Validate() != nil || r.Status != "failed" || !r.JournalFailed || r.Sequence != 0 {
		t.Fatal(r, f)
	}
	a, _ := os.ReadFile(path)
	b, _ := os.ReadFile(held)
	if string(a) != "private-replacement-canary" || len(b) != 0 {
		t.Fatal("inspection repaired or wrote replaced journal")
	}
}

func TestJournalStatusBusyCanceledAndInvalidAccountingFailClosed(t *testing.T) {
	for _, mode := range []string{"busy", "canceled", "negative", "ahead"} {
		t.Run(mode, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch mode {
			case "busy":
				s.mu.Lock()
				defer s.mu.Unlock()
			case "canceled":
				cancel()
			case "negative":
				s.pendingCompletions = -1
			case "ahead":
				s.pendingCompletions = 1
			}
			start := time.Now()
			r, f := s.JournalStatus(ctx)
			if f == nil || r.Version != 0 || time.Since(start) > time.Second {
				t.Fatal("invalid snapshot accepted or blocked", r, f)
			}
		})
	}
}
