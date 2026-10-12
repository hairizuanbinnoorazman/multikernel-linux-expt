//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func TestCloseIORetriesUntilGuestAcknowledges(t *testing.T) {
	client := &fakeAgentClient{fail: map[string]error{"CloseProcessStdin": errors.New("injected close failure")}}
	p := &process{id: "", status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err == nil {
		t.Fatal("failed guest close was accepted")
	}
	if !p.stdinClosed || p.stdinCloseAcked {
		t.Fatalf("failed close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	delete(client.fail, "CloseProcessStdin")
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !p.stdinClosed || !p.stdinCloseAcked {
		t.Fatalf("successful close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	calls := len(client.calls)
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != calls {
		t.Fatal("acknowledged repeated CloseIO contacted the guest")
	}
}

func TestCloseIOTerminalAndNonTerminalProcesses(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		name := "non-terminal"
		if terminal {
			name = "terminal"
		}
		t.Run(name, func(t *testing.T) {
			client := &fakeAgentClient{fail: map[string]error{}}
			p := &process{id: "exec", terminal: terminal, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
			s := &service{agent: client, processes: map[string]*process{"exec": p}}
			request := &taskapi.CloseIORequest{ExecID: "exec", Stdin: true}
			if _, err := s.CloseIO(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			if !p.stdinClosed || !p.stdinCloseAcked || fmt.Sprint(client.calls) != "[CloseProcessStdin]" {
				t.Fatalf("terminal=%v close state=requested:%v acknowledged:%v calls=%v",
					terminal, p.stdinClosed, p.stdinCloseAcked, client.calls)
			}
			if _, err := s.CloseIO(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			if len(client.calls) != 1 {
				t.Fatalf("terminal=%v repeated close calls=%v", terminal, client.calls)
			}
		})
	}
}

func TestProcessIOTeardownWhileFIFOPeersRemainAttached(t *testing.T) {
	directory := t.TempDir()
	stdinPath := filepath.Join(directory, "stdin")
	stdoutPath := filepath.Join(directory, "stdout")
	for _, path := range []string{stdinPath, stdoutPath} {
		if err := syscall.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stdinIdentity, err := inspectBoundProcessIOPath(stdinPath, true)
	if err != nil {
		t.Fatal(err)
	}
	stdoutIdentity, err := inspectBoundProcessIOPath(stdoutPath, false)
	if err != nil {
		t.Fatal(err)
	}
	stdoutFD, err := unix.Open(stdoutPath, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	stdoutPeer := os.NewFile(uintptr(stdoutFD), stdoutPath)
	defer stdoutPeer.Close()
	p := &process{terminal: true, status: tasktypes.Status_RUNNING, done: make(chan struct{}),
		stdin: stdinPath, stdinIdentity: stdinIdentity, stdout: stdoutPath, stdoutIdentity: stdoutIdentity}
	client := &stdinCaptureAgent{writes: make(chan []byte, 1)}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	if err = s.openProcessIO(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	stdinFD, err := unix.Open(stdinPath, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		closeProcessIO(p)
		t.Fatal(err)
	}
	stdinPeer := os.NewFile(uintptr(stdinFD), stdinPath)
	defer stdinPeer.Close()
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	if _, err = stdinPeer.WriteString("before-teardown"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-client.writes:
		if string(got) != "before-teardown" {
			t.Fatalf("guest stdin before teardown = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("stdin pump did not become active before teardown")
	}

	s.mu.Lock()
	closeProcessIO(p)
	s.mu.Unlock()
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("stdin pump survived teardown with its writer still attached")
	}
	if p.stdinReader != nil || p.stdoutWriter != nil || p.stdoutGuard != nil || p.stderrWriter != nil || p.stderrGuard != nil {
		t.Fatalf("teardown retained I/O handles: %+v", p)
	}
	if _, err = stdinPeer.WriteString("after-teardown"); err == nil {
		t.Fatal("attached stdin writer remained writable after reader teardown")
	}
	var value [1]byte
	deadline := time.Now().Add(time.Second)
	for {
		count, readErr := stdoutPeer.Read(value[:])
		if count == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("attached stdout reader did not observe teardown EOF: count=%d error=%v", count, readErr)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCloseIOReconnectsTransportWithinCallerDeadline(t *testing.T) {
	client := &outputReconnectAgent{closeFailures: 1}
	p := &process{id: "", status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	if !p.stdinClosed || !p.stdinCloseAcked || client.closeCalls != 2 || client.reconnects != 1 {
		t.Fatalf("close reconnect requested=%v acked=%v calls=%d reconnects=%d", p.stdinClosed, p.stdinCloseAcked, client.closeCalls, client.reconnects)
	}
}

func TestCloseIOPersistsCreatedIntentAndDoesNotMutateStoppedProcess(t *testing.T) {
	created := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	stopped := &process{id: "stopped", status: tasktypes.Status_STOPPED, done: make(chan struct{})}
	s := &service{processes: map[string]*process{"": created, "stopped": stopped}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !created.stdinClosed || created.stdinCloseAcked {
		t.Fatalf("created close state = requested:%v acknowledged:%v", created.stdinClosed, created.stdinCloseAcked)
	}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatalf("repeated created CloseIO error = %v", err)
	}
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{ExecID: "stopped", Stdin: true}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped CloseIO error = %v", err)
	}
	if stopped.stdinClosed || stopped.stdinCloseAcked {
		t.Fatalf("stopped CloseIO mutated state: %+v", stopped)
	}
}

func TestStdinPumpRetriesRequestedCloseAndRecordsAcknowledgement(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinReader: eofReadWriteCloser{}, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged, reader := p.stdinCloseAcked, p.stdinReader
		s.mu.Unlock()
		if acknowledged && reader == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pump close state = acknowledged:%v reader:%v", acknowledged, reader)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRecoveredNoFIFOPumpRetriesPendingClose(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinClosed: true, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged := p.stdinCloseAcked
		s.mu.Unlock()
		if acknowledged {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovered no-FIFO close was not acknowledged")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestResizePtyRejectsInvalidRequests(t *testing.T) {
	stopped := &process{terminal: true, status: tasktypes.Status_STOPPED, width: 80, height: 24, sizeSet: true}
	s := &service{processes: map[string]*process{
		"":        {status: tasktypes.Status_CREATED},
		"stopped": stopped,
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("non-terminal resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 65536}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("oversized resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{ExecID: "stopped", Width: 91, Height: 37}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped resize error = %v", err)
	}
	if stopped.width != 80 || stopped.height != 24 || !stopped.sizeSet {
		t.Fatalf("rejected stopped resize mutated state: %+v", stopped)
	}
}

func TestResizePtyRollsBackIntentWhenGuestRejects(t *testing.T) {
	injected := &agent.RemoteError{Failure: protocol.Error{Code: "FAILED_PRECONDITION", Message: "injected resize failure"}}
	fake := &fakeAgentClient{fail: map[string]error{"ResizeProcess": injected}}
	p := &process{terminal: true, status: tasktypes.Status_RUNNING, width: 80, height: 24, sizeSet: true}
	s := &service{agent: fake, processes: map[string]*process{"": p}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); !errors.Is(err, injected) {
		t.Fatalf("resize error = %v", err)
	}
	if p.width != 80 || p.height != 24 || !p.sizeSet {
		t.Fatalf("failed resize retained new intent: %+v", p)
	}
	delete(fake.fail, "ResizeProcess")
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 100, Height: 40}); err != nil {
		t.Fatal(err)
	}
	if p.width != 100 || p.height != 40 || !p.sizeSet {
		t.Fatalf("successful resize state: %+v", p)
	}
}
