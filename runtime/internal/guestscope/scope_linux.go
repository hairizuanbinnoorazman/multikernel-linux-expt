//go:build linux

// Package guestscope owns one non-delegated cgroup for a child-local service.
// It never adopts an existing group or sends signals using a numeric process ID.
package guestscope

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var ErrClosed = errors.New("service scope is closed")
var ErrIdentity = errors.New("service scope identity changed")

type Limits struct {
	PIDs        uint64
	MemoryBytes uint64
}
type Scope struct {
	mu                sync.Mutex
	parent, directory int
	name              string
	device            uint64
	inode             uint64
	closing, closed   bool
	owned             bool
}

func writeControl(directory int, name, value string) error {
	fd, err := unix.Openat(directory, name, unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	n, err := unix.Write(fd, []byte(value))
	if err == nil && n != len(value) {
		return errors.New("short service control write")
	}
	return err
}
func readControl(directory int, name string) (string, error) {
	return readBoundedControl(directory, name, 4096)
}

func readBoundedControl(directory int, name string, limit int) (string, error) {
	fd, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	buffer := make([]byte, limit+1)
	total := 0
	for total < len(buffer) {
		n, e := unix.Read(fd, buffer[total:])
		total += n
		if e != nil {
			return "", e
		}
		if n == 0 {
			break
		}
	}
	if total > limit {
		return "", errors.New("service control exceeds bound")
	}
	return strings.TrimSpace(string(buffer[:total])), nil
}

func openProtectedParent(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
		var status unix.Stat_t
		if unix.Fstat(fd, &status) != nil || status.Uid != 0 || status.Mode&0022 != 0 {
			_ = unix.Close(fd)
			return -1, errors.New("service parent has an unsafe ancestor")
		}
	}
	return fd, nil
}

// New requires a protected cgroup-v2 parent with memory and pids controllers
// already enabled. Controller enablement is an explicit trusted boot action.
func New(parent, name string, limits Limits) (scope *Scope, result error) {
	if !filepath.IsAbs(parent) || filepath.Clean(parent) != parent || !namePattern.MatchString(name) || limits.PIDs == 0 || limits.PIDs > 4096 || limits.MemoryBytes < 16<<20 || limits.MemoryBytes > 1<<40 || limits.MemoryBytes%4096 != 0 {
		return nil, errors.New("invalid service scope policy")
	}
	fd, err := openProtectedParent(parent)
	if err != nil {
		return nil, err
	}
	var status unix.Stat_t
	var filesystem unix.Statfs_t
	if unix.Fstat(fd, &status) != nil || unix.Fstatfs(fd, &filesystem) != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC || status.Uid != 0 || status.Mode&0022 != 0 {
		unix.Close(fd)
		return nil, errors.New("service parent is not protected cgroup v2")
	}
	if err = unix.Mkdirat(fd, name, 0700); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("refuse existing or unavailable service scope: %w", err)
	}
	directory, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		_ = unix.Unlinkat(fd, name, unix.AT_REMOVEDIR)
		unix.Close(fd)
		return nil, err
	}
	scope = &Scope{parent: fd, directory: directory, name: name, owned: true}
	defer func() {
		if result != nil {
			result = errors.Join(result, unix.Unlinkat(fd, name, unix.AT_REMOVEDIR), unix.Close(directory), unix.Close(fd))
			scope = nil
		}
	}()
	if err = unix.Fstat(directory, &status); err != nil {
		return nil, err
	}
	scope.device = uint64(status.Dev)
	scope.inode = status.Ino
	for _, control := range []struct{ name, value string }{{"pids.max", strconv.FormatUint(limits.PIDs, 10)}, {"memory.max", strconv.FormatUint(limits.MemoryBytes, 10)}} {
		if err = writeControl(directory, control.name, control.value); err != nil {
			return nil, err
		}
		actual, e := readControl(directory, control.name)
		if e != nil || actual != control.value {
			return nil, errors.New("service limit was not accepted")
		}
	}
	if err = writeControl(directory, "memory.oom.group", "1"); err != nil {
		return nil, err
	}
	// Some kernels omit swap accounting. No fallback removes the RAM/pid limits.
	if err = writeControl(directory, "memory.swap.max", "0"); err != nil && !errors.Is(err, unix.ENOENT) {
		return nil, err
	}
	// Refuse kernels without atomic descendant kill before admitting any process.
	killFD, err := unix.Openat(directory, "cgroup.kill", unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	_ = unix.Close(killFD)
	return scope, nil
}

// Start clones directly into the owned cgroup, before any child code executes.
// Keeping the lock through Start prevents a concurrent Stop from closing/reusing
// the cgroup descriptor between admission and clone3.
func (s *Scope) Start(command *exec.Cmd) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owned {
		return ErrIdentity
	}
	if s.closed || s.closing {
		return ErrClosed
	}
	if command == nil || command.Process != nil {
		return errors.New("a new service command is required")
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	// The per-command value must not share mutable state with other launches.
	attributes := *command.SysProcAttr
	if attributes.UseCgroupFD {
		return errors.New("caller already supplied a service cgroup")
	}
	attributes.UseCgroupFD = true
	attributes.CgroupFD = s.directory
	command.SysProcAttr = &attributes
	return command.Start()
}

func populated(text string) (bool, error) {
	seen := false
	active := false
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return false, errors.New("malformed service cgroup events")
		}
		if fields[0] == "populated" {
			if seen || fields[1] != "0" && fields[1] != "1" {
				return false, errors.New("ambiguous service population")
			}
			seen = true
			active = fields[1] == "1"
		}
	}
	if !seen {
		return false, errors.New("missing service population")
	}
	return active, nil
}

// Empty is an observation, not permission to adopt, reuse or remove a group.
func (s *Scope) Empty() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owned {
		return false, ErrIdentity
	}
	if s.closed {
		return true, nil
	}
	data, err := readControl(s.directory, "cgroup.events")
	if err != nil {
		return false, err
	}
	active, err := populated(data)
	return !active, err
}

// Stop atomically kills every descendant, including separate sessions, and
// confirms population zero before removing only the directory created by New.
// A timeout retains ownership for a later bounded retry; it is never success.
func (s *Scope) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("service stop context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owned {
		return ErrIdentity
	}
	if s.closed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.closing = true
	if err := writeControl(s.directory, "cgroup.kill", "1"); err != nil {
		return err
	}
	for {
		data, err := readControl(s.directory, "cgroup.events")
		if err != nil {
			return err
		}
		active, err := populated(data)
		if err != nil {
			return err
		}
		if !active {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	var actual unix.Stat_t
	if err := unix.Fstatat(s.parent, s.name, &actual, unix.AT_SYMLINK_NOFOLLOW); err != nil || uint64(actual.Dev) != s.device || actual.Ino != s.inode {
		return ErrIdentity
	}
	if err := unix.Unlinkat(s.parent, s.name, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	s.closed = true
	return errors.Join(unix.Close(s.directory), unix.Close(s.parent))
}

// ContainsLivePeer checks an exact socket peer obtained using SO_PEERPIDFD and
// SO_PEERCRED. The caller retains the pidfd until its socket operation completes.
// This non-delegated scope admits no nested cgroups. Numeric PIDs alone never
// authorize a peer; a live pinned process must appear in this owned group.
func (s *Scope) ContainsLivePeer(pid int, pidfd int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.owned {
		return false, ErrIdentity
	}
	if s.closed || s.closing {
		return false, ErrClosed
	}
	if pid <= 0 || pidfd < 0 {
		return false, errors.New("invalid service peer")
	}
	alive := func() (bool, error) {
		events := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
		n, err := unix.Poll(events, 0)
		return n == 0, err
	}
	if ok, err := alive(); err != nil || !ok {
		return false, err
	}
	// pids.max is bounded at 4096. A conservative 32 bytes per entry covers
	// every Linux PID without unbounded allocation or inventory disclosure.
	text, err := readBoundedControl(s.directory, "cgroup.procs", 4096*32)
	if err != nil {
		return false, err
	}
	found := false
	for _, line := range strings.Fields(text) {
		if line == strconv.Itoa(pid) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	return alive()
}
