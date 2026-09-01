package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Envelope struct {
	Version    int             `json:"version"`
	SandboxID  string          `json:"sandbox_id"`
	Generation string          `json:"generation"`
	Endpoint   uint32          `json:"endpoint"`
	Sequence   uint64          `json:"sequence"`
	Method     string          `json:"method"`
	Body       json.RawMessage `json:"body,omitempty"`
	MAC        string          `json:"mac"`
}
type Reply struct {
	Version  int    `json:"version"`
	Sequence uint64 `json:"sequence"`
	Body     any    `json:"body,omitempty"`
	Error    string `json:"error,omitempty"`
}
type Server struct {
	Manager               *Manager
	SandboxID, Generation string
	Endpoint              uint32
	Token                 []byte
	mu                    sync.Mutex
	last                  uint64
}

func (e Envelope) canonical() []byte { e.MAC = ""; b, _ := json.Marshal(e); return b }
func Sign(e *Envelope, token []byte) {
	m := hmac.New(sha256.New, token)
	m.Write(e.canonical())
	e.MAC = hex.EncodeToString(m.Sum(nil))
}
func (s *Server) verify(e Envelope) error {
	if e.Version != 1 {
		return errors.New("unsupported version")
	}
	if e.SandboxID != s.SandboxID || e.Generation != s.Generation || e.Endpoint != s.Endpoint {
		return errors.New("stale or wrong identity")
	}
	got, x := hex.DecodeString(e.MAC)
	if x != nil {
		return errors.New("invalid authentication")
	}
	m := hmac.New(sha256.New, s.Token)
	m.Write(e.canonical())
	if !hmac.Equal(got, m.Sum(nil)) {
		return errors.New("invalid authentication")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Sequence <= s.last {
		return errors.New("replayed sequence")
	}
	s.last = e.Sequence
	return nil
}
func decode(b []byte, v any) error {
	return protocol.StrictDecode(b, v)
}
func (s *Server) Dispatch(e Envelope) Reply {
	r := Reply{Version: 1, Sequence: e.Sequence}
	if x := s.verify(e); x != nil {
		r.Error = x.Error()
		return r
	}
	switch e.Method {
	case "Capabilities":
		r.Body = map[string]any{"protocol": 1, "oci_features": []string{"argv", "environment", "cwd", "split-stdio", "exit-code", "terminal", "terminal-resize"}}
	case "CreateProcess":
		var q struct{ ID, Bundle string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Create(q.ID, q.Bundle); x != nil {
			r.Error = x.Error()
		}
	case "ExecProcess":
		var q struct {
			ID   string      `json:"id"`
			Root string      `json:"root"`
			Spec ProcessSpec `json:"spec"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Exec(q.ID, q.Root, q.Spec); x != nil {
			r.Error = x.Error()
		}
	case "StartProcess":
		var q struct {
			ID          string `json:"id"`
			Width       uint32 `json:"width,omitempty"`
			Height      uint32 `json:"height,omitempty"`
			InitialSize bool   `json:"initial_size,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.StartWithSize(q.ID, q.Width, q.Height, q.InitialSize); x != nil {
			r.Error = x.Error()
		}
	case "SignalProcess":
		var q struct{ ID, Signal string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if sig, x := SignalNumber(q.Signal); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Signal(q.ID, sig); x != nil {
			r.Error = x.Error()
		}
	case "ResizeProcess":
		var q struct {
			ID     string `json:"id"`
			Width  uint32 `json:"width"`
			Height uint32 `json:"height"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Resize(q.ID, q.Width, q.Height); x != nil {
			r.Error = x.Error()
		}
	case "WaitProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if st, x := s.Manager.Wait(q.ID); x != nil {
			r.Error = x.Error()
		} else {
			r.Body = st
		}
	case "StateProcess":
		var q struct {
			ID string `json:"id"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if st, x := s.Manager.State(q.ID); x != nil {
			r.Error = x.Error()
		} else {
			r.Body = st
		}
	case "ConfigureNetwork":
		var q struct {
			Name, Address, Gateway string
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.ConfigureNetwork(q.Name, q.Address, q.Gateway); x != nil {
			r.Error = x.Error()
		}
	case "ExchangeNetwork":
		var q struct {
			Packet []byte `json:"packet,omitempty"`
		}
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if packet, x := s.Manager.ExchangeNetwork(q.Packet); x != nil {
			r.Error = x.Error()
		} else {
			r.Body = map[string][]byte{"packet": packet}
		}
	case "CloseNetwork":
		if x := s.Manager.CloseNetwork(); x != nil {
			r.Error = x.Error()
		}
	case "DeleteProcess":
		var q struct{ ID string }
		if x := decode(e.Body, &q); x != nil {
			r.Error = x.Error()
		} else if x = s.Manager.Delete(q.ID); x != nil {
			r.Error = x.Error()
		}
	case "Shutdown":
		if !s.Manager.Quiescent() {
			r.Error = "managed processes are still running"
		} else {
			r.Body = map[string]string{"status": "quiesced"}
		}
	default:
		r.Error = "unsupported method"
	}
	return r
}
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		go func() { defer c.Close(); s.ServeConn(ctx, c) }()
	}
}

func (s *Server) ServeConn(ctx context.Context, c net.Conn) error {
	go func() { <-ctx.Done(); c.Close() }()
	for {
		var header [4]byte
		if _, e := io.ReadFull(c, header[:]); e != nil {
			if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
				return nil
			}
			return e
		}
		size := binary.BigEndian.Uint32(header[:])
		var env Envelope
		reply := Reply{Version: 1}
		if size > 1<<20 {
			reply.Error = "frame too large"
		} else {
			b := make([]byte, size)
			if _, e := io.ReadFull(c, b); e != nil {
				return e
			}
			if decodeErr := decode(b, &env); decodeErr != nil {
				reply.Error = decodeErr.Error()
			} else {
				reply = s.Dispatch(env)
			}
		}
		b, e := json.Marshal(reply)
		if e != nil {
			return e
		}
		binary.BigEndian.PutUint32(header[:], uint32(len(b)))
		if _, e = c.Write(header[:]); e != nil {
			return e
		}
		if _, e = c.Write(b); e != nil {
			return e
		}
	}
}
