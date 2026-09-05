package network

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestServerRefusesToReplaceNonSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mknetd.sock")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: service(t, "172.31.0.0/30", &fakeBackend{}), AllowedUID: CurrentUID()}
	if err := server.Listen(context.Background(), path); err == nil || !strings.Contains(err.Error(), "non-socket") {
		t.Fatalf("Listen error = %v", err)
	}
}
