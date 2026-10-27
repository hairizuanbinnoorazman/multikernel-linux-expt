package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type observedAgentConn struct {
	net.Conn
	writes atomic.Int64
	closes atomic.Int64
}

func (c *observedAgentConn) Write(p []byte) (int, error) { c.writes.Add(1); return c.Conn.Write(p) }
func (c *observedAgentConn) Close() error                { c.closes.Add(1); return c.Conn.Close() }

func readAgentTestEnvelope(conn net.Conn) (Envelope, error) {
	var h [4]byte
	if _, err := io.ReadFull(conn, h[:]); err != nil {
		return Envelope{}, err
	}
	size := binary.BigEndian.Uint32(h[:])
	if size > 1<<20 {
		return Envelope{}, errors.New("test request exceeds bound")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return Envelope{}, err
	}
	var result Envelope
	err := json.Unmarshal(data, &result)
	return result, err
}
func writeAgentTestReply(conn net.Conn, reply Reply) error {
	body, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(body)))
	if err = protocol.WriteFull(conn, h[:]); err != nil {
		return err
	}
	return protocol.WriteFull(conn, body)
}

// A canceled read can leave an entire old reply pending. It must not be reused
// for a later RPC, even though the stream can still exchange valid frames.
func TestCanceledAgentCallCannotConsumeLateReplyOnNextRequest(t *testing.T) {
	left, right := net.Pipe()
	exerciseLateAgentReply(t, left, right)
}

func TestCanceledUnixAgentCallCannotConsumeLateReplyOnNextRequest(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "mk-agent-stream-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() { conn, _ := listener.Accept(); accepted <- conn }()
	left, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	select {
	case right := <-accepted:
		if right == nil {
			t.Fatal("accept failed")
		}
		exerciseLateAgentReply(t, left, right)
	case <-time.After(time.Second):
		t.Fatal("Unix accept timed out")
	}
}

func exerciseLateAgentReply(t *testing.T, left, right net.Conn) {
	t.Helper()
	wire := &observedAgentConn{Conn: left}
	defer wire.Close()
	defer right.Close()
	client := &Client{conn: wire, token: []byte("01234567890123456789012345678901"), sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7001}
	received := make(chan struct{})
	served := make(chan error, 1)
	laterRequest := make(chan bool, 1)
	go func() {
		first, err := readAgentTestEnvelope(right)
		if err != nil {
			served <- err
			return
		}
		close(received)
		_, err = readAgentTestEnvelope(right)
		if err != nil {
			laterRequest <- false
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				served <- nil
			} else {
				served <- err
			}
			return
		}
		laterRequest <- true
		served <- writeAgentTestReply(right, Reply{Version: 1, Sequence: first.Sequence, Body: map[string]any{"status": "old-request"}})
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() { firstDone <- client.CallContext(ctx, "StateProcess", map[string]string{"ID": "init"}, nil) }()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("first request did not reach peer")
	}
	cancel()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled read did not end")
	}
	writes := wire.writes.Load()
	nextCtx, nextCancel := context.WithTimeout(t.Context(), time.Second)
	defer nextCancel()
	err := client.CallContext(nextCtx, "StateProcess", map[string]string{"ID": "init"}, nil)
	t.Logf("later_call_error=%v additional_wire_writes=%d transport_closes=%d", err, wire.writes.Load()-writes, wire.closes.Load())
	if err == nil || err.Error() != "agent transport requires explicit reconnect" {
		t.Errorf("later call must require reconnect, got %v", err)
	}
	if wire.writes.Load() != writes {
		t.Error("later RPC was sent on the ambiguous stream")
	}
	if wire.closes.Load() == 0 {
		t.Error("ambiguous transport remained open")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("peer remained blocked")
	}
	if <-laterRequest {
		t.Error("peer accepted another request before explicit reconnect")
	}
}

type scriptedAgentConn struct {
	response     *bytes.Reader
	writes       int
	failWrite    int
	noProgress   bool
	failDeadline bool
	closed       bool
}

func (c *scriptedAgentConn) Read(p []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	return c.response.Read(p)
}
func (c *scriptedAgentConn) Write(p []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	c.writes++
	if c.writes == c.failWrite {
		if c.noProgress {
			return 0, nil
		}
		return len(p) / 2, io.ErrUnexpectedEOF
	}
	return len(p), nil
}
func (c *scriptedAgentConn) Close() error         { c.closed = true; return nil }
func (c *scriptedAgentConn) LocalAddr() net.Addr  { return memoryAddr("scripted-local") }
func (c *scriptedAgentConn) RemoteAddr() net.Addr { return memoryAddr("scripted-peer") }
func (c *scriptedAgentConn) SetDeadline(time.Time) error {
	if c.failDeadline {
		return errors.New("test deadline unavailable")
	}
	return nil
}
func (c *scriptedAgentConn) SetReadDeadline(d time.Time) error  { return c.SetDeadline(d) }
func (c *scriptedAgentConn) SetWriteDeadline(d time.Time) error { return c.SetDeadline(d) }

func TestAgentFramingFailuresRequireReconnectWithoutAnotherWireRequest(t *testing.T) {
	frame := func(body string) []byte {
		var h [4]byte
		binary.BigEndian.PutUint32(h[:], uint32(len(body)))
		return append(h[:], []byte(body)...)
	}
	for _, mode := range []string{"partial-request-header", "partial-request-payload", "no-progress-write", "partial-response-header", "partial-response-payload", "oversize", "malformed-json", "wrong-version", "wrong-sequence", "deadline-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			data := frame(`{"version":1,"sequence":1,"body":{}}`)
			conn := &scriptedAgentConn{}
			switch mode {
			case "partial-request-header":
				conn.failWrite = 1
			case "partial-request-payload":
				conn.failWrite = 2
			case "no-progress-write":
				conn.failWrite = 1
				conn.noProgress = true
			case "partial-response-header":
				data = []byte{0, 0}
			case "partial-response-payload":
				data = data[:len(data)-2]
			case "oversize":
				var h [4]byte
				binary.BigEndian.PutUint32(h[:], (1<<20)+1)
				data = h[:]
			case "malformed-json":
				data = frame("{")
			case "wrong-version":
				data = frame(`{"version":2,"sequence":1,"body":{}}`)
			case "wrong-sequence":
				data = frame(`{"version":1,"sequence":2,"body":{}}`)
			case "deadline-unavailable":
				conn.failDeadline = true
			}
			conn.response = bytes.NewReader(data)
			client := &Client{conn: conn, token: []byte("01234567890123456789012345678901"), sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7001}
			if err := client.CallContext(t.Context(), "Capabilities", map[string]any{}, nil); err == nil {
				t.Fatal("fault was accepted")
			}
			if !conn.closed {
				t.Fatal("ambiguous stream remained open")
			}
			writes, sequence := conn.writes, client.sequence
			if err := client.CallContext(t.Context(), "Capabilities", map[string]any{}, nil); !errors.Is(err, ErrReconnectRequired) {
				t.Fatalf("next call=%v", err)
			}
			if conn.writes != writes || client.sequence != sequence {
				t.Fatal("RPC retried or advanced sequence without reconnect")
			}
		})
	}
}

func TestAgentCompleteSemanticRepliesAndLocalErrorsKeepTransportUsable(t *testing.T) {
	for _, mode := range []string{"remote-error", "typed-body", "local-marshaling", "already-canceled"} {
		t.Run(mode, func(t *testing.T) {
			left, right := net.Pipe()
			wire := &observedAgentConn{Conn: left}
			defer wire.Close()
			server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			served := make(chan error, 1)
			go func() { served <- server.ServeConn(ctx, right) }()
			client := &Client{conn: wire, token: server.Token, sandboxID: server.SandboxID, generation: server.Generation, endpoint: server.Endpoint}
			var err error
			switch mode {
			case "remote-error":
				err = client.Call("StateProcess", map[string]string{"ID": "missing"}, nil)
				var remote *RemoteError
				if !errors.As(err, &remote) {
					t.Fatalf("lost structured remote error: %v", err)
				}
			case "typed-body":
				var wrong struct {
					Protocol string `json:"protocol"`
				}
				err = client.Call("Capabilities", map[string]any{}, &wrong)
			case "local-marshaling":
				err = client.Call("Capabilities", make(chan int), nil)
			case "already-canceled":
				cctx, stop := context.WithCancel(t.Context())
				stop()
				err = client.CallContext(cctx, "Capabilities", map[string]any{}, nil)
			}
			if err == nil {
				t.Fatal("expected rejected operation")
			}
			if wire.closes.Load() != 0 {
				t.Fatal("healthy framed transport was closed")
			}
			if err = client.Call("Capabilities", map[string]any{}, nil); err != nil {
				t.Fatalf("later healthy call: %v", err)
			}
			if wire.closes.Load() != 0 {
				t.Fatal("healthy stream was replaced")
			}
			cancel()
			_ = wire.Close()
			select {
			case <-served:
			case <-time.After(time.Second):
				t.Fatal("server remained blocked")
			}
		})
	}
}

type failAgentReadOnce struct {
	net.Conn
	failed bool
}

func (c *failAgentReadOnce) Read(p []byte) (int, error) {
	if !c.failed {
		c.failed = true
		return 0, io.ErrUnexpectedEOF
	}
	return c.Conn.Read(p)
}

func TestAgentOnlySuccessfulExplicitReconnectRestoresTransportAndPreservesSequence(t *testing.T) {
	listener := newPipeListener()
	defer listener.Close()
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, listener) }()
	oldDial := dialAgent
	defer func() { dialAgent = oldDial }()
	calls := 0
	failDial := false
	dialAgent = func(c context.Context, _, _ string) (net.Conn, error) {
		calls++
		if failDial {
			return nil, errors.New("synthetic dial failure")
		}
		left, right := net.Pipe()
		select {
		case listener.connections <- right:
		case <-c.Done():
			left.Close()
			right.Close()
			return nil, c.Err()
		}
		if calls == 1 {
			return &failAgentReadOnce{Conn: left}, nil
		}
		return left, nil
	}
	client, err := DialContext(t.Context(), "memory", server.SandboxID, server.Generation, server.Endpoint, server.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err = client.Call("Capabilities", map[string]any{}, nil); err == nil {
		t.Fatal("injected read failure accepted")
	}
	sequence := client.sequence
	failDial = true
	if err = client.ReconnectContext(t.Context(), "memory"); err == nil {
		t.Fatal("failed dial accepted")
	}
	if err = client.Call("Capabilities", map[string]any{}, nil); !errors.Is(err, ErrReconnectRequired) {
		t.Fatalf("failed reconnect restored stream: %v", err)
	}
	if client.sequence != sequence {
		t.Fatal("failed reconnect reset or advanced sequence")
	}
	failDial = false
	if err = client.ReconnectContext(t.Context(), "memory"); err != nil {
		t.Fatal(err)
	}
	if client.sequence != sequence {
		t.Fatal("explicit reconnect reset authenticated sequence")
	}
	if err = client.Call("Capabilities", map[string]any{}, nil); err != nil {
		t.Fatalf("continued authenticated session: %v", err)
	}
	if client.sequence != sequence+1 {
		t.Fatal("next authenticated request did not advance once")
	}
	cancel()
	_ = client.Close()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("server remained blocked")
	}
}
