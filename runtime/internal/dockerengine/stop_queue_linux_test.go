//go:build linux

package dockerengine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

func TestDockerStopOwnedProcessHelper(t *testing.T) {
	if os.Getenv("MK_G8_STOP_QUEUE_PROCESS") != "1" {
		return
	}
	signal.Ignore(syscall.SIGTERM)
	fmt.Println("CONTROLLED_STOP_PROCESS_READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func stopQueueServiceFixture(t *testing.T) *Service {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDockerStopOwnedProcessHelper$")
	cmd.Env = append(os.Environ(), "MK_G8_STOP_QUEUE_PROCESS=1", "GORACE=atexit_sleep_ms=0")
	pidfd := -1
	cmd.SysProcAttr = &syscall.SysProcAttr{PidFD: &pidfd}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		input.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if pidfd >= 0 {
			_ = unix.Close(pidfd)
		}
	})
	if pidfd < 0 {
		t.Fatal("fixture requires clone-bound pidfd support")
	}
	ready := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		ready <- err == nil && line == "CONTROLLED_STOP_PROCESS_READY\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("controlled process did not initialize")
		}
	case <-time.After(time.Second):
		t.Fatal("controlled process readiness deadline")
	}
	// The zero-value scope has no cgroup authority. The active stop is canceled
	// during the grace period, before any scope stop or filesystem release.
	return &Service{scope: &guestscope.Scope{}, pidfd: pidfd, done: make(chan struct{}), failed: make(chan struct{})}
}

func TestDockerStopQueuedCallerContextPreservesActiveOwner(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			service := stopQueueServiceFixture(t)
			originalFD := service.pidfd
			ownerCtx, cancelOwner := context.WithCancel(t.Context())
			ownerResult := make(chan error, 1)
			ownerDone := make(chan struct{})
			go func() { ownerResult <- service.Stop(ownerCtx); close(ownerDone) }()
			t.Cleanup(func() {
				cancelOwner()
				select {
				case <-ownerDone:
				case <-time.After(time.Second):
					t.Error("active owner did not finish after its own cancellation")
				}
			})
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			for !service.stopping.Load() {
				select {
				case <-ownerDone:
					t.Fatal("owner returned before holding teardown")
				case <-deadline.C:
					t.Fatal("owner did not enter teardown")
				case <-time.After(time.Millisecond):
				}
			}
			select {
			case <-ownerDone:
				t.Fatal("owner did not remain active")
			default:
			}
			var ctx context.Context
			var cancel context.CancelFunc
			expected := context.Canceled
			if mode == "cancel" {
				ctx, cancel = context.WithCancel(t.Context())
			} else {
				ctx, cancel = context.WithTimeout(t.Context(), 40*time.Millisecond)
				expected = context.DeadlineExceeded
			}
			defer cancel()
			queued := make(chan error, 1)
			entered := make(chan struct{})
			go func() { close(entered); queued <- service.Stop(ctx) }()
			<-entered
			if mode == "cancel" {
				select {
				case <-queued:
					t.Fatal("queued stop returned before cancellation")
				case <-time.After(20 * time.Millisecond):
				}
				cancel()
			}
			var queuedErr error
			returned := false
			select {
			case queuedErr = <-queued:
				returned = true
			case <-time.After(200 * time.Millisecond):
			}
			ownerStillActive := true
			select {
			case <-ownerDone:
				ownerStillActive = false
			default:
			}
			cancelOwner()
			select {
			case <-ownerDone:
			case <-time.After(time.Second):
				t.Fatal("owner cancellation did not complete")
			}
			if err := <-ownerResult; !errors.Is(err, context.Canceled) {
				t.Fatal("waiter disturbed owner result", err)
			}
			if !returned {
				select {
				case queuedErr = <-queued:
				case <-time.After(time.Second):
					t.Fatal("queued stop remained stuck after release")
				}
			}
			if !returned || !ownerStillActive || !errors.Is(queuedErr, expected) {
				t.Error("queued context did not return while active stop owner remained blocked", queuedErr)
			}
			if service.pidfd != originalFD || unix.PidfdSendSignal(originalFD, 0, nil, 0) != nil {
				t.Fatal("cancellation closed pidfd or killed controlled process")
			}
			// A subsequent admitted attempt can acquire the same gate and retain the
			// same authority on its own deadline. No numeric-PID signaling is used.
			retry, cancelRetry := context.WithTimeout(t.Context(), 30*time.Millisecond)
			defer cancelRetry()
			if err := service.Stop(retry); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("gate or cleanup ownership lost after cancellation", err)
			}
			if service.pidfd != originalFD || unix.PidfdSendSignal(originalFD, 0, nil, 0) != nil {
				t.Fatal("subsequent canceled attempt lost live ownership")
			}
		})
	}
}
