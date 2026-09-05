package network

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type Client struct {
	Path    string
	Timeout time.Duration
	dial    func(context.Context, string, string) (net.Conn, error)
}

func (c Client) Call(ctx context.Context, request Request) (Response, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dial := c.dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	connection, err := dial(ctx, "unix", c.Path)
	if err != nil {
		return Response{}, err
	}
	defer connection.Close()
	deadline, _ := ctx.Deadline()
	_ = connection.SetDeadline(deadline)
	data, err := json.Marshal(request)
	if err != nil {
		return Response{}, err
	}
	if _, err = connection.Write(append(data, '\n')); err != nil {
		return Response{}, err
	}
	if unixConnection, ok := connection.(*net.UnixConn); ok {
		_ = unixConnection.CloseWrite()
	}
	data, err = io.ReadAll(io.LimitReader(connection, 1<<20))
	if err != nil {
		return Response{}, err
	}
	var response Response
	if err = protocol.StrictDecode(data, &response); err != nil {
		return Response{}, err
	}
	if response.Version != ProtocolVersion || response.RequestID != request.RequestID {
		return Response{}, errors.New("mknetd response binding mismatch")
	}
	if response.Error != nil {
		return response, response.Error
	}
	return response, nil
}

type Server struct {
	Service    *Service
	AllowedUID uint32
	MaxFrame   int
	mu         sync.Mutex
	listener   net.Listener
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) Listen(ctx context.Context, path string) error {
	if s.Service == nil {
		return errors.New("mknetd service is required")
	}
	if s.MaxFrame == 0 {
		s.MaxFrame = 1 << 20
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("refusing to replace non-socket mknetd path")
		}
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err = os.Chmod(path, 0660); err != nil {
		listener.Close()
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}
			return acceptErr
		}
		if err = authorize(connection, s.AllowedUID); err != nil {
			_ = connection.Close()
			continue
		}
		go s.handle(ctx, connection)
	}
}

func authorize(connection net.Conn, allowedUID uint32) error {
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		return errors.New("mknetd peer is not a Unix connection")
	}
	raw, err := unixConnection.SyscallConn()
	if err != nil {
		return err
	}
	var credential *unix.Ucred
	var socketError error
	if err = raw.Control(func(fd uintptr) {
		credential, socketError = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if socketError != nil {
		return socketError
	}
	if credential == nil || credential.Uid != allowedUID {
		return errors.New("mknetd peer UID is not authorized")
	}
	return nil
}

func (s *Server) handle(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	maxFrame := s.MaxFrame
	if maxFrame == 0 {
		maxFrame = 1 << 20
	}
	data, err := bufio.NewReader(io.LimitReader(connection, int64(maxFrame+2))).ReadBytes('\n')
	response := Response{Version: ProtocolVersion}
	if err != nil && !errors.Is(err, io.EOF) {
		response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: err.Error()}
	} else {
		if len(data) > 0 && data[len(data)-1] == '\n' {
			data = data[:len(data)-1]
		}
	}
	if response.Error == nil && len(data) > maxFrame {
		response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "request exceeds the frame limit"}
	} else if response.Error == nil {
		var request Request
		if err = protocol.StrictDecode(data, &request); err != nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: err.Error()}
		} else {
			response = s.Service.Dispatch(ctx, request)
		}
	}
	_ = json.NewEncoder(connection).Encode(response)
}

func CurrentUID() uint32 { return uint32(syscall.Geteuid()) }
