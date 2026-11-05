//go:build linux

package dockerengine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

func TestDockerScopeStopDiagnosticsFixedClassification(t *testing.T) {
	for flags := 0; flags < 8; flags++ {
		for _, errno := range []uint64{0, 1, 4095, 4096, ^uint64(0)} {
			causes := []error{errors.New("SYNTHETIC_PRIVATE_ENGINE_OUTPUT /private/guest env=secret")}
			if flags&1 != 0 {
				causes = append(causes, context.DeadlineExceeded)
			}
			if flags&2 != 0 {
				causes = append(causes, context.Canceled)
			}
			if flags&4 != 0 {
				causes = append(causes, guestscope.ErrIdentity)
			}
			if errno != 0 {
				causes = append(causes, unix.Errno(errno))
			}
			record := classifyScopeStopFailure(errors.Join(causes...))
			wantErrno := errno
			if errno > 4095 {
				wantErrno = 0
			}
			if record.timeout != (flags&1 != 0) || record.canceled != (flags&2 != 0) || record.identity != (flags&4 != 0) || record.errno != wantErrno {
				t.Fatal("wrong failure classification")
			}
			var output bytes.Buffer
			r := scopeStopReporter{writer: &output}
			if !r.report(t.Context(), record) || r.report(t.Context(), record) {
				t.Fatal("diagnostic quota failed")
			}
			data := output.Bytes()
			if len(data) > scopeStopDiagnosticLimit || bytes.Count(data, []byte("\n")) != 1 || bytes.Contains(data, []byte("SYNTHETIC_")) || bytes.Contains(data, []byte("/private/")) || bytes.Contains(data, []byte("env=secret")) {
				t.Fatal("diagnostic bound or privacy failed")
			}
			if !bytes.Equal(data, record.bytes()) || !strings.HasPrefix(string(data), "MK_DOCKER_SCOPE_STOP_FAIL timeout=") {
				t.Fatal("legacy marker mismatch")
			}
		}
	}
	if bytes.Contains((scopeStopRecord{errno: ^uint64(0)}).bytes(), []byte("184467")) {
		t.Fatal("direct record bypassed errno bound")
	}
}

type scopeStopBlockedWriter struct {
	entered, release, finished chan struct{}
	calls                      atomic.Int64
}

func (w *scopeStopBlockedWriter) Write(data []byte) (int, error) {
	w.calls.Add(1)
	close(w.entered)
	<-w.release
	close(w.finished)
	return len(data), nil
}
func TestDockerScopeStopDiagnosticsContextBudgetAndPressure(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "timer"} {
		t.Run(mode, func(t *testing.T) {
			writer := &scopeStopBlockedWriter{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
			defer func() { close(writer.release); <-writer.finished }()
			reporter := scopeStopReporter{writer: writer}
			var ctx context.Context
			var cancel context.CancelFunc
			if mode == "deadline" {
				ctx, cancel = context.WithTimeout(t.Context(), 10*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(t.Context())
			}
			defer cancel()
			returned := make(chan bool, 1)
			go func() { returned <- reporter.report(ctx, scopeStopRecord{identity: true}) }()
			select {
			case <-writer.entered:
			case <-time.After(time.Second):
				t.Fatal("diagnostic writer did not start")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case ok := <-returned:
				if ok {
					t.Fatal("blocked writer claimed delivery")
				}
			case <-time.After(time.Second):
				t.Fatal("diagnostic ignored caller/timer budget")
			}
			if mode != "timer" && ctx.Err() == nil {
				t.Fatal("caller cancellation not observed")
			}
			var wg sync.WaitGroup
			for range 1000 {
				wg.Go(func() {
					if reporter.report(t.Context(), scopeStopRecord{timeout: true}) {
						t.Error("repeat diagnostic accepted")
					}
				})
			}
			wg.Wait()
			if writer.calls.Load() != 1 {
				t.Fatal("pressure accumulated writers")
			}
		})
	}
}

type scopeStopFailedWriter struct {
	short bool
	calls int
}

func (w *scopeStopFailedWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.short {
		return len(data) - 1, nil
	}
	return 0, io.ErrClosedPipe
}
func TestDockerScopeStopDiagnosticsFailedOrMissingDeliveryDoesNotRetry(t *testing.T) {
	for _, short := range []bool{false, true} {
		writer := &scopeStopFailedWriter{short: short}
		reporter := scopeStopReporter{writer: writer}
		if reporter.report(nil, scopeStopRecord{}) || reporter.attempted.Load() {
			t.Fatal("nil context consumed attempt")
		}
		if reporter.report(t.Context(), scopeStopRecord{}) || reporter.report(t.Context(), scopeStopRecord{}) || writer.calls != 1 {
			t.Fatal("failed delivery retried or claimed success")
		}
	}
	var reporter *scopeStopReporter
	if reporter.report(t.Context(), scopeStopRecord{}) {
		t.Fatal("nil reporter accepted")
	}
	missing := scopeStopReporter{}
	if missing.report(t.Context(), scopeStopRecord{}) || !missing.attempted.Load() {
		t.Fatal("missing writer did not consume quota")
	}
}
