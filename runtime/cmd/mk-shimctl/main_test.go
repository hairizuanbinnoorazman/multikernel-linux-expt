//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestShimctlBoundInspectionAndRefusal(t *testing.T) {
	for _, mode := range []string{"responding", "unavailable", "wrong-owner", "private-error", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(dir, "shim.sock")
			listener, err := unixsocket.Listen(socket, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			server, err := ttrpc.NewServer(ttrpc.WithServerHandshaker(ttrpc.UnixSocketRequireSameUser()))
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			owner := inspection.Request{Version: 1, Namespace: "moby", ContainerID: "container", SandboxID: "box", Generation: strings.Repeat("a", 32)}
			inspection.Register(server, func(ctx context.Context, q inspection.Request) (inspection.Report, error) {
				if q != owner {
					t.Error("owner drift")
				}
				if mode == "private-error" {
					return inspection.Report{}, errors.New("private-output /private/path secret")
				}
				if mode == "deadline" {
					<-ctx.Done()
					return inspection.Report{}, ctx.Err()
				}
				r := inspection.Report{Version: 1, Owner: q, Observation: protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}}
				if mode == "unavailable" {
					r.Observation.Status = "unavailable"
				}
				if mode == "wrong-owner" {
					r.Owner.Generation = strings.Repeat("b", 32)
				}
				return r, nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx, listener) }()
			budget, stop := context.WithTimeout(ctx, time.Second)
			defer stop()
			if mode == "deadline" {
				stop()
				budget, stop = context.WithTimeout(ctx, 20*time.Millisecond)
				defer stop()
			}
			var output bytes.Buffer
			args := []string{"--socket=" + socket, "--namespace=moby", "--container-id=container", "--sandbox-id=box", "--generation=" + owner.Generation}
			code := run(budget, args, &output)
			if mode == "responding" {
				if code != 0 {
					t.Fatal("valid inspection refused")
				}
			} else if code != 1 {
				t.Fatal("invalid inspection accepted")
			}
			if mode == "responding" || mode == "unavailable" {
				r, err := inspection.DecodeReport(output.Bytes(), owner)
				if err != nil || r.Observation.Status != mode {
					t.Fatal("wrong result", err)
				}
			} else if output.Len() != 0 {
				t.Fatal("failed inspection exposed success")
			}
			cancel()
			server.Close()
			listener.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
			}
		})
	}
}
