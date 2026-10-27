//go:build linux

package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
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

func TestAgentCheckedPeerRelayHelper(t *testing.T) {
	path := os.Getenv("MK_AGENT_PEER_RELAY_SOCKET")
	if path == "" {
		t.Skip("subprocess relay fixture")
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal("proxy listener unavailable")
	}
	defer listener.Close()
	if os.Chmod(path, 0600) != nil {
		t.Fatal("proxy mode unavailable")
	}
	for {
		front, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer front.Close()
			back, err := net.Dial("unix", os.Getenv("MK_AGENT_PEER_BACKEND_SOCKET"))
			if err != nil {
				return
			}
			defer back.Close()
			done := make(chan struct{})
			go func() { io.Copy(back, front); back.(*net.UnixConn).CloseWrite(); close(done) }()
			io.Copy(front, back)
			front.(*net.UnixConn).CloseWrite()
			<-done
		}()
	}
}

func TestAgentCheckedPeerExplicitReplacementPreservesSequenceAndPolicy(t *testing.T) {
	listener, backend := ownedAgentListener(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	go server.Serve(ctx, listener)
	path := filepath.Join(filepath.Dir(backend), "proxy.sock")
	start := func() *exec.Cmd {
		command := exec.Command(os.Args[0], "-test.run=^TestAgentCheckedPeerRelayHelper$", "-test.timeout=15s")
		command.Env = append(os.Environ(), "MK_AGENT_PEER_RELAY_SOCKET="+path, "MK_AGENT_PEER_BACKEND_SOCKET="+backend)
		if command.Start() != nil {
			t.Fatal("proxy start failed")
		}
		t.Cleanup(func() {
			if command.ProcessState == nil {
				command.Process.Kill()
				command.Wait()
			}
		})
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, err := os.Stat(path); err == nil {
				return command
			}
			if time.Now().After(deadline) {
				t.Fatal("proxy publication timed out")
			}
			time.Sleep(time.Millisecond)
		}
	}
	first := start()
	client, err := DialPeerContext(t.Context(), path, server.SandboxID, server.Generation, server.Endpoint, server.Token, first.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.Call("Capabilities", map[string]any{}, nil) != nil {
		t.Fatal("initial authenticated call failed")
	}
	sequence := client.sequence
	client.Close()
	if client.Call("Capabilities", map[string]any{}, nil) == nil {
		t.Fatal("closed stream accepted")
	}
	if first.Process.Kill() != nil {
		t.Fatal("proxy termination failed")
	}
	first.Wait()
	if os.Remove(path) != nil {
		t.Fatal("owned fixture socket removal failed")
	}
	second := start()
	old := dialAgent
	dialAgent = func(context.Context, string, string) (net.Conn, error) {
		t.Error("unverified fallback used")
		return nil, errors.New("unchecked")
	}
	defer func() { dialAgent = old }()
	if client.ReconnectContext(t.Context(), path) == nil {
		t.Fatal("ordinary reconnect silently selected a different process")
	}
	if client.ReconnectPeerContext(t.Context(), path, first.Process.Pid) == nil {
		t.Fatal("wrong replacement identity accepted")
	}
	if client.sequence != sequence || !client.unusable {
		t.Fatal("failed verification changed sequence or stream state")
	}
	if client.ReconnectContext(t.Context(), path) == nil {
		t.Fatal("failed explicit rebind changed the old peer policy")
	}
	if client.ReconnectPeerContext(t.Context(), path, second.Process.Pid) != nil {
		t.Fatal("owned replacement rejected")
	}
	if client.sequence != sequence {
		t.Fatal("replacement reset authenticated sequence")
	}
	if client.Call("Capabilities", map[string]any{}, nil) != nil {
		t.Fatal("continued authenticated session refused")
	}
	sequence = client.sequence
	client.Close()
	if client.ReconnectContext(t.Context(), path) != nil || client.sequence != sequence {
		t.Fatal("ordinary reconnect did not retain replacement policy and sequence")
	}
	if client.Call("Capabilities", map[string]any{}, nil) != nil {
		t.Fatal("replacement policy lost authentication continuity")
	}
}

func TestAgentUncheckedClientCannotSelectReplacementPeer(t *testing.T) {
	client := &Client{sequence: 41, unusable: true}
	if client.ReconnectPeerContext(t.Context(), "/unused", os.Getpid()) == nil || client.sequence != 41 || !client.unusable {
		t.Fatal("unchecked client bypassed peer-policy initialization")
	}
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
