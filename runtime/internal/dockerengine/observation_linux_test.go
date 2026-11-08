//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestEngineObservationLocalStates(t *testing.T) {
	for _, state := range []string{"unavailable", "stopping", "stopped", "exited"} {
		t.Run(state, func(t *testing.T) {
			s := &Service{done: make(chan struct{})}
			switch state {
			case "stopping":
				s.stopping.Store(true)
			case "stopped":
				s.closed.Store(true)
			case "exited":
				s.started = true
				close(s.done)
			}
			r, err := s.Observe(t.Context())
			if err != nil || r.Validate() != nil || r.Status != state {
				t.Fatalf("state %s: %+v %v", state, r, err)
			}
		})
	}
	s := &Service{}
	if r, err := s.Observe(nil); err == nil || r.Status != "" {
		t.Fatal("nil context accepted")
	}
}

func TestEngineObservationQueueBudgetPreservesOwner(t *testing.T) {
	s := &Service{done: make(chan struct{}), pidfd: -1}
	if err := s.stopGate.lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.stopGate.unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, err := s.Observe(ctx)
	if !errors.Is(err, context.Canceled) || r.Status != "" {
		t.Fatal("canceled observation returned state", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	r, err = s.Observe(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || r.Status != "" {
		t.Fatal("queued observation returned state", err)
	}
	if s.closed.Load() || s.stopping.Load() || s.pidfd != -1 || len(s.stopGate.held) != 1 {
		t.Fatal("observation changed cleanup ownership")
	}
}

// The launcher has exited while the supervisor's log-reaping channel remains
// open. Only the held kernel identity may authorize its liveness observation.
func TestEngineObservationRefusesUnreapedLauncher(t *testing.T) {
	fd := -1
	cmd := exec.Command("/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{PidFD: &fd}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer func() {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}()
	if fd < 0 {
		t.Fatal("missing launcher pidfd")
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if n, err := unix.Poll(fds, 3000); err != nil || n != 1 {
		t.Fatal("launcher did not exit", err)
	}
	s := &Service{started: true, pidfd: fd, done: make(chan struct{})}
	r, err := s.Observe(t.Context())
	if err != nil || r.Status != "exited" {
		t.Fatal("unreaped launcher accepted", r, err)
	}
}
