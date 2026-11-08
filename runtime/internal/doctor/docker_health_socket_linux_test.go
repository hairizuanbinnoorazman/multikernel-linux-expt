//go:build linux

package doctor

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	containerdlog "github.com/containerd/log"
	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"github.com/sirupsen/logrus"
)

// The deadline deliberately closes the client before the server can reply.
// Keep the resulting transport diagnostic private to this synthetic fixture,
// but still fail on any message other than its typed broken-pipe response.
// The server is drained before checking the hook so late logs cannot escape.
type engineFixtureLogs struct {
	sync.Mutex
	allowBrokenPipe bool
	count           int
	unexpected      bool
}

func (*engineFixtureLogs) Levels() []logrus.Level { return logrus.AllLevels }

func (l *engineFixtureLogs) Fire(entry *logrus.Entry) error {
	l.Lock()
	defer l.Unlock()
	l.count++
	err, ok := entry.Data["error"].(error)
	if !l.allowBrokenPipe || l.count > 1 || entry.Level != logrus.ErrorLevel || entry.Message != "failed sending message on channel" || len(entry.Data) != 1 || !ok || !errors.Is(err, syscall.EPIPE) {
		l.unexpected = true
	}
	return nil
}

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
				logs := &engineFixtureLogs{allowBrokenPipe: mode == "deadline"}
				logger := logrus.New()
				logger.SetOutput(io.Discard)
				logger.AddHook(logs)
				ctx, cancel := context.WithCancel(containerdlog.WithLogger(t.Context(), logrus.NewEntry(logger)))
				done := make(chan error, 1)
				go func() { done <- server.Serve(ctx, listener) }()
				t.Cleanup(func() {
					cancel()
					shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
					defer stop()
					if err := server.Shutdown(shutdown); err != nil {
						t.Error("synthetic inspection server did not drain")
						_ = server.Close()
					}
					select {
					case err := <-done:
						if err != nil && !errors.Is(err, ttrpc.ErrServerClosed) {
							t.Error("synthetic inspection listener failed")
						}
					case <-shutdown.Done():
						t.Error("synthetic inspection listener did not exit")
					}
					logs.Lock()
					defer logs.Unlock()
					if logs.unexpected {
						t.Error("unexpected synthetic inspection transport diagnostic")
					}
				})
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
