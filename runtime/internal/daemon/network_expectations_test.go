package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"strings"
	"testing"
)

func TestNetworkExpectationsNegotiatedReadOnlyAndLegacyRefusal(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := &Server{Service: lifecycle.New(st, nil, lifecycle.Artifacts{})}
	caller := callerFunc(func(ctx context.Context, r protocol.Request, body any) *protocol.Error {
		out := server.Dispatch(ctx, r)
		if out.Error != nil {
			return out.Error
		}
		data, _ := json.Marshal(out.Body)
		if err := protocol.StrictDecode(data, body); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	r, err := InspectNetworkExpectations(t.Context(), caller)
	if err != nil || r.Validate() != nil || len(r.Expectations) != 0 {
		t.Fatalf("RPC: %+v %v", r, err)
	}
	for _, body := range []string{"null", `{"secret":"PRIVATE_SECRET"}`, `{"x":1,"x":2}`} {
		out := server.Dispatch(t.Context(), protocol.Request{Version: 1, RequestID: "bad", Method: "InspectNetworkExpectations", Body: json.RawMessage(body)})
		if out.Error == nil || out.Body != nil || strings.Contains(out.Error.Message, "PRIVATE_SECRET") {
			t.Fatal("invalid read body accepted/reflected")
		}
	}
	legacy := callerFunc(func(_ context.Context, r protocol.Request, body any) *protocol.Error {
		if r.Method != "Capabilities" {
			t.Fatal("legacy fallback")
		}
		*body.(*protocol.DaemonCapabilities) = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{protocol.OwnershipCapability}}
		return nil
	})
	if _, err := InspectNetworkExpectations(t.Context(), legacy); !errors.Is(err, ErrNetworkExpectationsUnsupported) {
		t.Fatalf("legacy: %v", err)
	}
}
