package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"net"
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
			issue := client.Call(context.Background(), protocol.Request{Version: 1, RequestID: "private-local-failure", Method: "NodeInfo"}, nil)
			if issue == nil || issue.Code != "UNAVAILABLE" || !issue.Retryable {
				t.Fatal("local failure code or retryability changed")
			}
			if len(issue.Message) > 128 || strings.Contains(issue.Message, "SYNTHETIC_RPC_CLIENT_TOKEN") || strings.Contains(issue.Message, "/private/control/socket") {
				t.Fatal("local RPC failure discloses private data")
			}
			expected := 1
			if stage == "dial" {
				expected = 0
			}
			if connection.closes != expected {
				t.Fatal("failed local RPC did not close connection exactly once")
			}
		})
	}
}
