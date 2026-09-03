package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

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
