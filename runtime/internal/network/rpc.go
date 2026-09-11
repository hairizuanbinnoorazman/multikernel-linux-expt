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

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type Client struct {
	Path    string
	Timeout time.Duration
	dial    func(context.Context, string, string) (net.Conn, error)
}

func closeDescriptors(descriptors []int) {
	for _, descriptor := range descriptors {
		_ = unix.Close(descriptor)
	}
}

func parseReceivedRights(oob []byte) ([]int, error) {
	messages, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}
	var descriptors []int
	for index := range messages {
		rights, parseErr := unix.ParseUnixRights(&messages[index])
		descriptors = append(descriptors, rights...)
		if parseErr != nil {
			closeDescriptors(descriptors)
			return nil, parseErr
		}
	}
	for _, descriptor := range descriptors {
		unix.CloseOnExec(descriptor)
	}
	return descriptors, nil
}

type unixRightsWriter interface {
	WriteMsgUnix([]byte, []byte, *net.UnixAddr) (int, int, error)
}

func writeUnixRights(writer unixRightsWriter, payload []byte, descriptor int) error {
	rights := unix.UnixRights(descriptor)
	written, rightsWritten, err := writer.WriteMsgUnix(payload, rights, nil)
	if err != nil {
		return err
	}
	if written != len(payload) || rightsWritten != len(rights) {
		return io.ErrShortWrite
	}
	return nil
}

func (c Client) Attach(ctx context.Context, request Request) (Response, *os.File, error) {
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
		return Response{}, nil, err
	}
	unixConnection, ok := connection.(*net.UnixConn)
	if !ok {
		connection.Close()
		return Response{}, nil, errors.New("mknetd ATTACH requires a Unix socket")
	}
	defer unixConnection.Close()
	deadline, _ := ctx.Deadline()
	_ = unixConnection.SetDeadline(deadline)
	data, err := json.Marshal(request)
	if err != nil {
		return Response{}, nil, err
	}
	if err = protocol.WriteFull(unixConnection, append(data, '\n')); err != nil {
		return Response{}, nil, err
	}
	_ = unixConnection.CloseWrite()
	dataBuffer := make([]byte, (1<<20)+1)
	oob := make([]byte, unix.CmsgSpace(4))
	n, oobn, flags, _, err := unixConnection.ReadMsgUnix(dataBuffer, oob)
	if err != nil {
		return Response{}, nil, err
	}
	descriptors, err := parseReceivedRights(oob[:oobn])
	if err != nil {
		return Response{}, nil, errors.New("mknetd ATTACH returned invalid descriptor metadata")
	}
	retainDescriptor := false
	defer func() {
		if !retainDescriptor {
			closeDescriptors(descriptors)
		}
	}()
	if n > 1<<20 || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 || n == 0 || dataBuffer[n-1] != '\n' {
		return Response{}, nil, errors.New("mknetd ATTACH response is oversized, truncated, or unterminated")
	}
	n--
	var response Response
	if err = protocol.StrictDecode(dataBuffer[:n], &response); err != nil {
		return Response{}, nil, err
	}
	if response.Version != ProtocolVersion || response.RequestID != request.RequestID {
		return Response{}, nil, errors.New("mknetd ATTACH response binding mismatch")
	}
	if response.Error != nil {
		return response, nil, response.Error
	}
	if len(descriptors) != 1 {
		return response, nil, errors.New("mknetd ATTACH returned an invalid descriptor count")
	}
	retainDescriptor = true
	return response, os.NewFile(uintptr(descriptors[0]), "mknetd-tun"), nil
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
	if err = protocol.WriteFull(connection, append(data, '\n')); err != nil {
		return Response{}, err
	}
	if unixConnection, ok := connection.(*net.UnixConn); ok {
		_ = unixConnection.CloseWrite()
	}
	data, err = io.ReadAll(io.LimitReader(connection, (1<<20)+1))
	if err != nil {
		return Response{}, err
	}
	if len(data) > 1<<20 {
		return Response{}, errors.New("mknetd response exceeds the one-MiB limit")
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
	OpenTUN    func(Endpoint) (*os.File, error)
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

func (s *Server) Listen(ctx context.Context, path string) (retErr error) {
	if s.Service == nil {
		return errors.New("mknetd service is required")
	}
	if s.MaxFrame == 0 {
		s.MaxFrame = 1 << 20
	}
	listener, err := unixsocket.Listen(path, 0660)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, listener.Close()) }()
	serveContext, stopServing := context.WithCancel(ctx)
	defer stopServing()
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	stopCancellation := protocol.CloseOnContext(serveContext, listener)
	defer stopCancellation()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if serveContext.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}
			return acceptErr
		}
		if err = authorize(connection, s.AllowedUID); err != nil {
			_ = connection.Close()
			continue
		}
		go s.handle(serveContext, connection)
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
	stopCancellation := protocol.CloseOnContext(ctx, connection)
	defer stopCancellation()
	maxFrame := s.MaxFrame
	if maxFrame == 0 {
		maxFrame = 1 << 20
	}
	data, err := bufio.NewReader(io.LimitReader(connection, int64(maxFrame+2))).ReadBytes('\n')
	response := Response{Version: ProtocolVersion}
	var request Request
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
		if err = protocol.StrictDecode(data, &request); err != nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: err.Error()}
		} else {
			response = s.Service.Dispatch(ctx, request)
		}
	}
	encoded, _ := json.Marshal(response)
	encoded = append(encoded, '\n')
	if request.Method == "ATTACH" && response.Error == nil && response.Endpoint != nil {
		opener := s.OpenTUN
		if opener == nil {
			opener = OpenTUN
		}
		device, openErr := opener(*response.Endpoint)
		if openErr != nil {
			response.Endpoint = nil
			response.Error = &APIError{Code: "INTERNAL", Message: openErr.Error(), Retryable: true}
			encoded, _ = json.Marshal(response)
			encoded = append(encoded, '\n')
		} else {
			defer device.Close()
			if unixConnection, ok := connection.(*net.UnixConn); ok {
				if writeUnixRights(unixConnection, encoded, int(device.Fd())) != nil {
					return
				}
				return
			}
			response.Endpoint = nil
			response.Error = &APIError{Code: "INTERNAL", Message: "ATTACH requires a Unix socket"}
			encoded, _ = json.Marshal(response)
			encoded = append(encoded, '\n')
		}
	}
	_ = protocol.WriteFull(connection, encoded)
}

func CurrentUID() uint32 { return uint32(syscall.Geteuid()) }
