//go:build linux

// Package dockerengine supervises a fixed, trusted child-local Docker bootstrap.
// It accepts no OCI hooks, host sockets, workload-selected commands or TCP APIs.
package dockerengine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

const logLimit = 64 << 10

// tailLog always drains command output while retaining only a bounded tail.
// Logs stay local; error classifications never reflect daemon output.
type tailLog struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (b *tailLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) >= logLimit {
		b.truncated = b.truncated || len(b.data)+len(p) > logLimit
		b.data = append(b.data[:0], p[len(p)-logLimit:]...)
		return n, nil
	}
	if over := len(b.data) + len(p) - logLimit; over > 0 {
		copy(b.data, b.data[over:])
		b.data = b.data[:len(b.data)-over]
		b.truncated = true
	}
	b.data = append(b.data, p...)
	return n, nil
}

type Service struct {
	scope    *guestscope.Scope
	pidfd    int
	done     chan struct{}
	failed   chan struct{}
	stopping atomic.Bool
	stopMu   sync.Mutex
	closed   bool
	started  bool
	log      tailLog
}

// Launch takes ownership of scope. command must be a caller-constructed fixed
// outer-root bootstrap which remains foreground until engine shutdown. Even on
// failure a non-nil Service retains cleanup authority for retry. No shell is
// selected from workload input. The startup deadline is at most 30 seconds.
func Launch(ctx context.Context, scope *guestscope.Scope, command *exec.Cmd, socket string) (*Service, error) {
	if ctx == nil || scope == nil || command == nil || socket == "" {
		return nil, errors.New("Docker supervisor requires trusted startup inputs")
	}
	s := &Service{scope: scope, pidfd: -1, done: make(chan struct{}), failed: make(chan struct{})}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	if command.Process != nil || command.Stdout != nil || command.Stderr != nil {
		return s, errors.New("Docker bootstrap command is already in use")
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	attr := *command.SysProcAttr
	if attr.PidFD != nil {
		return s, errors.New("Docker bootstrap pidfd already supplied")
	}
	attr.PidFD = &s.pidfd
	command.SysProcAttr = &attr
	command.Stdout = &s.log
	command.Stderr = &s.log
	// Wait must not hang on inherited logging pipes after the leader exits.
	command.WaitDelay = time.Second
	if err := scope.Start(command); err != nil {
		return s, errors.New("Docker bootstrap launch failed")
	}
	s.started = true
	go func() {
		_ = command.Wait()
		close(s.done)
		if !s.stopping.Load() {
			close(s.failed)
		}
	}()
	fail := func(cause error) (*Service, error) {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s, errors.Join(cause, s.Stop(cleanup))
	}
	if s.pidfd < 0 {
		return fail(errors.New("Docker bootstrap requires kernel pidfd support"))
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		select {
		case <-s.done:
			return fail(errors.New("Docker bootstrap exited before readiness"))
		default:
		}
		if err := probe(startup, s.scope, socket); err == nil {
			select {
			case <-s.done:
				return fail(errors.New("Docker bootstrap exited during readiness"))
			default:
				return s, nil
			}
		}
		select {
		case <-s.done:
			return fail(errors.New("Docker bootstrap exited before readiness"))
		case <-startup.Done():
			return fail(errors.New("Docker engine readiness deadline exceeded"))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// UnexpectedExit closes only for an unrequested launcher exit. Requested Stop
// must allow the authenticated quiesce/shutdown reply to finish.
func (s *Service) UnexpectedExit() <-chan struct{} { return s.failed }

// Stop signals only the launcher's held pidfd, gives it a short grace period,
// then kills the entire owned scope. Returning success proves population zero.
// No helper from the mutable OCI root is executed during shutdown.
func (s *Service) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Docker stop context is required")
	}
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	if s.closed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.stopping.Store(true)
	if s.pidfd >= 0 {
		if err := unix.PidfdSendSignal(s.pidfd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
			return errors.New("Docker bootstrap signal failed")
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-s.done:
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		timer.Stop()
	}
	if err := s.scope.Stop(ctx); err != nil {
		return errors.New("Docker descendants have not quiesced")
	}
	if s.started {
		select {
		case <-s.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.pidfd >= 0 {
		_ = unix.Close(s.pidfd)
		s.pidfd = -1
	}
	s.closed = true
	return nil
}

func peerConnection(ctx context.Context, scope *guestscope.Scope, socket string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, errors.New("Docker socket unavailable")
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, errors.New("Docker socket is not Unix")
	}
	raw, err := u.SyscallConn()
	peerfd := -1
	if err == nil {
		var controlErr error
		controlErr = raw.Control(func(fd uintptr) {
			cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
			if e != nil || cred.Uid != 1000 {
				err = errors.New("Docker peer UID is not rootless")
				return
			}
			// SO_PEERPIDFD binds to the socket peer directly, avoiding pidfd_open(PID)
			// races when a process dies and its numeric PID is recycled.
			received, e := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PEERPIDFD)
			if e != nil {
				err = errors.New("Docker socket peer pidfd unavailable")
				return
			}
			peerfd = received
			unix.CloseOnExec(peerfd)
			member, e := scope.ContainsLivePeer(int(cred.Pid), peerfd)
			if e != nil || !member {
				err = errors.New("Docker socket peer is outside owned scope")
			}
		})
		if controlErr != nil {
			err = controlErr
		}
	}
	if err != nil {
		if peerfd >= 0 {
			unix.Close(peerfd)
		}
		conn.Close()
		return nil, err
	}
	return &peerConn{Conn: conn, pidfd: peerfd}, nil
}

type peerConn struct {
	net.Conn
	pidfd int
	once  sync.Once
}

func (c *peerConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { _ = unix.Close(c.pidfd) })
	return err
}

func probe(ctx context.Context, scope *guestscope.Scope, socket string) error {
	ctx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 8192,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return peerConnection(ctx, scope, socket) }}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Docker readiness redirect refused") }}
	get := func(path string, limit int64) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, errors.New("Docker readiness request failed")
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, errors.New("Docker readiness status refused")
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil || int64(len(data)) > limit {
			return nil, errors.New("Docker readiness body exceeds bound")
		}
		return data, nil
	}
	ping, err := get("/_ping", 2)
	if err != nil {
		return err
	}
	if string(ping) != "OK" {
		return errors.New("Docker ping response refused")
	}
	data, err := get("/info", 64<<10)
	if err != nil {
		return err
	}
	return validateInfo(data)
}

func validateInfo(data []byte) error {
	var info struct {
		OSType          string
		Driver          string
		DockerRootDir   string
		SecurityOptions []string
	}
	if json.Unmarshal(data, &info) != nil || info.OSType != "linux" || info.Driver != "vfs" || info.DockerRootDir != "/home/rootless/.local/share/docker" || !slices.Contains(info.SecurityOptions, "name=rootless") {
		return errors.New("Docker daemon does not match offline rootless profile")
	}
	return nil
}
