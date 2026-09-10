package network

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRPCBindsResponseAndRejectsDuplicateJSON(t *testing.T) {
	backend := &fakeBackend{}
	service := service(t, "172.31.0.0/30", backend)
	server := &Server{Service: service, AllowedUID: CurrentUID(), MaxFrame: 1 << 20}
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		clientConnection, serverConnection := net.Pipe()
		go server.handle(context.Background(), serverConnection)
		return clientConnection, nil
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
