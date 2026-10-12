//go:build linux

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

const namespaceHolderMode = "__multikernel_namespace_holder"

type commandNamespaceHolder struct{ command *exec.Cmd }

func (h *commandNamespaceHolder) PID() uint32 {
	if h == nil || h.command == nil || h.command.Process == nil {
		return 0
	}
	return uint32(h.command.Process.Pid)
}

func (h *commandNamespaceHolder) Validate(netns string) error {
	pid := h.PID()
	if pid == 0 {
		return errors.New("network namespace holder has no process identity")
	}
	target, err := os.Stat(netns)
	if err != nil {
		return err
	}
	held, err := os.Stat("/proc/" + strconv.Itoa(int(pid)) + "/ns/net")
	if err != nil {
		return err
	}
	if !os.SameFile(target, held) {
		return errors.New("network namespace holder entered a different namespace")
	}
	return nil
}

func (h *commandNamespaceHolder) Stop() error {
	if h == nil || h.command == nil || h.command.Process == nil {
		return nil
	}
	err := h.command.Process.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	waitErr := h.command.Wait()
	var exitErr *exec.ExitError
	if waitErr == nil || errors.As(waitErr, &exitErr) || errors.Is(waitErr, syscall.ECHILD) {
		return nil
	}
	return waitErr
}

func startNamespaceHolder(netns string) (_ namespaceHolder, retErr error) {
	if !filepath.IsAbs(netns) || filepath.Clean(netns) != netns {
		return nil, errors.New("network namespace holder requires an absolute canonical path")
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	// Enter the namespace before execing the Go helper. setns(2) is
	// thread-scoped, so calling it after the Go runtime has created threads can
	// leave /proc/PID/ns/net pointing at the original namespace.
	command := exec.Command("/usr/bin/nsenter", "--net="+netns, "--", self, namespaceHolderMode)
	command.ExtraFiles = []*os.File{writer}
	command.Stderr = os.Stderr
	command.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err = command.Start(); err != nil {
		writer.Close()
		return nil, err
	}
	holder := &commandNamespaceHolder{command: command}
	defer func() {
		_ = writer.Close()
		if retErr != nil {
			_ = holder.Stop()
		}
	}()
	if err = writer.Close(); err != nil {
		return nil, err
	}
	poll := []unix.PollFd{{Fd: int32(reader.Fd()), Events: unix.POLLIN | unix.POLLHUP}}
	ready, err := unix.Poll(poll, 5000)
	if err != nil {
		return nil, err
	}
	if ready != 1 || poll[0].Revents&unix.POLLIN == 0 {
		return nil, errors.New("network namespace holder did not become ready")
	}
	var status [1]byte
	if _, err = io.ReadFull(reader, status[:]); err != nil || status[0] != 1 {
		return nil, errors.Join(errors.New("network namespace holder rejected its target"), err)
	}
	if err = holder.Validate(netns); err != nil {
		return nil, err
	}
	return holder, nil
}

func runNamespaceHolder() int {
	ready := os.NewFile(3, "namespace-holder-ready")
	if ready == nil {
		return 1
	}
	if _, err := ready.Write([]byte{1}); err != nil {
		ready.Close()
		return 1
	}
	if err := ready.Close(); err != nil {
		return 1
	}
	for {
		_ = unix.Pause()
	}
}
