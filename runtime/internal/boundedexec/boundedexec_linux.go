//go:build linux

// Package boundedexec runs a Linux command and its descendants as one bounded
// process-group operation while retaining only a fixed amount of diagnostics.
package boundedexec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var ErrOutputLimit = errors.New("command output exceeds limit")

type capture struct {
	mu       sync.Mutex
	data     []byte
	maximum  int
	overflow bool
}

func (w *capture) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := w.maximum - len(w.data)
	if remaining > 0 {
		w.data = append(w.data, value[:min(remaining, len(value))]...)
	}
	if len(value) > remaining {
		w.overflow = true
	}
	// Always drain the full write so a noisy child cannot block after the
	// retention bound. Overflow is returned after the process exits.
	return len(value), nil
}

func (w *capture) result() ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.data), w.overflow
}

func Run(ctx context.Context, timeout time.Duration, binary string, arguments, environment []string, maximum int) ([]byte, error) {
	if timeout <= 0 || maximum <= 0 {
		return nil, errors.New("bounded command requires positive timeout and output limit")
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(bounded, binary, arguments...)
	cmd.Env = environment
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 5 * time.Second
	output := &capture{maximum: maximum}
	cmd.Stdout, cmd.Stderr = output, output
	runErr := cmd.Run()
	retained, overflow := output.result()
	if overflow {
		runErr = errors.Join(runErr, ErrOutputLimit)
	}
	if contextErr := bounded.Err(); contextErr != nil {
		runErr = errors.Join(runErr, contextErr)
	}
	return retained, runErr
}
