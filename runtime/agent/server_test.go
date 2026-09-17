package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type observedCloseConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *observedCloseConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestServeConnStopsCancellationCallbackAfterPeerDisconnect(t *testing.T) {
	clientConnection, rawServerConnection := net.Pipe()
	serverConnection := &observedCloseConn{Conn: rawServerConnection, closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{Manager: NewManager(true)}
	served := make(chan error, 1)
	go func() { served <- server.ServeConn(ctx, serverConnection) }()
	if err := clientConnection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("peer disconnect = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe peer disconnect")
	}
	cancel()
	select {
	case <-serverConnection.closed:
		t.Fatal("completed connection retained a server-cancellation callback")
	case <-time.After(25 * time.Millisecond):
	}
	_ = rawServerConnection.Close()
}

func TestServeConnCancellationClosesBlockedConnection(t *testing.T) {
	clientConnection, rawServerConnection := net.Pipe()
	defer clientConnection.Close()
	serverConnection := &observedCloseConn{Conn: rawServerConnection, closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- (&Server{Manager: NewManager(true)}).ServeConn(ctx, serverConnection) }()
	cancel()
	select {
	case <-serverConnection.closed:
	case <-time.After(time.Second):
		t.Fatal("server cancellation did not close blocked connection")
	}
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("server remained blocked after cancellation")
	}
}

type memoryConn struct {
	input  *bytes.Reader
	output bytes.Buffer
}

func newMemoryConn(input []byte) *memoryConn           { return &memoryConn{input: bytes.NewReader(input)} }
func (c *memoryConn) Read(p []byte) (int, error)       { return c.input.Read(p) }
func (c *memoryConn) Write(p []byte) (int, error)      { return c.output.Write(p) }
func (c *memoryConn) Close() error                     { return nil }
func (c *memoryConn) LocalAddr() net.Addr              { return memoryAddr("local") }
func (c *memoryConn) RemoteAddr() net.Addr             { return memoryAddr("remote") }
func (c *memoryConn) SetDeadline(time.Time) error      { return nil }
func (c *memoryConn) SetReadDeadline(time.Time) error  { return nil }
func (c *memoryConn) SetWriteDeadline(time.Time) error { return nil }

type memoryAddr string

func (a memoryAddr) Network() string { return "memory" }
func (a memoryAddr) String() string  { return string(a) }

func framed(payload []byte) []byte {
	result := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(result, uint32(len(payload)))
	copy(result[4:], payload)
	return result
}

func framedReplies(t *testing.T, raw []byte) []Reply {
	t.Helper()
	var replies []Reply
	for len(raw) > 0 {
		if len(raw) < 4 {
			t.Fatalf("truncated reply header: %x", raw)
		}
		size := int(binary.BigEndian.Uint32(raw[:4]))
		raw = raw[4:]
		if len(raw) < size {
			t.Fatalf("truncated reply body: have %d want %d", len(raw), size)
		}
		var reply Reply
		if err := json.Unmarshal(raw[:size], &reply); err != nil {
			t.Fatal(err)
		}
		replies = append(replies, reply)
		raw = raw[size:]
	}
	return replies
}

func TestOversizedFrameRepliesOnceAndEndsSession(t *testing.T) {
	input := make([]byte, 8)
	binary.BigEndian.PutUint32(input[:4], 1<<20+1)
	conn := newMemoryConn(input)
	server := &Server{Manager: NewManager(true)}
	if err := server.ServeConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	replies := framedReplies(t, conn.output.Bytes())
	if len(replies) != 1 || replies[0].Error != "INVALID_ARGUMENT: agent frame is too large" {
		t.Fatalf("replies = %+v", replies)
	}
}

func TestTruncatedFrameEndsSession(t *testing.T) {
	conn := newMemoryConn(append(framed(make([]byte, 10))[:4], []byte("{}")...))
	server := &Server{Manager: NewManager(true)}
	if err := server.ServeConn(context.Background(), conn); err != io.ErrUnexpectedEOF {
		t.Fatalf("ServeConn() error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
	if conn.output.Len() != 0 {
		t.Fatalf("unexpected reply to truncated frame: %x", conn.output.Bytes())
	}
}

func TestMalformedFrameDoesNotDesynchronizeNextFrame(t *testing.T) {
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	envelope := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "Capabilities"}
	Sign(&envelope, server.Token)
	valid, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	input := append(framed([]byte("{")), framed(valid)...)
	conn := newMemoryConn(input)
	if err = server.ServeConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	replies := framedReplies(t, conn.output.Bytes())
	if len(replies) != 2 || replies[0].Error == "" || replies[1].Error != "" {
		t.Fatalf("replies = %+v", replies)
	}
}

func TestAgentWireErrorsAreStructuredAndSecretSafe(t *testing.T) {
	raw, err := json.Marshal(Reply{Version: 1, Sequence: 7, Error: "open /private/path: token=super-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("/private/path")) || bytes.Contains(raw, []byte("super-secret")) {
		t.Fatalf("wire reply leaked raw error: %s", raw)
	}
	var wire struct {
		Error struct {
			Code        string `json:"code"`
			Message     string `json:"message"`
			OperationID string `json:"operation_id"`
			Retryable   bool   `json:"retryable"`
		} `json:"error"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Error.Code != "INTERNAL" || wire.Error.OperationID != "agent-7" || wire.Error.Message != "agent operation failed" {
		t.Fatalf("wire error = %+v", wire.Error)
	}
}

func TestStateReplyExcludesRetainedOutputAndReadIsBounded(t *testing.T) {
	manager := NewManager(true)
	done := make(chan struct{})
	close(done)
	process := &process{done: done, waited: true, state: ProcessState{ID: "large", Status: "STOPPED", ExitCode: 0}}
	payload := bytes.Repeat([]byte("x"), 2<<20)
	if _, err := process.stdout.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := process.stderr.Write(payload); err != nil {
		t.Fatal(err)
	}
	process.state.Stdout = process.stdout.String()
	process.state.Stderr = process.stderr.String()
	manager.processes["large"] = process
	server := &Server{Manager: manager, SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}

	request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "StateProcess", Body: json.RawMessage(`{"id":"large"}`)}
	Sign(&request, server.Token)
	reply := server.Dispatch(request)
	wire, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) >= 1<<20 || bytes.Contains(wire, bytes.Repeat([]byte("x"), 1024)) {
		t.Fatalf("state reply retained process output: size=%d", len(wire))
	}

	request.Sequence++
	request.Method = "ReadProcessOutput"
	request.Body = json.RawMessage(`{"id":"large","stdout_offset":0,"stderr_offset":0,"limit":65536}`)
	Sign(&request, server.Token)
	reply = server.Dispatch(request)
	wire, err = json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) >= 1<<20 {
		t.Fatalf("bounded output reply size=%d", len(wire))
	}
	body, ok := reply.Body.(map[string]any)
	if !ok || len(body["stdout"].([]byte)) != 65536 || len(body["stderr"].([]byte)) != 65536 {
		t.Fatalf("bounded split output body=%#v", reply.Body)
	}
}

func TestWriteProcessProtocolAcknowledgesExactIdempotentOffset(t *testing.T) {
	input := &bufferWriteCloser{}
	manager := NewManager(true)
	manager.processes["stdin"] = &process{stdin: input, state: ProcessState{ID: "stdin", Status: "RUNNING"}}
	server := &Server{Manager: manager, SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001,
		Token: []byte("01234567890123456789012345678901")}
	dispatch := func(sequence uint64, offset uint64, data []byte) Reply {
		body, err := json.Marshal(map[string]any{"id": "stdin", "offset": offset, "data": data})
		if err != nil {
			t.Fatal(err)
		}
		request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation,
			Endpoint: server.Endpoint, Sequence: sequence, Method: "WriteProcess", Body: body}
		Sign(&request, server.Token)
		return server.Dispatch(request)
	}
	for sequence := uint64(1); sequence <= 2; sequence++ {
		reply := dispatch(sequence, 0, []byte("once"))
		body, ok := reply.Body.(map[string]uint64)
		if reply.Error != "" || !ok || body["offset"] != 4 {
			t.Fatalf("write reply %d = %+v", sequence, reply)
		}
	}
	if input.String() != "once" {
		t.Fatalf("idempotent protocol wrote %q", input.String())
	}
	if reply := dispatch(3, 0, []byte("other")); reply.Error == "" {
		t.Fatal("same offset with different input was accepted")
	}
	body, err := json.Marshal(map[string]any{"id": "stdin", "data": []byte("-legacy")})
	if err != nil {
		t.Fatal(err)
	}
	legacy := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation,
		Endpoint: server.Endpoint, Sequence: 4, Method: "WriteProcess", Body: body}
	Sign(&legacy, server.Token)
	if reply := server.Dispatch(legacy); reply.Error != "" {
		t.Fatalf("legacy offset-free write failed: %+v", reply)
	}
	if input.String() != "once-legacy" {
		t.Fatalf("legacy-compatible input = %q", input.String())
	}
}

func TestSignalProcessOperationIDIsExactlyOnce(t *testing.T) {
	manager := NewManager(true)
	manager.processes["signal"] = &process{cmd: &exec.Cmd{Process: &os.Process{Pid: 41}}, state: ProcessState{ID: "signal", Status: "RUNNING"}}
	calls := 0
	manager.signalProcess = func(pid int, signal syscall.Signal) error {
		calls++
		return nil
	}
	server := &Server{Manager: manager, SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001,
		Token: []byte("01234567890123456789012345678901")}
	dispatch := func(sequence uint64, signal string) Reply {
		body, err := json.Marshal(map[string]any{"ID": "signal", "Signal": signal, "operation_id": strings.Repeat("a", 32)})
		if err != nil {
			t.Fatal(err)
		}
		request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation,
			Endpoint: server.Endpoint, Sequence: sequence, Method: "SignalProcess", Body: body}
		Sign(&request, server.Token)
		return server.Dispatch(request)
	}
	for sequence := uint64(1); sequence <= 2; sequence++ {
		if reply := dispatch(sequence, "10"); reply.Error != "" {
			t.Fatalf("signal replay %d = %+v", sequence, reply)
		}
	}
	if calls != 1 {
		t.Fatalf("signal calls = %d, want 1", calls)
	}
	if reply := dispatch(3, "12"); !strings.Contains(reply.Error, "reused") {
		t.Fatalf("changed signal replay = %+v", reply)
	}
	for sequence := uint64(4); sequence <= 5; sequence++ {
		body, err := json.Marshal(map[string]any{"ID": "signal", "operation_id": strings.Repeat("a", 32)})
		if err != nil {
			t.Fatal(err)
		}
		request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation,
			Endpoint: server.Endpoint, Sequence: sequence, Method: "AcknowledgeSignal", Body: body}
		Sign(&request, server.Token)
		if reply := server.Dispatch(request); reply.Error != "" {
			t.Fatalf("signal acknowledgement %d = %+v", sequence, reply)
		}
	}
	if len(manager.signalResults) != 0 {
		t.Fatalf("acknowledged signal ledger = %+v", manager.signalResults)
	}
}

func TestConcurrentSequencesAreSerializedAndOutOfOrderRejected(t *testing.T) {
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	start := make(chan struct{})
	replies := make(chan Reply, 100)
	var group sync.WaitGroup
	for sequence := uint64(1); sequence <= 100; sequence++ {
		group.Add(1)
		go func(sequence uint64) {
			defer group.Done()
			<-start
			request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: sequence, Method: "Capabilities"}
			Sign(&request, server.Token)
			replies <- server.Dispatch(request)
		}(sequence)
	}
	close(start)
	group.Wait()
	close(replies)
	successes := 0
	for reply := range replies {
		if reply.Error == "" {
			successes++
		} else if !strings.Contains(reply.Error, "replayed sequence") {
			t.Fatalf("concurrent reply = %+v", reply)
		}
	}
	if successes == 0 || server.last != 100 {
		t.Fatalf("successful sequences=%d last=%d", successes, server.last)
	}
	late := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 99, Method: "Capabilities"}
	Sign(&late, server.Token)
	if reply := server.Dispatch(late); !strings.Contains(reply.Error, "replayed sequence") {
		t.Fatalf("late sequence reply = %+v", reply)
	}
}

func TestServerEnforcesRuntimeOwnedBundle(t *testing.T) {
	server := &Server{
		Manager: NewManager(true), SandboxID: "box",
		Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001,
		Token: []byte("01234567890123456789012345678901"), Bundle: "/bundle",
	}
	request := Envelope{
		Version: 1, SandboxID: server.SandboxID, Generation: server.Generation,
		Endpoint: server.Endpoint, Sequence: 1, Method: "CreateProcess",
		Body: json.RawMessage(`{"id":"init","bundle":"/attacker-controlled"}`),
	}
	Sign(&request, server.Token)
	if reply := server.Dispatch(request); !strings.Contains(reply.Error, "runtime-owned bundle") {
		t.Fatalf("CreateProcess reply = %+v", reply)
	}

	request.Sequence++
	request.Method = "ExecProcess"
	request.Body = json.RawMessage(`{"id":"exec","parent_id":"missing","spec":{"args":["/bin/true"],"cwd":"/"}}`)
	Sign(&request, server.Token)
	if reply := server.Dispatch(request); !strings.Contains(reply.Error, "parent process") {
		t.Fatalf("ExecProcess reply = %+v", reply)
	}
}

func TestCapabilitiesReportRuntimeFacts(t *testing.T) {
	report := capabilityReport()
	kernel, ok := report["kernel"].(map[string]any)
	if !ok || kernel["architecture"] != runtime.GOARCH || kernel["release"] == "" {
		t.Fatalf("kernel capabilities = %#v", report["kernel"])
	}
	agentFacts, ok := report["agent"].(map[string]any)
	if !ok || agentFacts["uid"] != os.Getuid() || agentFacts["gid"] != os.Getgid() {
		t.Fatalf("agent capabilities = %#v", report["agent"])
	}
	for _, field := range []string{"effective_capabilities", "bounding_capabilities", "no_new_privileges"} {
		if value, exists := agentFacts[field]; !exists || value == "" {
			t.Fatalf("agent capability %s = %#v", field, value)
		}
	}
}
