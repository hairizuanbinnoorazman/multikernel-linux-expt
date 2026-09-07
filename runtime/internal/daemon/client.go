package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Client struct {
	Path    string
	Timeout time.Duration
	dial    func(context.Context, string, string) (net.Conn, error)
}

type Caller interface {
	Call(context.Context, protocol.Request, any) *protocol.Error
}

func (c Client) Call(ctx context.Context, request protocol.Request, body any) *protocol.Error {
	if err := ctx.Err(); err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	dial := c.dial
	if dial == nil {
		d := net.Dialer{}
		dial = d.DialContext
	}
	conn, err := dial(ctx, "unix", c.Path)
	if err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	defer conn.Close()
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	requestedDeadline, hasRequestedDeadline := ctx.Deadline()
	if hasRequestedDeadline && requestedDeadline.Before(deadline) {
		deadline = requestedDeadline
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	cancelDone := make(chan struct{})
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
		close(cancelDone)
	})
	defer func() {
		if !stopCancellation() {
			<-cancelDone
		}
	}()
	payload, err := json.Marshal(request)
	if err != nil {
		return &protocol.Error{Code: "INVALID_ARGUMENT", Message: err.Error()}
	}
	if _, err = conn.Write(payload); err != nil {
		err = daemonRequestIOError(ctx, err, requestedDeadline, hasRequestedDeadline)
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	if unix, ok := conn.(*net.UnixConn); ok {
		_ = unix.CloseWrite()
	}
	data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		err = daemonRequestIOError(ctx, err, requestedDeadline, hasRequestedDeadline)
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	var response protocol.Response
	if err = protocol.StrictDecode(data, &response); err != nil {
		return &protocol.Error{Code: "INTERNAL", Message: err.Error()}
	}
	if response.Version != protocol.Version || response.RequestID != request.RequestID {
		return &protocol.Error{Code: "INTERNAL", Message: "daemon response protocol version or request ID mismatch"}
	}
	if response.Error != nil {
		return response.Error
	}
	if body != nil {
		encoded, _ := json.Marshal(response.Body)
		if err = json.Unmarshal(encoded, body); err != nil {
			return &protocol.Error{Code: "INTERNAL", Message: err.Error()}
		}
	}
	return nil
}

func daemonRequestIOError(ctx context.Context, fallback error, deadline time.Time, hasDeadline bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// The connection deadline and the context timer represent the same caller
	// deadline. The socket can wake a few microseconds before ctx.Err becomes
	// observable, so classify that boundary deterministically.
	if hasDeadline && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return fallback
}

func Mutation(ctx context.Context, c Caller, method, id, generation, key string, config *protocol.SandboxConfig) (protocol.MutationResult, error) {
	request := protocol.Request{Version: 1, RequestID: key, Method: method, SandboxID: id, Generation: generation, IdempotencyKey: key}
	if config != nil {
		request.Body, _ = json.Marshal(config)
	}
	var result protocol.MutationResult
	if apiErr := c.Call(ctx, request, &result); apiErr != nil {
		return result, errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return result, nil
}
