//go:build linux

package main

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// These legacy evidence markers contain only fixed stages and a bounded errno.
// They are observations, never shutdown acknowledgements or release authority.
type shutdownDiagnostic uint8

const (
	dockerStopFailed shutdownDiagnostic = iota + 1
	dockerStorageQuiesceFailed
	rootQuiesceFailed
	rootQuiescePassed
	nbdDisconnectFailed
	nbdDisconnectPassed
)
const shutdownDiagnosticBudget = 50 * time.Millisecond
const shutdownDiagnosticLimit = 128

type shutdownDiagnosticLane struct {
	writer io.Writer
	active atomic.Bool
}
type shutdownDiagnosticReporter struct {
	attempted      atomic.Uint32
	stdout, stderr shutdownDiagnosticLane
}
type shutdownDescriptorWriter int

func (fd shutdownDescriptorWriter) Write(data []byte) (int, error) {
	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(duplicate), "shutdown-evidence")
	defer file.Close()
	return file.Write(data)
}

var processShutdownDiagnostics = shutdownDiagnosticReporter{
	stdout: shutdownDiagnosticLane{writer: shutdownDescriptorWriter(1)},
	stderr: shutdownDiagnosticLane{writer: shutdownDescriptorWriter(2)},
}

func shutdownDiagnosticRecord(kind shutdownDiagnostic, errno uint64) ([]byte, bool) {
	suffix := ""
	if errno > 0 && errno <= 4095 {
		suffix = fmt.Sprintf(" errno=%d", errno)
	}
	var line string
	stderr := false
	switch kind {
	case dockerStopFailed:
		line = "MK_DOCKER_STOP_FAIL" + suffix
		stderr = true
	case dockerStorageQuiesceFailed:
		line = "MK_DOCKER_STORAGE_QUIESCE_FAIL"
		stderr = true
	case rootQuiesceFailed:
		line = "MK_STORAGE_ROOT_QUIESCE_FAIL stage=remount-ro" + suffix
	case rootQuiescePassed:
		line = "MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0"
	case nbdDisconnectFailed:
		line = "MK_STORAGE_NBD_DISCONNECT_FAIL device=/dev/nbd0"
	case nbdDisconnectPassed:
		line = "MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0"
	default:
		return nil, false
	}
	return []byte(line + "\n"), stderr
}

func (r *shutdownDiagnosticReporter) report(kind shutdownDiagnostic, errno uint64) bool {
	if r == nil || kind < dockerStopFailed || kind > nbdDisconnectPassed {
		return false
	}
	mask := uint32(1) << uint(kind-1)
	for {
		old := r.attempted.Load()
		if old&mask != 0 {
			return false
		}
		if r.attempted.CompareAndSwap(old, old|mask) {
			break
		}
	}
	lane := &r.stdout
	if kind == dockerStopFailed || kind == dockerStorageQuiesceFailed {
		lane = &r.stderr
	}
	// A busy stream drops this stage permanently. No queue, retry, replacement
	// worker or second descriptor can accumulate behind a blocked write.
	if lane.writer == nil || !lane.active.CompareAndSwap(false, true) {
		return false
	}
	data, _ := shutdownDiagnosticRecord(kind, errno)
	if len(data) > shutdownDiagnosticLimit {
		lane.active.Store(false)
		return false
	}
	done := make(chan bool, 1)
	timer := time.NewTimer(shutdownDiagnosticBudget)
	defer timer.Stop()
	go func() {
		n, err := lane.writer.Write(data)
		lane.active.Store(false)
		done <- err == nil && n == len(data)
	}()
	select {
	case ok := <-done:
		return ok
	case <-timer.C:
		return false
	}
}

// Process stdout and the default stream use bounded process-wide delivery.
// Other explicit writers are trusted in-process evidence collectors/tests.
func emitShutdownDiagnostic(writer io.Writer, kind shutdownDiagnostic, errno uint64) {
	if writer == nil || writer == os.Stdout {
		processShutdownDiagnostics.report(kind, errno)
		return
	}
	data, _ := shutdownDiagnosticRecord(kind, errno)
	if len(data) != 0 {
		_, _ = writer.Write(data)
	}
}
