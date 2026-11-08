package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func engineReport() protocol.DockerEngineObservation {
	return protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}
}

func TestDockerEngineObservationAuthorizationAndValidation(t *testing.T) {
	for _, mode := range []string{"success", "bad-mac", "wrong-generation", "wrong-endpoint", "replay", "input-path", "unsupported", "quiesced", "quiescence-busy", "canceled", "callback-error", "invalid-report", "late-cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			s := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "generation", Endpoint: 7001, Token: []byte("synthetic-token")}
			s.ObserveDockerEngine = func(ctx context.Context) (protocol.DockerEngineObservation, error) {
				calls++
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 750*time.Millisecond {
					t.Fatal("missing callback budget")
				}
				if mode == "callback-error" {
					return engineReport(), errors.New("private-output /private/path token=secret")
				}
				r := engineReport()
				if mode == "invalid-report" {
					r.Status = "ready"
				}
				if mode == "late-cancel" {
					cancel()
				}
				return r, nil
			}
			if mode == "unsupported" {
				s.ObserveDockerEngine = nil
			}
			if mode == "quiesced" {
				s.quiesced = true
			}
			if mode == "quiescence-busy" {
				s.quiesceMu.Lock()
				defer s.quiesceMu.Unlock()
			}
			if mode == "canceled" {
				cancel()
			}
			e := Envelope{Version: 1, SandboxID: s.SandboxID, Generation: s.Generation, Endpoint: s.Endpoint, Sequence: 1, Method: "InspectDockerEngine", Body: json.RawMessage(`{}`)}
			switch mode {
			case "wrong-generation":
				e.Generation = "other"
			case "wrong-endpoint":
				e.Endpoint++
			case "input-path":
				e.Body = json.RawMessage(`{"socket":"/private/host-docker.sock"}`)
			case "replay":
				s.last = 1
			}
			Sign(&e, s.Token)
			if mode == "bad-mac" {
				e.MAC = "00"
			}
			reply := s.DispatchContext(ctx, e)
			if mode == "success" {
				if reply.Error != "" || calls != 1 {
					t.Fatal("observation failed", reply.Error)
				}
				return
			}
			if reply.Error == "" || reply.Body != nil {
				t.Fatal("refusal exposed success")
			}
			wantCalls := 0
			if mode == "callback-error" || mode == "invalid-report" || mode == "late-cancel" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("unqualified callback invoked %d times", calls)
			}
			wire, err := json.Marshal(reply)
			if err != nil || strings.Contains(string(wire), "private") || strings.Contains(string(wire), "secret") {
				t.Fatal("private diagnostic escaped")
			}
		})
	}
}

func TestDockerEngineObservationClientNegotiation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{true: "enabled", false: "unsupported"}[enabled], func(t *testing.T) {
			peer, conn := net.Pipe()
			defer peer.Close()
			defer conn.Close()
			calls := 0
			s := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "generation", Endpoint: 7001, Token: []byte("synthetic-token")}
			if enabled {
				s.ObserveDockerEngine = func(context.Context) (protocol.DockerEngineObservation, error) { calls++; return engineReport(), nil }
			}
			done := make(chan error, 1)
			go func() { done <- s.ServeConn(t.Context(), conn) }()
			c := &Client{conn: peer, sandboxID: s.SandboxID, generation: s.Generation, endpoint: s.Endpoint, token: s.Token}
			r, err := c.InspectDockerEngine(t.Context())
			if enabled {
				if err != nil || r.Status != "responding" {
					t.Fatal("negotiated observation failed", err)
				}
			} else if !errors.Is(err, ErrDockerEngineObservationUnsupported) || r.Status != "" {
				t.Fatal("unsupported agent accepted", err)
			}
			peer.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("server did not finish")
			}
			want := 0
			if enabled {
				want = 1
			}
			if calls != want {
				t.Fatal("wrong callback count")
			}
		})
	}
}
