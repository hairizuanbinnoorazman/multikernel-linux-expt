package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func auditTestRecord() AuditRecord {
	return AuditRecord{Version: 1, Component: "mkruntimed", Sequence: 1, At: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), OperationID: "operation", SandboxID: "box", Generation: strings.Repeat("a", 32), Method: "CreateSandbox", Phase: "intent", Outcome: "pending", State: "ALLOCATING"}
}

func TestAuditPageRejectsBrokenOwnershipOutcomeAndCursor(t *testing.T) {
	for _, field := range []string{"version", "component", "operation", "generation", "sandbox", "method", "time", "state", "phase", "outcome", "error-code", "sequence-gap", "duplicate", "next", "high-water", "more", "empty-skip", "null-records", "limit"} {
		t.Run(field, func(t *testing.T) {
			q := AuditQuery{Version: 1, Limit: 1}
			p := AuditPage{Version: 1, Records: []AuditRecord{auditTestRecord()}, NextSequence: 1, HighWaterSequence: 1}
			switch field {
			case "version":
				p.Version = 2
			case "component":
				p.Records[0].Component = "other"
			case "operation":
				p.Records[0].OperationID = ""
			case "generation":
				p.Records[0].Generation = "other"
			case "sandbox":
				p.Records[0].SandboxID = "../other"
			case "method":
				p.Records[0].Method = "other"
			case "time":
				p.Records[0].At = time.Time{}
			case "state":
				p.Records[0].State = "unknown"
			case "phase":
				p.Records[0].Phase = "unknown"
			case "outcome":
				p.Records[0].Outcome = "succeeded"
			case "error-code":
				p.Records[0].ErrorCode = "BACKEND_FAILURE"
			case "sequence-gap":
				p.Records[0].Sequence = 2
				p.NextSequence = 2
				p.HighWaterSequence = 2
			case "duplicate":
				p.Records = append(p.Records, p.Records[0])
				q.Limit = 2
			case "next":
				p.NextSequence = 2
			case "high-water":
				p.HighWaterSequence = 0
			case "more":
				p.HasMore = true
			case "empty-skip":
				p.Records = []AuditRecord{}
				p.HasMore = true
				p.NextSequence = 0
			case "null-records":
				p.Records = nil
			case "limit":
				q.Limit = 513
			}
			if p.Validate(q) == nil {
				t.Fatal("invalid audit page accepted")
			}
		})
	}
}

func TestAuditMaximumPageFitsDaemonWireLimit(t *testing.T) {
	page := AuditPage{Version: 1, Records: make([]AuditRecord, MaxAuditRecords), NextSequence: MaxAuditRecords, HighWaterSequence: MaxAuditRecords}
	for i := range page.Records {
		r := auditTestRecord()
		r.Sequence = uint64(i + 1)
		r.OperationID = strings.Repeat("\"", 128)
		r.SandboxID = strings.Repeat("a", 63)
		r.Method = "DeleteSandbox"
		r.Phase = "complete"
		r.Outcome = "failed"
		r.ErrorCode = "FAILED_PRECONDITION"
		r.State = "RELEASING"
		page.Records[i] = r
	}
	if err := page.Validate(AuditQuery{Version: 1, Limit: MaxAuditRecords}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(Response{Version: 1, RequestID: strings.Repeat("\\", 128), Body: page})
	if err != nil || len(encoded)+1 > 1<<20 {
		t.Fatalf("maximum escaped metadata exceeded daemon wire limit: %d %v", len(encoded), err)
	}
}

// The same wire fixtures are validated by the JSON Schema suite and real Go
// strict decoders, so capability/body evolution cannot silently diverge.
func TestAuditSharedContractFixtures(t *testing.T) {
	cases := map[string]string{
		"audit-query.valid.json": "query", "audit-page.valid.json": "page", "audit-capabilities.valid.json": "capabilities",
		"audit-query-limit.invalid.json": "query", "audit-query-version.invalid.json": "query", "audit-capabilities-duplicate.invalid.json": "capabilities",
		"audit-page-secret.invalid.json": "page", "audit-page-outcome.invalid.json": "page", "audit-page-identity.invalid.json": "page", "audit-page-null.invalid.json": "page",
	}
	for name, kind := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "docs", "runtime", "contracts", "fixtures", name))
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "query":
				var value AuditQuery
				err = StrictDecode(data, &value)
				if err == nil {
					err = value.Validate()
				}
			case "page":
				var value AuditPage
				err = StrictDecode(data, &value)
				if err == nil {
					err = value.Validate(AuditQuery{Version: 1, Limit: 1})
				}
			case "capabilities":
				var value DaemonCapabilities
				err = StrictDecode(data, &value)
				if err == nil {
					err = value.Validate()
				}
			}
			valid := strings.HasSuffix(name, ".valid.json")
			if (err == nil) != valid {
				t.Fatalf("Go/schema fixture disagreement: valid=%t error=%v", valid, err)
			}
		})
	}
}
