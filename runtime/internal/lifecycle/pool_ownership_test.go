package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPoolOwnershipReadOnlyRedactedAndCanceled(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := New(store, nil, Artifacts{}) // Inspection must not call a backend.
	if _, f := svc.PoolOwnership(context.Background()); f == nil || f.Code != "UNSUPPORTED" {
		t.Fatal("unset configuration accepted")
	}
	svc.SetPoolCPUs([]int{15, 8, 10})
	svc.SetPoolMemory(8<<30, 1<<30)
	svc.poolReady = true
	if err = store.SetSandbox(ownershipFixture("private-task")); err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	report, f := svc.PoolOwnership(context.Background())
	if f != nil || report.Validate() != nil || !reflect.DeepEqual(report.ConfiguredCPUs, []int{8, 10, 15}) || !report.Initialized {
		t.Fatalf("%+v %v", report, f)
	}
	data, _ := json.Marshal(report)
	for _, secret := range []string{"synthetic-private-secret", "secret-private-path", "labels", "bundle"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("private metadata escaped")
		}
	}
	if !reflect.DeepEqual(before, store.Snapshot()) {
		t.Fatal("inspection mutated state")
	}
	svc.global.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, f = svc.PoolOwnership(ctx)
	svc.global.Unlock()
	if f == nil || f.Code != "UNAVAILABLE" || time.Since(start) > time.Second {
		t.Fatalf("blocked inspection did not honor cancellation: %v", f)
	}
}
