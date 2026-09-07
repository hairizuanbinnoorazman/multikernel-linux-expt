package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Client struct{ Path string }

type Caller interface {
	Call(context.Context, protocol.Request, any) *protocol.Error
}

func (c Client) Call(ctx context.Context, request protocol.Request, body any) *protocol.Error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", c.Path)
	if err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	defer conn.Close()
	payload, err := json.Marshal(request)
	if err != nil {
		return &protocol.Error{Code: "INVALID_ARGUMENT", Message: err.Error()}
	}
	if _, err = conn.Write(payload); err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	if unix, ok := conn.(*net.UnixConn); ok {
		_ = unix.CloseWrite()
	}
	data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return &protocol.Error{Code: "UNAVAILABLE", Message: err.Error(), Retryable: true}
	}
	var response protocol.Response
	if err = json.Unmarshal(data, &response); err != nil {
		return &protocol.Error{Code: "INTERNAL", Message: err.Error()}
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
