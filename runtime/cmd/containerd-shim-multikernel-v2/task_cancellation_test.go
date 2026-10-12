//go:build linux

package main

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestTaskStateGuardsRejectInvalidTransitionsBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	init := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	execProcess := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}

	if _, err := s.Start(context.Background(), &taskapi.StartRequest{ExecID: "exec"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec start before init error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "missing", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrNotFound) {
		t.Fatalf("missing kill error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("created kill error = %v", err)
	}
	init.status = tasktypes.Status_STOPPED
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "new-exec", Spec: encoded}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec after init stop error = %v", err)
	}
	if _, err = s.Delete(context.Background(), &taskapi.DeleteRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("init delete with retained exec error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("invalid transitions contacted agent: %v", fake.calls)
	}
}

func TestPreCancelledTaskMutationsPreserveStateAndAvoidGuestContact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assertCanceled := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	}

	t.Run("create", func(t *testing.T) {
		s := &service{id: "task", bundle: "/bundle", processes: map[string]*process{}}
		_, err := s.Create(ctx, &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"})
		assertCanceled(t, err)
		if len(s.processes) != 0 {
			t.Fatalf("processes = %+v", s.processes)
		}
	})

	t.Run("start", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		init := &process{status: tasktypes.Status_RUNNING}
		execProcess := &process{status: tasktypes.Status_CREATED}
		s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}
		_, err := s.Start(ctx, &taskapi.StartRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if execProcess.status != tasktypes.Status_CREATED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", execProcess.status, fake.calls)
		}
	})

	t.Run("kill", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Kill(ctx, &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM)})
		assertCanceled(t, err)
		if len(fake.calls) != 0 {
			t.Fatalf("calls=%v", fake.calls)
		}
	})

	t.Run("exec", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Exec(ctx, &taskapi.ExecProcessRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if len(s.processes) != 1 || len(fake.calls) != 0 {
			t.Fatalf("processes=%+v calls=%v", s.processes, fake.calls)
		}
	})

	t.Run("delete", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_STOPPED}
		s := &service{agent: fake, processes: map[string]*process{"exec": p}}
		_, err := s.Delete(ctx, &taskapi.DeleteRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if p.deleting || p.exitEventQueued || p.deleteEventQueued || len(fake.calls) != 0 {
			t.Fatalf("process=%+v calls=%v", p, fake.calls)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		s := &service{processes: map[string]*process{}}
		_, err := s.Shutdown(ctx, &taskapi.ShutdownRequest{})
		assertCanceled(t, err)
		if s.shuttingDown {
			t.Fatal("cancelled shutdown sealed service")
		}
	})

	t.Run("resize", func(t *testing.T) {
		p := &process{terminal: true, status: tasktypes.Status_CREATED, width: 80, height: 24, sizeSet: true}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.ResizePty(ctx, &taskapi.ResizePtyRequest{Width: 100, Height: 40})
		assertCanceled(t, err)
		if p.width != 80 || p.height != 24 || !p.sizeSet {
			t.Fatalf("size=%dx%d set=%v", p.width, p.height, p.sizeSet)
		}
	})

	t.Run("pause", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_RUNNING}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Pause(ctx, &taskapi.PauseRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_RUNNING || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("resume", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_PAUSED}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Resume(ctx, &taskapi.ResumeRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_PAUSED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("close-io", func(t *testing.T) {
		p := &process{status: tasktypes.Status_CREATED}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true})
		assertCanceled(t, err)
		if p.stdinClosed || p.stdinCloseAcked {
			t.Fatalf("close state=requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
		}
	})
}

func TestPreCancelledTaskReadsAndWaitAvoidGuestContact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeAgentClient{fail: map[string]error{}}
	done := make(chan struct{})
	s := &service{agent: fake, processes: map[string]*process{"": {
		pid: 41, status: tasktypes.Status_RUNNING, done: done,
	}}}
	for name, invoke := range map[string]func() error{
		"state": func() error {
			_, err := s.State(ctx, &taskapi.StateRequest{})
			return err
		},
		"wait": func() error {
			_, err := s.Wait(ctx, &taskapi.WaitRequest{})
			return err
		},
		"pids": func() error {
			_, err := s.Pids(ctx, &taskapi.PidsRequest{})
			return err
		},
		"connect": func() error {
			_, err := s.Connect(ctx, &taskapi.ConnectRequest{})
			return err
		},
		"stats": func() error {
			_, err := s.Stats(ctx, &taskapi.StatsRequest{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invoke(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
	if len(fake.calls) != 0 {
		t.Fatalf("cancelled reads contacted guest: %v", fake.calls)
	}
	select {
	case <-done:
		t.Fatal("cancelled Wait changed process completion")
	default:
	}
}

func TestTaskEntryLockWaitHonorsCancellation(t *testing.T) {
	for name, invoke := range map[string]func(context.Context, *service) error{
		"state": func(ctx context.Context, s *service) error {
			_, err := s.State(ctx, &taskapi.StateRequest{})
			return err
		},
		"close-io": func(ctx context.Context, s *service) error {
			_, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &service{processes: map[string]*process{"": {status: tasktypes.Status_CREATED}}}
			s.mu.Lock()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() { result <- invoke(ctx, s) }()
			time.Sleep(10 * time.Millisecond)
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					s.mu.Unlock()
					t.Fatalf("contended call error = %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				s.mu.Unlock()
				t.Fatal("contended Task call ignored cancellation")
			}
			if s.processes[""].stdinClosed {
				s.mu.Unlock()
				t.Fatal("cancelled contended call mutated process state")
			}
			s.mu.Unlock()
		})
	}
}

func TestWaitPostExitLockHonorsCancellation(t *testing.T) {
	done := make(chan struct{})
	s := &service{processes: map[string]*process{"": {status: tasktypes.Status_RUNNING, done: done}}}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := s.Wait(ctx, &taskapi.WaitRequest{})
		result <- err
	}()
	// Allow Wait to retain the process and block on its completion channel.
	time.Sleep(10 * time.Millisecond)
	s.mu.Lock()
	close(done)
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.mu.Unlock()
			t.Fatalf("post-exit lock error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.mu.Unlock()
		t.Fatal("Wait ignored cancellation while reacquiring its post-exit lock")
	}
	s.mu.Unlock()
}
