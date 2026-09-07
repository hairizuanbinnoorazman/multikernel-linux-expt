package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestClientCancellationInterruptsResponseRead(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer serverConnection.Close()
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}
	requestRead := make(chan struct{})
	go func() {
		buffer := make([]byte, 4096)
		_, _ = serverConnection.Read(buffer)
		close(requestRead)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	issue := client.Call(ctx, protocol.Request{Version: 1, RequestID: "cancel-read", Method: "NodeInfo"}, nil)
	if issue == nil || issue.Code != "UNAVAILABLE" || !issue.Retryable || !strings.Contains(issue.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("cancellation issue = %+v", issue)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled read took %s", elapsed)
	}
	<-requestRead
}

func TestClientCancellationInterruptsRequestWrite(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer serverConnection.Close()
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	issue := client.Call(ctx, protocol.Request{Version: 1, RequestID: "cancel-write", Method: "NodeInfo"}, nil)
	if issue == nil || issue.Code != "UNAVAILABLE" || !issue.Retryable || !strings.Contains(issue.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("cancellation issue = %+v", issue)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cancelled write took %s", elapsed)
	}
}

func TestClientSuccessfulRoundTrip(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer serverConnection.Close()
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}
	go func() {
		buffer := make([]byte, 4096)
		_, _ = serverConnection.Read(buffer)
		encoded, _ := json.Marshal(protocol.Response{Version: 1, RequestID: "success", Body: map[string]any{"value": "ok"}})
		_, _ = serverConnection.Write(encoded)
		_ = serverConnection.Close()
	}()
	var body struct {
		Value string `json:"value"`
	}
	if issue := client.Call(context.Background(), protocol.Request{Version: 1, RequestID: "success", Method: "NodeInfo"}, &body); issue != nil {
		t.Fatal(issue)
	}
	if body.Value != "ok" {
		t.Fatalf("response value = %q", body.Value)
	}
}

func TestClientRejectsUnboundOrMalformedResponse(t *testing.T) {
	for name, response := range map[string]string{
		"wrong version":    `{"version":2,"request_id":"request"}`,
		"wrong request ID": `{"version":1,"request_id":"other"}`,
		"unknown field":    `{"version":1,"request_id":"request","extra":true}`,
		"duplicate field":  `{"version":1,"version":1,"request_id":"request"}`,
	} {
		t.Run(name, func(t *testing.T) {
			clientConnection, serverConnection := net.Pipe()
			defer serverConnection.Close()
			client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
				return clientConnection, nil
			}}
			go func() {
				buffer := make([]byte, 4096)
				_, _ = serverConnection.Read(buffer)
				_, _ = serverConnection.Write([]byte(response))
				_ = serverConnection.Close()
			}()
			issue := client.Call(context.Background(), protocol.Request{
				Version: 1, RequestID: "request", Method: "NodeInfo",
			}, nil)
			if issue == nil || issue.Code != "INTERNAL" {
				t.Fatalf("malformed response issue = %+v", issue)
			}
		})
	}
}

func TestClientDefaultBoundInterruptsResponseRead(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer serverConnection.Close()
	client := Client{Path: "memory", Timeout: 50 * time.Millisecond, dial: func(context.Context, string, string) (net.Conn, error) {
		return clientConnection, nil
	}}
	go func() {
		buffer := make([]byte, 4096)
		_, _ = serverConnection.Read(buffer)
	}()
	started := time.Now()
	issue := client.Call(context.Background(), protocol.Request{Version: 1, RequestID: "bounded-read", Method: "NodeInfo"}, nil)
	if issue == nil || issue.Code != "UNAVAILABLE" || !issue.Retryable {
		t.Fatalf("timeout issue = %+v", issue)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded read took %s", elapsed)
	}
}

func TestClientCancelledBeforeDial(t *testing.T) {
	var calls atomic.Int32
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, errors.New("unexpected dial")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	issue := client.Call(ctx, protocol.Request{Version: 1, RequestID: "cancel-dial", Method: "NodeInfo"}, nil)
	if issue == nil || !strings.Contains(issue.Message, context.Canceled.Error()) || calls.Load() != 0 {
		t.Fatalf("pre-cancel issue = %+v, dials = %d", issue, calls.Load())
	}
}
