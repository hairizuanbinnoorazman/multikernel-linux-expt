package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
)

type poolCaller struct {
	server         *Server
	calls          int
	legacy         bool
	privateFailure bool
}

func (p *poolCaller) Call(ctx context.Context, r protocol.Request, v any) *protocol.Error {
	p.calls++
	if p.privateFailure {
		return &protocol.Error{Code: "UNAVAILABLE", Message: "synthetic-private-token"}
	}
	if p.legacy {
		data, _ := json.Marshal(protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{}})
		_ = json.Unmarshal(data, v)
		return nil
	}
	response := p.server.Dispatch(ctx, r)
	if response.Error != nil {
		return response.Error
	}
	data, _ := json.Marshal(response.Body)
	if json.Unmarshal(data, v) != nil {
		panic("fixture encoding")
	}
	return nil
}
func TestPoolOwnershipRPCNegotiatedReadOnlyAndStrict(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := lifecycle.New(store, nil, lifecycle.Artifacts{})
	svc.SetPoolCPUs([]int{8, 9})
	svc.SetPoolMemory(8<<30, 1<<30)
	server := &Server{Service: svc}
	before := store.Snapshot()
	caller := &poolCaller{server: server}
	report, err := InspectPoolOwnership(context.Background(), caller)
	if err != nil || report.Validate() != nil || caller.calls != 2 {
		t.Fatalf("%+v %v calls=%d", report, err, caller.calls)
	}
	bad := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "test", Method: "InspectPoolOwnership", Body: json.RawMessage(`{"private":"synthetic-private-token"}`)})
	if bad.Error == nil || bad.Error.Code != "INVALID_ARGUMENT" || strings.Contains(bad.Error.Message, "synthetic") {
		t.Fatal("invalid body accepted or reflected")
	}
	legacy := &poolCaller{legacy: true}
	if _, err = InspectPoolOwnership(context.Background(), legacy); !errors.Is(err, ErrPoolOwnershipUnsupported) || legacy.calls != 1 {
		t.Fatal("legacy fallback occurred")
	}
	if _, err = InspectPoolOwnership(context.Background(), &poolCaller{privateFailure: true}); err == nil || strings.Contains(err.Error(), "synthetic-private-token") {
		t.Fatal("RPC error was not sanitized")
	}
	if !reflect.DeepEqual(before, store.Snapshot()) {
		t.Fatal("inspection mutated durable state")
	}
}
