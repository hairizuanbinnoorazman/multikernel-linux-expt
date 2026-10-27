//go:build linux

package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ownedAgentListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "mk-peer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	path := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	return listener, path
}
func TestAgentCheckedPeerRejectsBeforeWriting(t *testing.T) {
	for _, mode := range []string{"wrong-pid", "unsafe-socket", "invalid-pid", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			listener, path := ownedAgentListener(t)
			expected := os.Getpid()
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if mode == "unsafe-socket" {
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "invalid-pid" {
				expected = 0
			}
			if mode == "wrong-pid" {
				expected += 100000
			}
			if mode == "canceled" {
				cancel()
			}
			read := make(chan int, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					read <- 0
					return
				}
				defer conn.Close()
				conn.SetReadDeadline(time.Now().Add(time.Second))
				var p [1]byte
				n, _ := conn.Read(p[:])
				read <- n
			}()
			client, err := DialPeerContext(ctx, path, "box", "0123456789abcdef0123456789abcdef", 7001, []byte("01234567890123456789012345678901"), expected)
			if err == nil || client != nil {
				t.Fatal("unchecked peer was accepted")
			}
			listener.Close()
			select {
			case n := <-read:
				if n != 0 {
					t.Fatal("request leaked before peer validation")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("peer check blocked")
			}
		})
	}
}
func TestAgentCheckedPeerRetainsIdentityPolicyOnReconnect(t *testing.T) {
	listener, path := ownedAgentListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, listener) }()
	client, err := DialPeerContext(t.Context(), path, server.SandboxID, server.Generation, server.Endpoint, server.Token, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.conn = &failAgentReadOnce{Conn: client.conn}
	if err = client.Call("Capabilities", map[string]any{}, nil); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("fault=%v", err)
	}
	sequence := client.sequence
	old := dialAgent
	dialAgent = func(context.Context, string, string) (net.Conn, error) {
		t.Error("checked reconnect used unverified fallback")
		return nil, errors.New("unverified")
	}
	defer func() { dialAgent = old }()
	if err = client.ReconnectContext(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if client.sequence != sequence {
		t.Fatal("checked reconnect reset sequence")
	}
	if err = client.Call("Capabilities", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	cancel()
	client.Close()
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}
