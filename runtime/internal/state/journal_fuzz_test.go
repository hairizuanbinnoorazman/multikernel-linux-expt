package state

import (
	"bytes"
	"encoding/json"
	"testing"
)

func FuzzJournalRecoveryAssociations(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("{\"sequence\":1,\"operation_id\":\"op\",\"sandbox_id\":\"box\",\"generation\":\"gen\",\"method\":\"DeleteSandbox\",\"phase\":\"intent\"}\n"))
	f.Add([]byte("{\"sequence\":1,\"operation_id\":\"op\",\"phase\":\"complete\"}\n"))
	f.Add([]byte("null\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		entries, err := parseJournal(data)
		if err != nil {
			return
		}
		var canonical []byte
		pending := make(map[string]JournalEntry)
		for _, entry := range entries {
			if entry.Phase == "intent" {
				pending[entry.OperationID] = entry
			} else {
				delete(pending, entry.OperationID)
			}
			line, err := json.Marshal(entry)
			if err != nil {
				t.Fatal(err)
			}
			canonical = append(canonical, append(line, '\n')...)
		}
		decoded, err := parseJournal(canonical)
		if err != nil || len(decoded) != len(entries) {
			t.Fatalf("valid journal failed round trip: %v", err)
		}
		incomplete := Incomplete(entries)
		if len(incomplete) != len(pending) {
			t.Fatal("recovery set differs from unmatched intents")
		}
		for _, intent := range incomplete {
			original, exists := pending[intent.OperationID]
			if !exists || original.Generation != intent.Generation || original.SandboxID != intent.SandboxID {
				t.Fatal("recovery intent changed ownership")
			}
		}
		if len(canonical) > 0 {
			if _, err := parseJournal(bytes.TrimSuffix(canonical, []byte{'\n'})); err == nil {
				t.Fatal("truncated journal accepted")
			}
		}
	})
}
