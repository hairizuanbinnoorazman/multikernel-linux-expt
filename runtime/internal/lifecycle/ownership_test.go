package lifecycle

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func ownershipFixture(id string) protocol.Sandbox {
	return protocol.Sandbox{ID: id, Generation: strings.Repeat("a", 32), State: "RUNNING", Config: protocol.SandboxConfig{ID: id, CPUs: []int{10, 8}, MemoryBytes: 4 << 30, ChildCID: 27, AgentPort: 7717, Bundle: "/secret-private-path", Labels: map[string]string{"token": "synthetic-private-secret"}}, Error: &protocol.Error{Code: "BACKEND_FAILURE", Message: "synthetic-private-secret"}}
}

func TestOwnershipProjectionIsReadOnlyAndBounded(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	service := New(store, nil, Artifacts{}) // A backend call would panic.
	report, failure := service.Ownership()
	if failure != nil || report.Validate() != nil || len(report.Sandboxes) != 0 {
		t.Fatalf("empty report: %+v %v", report, failure)
	}
	for _, id := range []string{"work-b", "work-a"} {
		if err = store.SetSandbox(ownershipFixture(id)); err != nil {
			t.Fatal(err)
		}
	}
	before := store.Snapshot()
	report, failure = service.Ownership()
	if failure != nil || report.Validate() != nil {
		t.Fatalf("active report: %+v %v", report, failure)
	}
	if report.Sandboxes[0].SandboxID != "work-a" || !reflect.DeepEqual(report.Sandboxes[0].CPUs, []int{8, 10}) {
		t.Fatal("inventory is not deterministic")
	}
	encoded, _ := json.Marshal(report)
	for _, secret := range []string{"synthetic-private-secret", "secret-private-path", "BACKEND_FAILURE", "labels", "bundle", "error"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("ownership leaked %q", secret)
		}
	}
	if !reflect.DeepEqual(before, store.Snapshot()) {
		t.Fatal("inspection mutated durable state")
	}
	for i := 2; i <= protocol.MaxOwnershipRecords; i++ {
		if err = store.SetSandbox(ownershipFixture(fmt.Sprintf("work-%03d", i))); err != nil {
			t.Fatal(err)
		}
	}
	report, failure = service.Ownership()
	if failure == nil || failure.Code != "RESOURCE_EXHAUSTED" || report.Sandboxes != nil {
		t.Fatal("oversized inventory returned partial success")
	}
}

func TestOwnershipRefusesInvalidMetadataWithoutReflection(t *testing.T) {
	for _, mode := range []string{"id", "generation", "state", "cpu", "cpu-limit", "cid"} {
		t.Run(mode, func(t *testing.T) {
			store, err := state.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			sb := ownershipFixture("work-a")
			want := "OPERATOR_ACTION"
			switch mode {
			case "id":
				sb.Config.ID = "wrong-id"
			case "generation":
				sb.Generation = "synthetic-private-secret"
			case "state":
				sb.State = "synthetic-private-secret"
			case "cpu":
				sb.Config.CPUs = []int{8, 8}
			case "cpu-limit":
				sb.Config.CPUs = make([]int, protocol.MaxOwnershipCPUs+1)
				want = "RESOURCE_EXHAUSTED"
			case "cid":
				sb.Config.ChildCID = 2
			}
			if err = store.SetSandbox(sb); err != nil {
				t.Fatal(err)
			}
			report, failure := New(store, nil, Artifacts{}).Ownership()
			if failure == nil || failure.Code != want || report.Sandboxes != nil || strings.Contains(failure.Message, "synthetic-private-secret") {
				t.Fatalf("unsafe inspection: %+v %+v", report, failure)
			}
		})
	}
}
