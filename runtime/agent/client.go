package agent

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
)

type Client struct {
	conn                  net.Conn
	token                 []byte
	sandboxID, generation string
	endpoint              uint32
	mu                    sync.Mutex
	sequence              uint64
}

func Dial(path, sandboxID, generation string, endpoint uint32, token []byte) (*Client, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &Client{conn: c, token: token, sandboxID: sandboxID, generation: generation, endpoint: endpoint}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) Call(method string, request, response any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
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
		return err
	}
	if _, err = c.conn.Write(payload); err != nil {
		return err
	}
	if _, err = io.ReadFull(c.conn, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > 1<<20 {
		return errors.New("agent response too large")
	}
	replyBytes := make([]byte, size)
	if _, err = io.ReadFull(c.conn, replyBytes); err != nil {
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
