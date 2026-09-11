package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type Server struct {
	Service    *lifecycle.Service
	Rootfs     *rootfs.Service
	MaxFrame   int
	AllowedUID uint32
	mu         sync.Mutex
	listener   net.Listener
}

func daemonFrameLimit(configured int) int {
	if configured <= 0 || configured > 1<<20 {
		return 1 << 20
	}
	return configured
}

func encodeDaemonResponse(response protocol.Response, maximum int) ([]byte, error) {
	maximum = daemonFrameLimit(maximum)
	encoded, err := json.Marshal(response)
	if err == nil {
		encoded = append(encoded, '\n')
		if len(encoded) <= maximum {
			return encoded, nil
		}
	}
	message := "daemon response exceeds the frame limit"
	if err != nil {
		message = "daemon response encoding failed"
	}
	fallback := protocol.Response{Version: protocol.Version, RequestID: response.RequestID,
		Error: &protocol.Error{Code: "INTERNAL", Message: message}}
	encoded, err = json.Marshal(fallback)
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maximum {
		return nil, errors.New("daemon response frame limit cannot hold an error response")
	}
	return encoded, nil
}

func (s *Server) Listen(ctx context.Context, path string) (retErr error) {
	if s.MaxFrame == 0 {
		s.MaxFrame = 1 << 20
	}
	l, e := unixsocket.Listen(path, 0660)
	if e != nil {
		return e
	}
	defer func() { retErr = errors.Join(retErr, l.Close()) }()
	serveContext, stopServing := context.WithCancel(ctx)
	defer stopServing()
	if l.Owner() != s.AllowedUID {
		return errors.New("Unix socket ownership does not match the allowed peer UID")
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	stopCancellation := protocol.CloseOnContext(serveContext, l)
	defer stopCancellation()
	for {
		c, e := l.Accept()
		if e != nil {
			if serveContext.Err() != nil || errors.Is(e, net.ErrClosed) {
				return nil
			}
			return e
		}
		if e = authorizePeer(c, s.AllowedUID); e != nil {
			c.Close()
			continue
		}
		go s.handle(serveContext, c)
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
	stopCancellation := protocol.CloseOnContext(ctx, c)
	defer stopCancellation()
	maximum := daemonFrameLimit(s.MaxFrame)
	data, e := io.ReadAll(io.LimitReader(c, int64(maximum+1)))
	if e != nil {
		return
	}
	resp := protocol.Response{Version: 1}
	if len(data) > maximum {
		resp.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: "frame too large"}
	} else {
		var req protocol.Request
		if e = protocol.StrictDecode(data, &req); e != nil {
			resp.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
		} else {
			resp = s.Dispatch(ctx, req)
		}
	}
	encoded, e := encodeDaemonResponse(resp, 1<<20)
	if e == nil {
		_ = protocol.WriteFull(c, encoded)
	}
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
	case "PrepareRootfs":
		if s.Rootfs == nil {
			out.Error = &protocol.Error{Code: "UNSUPPORTED", Message: "rootfs preparation service is unavailable"}
			break
		}
		var request rootfs.PrepareRequest
		if e := protocol.StrictDecode(r.Body, &request); e != nil {
			out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
			break
		}
		var e error
		out.Body, e = s.Rootfs.Prepare(ctx, request)
		if e != nil {
			out.Error = &protocol.Error{Code: "FAILED_PRECONDITION", Message: e.Error()}
		}
	case "CleanupRootfs":
		if s.Rootfs == nil {
			out.Error = &protocol.Error{Code: "UNSUPPORTED", Message: "rootfs preparation service is unavailable"}
			break
		}
		var request rootfs.CleanupRequest
		if e := protocol.StrictDecode(r.Body, &request); e != nil {
			out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
			break
		}
		if e := s.Rootfs.Cleanup(ctx, request); e != nil {
			out.Error = &protocol.Error{Code: "FAILED_PRECONDITION", Message: e.Error()}
		} else {
			out.Body = map[string]bool{"cleaned": true}
		}
	case "CreateSandbox":
		var c protocol.SandboxConfig
		if e := protocol.StrictDecode(r.Body, &c); e != nil {
			out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
			break
		}
		out.Body, out.Error = s.Service.Create(ctx, c, r.IdempotencyKey)
	case "CancelCreateSandbox":
		var c protocol.SandboxConfig
		if e := protocol.StrictDecode(r.Body, &c); e != nil {
			out.Error = &protocol.Error{Code: "INVALID_ARGUMENT", Message: e.Error()}
			break
		}
		var safe bool
		safe, out.Error = s.Service.CancelCreate(ctx, c, r.IdempotencyKey)
		if out.Error == nil {
			out.Body = map[string]bool{"safe_to_cleanup": safe}
		}
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
