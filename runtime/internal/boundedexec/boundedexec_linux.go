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
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrOutputLimit = errors.New("command output exceeds limit")

// resolveExecutable uses the environment the command will actually receive.
// exec.Command's basename lookup otherwise searches the parent's ambient PATH.
func resolveExecutable(binary string, environment []string) (string, error) {
	if filepath.IsAbs(binary) {
		return binary, nil
	}
	if binary == "" || strings.ContainsRune(binary, filepath.Separator) {
		return "", errors.New("relative executable paths are prohibited")
	}
	var search string
	for index := len(environment) - 1; index >= 0; index-- {
		if value, found := strings.CutPrefix(environment[index], "PATH="); found {
			search = value
			break
		}
	}
	if search == "" {
		return "", errors.New("basename executable requires an explicit absolute search path")
	}
	directories := filepath.SplitList(search)
	for _, directory := range directories {
		if !filepath.IsAbs(directory) {
			return "", errors.New("executable search path contains a relative or empty entry")
		}
	}
	for _, directory := range directories {
		candidate := filepath.Join(directory, binary)
		if executable, err := exec.LookPath(candidate); err == nil {
			return executable, nil
		}
	}
	return "", &exec.Error{Name: binary, Err: exec.ErrNotFound}
}

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
	return RunWithFiles(ctx, timeout, binary, arguments, environment, maximum, nil)
}

// RunWithFiles is Run with an explicit, ordered set of descriptors inherited
// by the command as file descriptors 3 and above. The caller retains ownership
// of the supplied files and must keep them open until this function returns.
func RunWithFiles(ctx context.Context, timeout time.Duration, binary string, arguments, environment []string, maximum int, files []*os.File) ([]byte, error) {
	if timeout <= 0 || maximum <= 0 {
		return nil, errors.New("bounded command requires positive timeout and output limit")
	}
	for _, file := range files {
		if file == nil {
			return nil, errors.New("bounded command inherited file is nil")
		}
	}
	executable, err := resolveExecutable(binary, environment)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(bounded, executable, arguments...)
	// A nil Cmd.Env inherits all parent variables. An explicit empty environment
	// instead means no inheritance, including when the caller supplies nil.
	cmd.Env = make([]string, len(environment))
	copy(cmd.Env, environment)
	cmd.ExtraFiles = files
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
