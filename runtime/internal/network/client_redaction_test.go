package network

import (
	"bufio"
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var privateClientFailure = errors.New(strings.Repeat("SYNTHETIC_RPC_CLIENT_TOKEN /private/control/socket\n", 256))

type clientFaultConn struct {
	net.Conn
	stage  string
	closes int
}

func (c *clientFaultConn) Read([]byte) (int, error) { return 0, privateClientFailure }
func (c *clientFaultConn) Write(p []byte) (int, error) {
	if c.stage == "write" {
		return 0, privateClientFailure
	}
	return len(p), nil
}
func (c *clientFaultConn) SetDeadline(time.Time) error {
	if c.stage == "deadline" {
		return privateClientFailure
	}
	return nil
}
func (c *clientFaultConn) Close() error { c.closes++; return nil }

func TestClientLocalFailuresOmitPrivateTransportData(t *testing.T) {
	for _, stage := range []string{"dial", "deadline", "write", "read"} {
		t.Run(stage, func(t *testing.T) {
			connection := &clientFaultConn{stage: stage}
			client := Client{Path: "/private/control/socket", dial: func(context.Context, string, string) (net.Conn, error) {
				if stage == "dial" {
					return nil, privateClientFailure
				}
				return connection, nil
			}}
			_, err := client.Call(context.Background(), Request{Version: 1, RequestID: "private-local-failure", Method: "LIST"})
			if err == nil || len(err.Error()) > 128 || strings.Contains(err.Error(), "SYNTHETIC_RPC_CLIENT_TOKEN") || strings.Contains(err.Error(), "/private/control/socket") {
				t.Fatal("local network RPC failure discloses private data")
			}
			expected := 1
			if stage == "dial" {
				expected = 0
			}
			if connection.closes != expected {
				t.Fatal("failed network RPC did not close connection exactly once")
			}
		})
	}
}
func TestAttachMissingSocketOmitsPrivatePath(t *testing.T) {
	_, fd, err := (Client{Path: filepath.Join(t.TempDir(), "SYNTHETIC_RPC_CLIENT_TOKEN.sock")}).Attach(context.Background(), Request{Version: 1, RequestID: "private-attach", Method: "ATTACH"})
	if fd != nil {
		fd.Close()
		t.Fatal("failed ATTACH returned descriptor")
	}
	if err == nil || len(err.Error()) > 128 || strings.Contains(err.Error(), "SYNTHETIC_RPC_CLIENT_TOKEN") {
		t.Fatal("ATTACH dial error discloses private socket path")
	}
}
func TestAttachCancellationClosesRealUnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpc.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	peerDone := make(chan struct{})
	defer func() {
		close(release)
		select {
		case <-peerDone:
		case <-time.After(time.Second):
			t.Error("ATTACH retained peer handler")
		}
	}()
	go func() {
		defer close(peerDone)
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err = bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			return
		}
		cancel()
		// Retain the reply side after the client half-closes its request. Cancellation
		// must interrupt ReadMsgUnix without the peer closing or sending a response.
		<-release
	}()
	var connection net.Conn
	client := Client{Path: path, Timeout: 2 * time.Second, dial: func(ctx context.Context, network, path string) (net.Conn, error) {
		var err error
		connection, err = (&net.Dialer{}).DialContext(ctx, network, path)
		return connection, err
	}}
	start := time.Now()
	_, fd, err := client.Attach(ctx, Request{Version: 1, RequestID: "cancel-attach", Method: "ATTACH"})
	if fd != nil {
		fd.Close()
		t.Fatal("cancelled ATTACH returned descriptor")
	}
	if !errors.Is(err, context.Canceled) || time.Since(start) > 500*time.Millisecond {
		t.Fatal("ATTACH did not cancel promptly with context identity")
	}
	if connection == nil {
		t.Fatal("ATTACH never connected")
	}
	if _, err = connection.Write([]byte("late")); !errors.Is(err, net.ErrClosed) {
		t.Fatal("ATTACH retained its Unix connection")
	}
}

func TestNetworkClientsCancelledBeforeDial(t *testing.T) {
	for _, attach := range []bool{false, true} {
		calls := 0
		client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) { calls++; return nil, privateClientFailure }}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var err error
		if attach {
			_, _, err = client.Attach(ctx, Request{})
		} else {
			_, err = client.Call(ctx, Request{})
		}
		if calls != 0 || !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled network client dialed or lost context identity")
		}
	}
}
