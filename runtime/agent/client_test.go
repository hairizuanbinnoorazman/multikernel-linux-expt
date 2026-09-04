package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
}
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}
func (l *pipeListener) Addr() net.Addr { return memoryAddr("pipe-listener") }

func TestClientRejectsMismatchedReplyBinding(t *testing.T) {
	tests := []struct {
		name    string
		version int
		seq     uint64
		want    string
	}{
		{"version", 2, 1, "version mismatch"},
		{"sequence", 1, 99, "sequence mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()
			client := &Client{conn: clientConn, token: []byte("01234567890123456789012345678901"), sandboxID: "box", generation: "0123456789abcdef0123456789abcdef", endpoint: 7001}
			go func() {
				var header [4]byte
				if _, err := io.ReadFull(serverConn, header[:]); err != nil {
					return
				}
				request := make([]byte, binary.BigEndian.Uint32(header[:]))
				if _, err := io.ReadFull(serverConn, request); err != nil {
					return
				}
				response, _ := json.Marshal(Reply{Version: test.version, Sequence: test.seq, Body: map[string]any{"protocol": 1}})
				binary.BigEndian.PutUint32(header[:], uint32(len(response)))
				_, _ = serverConn.Write(header[:])
				_, _ = serverConn.Write(response)
			}()
			if err := client.Call("Capabilities", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Call() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAuthenticatedReconnectPreservesSequenceAndShutdownEndsSession(t *testing.T) {
	listener := newPipeListener()
	defer listener.Close()
	oldDial := dialAgent
	dialAgent = func(_, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		listener.connections <- server
		return client, nil
	}
	defer func() { dialAgent = oldDial }()
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, listener) }()

	client, err := Dial("memory", server.SandboxID, server.Generation, server.Endpoint, server.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Call("Capabilities", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	if err = client.Reconnect("memory"); err != nil {
		t.Fatal(err)
	}
	if err = client.Call("Capabilities", map[string]any{}, nil); err != nil {
		t.Fatalf("continued sequence after reconnect: %v", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}

	replayed, err := Dial("memory", server.SandboxID, server.Generation, server.Endpoint, server.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = replayed.Call("Capabilities", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "UNAUTHENTICATED") {
		t.Fatalf("restarted sequence error = %v", err)
	}
	_ = replayed.Close()

	if err = client.Reconnect("memory"); err != nil {
		t.Fatal(err)
	}
	if err = client.Call("Shutdown", map[string]any{}, nil); err != nil {
		t.Fatalf("Shutdown response was not delivered: %v", err)
	}
	select {
	case err = <-served:
		if !errors.Is(err, ErrShutdownRequested) {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not end after Shutdown")
	}
}
