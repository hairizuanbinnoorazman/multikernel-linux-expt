//go:build linux

package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A real Unix peer holds the first frame open. A queued cancellation must not
// wait for that owner, write a frame, dial a replacement, consume a sequence or
// poison the active stream. The owner still completes after its release.
func TestAgentClientContextQueueCancellationIsSideEffectFree(t *testing.T) {
	for _, method := range []string{"call", "reconnect", "peer-reconnect"} {
		for _, mode := range []string{"cancel", "deadline"} {
			t.Run(method+"/"+mode, func(t *testing.T) {
				dir, err := os.MkdirTemp("/tmp", "agent-gate-")
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(dir)
				path := filepath.Join(dir, "peer.sock")
				listener, err := net.Listen("unix", path)
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				conn, err := net.Dial("unix", path)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				peer, err := listener.Accept()
				if err != nil {
					t.Fatal(err)
				}
				defer peer.Close()
				key := []byte("01234567890123456789012345678901")
				client := &Client{conn: conn, token: key, sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7200, sequence: 17, Timeout: 5 * time.Second}
				var dialCalls atomic.Int32
				client.reconnectDial = func(ctx context.Context, _ string) (net.Conn, error) { dialCalls.Add(1); return nil, ctx.Err() }
				client.peerDial = func(ctx context.Context, _ string, _ int) (net.Conn, error) { dialCalls.Add(1); return nil, ctx.Err() }
				entered := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				defer unblock()
				served := make(chan error, 1)
				go func() {
					var header [4]byte
					if _, e := io.ReadFull(peer, header[:]); e != nil {
						served <- e
						return
					}
					size := binary.BigEndian.Uint32(header[:])
					if size > 4096 {
						served <- errors.New("oversized fixture frame")
						return
					}
					payload := make([]byte, size)
					if _, e := io.ReadFull(peer, payload); e != nil {
						served <- e
						return
					}
					var request Envelope
					if json.Unmarshal(payload, &request) != nil {
						served <- errors.New("invalid fixture frame")
						return
					}
					signed := request
					Sign(&signed, key)
					if request.Method != "Capabilities" || request.Sequence != 18 || request.SandboxID != "box" || signed.MAC != request.MAC {
						served <- errors.New("owner request changed")
						return
					}
					close(entered)
					<-release
					body, _ := json.Marshal(Reply{Version: 1, Sequence: request.Sequence, Body: map[string]bool{"ok": true}})
					binary.BigEndian.PutUint32(header[:], uint32(len(body)))
					if _, e := peer.Write(append(header[:], body...)); e != nil {
						served <- e
						return
					}
					peer.SetReadDeadline(time.Now().Add(75 * time.Millisecond))
					var extra [1]byte
					n, e := peer.Read(extra[:])
					var timeout net.Error
					if n != 0 || !errors.As(e, &timeout) || !timeout.Timeout() {
						served <- errors.New("cancelled waiter wrote a frame or closed the active stream")
						return
					}
					served <- nil
				}()
				ownerDone := make(chan error, 1)
				go func() {
					var response map[string]bool
					e := client.CallContext(context.Background(), "Capabilities", map[string]any{}, &response)
					if e == nil && !response["ok"] {
						e = errors.New("owner reply changed")
					}
					ownerDone <- e
				}()
				select {
				case <-entered:
				case e := <-served:
					t.Fatal("owner fixture failed", e)
				case <-time.After(time.Second):
					t.Fatal("owner did not enter wire")
				}
				var ctx context.Context
				var cancel context.CancelFunc
				expected := error(context.Canceled)
				if mode == "deadline" {
					ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
					expected = context.DeadlineExceeded
				} else {
					ctx, cancel = context.WithCancel(t.Context())
				}
				defer cancel()
				waiterDone := make(chan error, 1)
				started := make(chan struct{})
				go func() {
					close(started)
					switch method {
					case "call":
						waiterDone <- client.CallContext(ctx, "StateProcess", map[string]string{"id": "init"}, nil)
					case "reconnect":
						waiterDone <- client.ReconnectContext(ctx, "unused")
					case "peer-reconnect":
						waiterDone <- client.ReconnectPeerContext(ctx, "unused", 123)
					}
				}()
				<-started
				if mode == "cancel" {
					cancel()
				}
				returned := false
				select {
				case e := <-waiterDone:
					returned = true
					if !errors.Is(e, expected) {
						t.Error("queued caller lost context result", e)
					}
				case <-time.After(500 * time.Millisecond):
					t.Error("queued context did not return while wire owner remained blocked")
				}
				select {
				case e := <-ownerDone:
					t.Fatal("queued cancellation interrupted owner", e)
				default:
				}
				unblock()
				select {
				case e := <-ownerDone:
					if e != nil {
						t.Error("owner did not preserve its reply", e)
					}
				case <-time.After(time.Second):
					t.Fatal("owner did not complete")
				}
				if !returned {
					select {
					case <-waiterDone:
					case <-time.After(time.Second):
						t.Fatal("waiter did not join after owner release")
					}
				}
				if client.conn != conn || client.sequence != 18 || client.unusable || dialCalls.Load() != 0 {
					t.Error("queued cancellation changed transport, policy or sequence")
				}
				select {
				case e := <-served:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(time.Second):
					t.Fatal("wire fixture did not join")
				}
			})
		}
	}
}

func TestAgentClientGateConcurrentCallsPreserveFramesAndSequence(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "agent-serial-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "peer.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(5 * time.Second))
	key := []byte("01234567890123456789012345678901")
	client := &Client{conn: conn, token: key, sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7200, sequence: 17}
	const calls = 32
	served := make(chan error, 1)
	go func() {
		for i := 0; i < calls; i++ {
			var header [4]byte
			if _, e := io.ReadFull(peer, header[:]); e != nil {
				served <- e
				return
			}
			size := binary.BigEndian.Uint32(header[:])
			if size > 4096 {
				served <- errors.New("oversized serialized frame")
				return
			}
			raw := make([]byte, size)
			if _, e := io.ReadFull(peer, raw); e != nil {
				served <- e
				return
			}
			var request Envelope
			if json.Unmarshal(raw, &request) != nil {
				served <- errors.New("invalid serialized envelope")
				return
			}
			signed := request
			Sign(&signed, key)
			if request.Sequence != uint64(18+i) || signed.MAC != request.MAC || request.Method != "StateProcess" {
				served <- errors.New("interleaved frame or authentication sequence")
				return
			}
			var payload map[string]int
			if json.Unmarshal(request.Body, &payload) != nil || len(payload) != 1 {
				served <- errors.New("invalid serialized payload")
				return
			}
			body, _ := json.Marshal(Reply{Version: 1, Sequence: request.Sequence, Body: payload})
			binary.BigEndian.PutUint32(header[:], uint32(len(body)))
			frame := append(header[:], body...)
			if n, e := peer.Write(frame); e != nil || n != len(frame) {
				served <- errors.New("serialized fixture reply failed")
				return
			}
		}
		served <- nil
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, calls)
	for i := 0; i < calls; i++ {
		go func(slot int) {
			var reply map[string]int
			e := client.CallContext(ctx, "StateProcess", map[string]int{"slot": slot}, &reply)
			if e == nil && (len(reply) != 1 || reply["slot"] != slot) {
				e = errors.New("reply crossed serialized callers")
			}
			results <- e
		}(i)
	}
	for i := 0; i < calls; i++ {
		if e := <-results; e != nil {
			t.Error("concurrent caller failed", e)
		}
	}
	if e := <-served; e != nil {
		t.Error(e)
	}
	if client.sequence != 17+calls || client.unusable {
		t.Fatal("successful serialization changed sequence or stream state")
	}
}
