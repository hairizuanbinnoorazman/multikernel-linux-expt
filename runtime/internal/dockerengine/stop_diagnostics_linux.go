//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

const scopeStopDiagnosticBudget = 50 * time.Millisecond
const scopeStopDiagnosticLimit = 128

// Only fixed classifications reach the writer; never retain or format a cause.
type scopeStopRecord struct {
	timeout, canceled, identity bool
	errno                       uint64
}

func classifyScopeStopFailure(err error) scopeStopRecord {
	var errno unix.Errno
	errors.As(err, &errno)
	number := uint64(errno)
	if number > 4095 {
		number = 0
	}
	return scopeStopRecord{errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, guestscope.ErrIdentity), number}
}
func (r scopeStopRecord) bytes() []byte {
	errno := r.errno
	if errno > 4095 {
		errno = 0
	}
	return []byte(fmt.Sprintf("MK_DOCKER_SCOPE_STOP_FAIL timeout=%t canceled=%t identity=%t errno=%d\n", r.timeout, r.canceled, r.identity, errno))
}

type scopeStopStderrWriter struct{}

func (scopeStopStderrWriter) Write(data []byte) (int, error) {
	fd, err := unix.FcntlInt(2, unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(fd), "docker-scope-stop-stderr")
	defer file.Close()
	return file.Write(data)
}

type scopeStopReporter struct {
	writer    io.Writer
	attempted atomic.Bool
}

var processScopeStopDiagnostics = scopeStopReporter{writer: scopeStopStderrWriter{}}

// One process-wide attempt, even after timeout/failure. There is no retry queue
// or waiter worker. A timed-out write may retain one record/descriptor/goroutine
// until stderr resumes or the process exits; it cannot block cleanup callers.
func (r *scopeStopReporter) report(ctx context.Context, record scopeStopRecord) bool {
	return r.reportWithBudget(ctx, record, scopeStopDiagnosticBudget)
}

// The production wrapper fixes the wait at 50 ms. A separate bounded budget
// lets controls distinguish caller cancellation from timer expiry without tight
// scheduling thresholds; it is not runtime configuration or an exported API.
func (r *scopeStopReporter) reportWithBudget(ctx context.Context, record scopeStopRecord, budget time.Duration) bool {
	if r == nil || ctx == nil || budget <= 0 || budget > time.Second || !r.attempted.CompareAndSwap(false, true) {
		return false
	}
	if r.writer == nil {
		return false
	}
	data := record.bytes()
	if len(data) > scopeStopDiagnosticLimit {
		return false
	}
	done := make(chan bool, 1)
	timer := time.NewTimer(budget)
	defer timer.Stop()
	go func() { n, err := r.writer.Write(data); done <- err == nil && n == len(data) }()
	select {
	case ok := <-done:
		return ok
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}
