package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type Server struct {
	Service    *lifecycle.Service
	MaxFrame   int
	AllowedUID uint32
	mu         sync.Mutex
	listener   net.Listener
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
	info, e := os.Lstat(path)
	if e != nil {
		l.Close()
		return e
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != s.AllowedUID || info.Mode().Perm() != 0660 {
		l.Close()
		return errors.New("Unix socket ownership or mode does not match policy")
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
		if e = authorizePeer(c, s.AllowedUID); e != nil {
			c.Close()
			continue
		}
		go s.handle(ctx, c)
	}
}

func authorizePeer(connection net.Conn, allowedUID uint32) error {
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return errors.New("peer is not a Unix-domain connection")
	}
	raw, err := unixConnection.SyscallConn()
	if err != nil {
		return err
	}
	var credential *unix.Ucred
	var socketErr error
	if err = raw.Control(func(fd uintptr) {
		credential, socketErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if !credentialAuthorized(credential, allowedUID) {
		return errors.New("Unix peer is not authorized")
	}
	return nil
}

func credentialAuthorized(credential *unix.Ucred, allowedUID uint32) bool {
	return credential != nil && credential.Uid == allowedUID
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
	if !validRequestID(r.RequestID) {
		out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: "request ID must be 1-128 printable ASCII bytes"}
		return out
	}
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
	case "WatchEvents":
		var query protocol.EventQuery
		if len(r.Body) != 0 {
			if e := protocol.StrictDecode(r.Body, &query); e != nil {
				out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
				break
			}
		}
		out.Body, out.Error = s.Service.Events(query.AfterSequence, query.Limit)
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

func validRequestID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}
