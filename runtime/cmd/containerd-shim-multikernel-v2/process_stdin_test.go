//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func TestStdinFIFOAcceptsLateAndRepeatedWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := inspectBoundProcessIOPath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	p := &process{stdin: path, stdinIdentity: identity, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	agentClient := &stdinCaptureAgent{writes: make(chan []byte, 3)}
	s := &service{agent: agentClient, processes: map[string]*process{"": p}}
	if err = s.openProcessIO(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	// Exercise the no-initial-peer state before attaching the first writer.
	time.Sleep(20 * time.Millisecond)
	assertWrite := func(want string) {
		t.Helper()
		select {
		case got := <-agentClient.writes:
			if string(got) != want {
				t.Fatalf("guest stdin = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("stdin %q was not forwarded", want)
		}
	}
	for attachment, writes := range [][]string{{"late-writer", "same-writer-after-idle"}, {"reattached-writer"}} {
		descriptor, openErr := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if openErr != nil {
			t.Fatal(openErr)
		}
		writer := os.NewFile(uintptr(descriptor), path)
		for index, want := range writes {
			if _, openErr = writer.WriteString(want); openErr != nil {
				_ = writer.Close()
				t.Fatal(openErr)
			}
			assertWrite(want)
			if attachment == 0 && index == 0 {
				// Leave the writer attached while the nonblocking reader observes
				// an empty FIFO. A transient EAGAIN must not terminate the pump.
				time.Sleep(20 * time.Millisecond)
			}
		}
		if openErr = writer.Close(); openErr != nil {
			t.Fatal(openErr)
		}
	}
	s.mu.Lock()
	closeProcessIO(p)
	s.mu.Unlock()
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("stdin pump survived descriptor teardown")
	}
}

func TestCloseIOStopsContinuouslyReadableStdinAfterInflightChunk(t *testing.T) {
	reader := &controlledInput{reads: make(chan chan []byte)}
	client := &stdinCaptureAgent{writes: make(chan []byte, 2)}
	p := &process{stdinReader: reader, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	first := <-reader.reads
	first <- []byte("before-close")
	if got := <-client.writes; string(got) != "before-close" {
		t.Fatalf("first guest stdin = %q", got)
	}
	// Prove the next read is already in flight when CloseIO records its intent.
	second := <-reader.reads
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	second <- []byte("in-flight")
	if got := <-client.writes; string(got) != "in-flight" {
		t.Fatalf("in-flight guest stdin = %q", got)
	}
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("CloseIO did not stop a continuously readable stdin pump")
	}
	if !p.stdinClosed || !p.stdinCloseAcked || p.stdinReader != nil {
		t.Fatalf("closed stdin state = requested:%v acknowledged:%v reader:%v", p.stdinClosed, p.stdinCloseAcked, p.stdinReader)
	}
	select {
	case <-reader.reads:
		t.Fatal("stdin pump accepted another chunk after CloseIO")
	default:
	}
}

func TestPendingStdinReplaysLostAcknowledgementExactlyOnce(t *testing.T) {
	client := &stdinReplayAgent{}
	p := &process{status: tasktypes.Status_RUNNING, stdinPending: []byte("replay-safe"), done: make(chan struct{})}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	if err := s.deliverPendingStdin("init", p); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || client.reconnects != 1 || string(client.accepted) != "replay-safe" {
		t.Fatalf("stdin replay calls=%d reconnects=%d accepted=%q", client.calls, client.reconnects, client.accepted)
	}
	if p.stdinOffset != uint64(len("replay-safe")) || len(p.stdinPending) != 0 {
		t.Fatalf("stdin durable acknowledgement offset=%d pending=%q", p.stdinOffset, p.stdinPending)
	}
}

func TestStdinIntentPersistenceFailurePrecedesGuestMutation(t *testing.T) {
	bundle := privateTestDirectory(t)
	path := filepath.Join(bundle, "stdin")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := inspectBoundProcessIOPath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	p := &process{stdin: path, stdinIdentity: identity, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	agentClient := &stdinCaptureAgent{writes: make(chan []byte, 1)}
	s := &service{bundle: bundle, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent: agentClient, processes: map[string]*process{"": p}}
	// Deliberately omit bundle/.multikernel so the pending-byte publication
	// fails before WriteProcess can mutate guest stdin.
	if err = s.openProcessIO(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	descriptor, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	writer := os.NewFile(uintptr(descriptor), path)
	if _, err = writer.WriteString("durable-first"); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("stdin pump did not stop after durable intent failure")
	}
	select {
	case data := <-agentClient.writes:
		t.Fatalf("guest received stdin before durable intent: %q", data)
	default:
	}
	if p.stdinOffset != 0 || string(p.stdinPending) != "durable-first" || p.stdinReader != nil {
		t.Fatalf("failed stdin intent offset=%d pending=%q reader=%v", p.stdinOffset, p.stdinPending, p.stdinReader)
	}
}

func TestStdinAcknowledgementPersistenceFailureRetainsReplayIdentity(t *testing.T) {
	client := &stdinReplayAgent{}
	p := &process{status: tasktypes.Status_RUNNING, stdinPending: []byte("replay-safe"), done: make(chan struct{})}
	s := &service{bundle: privateTestDirectory(t), sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	// Missing .multikernel makes the post-ack durable update fail. The guest may
	// have accepted the bytes, so the exact pending tuple must remain retryable.
	if err := s.deliverPendingStdin("init", p); err == nil || !strings.Contains(err.Error(), "persist acknowledged stdin offset") {
		t.Fatalf("stdin acknowledgement persistence error = %v", err)
	}
	if p.stdinOffset != 0 || string(p.stdinPending) != "replay-safe" || string(client.accepted) != "replay-safe" {
		t.Fatalf("failed acknowledgement offset=%d pending=%q accepted=%q", p.stdinOffset, p.stdinPending, client.accepted)
	}
}

func TestBoundProcessIORejectsReplacementBeforeStartOrRecovery(t *testing.T) {
	for _, output := range []bool{false, true} {
		name := "stdin"
		if output {
			name = "stdout"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if output {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			identity, err := inspectBoundProcessIOPath(path, !output)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if output {
				if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err = syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			p := &process{done: make(chan struct{})}
			if output {
				p.stdout, p.stdoutIdentity = path, identity
			} else {
				p.stdin, p.stdinIdentity = path, identity
			}
			if err = (&service{}).openProcessIO(context.Background(), p); err == nil || !strings.Contains(err.Error(), "identity changed") {
				t.Fatalf("replacement I/O error = %v", err)
			}
			if _, err = os.Lstat(path); err != nil {
				t.Fatalf("replacement was changed: %v", err)
			}
		})
	}
}
