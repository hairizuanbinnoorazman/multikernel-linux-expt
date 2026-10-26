package state

import (
	"encoding/json"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
)

func TestNetworkExpectationsPreserveUnknownIntentAndPrivateState(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, value := range []string{"unknown", "required", "none"} {
		sb := resourceSandbox()
		if value != "unknown" {
			sb.Config.Labels[protocol.NetworkExpectationLabel] = value
		}
		if err := st.SetSandbox(sb); err != nil {
			t.Fatal(err)
		}
		before := st.Snapshot()
		r, f := st.NetworkExpectationsSnapshot()
		if f != nil || r.Validate() != nil || len(r.Expectations) != 1 || r.Expectations[0].Expectation != value {
			t.Fatalf("expectation=%+v %v", r, f)
		}
		data, _ := json.Marshal(r)
		for _, s := range []string{"PRIVATE_BUNDLE", "PRIVATE_PATH", "PRIVATE_IMAGE", "PRIVATE_LABEL", "labels", "bundle"} {
			if strings.Contains(string(data), s) {
				t.Fatal("private state exposed")
			}
		}
		if !reflect.DeepEqual(before, st.Snapshot()) {
			t.Fatal("inspection mutated state")
		}
	}
	st.mu.Lock()
	_, f := st.NetworkExpectationsSnapshot()
	st.mu.Unlock()
	if f == nil || f.Code != "UNAVAILABLE" || !f.Retryable {
		t.Fatal("busy projection not refused")
	}
	sb := resourceSandbox()
	sb.Config.Labels[protocol.NetworkExpectationLabel] = "PRIVATE_SECRET"
	st.data.Sandboxes[sb.ID] = sb
	if _, f := st.NetworkExpectationsSnapshot(); f == nil || f.Code != "OPERATOR_ACTION" || strings.Contains(f.Message, "PRIVATE_SECRET") {
		t.Fatal("invalid intent accepted/disclosed")
	}
	st.data.Sandboxes = make(map[string]protocol.Sandbox, protocol.MaxOwnershipRecords+1)
	for n := 0; n <= protocol.MaxOwnershipRecords; n++ {
		st.data.Sandboxes[strings.Repeat("x", n+1)] = protocol.Sandbox{}
	}
	if _, f := st.NetworkExpectationsSnapshot(); f == nil || f.Code != "RESOURCE_EXHAUSTED" {
		t.Fatal("oversize inventory accepted")
	}
}
