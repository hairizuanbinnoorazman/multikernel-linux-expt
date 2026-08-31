package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Server struct {
	Service  *lifecycle.Service
	MaxFrame int
	mu       sync.Mutex
	listener net.Listener
}

func (s *Server) Listen(ctx context.Context, path string) error {
	if s.MaxFrame == 0 {
		s.MaxFrame = 1 << 20
	}
	if st, e := os.Lstat(path); e == nil {
		if st.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace non-socket")
		}
		os.Remove(path)
	}
	l, e := net.Listen("unix", path)
	if e != nil {
		return e
	}
	if e = os.Chmod(path, 0660); e != nil {
		l.Close()
		return e
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	go func() { <-ctx.Done(); l.Close() }()
	for {
		c, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		go s.handle(ctx, c)
	}
}
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
func (s *Server) handle(ctx context.Context, c net.Conn) {
	defer c.Close()
	data, e := io.ReadAll(io.LimitReader(c, int64(s.MaxFrame+1)))
	if e != nil {
		return
	}
	resp := protocol.Response{Version: 1}
	if len(data) > s.MaxFrame {
		resp.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: "frame too large"}
	} else {
		var req protocol.Request
		if e = protocol.StrictDecode(data, &req); e != nil {
			resp.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
		} else {
			resp = s.Dispatch(ctx, req)
		}
	}
	json.NewEncoder(c).Encode(resp)
}
func (s *Server) Dispatch(ctx context.Context, r protocol.Request) protocol.Response {
	out := protocol.Response{Version: 1, RequestID: r.RequestID}
	if r.Version != 1 {
		out.Error = &protocol.Error{Code: "UNSUPPORTED", Message: "protocol version must be 1"}
		return out
	}
	switch r.Method {
	case "NodeInfo":
		out.Body = map[string]any{"protocol_version": 1, "sandboxes": len(s.Service.List())}
	case "ListSandboxes":
		out.Body = s.Service.List()
	case "SandboxState":
		if x, ok := s.Service.Get(r.SandboxID); ok {
			out.Body = x
		} else {
			out.Error = &protocol.Error{Code: "NOT_FOUND", Message: "sandbox not found"}
		}
	case "CreateSandbox":
		var c protocol.SandboxConfig
		if e := protocol.StrictDecode(r.Body, &c); e != nil {
			out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
			break
		}
		out.Body, out.Error = s.Service.Create(ctx, c, r.IdempotencyKey)
	case "LoadSandbox":
		out.Body, out.Error = s.Service.Load(ctx, r.SandboxID, r.Generation, r.IdempotencyKey)
	case "StartSandbox":
		out.Body, out.Error = s.Service.Start(ctx, r.SandboxID, r.Generation, r.IdempotencyKey)
	case "StopSandbox":
		out.Body, out.Error = s.Service.Stop(ctx, r.SandboxID, r.Generation, r.IdempotencyKey)
	case "DeleteSandbox":
		out.Body, out.Error = s.Service.Delete(ctx, r.SandboxID, r.Generation, r.IdempotencyKey)
	default:
		out.Error = &protocol.Error{Code: "UNSUPPORTED", Message: "unknown method"}
	}
	return out
}
