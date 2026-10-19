package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestAuditNegotiationPreservesLegacyBodiesAndRejectsUnsafeQueries(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := &Server{Service: lifecycle.New(store, nil, lifecycle.Artifacts{})}
	caller := callerFunc(func(ctx context.Context, request protocol.Request, response any) *protocol.Error {
		out := server.Dispatch(ctx, request)
		if out.Error != nil {
			return out.Error
		}
		data, err := json.Marshal(out.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err = protocol.StrictDecode(data, response); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	page, err := ReadAuditEvents(context.Background(), caller, protocol.AuditQuery{Version: 1})
	if err != nil || page.Records == nil || len(page.Records) != 0 {
		t.Fatalf("negotiated read failed: %+v %v", page, err)
	}
	legacy := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "legacy", Method: "NodeInfo"})
	data, _ := json.Marshal(legacy.Body)
	var oldNode struct {
		ProtocolVersion int `json:"protocol_version"`
		Sandboxes       int `json:"sandboxes"`
	}
	if err = protocol.StrictDecode(data, &oldNode); err != nil || oldNode.ProtocolVersion != 1 {
		t.Fatalf("legacy NodeInfo changed: %v", err)
	}
	events := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "legacy-events", Method: "WatchEvents"})
	data, _ = json.Marshal(events.Body)
	var oldEvents []protocol.Event
	if err = protocol.StrictDecode(data, &oldEvents); err != nil {
		t.Fatalf("legacy WatchEvents changed: %v", err)
	}
	for _, body := range []string{"null", "[]", "{}", `{"version":1,"version":1}`, `{"version":1,"private-secret":true}`, `{"version":1,"limit":513}`} {
		out := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "bad-audit", Method: "ReadAuditEvents", Body: json.RawMessage(body)})
		if out.Error == nil || out.Body != nil || strings.Contains(out.Error.Message, "private-secret") {
			t.Fatal("invalid query returned records or reflected input")
		}
	}
	out := server.Dispatch(context.Background(), protocol.Request{Version: 1, RequestID: "bad-cap", Method: "Capabilities", Body: json.RawMessage(`{"private-secret":true}`)})
	if out.Error == nil || strings.Contains(out.Error.Message, "private-secret") {
		t.Fatal("capabilities reflected invalid input")
	}
}

func TestAuditClientNeverFallsBackToLegacyAndValidatesReplies(t *testing.T) {
	for _, mode := range []string{"old-daemon", "not-advertised", "bad-capabilities", "bad-cursor", "unsupported-after-discovery"} {
		t.Run(mode, func(t *testing.T) {
			var methods []string
			caller := callerFunc(func(_ context.Context, request protocol.Request, response any) *protocol.Error {
				methods = append(methods, request.Method)
				if request.Method == "Capabilities" {
					if mode == "old-daemon" {
						return &protocol.Error{Code: "UNSUPPORTED", Message: "unknown method"}
					}
					c := response.(*protocol.DaemonCapabilities)
					c.ProtocolVersion = 1
					c.Features = []string{}
					if mode != "not-advertised" {
						c.Features = []string{protocol.LifecycleAuditCapability}
					}
					if mode == "bad-capabilities" {
						c.Features = append(c.Features, c.Features[0])
					}
					return nil
				}
				if request.Method != "ReadAuditEvents" {
					t.Fatal("client fell back to a legacy method")
				}
				if mode == "unsupported-after-discovery" {
					return &protocol.Error{Code: "UNSUPPORTED", Message: "unavailable after discovery"}
				}
				*response.(*protocol.AuditPage) = protocol.AuditPage{Version: 1, Records: []protocol.AuditRecord{}, NextSequence: 123, HighWaterSequence: 123}
				return nil
			})
			if _, err := ReadAuditEvents(context.Background(), caller, protocol.AuditQuery{Version: 1}); err == nil {
				t.Fatal("unsafe audit response accepted")
			}
			if len(methods) > 2 || len(methods) == 0 || methods[0] != "Capabilities" {
				t.Fatalf("bad negotiation sequence: %v", methods)
			}
			if mode == "old-daemon" || mode == "not-advertised" || mode == "bad-capabilities" {
				if len(methods) != 1 {
					t.Fatal("unadvertised read was sent")
				}
			}
		})
	}
	_, err := ReadAuditEvents(context.Background(), callerFunc(func(context.Context, protocol.Request, any) *protocol.Error {
		t.Fatal("invalid local query reached server")
		return nil
	}), protocol.AuditQuery{Version: 2})
	if err == nil {
		t.Fatal("local query validation failed")
	}
}

func TestAuditClientErrorsExcludeRemoteFreeFormFields(t *testing.T) {
	for _, code := range []string{"UNSUPPORTED", "synthetic-private-code"} {
		_, err := ReadAuditEvents(context.Background(), callerFunc(func(context.Context, protocol.Request, any) *protocol.Error {
			return &protocol.Error{Code: code, Message: "synthetic-private-message", OperationID: "synthetic-private-identity", Retryable: true}
		}), protocol.AuditQuery{Version: 1})
		if err == nil || strings.Contains(err.Error(), "synthetic-private") {
			t.Fatal("collector error reflected remote free-form fields")
		}
		expected := "INTERNAL"
		if code == "UNSUPPORTED" {
			expected = code
		}
		actual, ok := ErrorCode(err)
		if !ok || actual != expected {
			t.Fatal("collector error lost stable classification")
		}
	}
}

func TestAuditUnixWireMetadataOmissionAndReadOnlyCleanup(t *testing.T) {
	// Keep the bound Unix path below sockaddr_un's limit even when the VM
	// runner selects a long private TMPDIR.
	dir, err := os.MkdirTemp("", "mk-audit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	store, err := state.Open(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secret := "synthetic-wire-credential-" + strings.Repeat("b7", 32)
	intent := state.JournalEntry{OperationID: "wire-operation", SandboxID: "box", Generation: strings.Repeat("a", 32), Method: "CreateSandbox", Phase: "intent", State: "CREATED", IdempotencyKey: secret, Fingerprint: secret}
	if err = store.Append(intent); err != nil {
		t.Fatal(err)
	}
	intent.Phase = "complete"
	intent.Error = &protocol.Error{Code: "BACKEND_FAILURE", Message: secret, OperationID: secret}
	if err = store.Append(intent); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "state", "journal.jsonl")
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: lifecycle.New(store, nil, lifecycle.Artifacts{}), AllowedUID: uint32(os.Geteuid())}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	socket := filepath.Join(dir, "audit.sock")
	go func() { done <- server.Listen(ctx, socket) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("audit listener shutdown: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("audit listener failed to join")
		}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		server.mu.Lock()
		ready := server.listener != nil
		server.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("audit socket did not become ready")
		}
		time.Sleep(time.Millisecond)
	}
	client := Client{Path: socket, Timeout: time.Second}
	for cursor := uint64(0); cursor < 2; cursor++ {
		page, err := ReadAuditEvents(ctx, client, protocol.AuditQuery{Version: 1, AfterSequence: cursor, Limit: 1})
		if err != nil || len(page.Records) != 1 || page.NextSequence != cursor+1 || page.HighWaterSequence != 2 {
			t.Fatalf("authenticated Unix audit read: %+v %v", page, err)
		}
		encoded, _ := json.Marshal(page)
		if strings.Contains(string(encoded), secret) {
			t.Fatal("wire response exported private journal content")
		}
	}
	var oldEvents []protocol.Event
	if failure := client.Call(ctx, protocol.Request{Version: 1, RequestID: "legacy-control", Method: "WatchEvents"}, &oldEvents); failure != nil || len(oldEvents) != 1 || oldEvents[0].Error.Message != secret {
		t.Fatal("legacy typed-body control changed")
	}
	after, err := os.ReadFile(journal)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("read-only Unix audit mutated its source journal")
	}
}
