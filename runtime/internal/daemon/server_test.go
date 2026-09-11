package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type memoryConn struct {
	input  *bytes.Reader
	output bytes.Buffer
}

func TestUnixPeerAuthorization(t *testing.T) {
	if !credentialAuthorized(&unix.Ucred{Uid: 1234}, 1234) || credentialAuthorized(&unix.Ucred{Uid: 1234}, 4321) || credentialAuthorized(nil, 1234) {
		t.Fatal("credential policy mismatch")
	}
}

func newMemoryConn(input string) *memoryConn {
	return &memoryConn{input: bytes.NewReader([]byte(input))}
}
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

func TestDaemonHandlerCancellationClosesIncompleteRequest(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Server{MaxFrame: 1024}).handle(ctx, serverConnection)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daemon handler remained blocked after service cancellation")
	}
}

func TestDaemonResponseIsBoundedAndFailureRemainsRequestBound(t *testing.T) {
	for name, body := range map[string]any{
		"oversized":   map[string]string{"value": strings.Repeat("x", 1024)},
		"unencodable": func() {},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := encodeDaemonResponse(protocol.Response{Version: 1, RequestID: "bounded-response", Body: body}, 256)
			if err != nil {
				t.Fatal(err)
			}
			if len(encoded) > 256 {
				t.Fatalf("response size = %d", len(encoded))
			}
			var response protocol.Response
			if err = protocol.StrictDecode(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response.RequestID != "bounded-response" || response.Error == nil || response.Error.Code != "INTERNAL" {
				t.Fatalf("bounded response = %+v", response)
			}
		})
	}
	if _, err := encodeDaemonResponse(protocol.Response{Version: 1, RequestID: "tiny", Body: map[string]string{"value": "x"}}, 8); err == nil {
		t.Fatal("impossibly small response frame limit succeeded")
	}
}

func TestDaemonWireRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		maxFrame int
		want     string
	}{
		{"unknown envelope field", `{"version":1,"request_id":"1","method":"NodeInfo","extra":true}`, 1024, "unknown field"},
		{"trailing value", `{"version":1,"request_id":"1","method":"NodeInfo"}{}`, 1024, "multiple JSON values"},
		{"unknown method body field", `{"version":1,"request_id":"1","method":"CreateSandbox","body":{"schema_version":1,"id":"box","cpus":[2],"memory_bytes":4096,"kernel_manifest":"mk","bundle":"/bundle","agent_port":7001,"child_cid":3,"extra":true}}`, 1024, "unknown field"},
		{"numeric overflow", `{"version":1,"request_id":"1","method":"CreateSandbox","body":{"schema_version":1,"id":"box","cpus":[2],"memory_bytes":18446744073709551616,"kernel_manifest":"mk","bundle":"/bundle","agent_port":7001,"child_cid":3}}`, 1024, "cannot unmarshal number"},
		{"frame bound", strings.Repeat("x", 65), 64, "frame too large"},
		{"empty request ID", `{"version":1,"request_id":"","method":"NodeInfo"}`, 1024, "request ID"},
		{"non-printable request ID", "{\"version\":1,\"request_id\":\"bad\\nrequest\",\"method\":\"NodeInfo\"}", 1024, "request ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := newMemoryConn(test.payload)
			server := &Server{MaxFrame: test.maxFrame}
			server.handle(context.Background(), conn)
			var response protocol.Response
			if err := json.Unmarshal(conn.output.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error == nil || response.Error.Code != "INVALID_ARGUMENT" || !strings.Contains(response.Error.Message, test.want) {
				t.Fatalf("response = %+v, want INVALID_ARGUMENT containing %q", response, test.want)
			}
		})
	}
}
