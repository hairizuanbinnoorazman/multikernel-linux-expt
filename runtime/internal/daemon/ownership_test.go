package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestOwnershipNegotiationAndStrictBody(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := &Server{Service: lifecycle.New(store, nil, lifecycle.Artifacts{})}
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
	report, err := InspectOwnership(context.Background(), caller)
	if err != nil || report.Validate() != nil {
		t.Fatalf("inspection: %+v %v", report, err)
	}
	for _, body := range []string{"null", "[]", `{"secret":"synthetic-private-secret"}`, `{"x":1,"x":2}`} {
		out := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "bad-ownership", Method: "InspectOwnership", Body: json.RawMessage(body)})
		if out.Error == nil || out.Body != nil || strings.Contains(out.Error.Message, "synthetic-private-secret") {
			t.Fatal("invalid ownership query accepted or reflected")
		}
	}
}

func TestOwnershipClientRefusesUnsafeRepliesAndLegacyFallback(t *testing.T) {
	for _, mode := range []string{"unavailable", "old-daemon", "bad-capabilities", "bad-report", "remote-error"} {
		t.Run(mode, func(t *testing.T) {
			caller := callerFunc(func(_ context.Context, r protocol.Request, body any) *protocol.Error {
				if r.Method == "Capabilities" {
					if mode == "unavailable" {
						return &protocol.Error{Code: "UNAVAILABLE", Message: "synthetic-private-secret"}
					}
					c := body.(*protocol.DaemonCapabilities)
					c.ProtocolVersion = 1
					c.Features = []string{protocol.OwnershipCapability}
					if mode == "old-daemon" {
						c.Features = []string{}
					}
					if mode == "bad-capabilities" {
						c.Features = append(c.Features, c.Features[0])
					}
					return nil
				}
				if mode == "old-daemon" || mode == "bad-capabilities" || r.Method != "InspectOwnership" {
					t.Fatal("unsafe fallback")
				}
				if mode == "remote-error" {
					return &protocol.Error{Code: "synthetic-private-secret", Message: "synthetic-private-secret"}
				}
				*body.(*protocol.OwnershipReport) = protocol.OwnershipReport{Version: 1, Source: "synthetic-private-secret"}
				return nil
			})
			report, err := InspectOwnership(context.Background(), caller)
			if err == nil || report.Sandboxes != nil || strings.Contains(err.Error(), "synthetic-private-secret") {
				t.Fatalf("unsafe reply accepted/reflected: %+v %v", report, err)
			}
		})
	}
}
