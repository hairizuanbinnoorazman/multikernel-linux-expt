package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestComponentBuildNegotiatesWithoutServiceOrMutationAndRefusesSelectors(t *testing.T) {
	server := &Server{} // No lifecycle/store/backend exists to initialize or reconcile.
	calls := 0
	caller := callerFunc(func(ctx context.Context, r protocol.Request, body any) *protocol.Error {
		calls++
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
	report, err := InspectComponentBuild(t.Context(), caller)
	if err != nil || report.Validate() != nil || report.Component != "mkruntimed" || calls != 2 || report.ArtifactVerified || report.KernelVerified {
		t.Fatalf("%+v %v calls=%d", report, err, calls)
	}
	for _, body := range []string{"null", "[]", `{"private":"synthetic-private-token"}`, `{"version":1,"version":2}`} {
		r := server.Dispatch(t.Context(), protocol.Request{Version: 1, RequestID: "bad-build", Method: "InspectComponentBuild", Body: json.RawMessage(body)})
		if r.Error == nil || r.Error.Code != "INVALID_ARGUMENT" || r.Body != nil || strings.Contains(r.Error.Message, "synthetic") {
			t.Fatal("body accepted or reflected")
		}
	}
	for _, request := range []protocol.Request{{SandboxID: "private"}, {Generation: "private"}, {IdempotencyKey: "private"}} {
		request.Version = 1
		request.RequestID = "bad-build"
		request.Method = "InspectComponentBuild"
		r := server.Dispatch(t.Context(), request)
		if r.Error == nil || r.Body != nil {
			t.Fatal("selector accepted")
		}
	}
}
func TestComponentBuildClientRefusesLegacyInvalidIdentityAndPrivateErrors(t *testing.T) {
	for _, mode := range []string{"legacy", "duplicate-capability", "private-error", "unsupported-after-discovery", "wrong-component", "unverified-claim", "invalid-revision"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			caller := callerFunc(func(ctx context.Context, r protocol.Request, body any) *protocol.Error {
				calls++
				if mode == "private-error" {
					return &protocol.Error{Code: "UNAVAILABLE", Message: "synthetic-private-token /private/path"}
				}
				if r.Method == "Capabilities" {
					caps := body.(*protocol.DaemonCapabilities)
					*caps = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{protocol.ComponentBuildCapability}}
					if mode == "legacy" {
						caps.Features = []string{}
					}
					if mode == "duplicate-capability" {
						caps.Features = append(caps.Features, caps.Features[0])
					}
					return nil
				}
				if mode == "unsupported-after-discovery" {
					return &protocol.Error{Code: "UNSUPPORTED", Message: "synthetic-private-token"}
				}
				*body.(*protocol.ComponentBuildReport) = protocol.ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: "mkruntimed", BuildVersion: "0.0.0-dev", BuildRevision: "abcdef0"}
				v := body.(*protocol.ComponentBuildReport)
				if mode == "wrong-component" {
					v.Component = "mknetd"
				}
				if mode == "unverified-claim" {
					v.ArtifactVerified = true
				}
				if mode == "invalid-revision" {
					v.BuildRevision = "synthetic-private-token"
				}
				return nil
			})
			r, err := InspectComponentBuild(t.Context(), caller)
			if err == nil || r.Component != "" || strings.Contains(err.Error(), "synthetic-private-token") || strings.Contains(err.Error(), "/private/path") {
				t.Fatal("unsafe reply accepted/reflected", r, err)
			}
			if (mode == "legacy" || mode == "unsupported-after-discovery") && !errors.Is(err, ErrComponentBuildUnsupported) {
				t.Fatal("compatibility lost")
			}
			if (mode == "legacy" || mode == "duplicate-capability" || mode == "private-error") && calls != 1 {
				t.Fatal("legacy/invalid discovery caused fallback")
			}
		})
	}
}
func TestComponentBuildCancellationStopsBeforeSubsequentCalls(t *testing.T) {
	for _, at := range []string{"before", "after-capabilities"} {
		t.Run(at, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			if at == "before" {
				cancel()
			}
			caller := callerFunc(func(got context.Context, r protocol.Request, body any) *protocol.Error {
				calls++
				if got != ctx {
					t.Fatal("caller context replaced")
				}
				if r.Method != "Capabilities" {
					t.Fatal("read after cancellation")
				}
				*body.(*protocol.DaemonCapabilities) = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{protocol.ComponentBuildCapability}}
				cancel()
				return nil
			})
			report, err := InspectComponentBuild(ctx, caller)
			want := 1
			if at == "before" {
				want = 0
			}
			if err == nil || report.Component != "" || calls != want {
				t.Fatalf("%+v %v calls=%d", report, err, calls)
			}
		})
	}
}
