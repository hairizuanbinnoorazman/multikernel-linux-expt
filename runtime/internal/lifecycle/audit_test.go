package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestAuditEventsIdentityOutcomesSecretsPaginationAndRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fake{states: map[string]string{}}
	service := New(store, backend, Artifacts{})
	canary := "synthetic-credential-" + strings.Repeat("a9", 32)
	c := config("box-a", 8, 7001)
	c.Labels = map[string]string{"fixture": canary}
	created, failure := service.Create(context.Background(), c, canary)
	if failure != nil {
		t.Fatal(failure)
	}
	backend.fail = "create"
	if _, failure = service.Create(context.Background(), config("box-b", 10, 7002), "failed-create"); failure == nil || failure.Code != "BACKEND_FAILURE" {
		t.Fatalf("failure control: %+v", failure)
	}
	// Older private history can carry free-form errors, which must never be
	// reflected into the new audit view, even as nested operation identity.
	intent := state.JournalEntry{OperationID: "legacy-operation", SandboxID: c.ID, Generation: created.Sandbox.Generation, Method: "StartSandbox", Phase: "intent", State: "CREATED", IdempotencyKey: canary, Fingerprint: canary, Sandbox: &created.Sandbox}
	if err = store.Append(intent); err != nil {
		t.Fatal(err)
	}
	intent.Phase = "complete"
	intent.Error = &protocol.Error{Code: "BACKEND_FAILURE", Message: canary, OperationID: canary}
	if err = store.Append(intent); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(dir, "journal.jsonl")
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(before, []byte(canary)) {
		t.Fatal("secret omission control was not seeded")
	}
	var records []protocol.AuditRecord
	query := protocol.AuditQuery{Version: 1, Limit: 1}
	for {
		page, apiErr := service.AuditEvents(query)
		if apiErr != nil || page.Validate(query) != nil {
			t.Fatalf("audit page invalid: %+v %v", page, apiErr)
		}
		encoded, _ := json.Marshal(page)
		if bytes.Contains(encoded, []byte(canary)) {
			t.Fatal("audit view reflected protected journal content")
		}
		records = append(records, page.Records...)
		if !page.HasMore {
			break
		}
		query.AfterSequence = page.NextSequence
	}
	if len(records) != 6 {
		t.Fatalf("records=%d, want six intent/completion records", len(records))
	}
	for i := 0; i < len(records); i += 2 {
		a, b := records[i], records[i+1]
		if a.Phase != "intent" || a.Outcome != "pending" || b.Phase != "complete" || a.OperationID != b.OperationID || a.SandboxID != b.SandboxID || a.Generation != b.Generation {
			t.Fatal("audit lost operation ownership across phases")
		}
	}
	if records[1].Outcome != "succeeded" || records[1].ErrorCode != "" || records[3].Outcome != "failed" || records[3].ErrorCode != "BACKEND_FAILURE" || records[5].ErrorCode != "BACKEND_FAILURE" {
		t.Fatal("audit lost success/failure outcomes")
	}
	after, err := os.ReadFile(journalPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("audit read mutated journal")
	}
	legacy, apiErr := service.Events(0, 128)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	raw, _ := json.Marshal(legacy)
	if !bytes.Contains(raw, []byte(canary)) {
		t.Fatal("legacy error-body control missing; test cannot establish omission")
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
	page, apiErr := restarted.AuditEvents(protocol.AuditQuery{Version: 1})
	if apiErr != nil || !reflect.DeepEqual(page.Records, records) {
		t.Fatalf("restart changed durable audit: %v", apiErr)
	}
	empty, apiErr := restarted.AuditEvents(protocol.AuditQuery{Version: 1, AfterSequence: page.NextSequence})
	if apiErr != nil || len(empty.Records) != 0 || empty.NextSequence != page.NextSequence || empty.HasMore {
		t.Fatal("caught-up cursor invented records")
	}
}

func TestAuditEventsRejectInvalidQueriesAndMetadataWithoutReflection(t *testing.T) {
	service, store, _ := setup(t)
	defer store.Close()
	for _, query := range []protocol.AuditQuery{{Version: 0}, {Version: 2}, {Version: 1, Limit: 513}, {Version: 1, AfterSequence: 1}} {
		page, failure := service.AuditEvents(query)
		if failure == nil || page.Records != nil {
			t.Fatal("invalid query returned audit records")
		}
	}
	for _, field := range []string{"operation", "sandbox", "generation", "method", "state", "error-code"} {
		t.Run(field, func(t *testing.T) {
			s, st, _ := setup(t)
			defer st.Close()
			secret := "synthetic-private-content"
			intent := state.JournalEntry{OperationID: "operation", SandboxID: "box", Generation: strings.Repeat("a", 32), Method: "CreateSandbox", Phase: "intent", State: "CREATED"}
			switch field {
			case "operation":
				intent.OperationID = strings.Repeat(secret, 20)
			case "sandbox":
				intent.SandboxID = secret + "/private"
			case "generation":
				intent.Generation = secret
			case "method":
				intent.Method = secret
			case "state":
				intent.State = secret
			}
			if err := st.Append(intent); err != nil {
				t.Fatal(err)
			}
			if field == "error-code" {
				intent.Phase = "complete"
				intent.Error = &protocol.Error{Code: secret, Message: secret}
				if err := st.Append(intent); err != nil {
					t.Fatal(err)
				}
			}
			page, failure := s.AuditEvents(protocol.AuditQuery{Version: 1})
			if failure == nil || failure.Code != "INTERNAL" || page.Records != nil || strings.Contains(failure.Message, secret) {
				t.Fatal("invalid metadata was reflected or partially exported")
			}
		})
	}
}
