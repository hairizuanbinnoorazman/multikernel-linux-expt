package doctor

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// A scripted Unix server exercises the actual negotiated clients and their wire
// validators. It is not a production daemon, backend CHECK or admission server.
func healthRPCSocket(t *testing.T, reply func([]byte) any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "health.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				var request json.RawMessage
				if json.NewDecoder(io.LimitReader(conn, 1<<20)).Decode(&request) != nil {
					return
				}
				_ = json.NewEncoder(conn).Encode(reply(request))
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done; workers.Wait() })
	return path
}

func TestHealthReportNegotiatesRealUnixClientsWithoutLegacyFallback(t *testing.T) {
	for _, mode := range []string{"matched", "journal-failed", "network-failed", "legacy-journal", "legacy-network", "malformed-journal", "malformed-network"} {
		t.Run(mode, func(t *testing.T) {
			c, p, b, fixture := healthFixture(t)
			journal, _ := fixture.Journal(t.Context())
			health, _ := fixture.Network(t.Context())
			if mode == "journal-failed" {
				journal.Status = "failed"
				journal.JournalFailed = true
			}
			if mode == "network-failed" {
				health.Status = "degraded"
				health.Endpoints[0].Code = "BACKEND_CHECK_FAILED"
			}
			var runtimeCalls, networkCalls atomic.Int64
			runtimeSocket := healthRPCSocket(t, func(data []byte) any {
				runtimeCalls.Add(1)
				var request protocol.Request
				if protocol.StrictDecode(data, &request) != nil {
					t.Error("invalid runtime wire request")
				}
				response := map[string]any{"version": 1, "request_id": request.RequestID}
				switch request.Method {
				case "Capabilities":
					features := []string{protocol.JournalStatusCapability}
					if mode == "legacy-journal" {
						features = []string{}
					}
					response["body"] = protocol.DaemonCapabilities{ProtocolVersion: 1, Features: features}
				case "InspectJournalStatus":
					response["body"] = journal
					if mode == "malformed-journal" {
						response["body"] = map[string]any{"version": 1, "source": "in-process-lifecycle-journal", "status": "idle", "sequence": journal.Sequence}
					}
				default:
					t.Error("health inspection attempted unrelated runtime method")
				}
				return response
			})
			networkSocket := healthRPCSocket(t, func(data []byte) any {
				networkCalls.Add(1)
				var request network.Request
				if protocol.StrictDecode(data, &request) != nil {
					t.Error("invalid network wire request")
				}
				response := network.Response{Version: 1, RequestID: request.RequestID}
				switch request.Method {
				case "CAPABILITIES":
					features := []string{network.HealthCapability}
					if mode == "legacy-network" {
						features = []string{network.OwnershipCapability}
					}
					response.Capabilities = &network.Capabilities{Version: 1, Features: features}
				case "INSPECT_HEALTH":
					v := health
					response.Health = &v
					if mode == "malformed-network" {
						response.Health.Status = "ready"
					}
				default:
					t.Error("health inspection attempted unrelated network method")
				}
				return response
			})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			inputs := HealthInputs{Journal: func(ctx context.Context) (protocol.JournalStatusReport, error) {
				return daemon.InspectJournalStatus(ctx, daemon.Client{Path: runtimeSocket, Timeout: time.Second})
			}, Network: func(ctx context.Context) (network.HealthReport, error) {
				return network.InspectHealth(ctx, network.Client{Path: networkSocket, Timeout: time.Second})
			}}
			r := InspectHealth(ctx, c, p, b, inputs, "inspector")
			want := "consistent"
			if mode == "journal-failed" || mode == "network-failed" {
				want = "degraded"
			} else if mode != "matched" {
				want = "inconclusive"
			}
			if r.Status != want {
				t.Fatal("wire health aggregate severity mismatch", r.Status, r.Findings)
			}
			if want == "inconclusive" {
				if r.Operational != nil || r.Journal != nil || r.Network != nil {
					t.Fatal("partial wire failure retained payload")
				}
			} else if runtimeCalls.Load() != 4 || networkCalls.Load() != 4 || r.Journal == nil || r.Network == nil {
				t.Fatal("health negotiation/sampling incomplete", runtimeCalls.Load(), networkCalls.Load())
			}
			if mode == "legacy-journal" && (runtimeCalls.Load() != 1 || networkCalls.Load() != 0) {
				t.Fatal("runtime legacy fallback attempted")
			}
			if mode == "legacy-network" && (runtimeCalls.Load() != 2 || networkCalls.Load() != 1) {
				t.Fatal("network legacy fallback attempted")
			}
		})
	}
}
