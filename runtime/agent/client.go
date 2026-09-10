package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var initialSequence = func() uint64 { return uint64(time.Now().UnixNano()) }

type Client struct {
	conn                  net.Conn
	token                 []byte
	sandboxID, generation string
	endpoint              uint32
	mu                    sync.Mutex
	sequence              uint64
	Timeout               time.Duration
}

// RemoteError is a successfully authenticated agent reply that rejected the
// operation. It is distinct from a transport failure and must not trigger a
// reconnect/replay of a non-idempotent request.
type RemoteError struct{ Failure protocol.Error }

func (e *RemoteError) Error() string { return e.Failure.Code + ": " + e.Failure.Message }

var dialAgent = func(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func Dial(path, sandboxID, generation string, endpoint uint32, token []byte) (*Client, error) {
	return DialContext(context.Background(), path, sandboxID, generation, endpoint, token)
}

func DialContext(ctx context.Context, path, sandboxID, generation string, endpoint uint32, token []byte) (*Client, error) {
	c, err := dialAgent(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	return &Client{conn: c, token: token, sandboxID: sandboxID, generation: generation, endpoint: endpoint, sequence: initialSequence()}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// Reconnect replaces a failed transport while preserving the authenticated
// sequence. The server retains its last accepted sequence, so restarting at
// one would correctly be rejected as replay.
func (c *Client) Reconnect(path string) error {
	return c.ReconnectContext(context.Background(), path)
}

func (c *Client) ReconnectContext(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	conn, err := dialAgent(ctx, "unix", path)
	if err != nil {
		return err
	}
	old := c.conn
	c.conn = conn
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (c *Client) Call(method string, request, response any) error {
	return c.CallContext(context.Background(), method, request, response)
}

func (c *Client) CallContext(ctx context.Context, method string, request, response any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return err
	}
	cancelDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = c.conn.SetDeadline(time.Now())
		close(cancelDone)
	})
	defer func() {
		if !stop() {
			<-cancelDone
		}
		_ = c.conn.SetDeadline(time.Time{})
	}()
	c.sequence++
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	env := Envelope{Version: 1, SandboxID: c.sandboxID, Generation: c.generation, Endpoint: c.endpoint, Sequence: c.sequence, Method: method, Body: body}
	Sign(&env, c.token)
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err = c.conn.Write(header[:]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if _, err = c.conn.Write(payload); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if _, err = io.ReadFull(c.conn, header[:]); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > 1<<20 {
		return errors.New("agent response too large")
	}
	replyBytes := make([]byte, size)
	if _, err = io.ReadFull(c.conn, replyBytes); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	var reply Reply
	if err = json.Unmarshal(replyBytes, &reply); err != nil {
		return err
	}
	if reply.Version != 1 {
		return errors.New("agent response protocol version mismatch")
	}
	if reply.Sequence != c.sequence {
		return errors.New("agent response sequence mismatch")
	}
	if reply.Error != "" {
		if reply.Failure != nil {
			return &RemoteError{Failure: *reply.Failure}
		}
		return errors.New(reply.Error)
	}
	if response == nil || reply.Body == nil {
		return nil
	}
	b, err := json.Marshal(reply.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, response)
}
