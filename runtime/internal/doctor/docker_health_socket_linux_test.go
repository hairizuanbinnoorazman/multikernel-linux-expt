//go:build linux

package doctor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestDockerHealthThroughProtectedShimSockets(t *testing.T) {
	for _, mode := range []string{"responding", "unavailable", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			in, _, targets := dockerHealthFixture(t)
			targets = append([]EngineTarget{}, targets...)
			var calls atomic.Int32
			for i := range targets {
				dir := t.TempDir()
				_ = os.Chmod(dir, 0700)
				address := filepath.Join(dir, "shim.sock")
				listener, err := net.Listen("unix", address)
				if err != nil {
					t.Fatal(err)
				}
				_ = os.Chmod(address, 0600)
				server, err := ttrpc.NewServer(ttrpc.WithServerHandshaker(ttrpc.UnixSocketRequireSameUser()))
				if err != nil {
					t.Fatal(err)
				}
				owner := targets[i].Owner
				inspection.Register(server, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
					calls.Add(1)
					if q != owner {
						t.Error("wrong owning shim target")
					}
					if mode == "deadline" {
						<-ctx.Done()
						return inspection.Report{}, ctx.Err()
					}
					state := "responding"
					if mode == "unavailable" {
						state = mode
					}
					return inspection.Report{Version: 1, Owner: q, Observation: protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: state}}, nil
				})
				ctx, cancel := context.WithCancel(t.Context())
				go func() { _ = server.Serve(ctx, listener) }()
				t.Cleanup(func() { cancel(); _ = server.Close(); _ = listener.Close() })
				targets[i].Socket = address
			}
			in.Targets = func(context.Context) ([]EngineTarget, error) { return targets, nil }
			in.Engine = func(ctx context.Context, target EngineTarget) (inspection.Report, error) {
				return inspection.ObserveSocket(ctx, target.Socket, target.Owner)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			if mode == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 40*time.Millisecond)
				defer cancel()
			}
			r := InspectDockerHealth(ctx, in, "inspector")
			want := "consistent"
			if mode == "unavailable" {
				want = "degraded"
			}
			if mode == "deadline" {
				want = "inconclusive"
			}
			if r.Status != want {
				t.Fatalf("wire aggregate %s: %+v", mode, r)
			}
			if mode != "deadline" && calls.Load() != int32(2*len(targets)) {
				t.Fatal("wire request repeated or missing")
			}
			if mode == "deadline" && (calls.Load() != 1 || r.Health != nil || r.Engines != nil) {
				t.Fatal("deadline did not stop subsequent work")
			}
		})
	}
}
