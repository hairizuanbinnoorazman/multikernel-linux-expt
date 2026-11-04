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

type MutationError struct{ Failure protocol.Error }

func (e *MutationError) Error() string { return e.Failure.Code + ": " + e.Failure.Message }

func ErrorCode(err error) (string, bool) {
	var remote *MutationError
	if !errors.As(err, &remote) {
		return "", false
	}
	return remote.Failure.Code, true
}

type Client struct {
	Path    string
	Timeout time.Duration
	dial    func(context.Context, string, string) (net.Conn, error)
}

type Caller interface {
	Call(context.Context, protocol.Request, any) *protocol.Error
}

type responseEnvelope struct {
	Version   int             `json:"version"`
	RequestID string          `json:"request_id"`
	Body      json.RawMessage `json:"body,omitempty"`
	Error     *protocol.Error `json:"error,omitempty"`
}

func (c Client) Call(ctx context.Context, request protocol.Request, body any) *protocol.Error {
	if err := ctx.Err(); err != nil {
		return clientUnavailable(ctx, protocol.DaemonConnect, err)
	}
	dial := c.dial
	if dial == nil {
		d := net.Dialer{}
		dial = d.DialContext
	}
	conn, err := dial(ctx, "unix", c.Path)
	if err != nil {
		return clientUnavailable(ctx, protocol.DaemonConnect, err)
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
		return clientUnavailable(ctx, protocol.DaemonDeadline, err)
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
		return &protocol.Error{Code: "INVALID_ARGUMENT", Message: "daemon request encoding failed"}
	}
	// Terminate the request without half-closing the socket. The server can
	// then observe a later caller disconnect and cancel in-flight backend work.
	payload = append(payload, '\n')
	if err = protocol.WriteFull(conn, payload); err != nil {
		return clientUnavailable(ctx, protocol.DaemonWrite, err)
	}
	data, err := io.ReadAll(io.LimitReader(conn, (1<<20)+1))
	if err != nil {
		return clientUnavailable(ctx, protocol.DaemonRead, err)
	}
	if len(data) > 1<<20 {
		return &protocol.Error{Code: "INTERNAL", Message: "daemon response exceeds the one-MiB limit"}
	}
	var response responseEnvelope
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
		if len(response.Body) == 0 {
			return &protocol.Error{Code: "INTERNAL", Message: "daemon response omitted its body"}
		}
		if err = protocol.StrictDecode(response.Body, body); err != nil {
			return &protocol.Error{Code: "INTERNAL", Message: err.Error()}
		}
	}
	return nil
}

func clientUnavailable(ctx context.Context, stage protocol.ClientFailureStage, cause error) *protocol.Error {
	return &protocol.Error{Code: "UNAVAILABLE", Message: protocol.ClientFailure(ctx, stage, cause).Error(), Retryable: true}
}

func Mutation(ctx context.Context, c Caller, method, id, generation, key string, config *protocol.SandboxConfig) (protocol.MutationResult, error) {
	request := protocol.Request{Version: 1, RequestID: key, Method: method, SandboxID: id, Generation: generation, IdempotencyKey: key}
	if config != nil {
		request.Body, _ = json.Marshal(config)
	}
	var result protocol.MutationResult
	if apiErr := c.Call(ctx, request, &result); apiErr != nil {
		return result, &MutationError{Failure: *apiErr}
	}
	return result, nil
}
