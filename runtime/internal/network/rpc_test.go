package network

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type shortWriteConn struct {
	net.Conn
	maximum int
}

type queueListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func newQueueListener() *queueListener {
	return &queueListener{connections: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *queueListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *queueListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (*queueListener) Addr() net.Addr { return &net.UnixAddr{Name: "queue", Net: "unix"} }

type fakeRightsWriter struct {
	payloadAdjustment int
	rightsAdjustment  int
	err               error
}

func (w fakeRightsWriter) WriteMsgUnix(payload, rights []byte, _ *net.UnixAddr) (int, int, error) {
	return len(payload) + w.payloadAdjustment, len(rights) + w.rightsAdjustment, w.err
}

func TestDescriptorSendRequiresCompletePayloadAndRights(t *testing.T) {
	for name, writer := range map[string]fakeRightsWriter{
		"payload truncated": {payloadAdjustment: -1},
		"rights truncated":  {rightsAdjustment: -1},
		"transport error":   {err: errors.New("injected send failure")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := writeUnixRights(writer, []byte("response\n"), 1); err == nil {
				t.Fatal("incomplete descriptor send succeeded")
			}
		})
	}
	if err := writeUnixRights(fakeRightsWriter{}, []byte("response\n"), 1); err != nil {
		t.Fatalf("complete descriptor send = %v", err)
	}
}

func (c *shortWriteConn) Write(data []byte) (int, error) {
	if len(data) > c.maximum {
		data = data[:c.maximum]
	}
	return c.Conn.Write(data)
}

func TestNetworkHandlerCancellationClosesIncompleteRequest(t *testing.T) {
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
		t.Fatal("mknetd handler remained blocked after service cancellation")
	}
}

func TestNetworkHandlerLimitCannotBeDisabledOrMadeUnbounded(t *testing.T) {
	for configured, expected := range map[int]int{-1: 128, 0: 128, 1: 1, 512: 512, 4096: 1024} {
		if observed := networkHandlerLimit(configured); observed != expected {
			t.Fatalf("handler limit(%d) = %d, want %d", configured, observed, expected)
		}
	}
}

func TestRPCBindsResponseAndRejectsDuplicateJSON(t *testing.T) {
	backend := &fakeBackend{}
	service := service(t, "172.31.0.0/30", backend)
	server := &Server{Service: service, AllowedUID: CurrentUID(), MaxFrame: 1 << 20}
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		clientConnection, serverConnection := net.Pipe()
		go server.handle(context.Background(), &shortWriteConn{Conn: serverConnection, maximum: 3})
		return &shortWriteConn{Conn: clientConnection, maximum: 2}, nil
	}}
	request := Request{Version: 1, RequestID: "add-1", Method: "ADD", Endpoint: func() *Endpoint { value := endpoint("rpc"); return &value }()}
	response, err := client.Call(context.Background(), request)
	if err != nil || response.Endpoint == nil || response.Endpoint.Generation == "" {
		t.Fatalf("RPC response=%+v error=%v", response, err)
	}
}

func TestClientRejectsOversizedValidResponsePrefix(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer serverConnection.Close()
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}
	go func() {
		buffer := make([]byte, 4096)
		_, _ = serverConnection.Read(buffer)
		prefix := []byte(`{"version":1,"request_id":"request"}`)
		_, _ = serverConnection.Write(append(prefix, bytes.Repeat([]byte(" "), (1<<20)+1)...))
		_ = serverConnection.Close()
	}()
	_, err := client.Call(context.Background(), Request{Version: 1, RequestID: "request", Method: "CHECK"})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}
}

func TestServerRefusesToReplaceNonSocket(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "mknetd.sock")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: service(t, "172.31.0.0/30", &fakeBackend{}), AllowedUID: CurrentUID()}
	if err := server.Listen(context.Background(), path); err == nil || !strings.Contains(err.Error(), "socket path") {
		t.Fatalf("Listen error = %v", err)
	}
}

func TestNetworkListenerBoundsAndJoinsIncompleteHandlers(t *testing.T) {
	listener := newQueueListener()
	server := &Server{MaxHandlers: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listened := make(chan error, 1)
	go func() { listened <- server.serve(ctx, listener, func(net.Conn, uint32) error { return nil }) }()
	first, firstServer := net.Pipe()
	defer first.Close()
	listener.connections <- firstServer
	if _, err := first.Write([]byte("{")); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); server.activeHandlers.Load() != 1; {
		if time.Now().After(deadline) {
			t.Fatal("first incomplete handler did not become active")
		}
		time.Sleep(time.Millisecond)
	}
	second, secondServer := net.Pipe()
	defer second.Close()
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	listener.connections <- secondServer
	if _, err := second.Read(make([]byte, 1)); err == nil {
		t.Fatal("handler above configured limit remained open")
	}
	cancel()
	select {
	case err := <-listened:
		if err != nil {
			t.Fatalf("cancelled mknetd listener = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("mknetd listener did not join incomplete handler")
	}
	if active := server.activeHandlers.Load(); active != 0 {
		t.Fatalf("active handlers after listener return = %d", active)
	}
}

func TestAttachPassesExactlyOneGenerationBoundDescriptor(t *testing.T) {
	backend := &fakeBackend{}
	service := service(t, "172.31.0.0/30", backend)
	allocated, issue := service.Add(context.Background(), endpoint("attach"))
	if issue != nil {
		t.Fatal(issue)
	}
	directory := t.TempDir()
	payload := filepath.Join(directory, "payload")
	if err := os.WriteFile(payload, []byte("descriptor-proof"), 0600); err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: service, AllowedUID: CurrentUID(), OpenTUN: func(Endpoint) (*os.File, error) { return os.Open(payload) }}
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	clientFile, serverFile := os.NewFile(uintptr(fds[0]), "client"), os.NewFile(uintptr(fds[1]), "server")
	t.Cleanup(func() { _ = clientFile.Close(); _ = serverFile.Close() })
	clientConnection, err := net.FileConn(clientFile)
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("sandbox forbids Unix descriptor socket inspection")
		}
		t.Fatal(err)
	}
	serverConnection, err := net.FileConn(serverFile)
	if err != nil {
		t.Fatal(err)
	}
	_ = clientFile.Close()
	_ = serverFile.Close()
	go server.handle(context.Background(), serverConnection)
	request := Request{Version: 1, RequestID: "attach-1", Method: "ATTACH", Endpoint: &Endpoint{
		NetNS: allocated.NetNS, Generation: allocated.Generation, SandboxID: "sandbox-one", SandboxGeneration: "11111111111111111111111111111111",
	}}
	response, descriptor, err := (Client{Path: "socketpair", dial: func(context.Context, string, string) (net.Conn, error) { return clientConnection, nil }}).Attach(context.Background(), request)
	if err != nil || descriptor == nil || response.Endpoint == nil || response.Endpoint.Generation != allocated.Generation {
		t.Fatalf("response=%+v descriptor=%v error=%v", response, descriptor, err)
	}
	data, readErr := io.ReadAll(descriptor)
	_ = descriptor.Close()
	if readErr != nil || string(data) != "descriptor-proof" {
		t.Fatalf("descriptor data=%q error=%v", data, readErr)
	}
}

func TestAttachClosesReceivedDescriptorWhenPayloadIsTruncated(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	clientFile, serverFile := os.NewFile(uintptr(fds[0]), "client"), os.NewFile(uintptr(fds[1]), "server")
	clientConnection, err := net.FileConn(clientFile)
	if err != nil {
		_ = clientFile.Close()
		_ = serverFile.Close()
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("sandbox forbids Unix descriptor socket inspection")
		}
		t.Fatal(err)
	}
	serverConnection, err := net.FileConn(serverFile)
	_ = clientFile.Close()
	_ = serverFile.Close()
	if err != nil {
		_ = clientConnection.Close()
		t.Fatal(err)
	}
	unixServer, ok := serverConnection.(*net.UnixConn)
	if !ok {
		_ = clientConnection.Close()
		_ = serverConnection.Close()
		t.Fatal("socketpair did not produce a Unix connection")
	}
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer writePipe.Close()
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		defer unixServer.Close()
		_, _ = bufio.NewReader(unixServer).ReadBytes('\n')
		_, _, _ = unixServer.WriteMsgUnix([]byte("{"), unix.UnixRights(int(readPipe.Fd())), nil)
		_ = readPipe.Close()
	}()
	request := Request{Version: 1, RequestID: "truncated-rights", Method: "ATTACH"}
	_, descriptor, err := (Client{Path: "socketpair", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}).Attach(context.Background(), request)
	if err == nil || descriptor != nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("truncated ATTACH descriptor=%v error=%v", descriptor, err)
	}
	<-sent
	if _, writeErr := unix.Write(int(writePipe.Fd()), []byte{1}); !errors.Is(writeErr, syscall.EPIPE) {
		t.Fatalf("received descriptor leaked after rejected payload: %v", writeErr)
	}
}
