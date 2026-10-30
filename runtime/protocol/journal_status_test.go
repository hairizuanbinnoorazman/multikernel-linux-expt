package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJournalStatusRequiresCompleteExplicitConsistentWireFields(t *testing.T) {
	valid := JournalStatusReport{Version: 1, Source: "in-process-lifecycle-journal", Status: "idle"}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var decoded JournalStatusReport
	if err := StrictDecode(raw, &decoded); err != nil || decoded != valid {
		t.Fatal(decoded, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for field := range fields {
		for _, mode := range []string{"missing", "null"} {
			t.Run(field+"-"+mode, func(t *testing.T) {
				copy := map[string]json.RawMessage{}
				for k, v := range fields {
					copy[k] = v
				}
				if mode == "missing" {
					delete(copy, field)
				} else {
					copy[field] = json.RawMessage("null")
				}
				invalid, _ := json.Marshal(copy)
				decoded = valid
				if err := StrictDecode(invalid, &decoded); err == nil || decoded.Version != 0 {
					t.Fatal("incomplete status accepted or retained", decoded, err)
				}
			})
		}
	}
	for _, bad := range [][]byte{append([]byte(`{"version":1,`), raw[1:]...), []byte(strings.Replace(string(raw), `"sequence":0`, `"sequence":false`, 1)), []byte(strings.Replace(string(raw), `"journal_failed":false`, `"journal_failed":true`, 1)), []byte(strings.Replace(string(raw), `"pending_completions":0`, `"pending_completions":1`, 1)), []byte(strings.Replace(string(raw), `"status":"idle"`, `"status":"ready"`, 1)), []byte(strings.Replace(string(raw), `"sequence":0`, `"sequence":0,"private":"credential"`, 1))} {
		if err := StrictDecode(bad, &decoded); err == nil || strings.Contains(err.Error(), "credential") {
			t.Fatal("unsafe status accepted/reflected", err)
		}
	}
}

func TestJournalStatusRefusesCaseAliasesThatMaskFailureAndOversize(t *testing.T) {
	valid := `{"version":1,"source":"in-process-lifecycle-journal","status":"idle","sequence":0,"pending_completions":0,"journal_failed":false}`
	for _, raw := range []string{
		strings.Replace(valid, `"journal_failed":false`, `"journal_failed":true,"JOURNAL_FAILED":false`, 1),
		strings.Replace(valid, `"journal_failed":false`, `"JOURNAL_FAILED":false`, 1),
		strings.Replace(valid, `"sequence":0`, `"sequence":1,"SEQUENCE":0`, 1),
		strings.Replace(valid, `"source":"in-process-lifecycle-journal"`, `"source":"private-credential","SOURCE":"in-process-lifecycle-journal"`, 1),
		valid[:len(valid)-1] + strings.Repeat(" ", 4096) + "}",
	} {
		var report JournalStatusReport
		err := StrictDecode([]byte(raw), &report)
		if err == nil || report.Version != 0 || strings.Contains(err.Error(), "private-credential") {
			t.Fatal("alias masking or oversize accepted/reflected", report, err)
		}
	}
}
