package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostconfig"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestJournalStatusNegotiationRejectsMalformedLegacyAndPrivateErrors(t *testing.T) {
	for _, mode := range []string{"idle", "pending", "failed", "legacy", "duplicate-capability", "private-error", "bad-source", "bool-equivalent-status", "pending-ahead", "unsupported-after-discovery", "canceled-after-discovery"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			caller := callerFunc(func(ctx context.Context, req protocol.Request, body any) *protocol.Error {
				calls++
				if mode == "private-error" {
					return &protocol.Error{Code: "INTERNAL", Message: "private-credential /private/path"}
				}
				if req.Method == "Capabilities" {
					caps := body.(*protocol.DaemonCapabilities)
					*caps = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{protocol.JournalStatusCapability}}
					if mode == "legacy" {
						caps.Features = []string{protocol.ComponentBuildCapability}
					}
					if mode == "duplicate-capability" {
						caps.Features = append(caps.Features, caps.Features[0])
					}
					if mode == "canceled-after-discovery" {
						cancel()
					}
					return nil
				}
				if mode == "unsupported-after-discovery" {
					return &protocol.Error{Code: "UNSUPPORTED", Message: "private-credential"}
				}
				r := body.(*protocol.JournalStatusReport)
				*r = protocol.JournalStatusReport{Version: 1, Source: "in-process-lifecycle-journal", Status: "idle"}
				switch mode {
				case "pending":
					r.Status = "pending"
					r.Sequence = 1
					r.PendingCompletions = 1
				case "failed":
					r.Status = "failed"
					r.JournalFailed = true
				case "bad-source":
					r.Source = "complete-runtime-readiness"
				case "bool-equivalent-status":
					r.JournalFailed = true
				case "pending-ahead":
					r.Status = "pending"
					r.PendingCompletions = 1
				}
				return nil
			})
			r, err := InspectJournalStatus(ctx, caller)
			if mode == "idle" || mode == "pending" || mode == "failed" {
				if err != nil || r.Validate() != nil || calls != 2 {
					t.Fatal(r, err, calls)
				}
				return
			}
			if err == nil || r.Version != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe response accepted/reflected", r, err)
			}
			if (mode == "legacy" || mode == "unsupported-after-discovery") && !errors.Is(err, ErrJournalStatusUnsupported) {
				t.Fatal("compatibility lost")
			}
			if (mode == "legacy" || mode == "duplicate-capability" || mode == "private-error" || mode == "canceled-after-discovery") && calls != 1 {
				t.Fatal("unexpected fallback")
			}
		})
	}
}

func TestJournalStatusRefusesSelectorsBodiesAndMissingService(t *testing.T) {
	s, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	server := &Server{Service: lifecycle.New(s, nil, lifecycle.Artifacts{})}
	for _, req := range []protocol.Request{{SandboxID: "private"}, {Generation: "private"}, {IdempotencyKey: "private"}, {Body: json.RawMessage(`{"private":"token"}`)}, {Body: json.RawMessage(`null`)}, {Body: json.RawMessage(`[]`)}} {
		req.Version = 1
		req.RequestID = "status"
		req.Method = "InspectJournalStatus"
		out := server.Dispatch(t.Context(), req)
		if out.Error == nil || out.Error.Code != "INVALID_ARGUMENT" || out.Body != nil {
			t.Fatal(out)
		}
	}
	out := (&Server{}).Dispatch(t.Context(), protocol.Request{Version: 1, RequestID: "status", Method: "InspectJournalStatus"})
	if out.Error == nil || out.Error.Code != "UNSUPPORTED" {
		t.Fatal(out)
	}
}

// This fixture uses the real protected Unix RPC and real journal identity
// refusal, without any Kerf backend or child/resource allocation.
func TestJournalStatusUnixRPCDetectsFailedJournalWhileMetadataStillResponds(t *testing.T) {
	dir, err := os.MkdirTemp("", "mk-journal-")
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
	server := &Server{Service: lifecycle.New(store, nil, lifecycle.Artifacts{}), AllowedUID: uint32(os.Geteuid())}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	defer server.Close()
	socket := filepath.Join(dir, "journal.sock")
	done := make(chan error, 1)
	go func() { done <- server.Listen(ctx, socket) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Lstat(socket); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("listener refused: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("socket unavailable")
		}
		time.Sleep(time.Millisecond)
	}
	client := Client{Path: socket, Timeout: time.Second}
	cli := os.Getenv("MK_G8_JOURNAL_CLI")
	configPath := filepath.Join(dir, "host.json")
	if cli != "" {
		if os.Geteuid() != 0 || !filepath.IsAbs(cli) {
			t.Fatal("CLI fixture requires root and an explicit absolute binary")
		}
		data, err := os.ReadFile(os.Getenv("MK_G8_JOURNAL_CONFIG_TEMPLATE"))
		if err != nil {
			t.Fatal(err)
		}
		var config hostconfig.Config
		if err := protocol.StrictDecode(data, &config); err != nil {
			t.Fatal(err)
		}
		config.SocketPath, config.StateDirectory = socket, filepath.Join(dir, "state")
		data, err = json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	checkCLI := func(expected protocol.JournalStatusReport) {
		t.Helper()
		if cli == "" {
			return
		}
		command := exec.CommandContext(ctx, cli, "--inspect-journal", "--config="+configPath)
		var stdout, stderr strings.Builder
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		exit := 0
		if err != nil {
			var failure *exec.ExitError
			if !errors.As(err, &failure) {
				t.Fatal("journal CLI could not execute")
			}
			exit = failure.ExitCode()
		}
		want := 0
		if expected.Status != "idle" {
			want = 1
		}
		var report protocol.JournalStatusReport
		if exit != want || stderr.Len() != 0 || protocol.StrictDecode([]byte(stdout.String()), &report) != nil || report != expected {
			t.Fatal("journal CLI status, framing or exit contract failed")
		}
	}
	before, err := InspectJournalStatus(ctx, client)
	if err != nil || before.Status != "idle" {
		t.Fatal(before, err)
	}
	checkCLI(before)
	intent := state.JournalEntry{OperationID: "private-pending", SandboxID: "private-sandbox", Generation: "generation", Method: "DeleteSandbox", Phase: "intent", IdempotencyKey: "private-credential", Fingerprint: "private-fingerprint"}
	if err := store.Append(intent); err != nil {
		t.Fatal(err)
	}
	pending, err := InspectJournalStatus(ctx, client)
	if err != nil || pending.Status != "pending" || pending.PendingCompletions != 1 || pending.Sequence != 1 {
		t.Fatal(pending, err)
	}
	checkCLI(pending)
	path := filepath.Join(dir, "state/journal.jsonl")
	heldBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("private-replacement-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(state.JournalEntry{OperationID: "private-operation", SandboxID: "private-sandbox", Phase: "intent"}); err == nil {
		t.Fatal("replaced journal accepted")
	}
	metadata, err := InspectOwnership(ctx, client)
	if err != nil || metadata.Validate() != nil {
		t.Fatal("metadata became unavailable", metadata, err)
	}
	after, err := InspectJournalStatus(ctx, client)
	if err != nil || after.Validate() != nil || after.Status != "failed" || !after.JournalFailed || after.Sequence != 1 || after.PendingCompletions != 1 {
		t.Fatal(after, err)
	}
	checkCLI(after)
	raw, _ := json.Marshal(after)
	if strings.Contains(string(raw), "private") {
		t.Fatal("private journal data reflected")
	}
	replacement, _ := os.ReadFile(path)
	original, _ := os.ReadFile(path + ".held")
	if string(replacement) != "private-replacement-canary" || string(original) != string(heldBefore) {
		t.Fatal("status repaired or mutated journal")
	}
	server.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server remained active")
	}
	t.Log("AUTHENTICATED_JOURNAL_STATUS_IDLE_TO_FAILED_METADATA_RESPONSIVE_NO_PRIVATE_PAYLOAD_NO_REPAIR")
	if cli != "" {
		t.Log("JOURNAL_CLI_IDLE_PENDING_FAILED_EXACT_REPORTS_EXIT_0_1_1_PASS")
	}
}

func TestJournalStatusClientBoundsDiscoveryAndReadTogether(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	caller := callerFunc(func(ctx context.Context, _ protocol.Request, body any) *protocol.Error {
		calls++
		if calls == 1 {
			*body.(*protocol.DaemonCapabilities) = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: []string{protocol.JournalStatusCapability}}
			return nil
		}
		<-ctx.Done()
		return &protocol.Error{Code: "UNAVAILABLE", Message: "private-timeout"}
	})
	start := time.Now()
	r, err := InspectJournalStatus(ctx, caller)
	if err == nil || r.Version != 0 || time.Since(start) > time.Second {
		t.Fatal(r, err)
	}
	// A real caller honors this same inherited context for both RPCs.
}
