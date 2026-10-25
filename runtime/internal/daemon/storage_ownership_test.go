package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestStorageOwnershipNegotiationAndStrictBody(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	storageStore, err := storage.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := lifecycle.New(store, nil, lifecycle.Artifacts{})
	service.SetStorage(storage.NewService(storageStore, nil))
	server := &Server{Service: service}
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
	report, err := InspectStorageOwnership(context.Background(), caller)
	if err != nil || report.Validate() != nil {
		t.Fatalf("inspection: %+v %v", report, err)
	}
	for _, body := range []string{"null", "[]", `{"secret":"synthetic-private-secret"}`, `{"x":1,"x":2}`} {
		out := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "bad-ownership", Method: "InspectStorageOwnership", Body: json.RawMessage(body)})
		if out.Error == nil || out.Body != nil || strings.Contains(out.Error.Message, "synthetic-private-secret") {
			t.Fatal("invalid ownership query accepted or reflected")
		}
	}
	service.SetStorage(nil)
	if _, err = InspectStorageOwnership(context.Background(), caller); !errors.Is(err, ErrStorageOwnershipUnsupported) {
		t.Fatal("missing storage service advertised support")
	}
	out := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "unsupported-storage", Method: "InspectStorageOwnership"})
	if out.Error == nil || out.Error.Code != "UNSUPPORTED" || out.Body != nil {
		t.Fatal("missing storage service returned an inventory")
	}
}

func TestStorageOwnershipClientRefusesUnsafeRepliesAndLegacyFallback(t *testing.T) {
	for _, mode := range []string{"unavailable", "old-daemon", "bad-capabilities", "bad-report", "remote-error"} {
		t.Run(mode, func(t *testing.T) {
			caller := callerFunc(func(_ context.Context, r protocol.Request, body any) *protocol.Error {
				if r.Method == "Capabilities" {
					if mode == "unavailable" {
						return &protocol.Error{Code: "UNAVAILABLE", Message: "synthetic-private-secret"}
					}
					c := body.(*protocol.DaemonCapabilities)
					c.ProtocolVersion = 1
					c.Features = []string{protocol.StorageOwnershipCapability}
					if mode == "old-daemon" {
						c.Features = []string{}
					}
					if mode == "bad-capabilities" {
						c.Features = append(c.Features, c.Features[0])
					}
					return nil
				}
				if mode == "old-daemon" || mode == "bad-capabilities" || r.Method != "InspectStorageOwnership" {
					t.Fatal("unsafe fallback")
				}
				if mode == "remote-error" {
					return &protocol.Error{Code: "synthetic-private-secret", Message: "synthetic-private-secret"}
				}
				*body.(*protocol.StorageOwnershipReport) = protocol.StorageOwnershipReport{Version: 1, Source: "synthetic-private-secret"}
				return nil
			})
			report, err := InspectStorageOwnership(context.Background(), caller)
			if errors.Is(err, ErrStorageOwnershipUnsupported) != (mode == "old-daemon") {
				t.Fatalf("incorrect capability classification for %s: %v", mode, err)
			}
			if err == nil || report.Exports != nil || strings.Contains(err.Error(), "synthetic-private-secret") {
				t.Fatalf("unsafe reply accepted/reflected: %+v %v", report, err)
			}
		})
	}
}
