//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func inspectProcessIOPath(path string) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("stdio path must be absolute and canonical")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 || info.Mode().Perm()&0077 != 0 ||
		(!info.Mode().IsRegular() && info.Mode()&os.ModeNamedPipe == 0) {
		return nil, errors.New("stdio path must be a private caller-owned single-link FIFO or regular file")
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_PATH | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	opened, statErr := file.Stat()
	closeErr := file.Close()
	if err = errors.Join(statErr, closeErr); err != nil {
		return nil, err
	}
	if !sameProcessIOIdentity(info, opened) {
		return nil, errors.New("stdio path identity changed while validating")
	}
	return opened, nil
}

func normalizeProcessIOPathAlias(path, alias, target string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!filepath.IsAbs(alias) || filepath.Clean(alias) != alias ||
		!filepath.IsAbs(target) || filepath.Clean(target) != target {
		return "", errors.New("stdio path must be absolute and canonical")
	}
	if !strings.HasPrefix(path, alias+string(os.PathSeparator)) {
		return path, nil
	}
	before, err := os.Lstat(alias)
	if err != nil {
		return "", err
	}
	identity, ok := before.Sys().(*syscall.Stat_t)
	if !ok || before.Mode()&os.ModeSymlink == 0 || identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return "", errors.New("stdio runtime alias is not a caller-owned single-link symlink")
	}
	linked, err := os.Readlink(alias)
	if err != nil {
		return "", err
	}
	after, err := os.Lstat(alias)
	if err != nil || !sameProcessIOIdentity(before, after) {
		return "", errors.New("stdio runtime alias identity changed while validating")
	}
	if linked != target {
		return "", errors.New("stdio runtime alias has an unexpected target")
	}
	return filepath.Join(target, strings.TrimPrefix(path, alias+string(os.PathSeparator))), nil
}

func normalizeProcessIOPath(path string) (string, error) {
	return normalizeProcessIOPathAlias(path, "/var/run", "/run")
}

func sameProcessIOIdentity(before, after os.FileInfo) bool {
	left, leftOK := before.Sys().(*syscall.Stat_t)
	right, rightOK := after.Sys().(*syscall.Stat_t)
	return leftOK && rightOK && os.SameFile(before, after) && before.Mode() == after.Mode() &&
		left.Uid == right.Uid && left.Gid == right.Gid && left.Nlink == right.Nlink && left.Ctim == right.Ctim
}

func inspectBoundProcessIOPath(path string, stdin bool) (processIOIdentity, error) {
	if path == "" {
		return processIOIdentity{}, nil
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		return processIOIdentity{}, err
	}
	value, ok := processIOIdentityFromInfo(info)
	if !ok || !validProcessIOIdentity(path, value, stdin) {
		if stdin {
			return processIOIdentity{}, errors.New("stdin must be a private named pipe")
		}
		return processIOIdentity{}, errors.New("output must be a private named pipe or regular file")
	}
	return value, nil
}

type boundProcessIOPaths struct {
	stdin, stdout, stderr                         string
	stdinIdentity, stdoutIdentity, stderrIdentity processIOIdentity
}

func inspectNormalizedProcessIOPath(path string, stdin bool) (string, processIOIdentity, error) {
	if path == "" {
		return "", processIOIdentity{}, nil
	}
	normalized, err := normalizeProcessIOPath(path)
	if err != nil {
		return "", processIOIdentity{}, err
	}
	identity, err := inspectBoundProcessIOPath(normalized, stdin)
	return normalized, identity, err
}

func inspectBoundProcessIOPaths(stdin, stdout, stderr string) (boundProcessIOPaths, error) {
	var result boundProcessIOPaths
	var err error
	result.stdin, result.stdinIdentity, err = inspectNormalizedProcessIOPath(stdin, true)
	if err != nil {
		return boundProcessIOPaths{}, fmt.Errorf("inspect stdin: %w", err)
	}
	result.stdout, result.stdoutIdentity, err = inspectNormalizedProcessIOPath(stdout, false)
	if err != nil {
		return boundProcessIOPaths{}, fmt.Errorf("inspect stdout: %w", err)
	}
	result.stderr, result.stderrIdentity, err = inspectNormalizedProcessIOPath(stderr, false)
	if err != nil {
		return boundProcessIOPaths{}, fmt.Errorf("inspect stderr: %w", err)
	}
	return result, nil
}

func openKnownProcessIOPath(ctx context.Context, path string, flags int, expected processIOIdentity) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	observed, ok := processIOIdentityFromInfo(opened)
	if !ok || observed != expected {
		_ = file.Close()
		return nil, errors.New("stdio path identity changed while opening")
	}
	if err = ctx.Err(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openOutput(ctx context.Context, path string, expected processIOIdentity) (io.WriteCloser, io.Closer, error) {
	if path == "" {
		return nil, nil, nil
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		return nil, nil, err
	}
	observed, ok := processIOIdentityFromInfo(info)
	if !ok || observed != expected {
		return nil, nil, errors.New("stdio path identity changed before opening")
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		f, err := openKnownProcessIOPath(ctx, path, unix.O_WRONLY|unix.O_APPEND, expected)
		return f, nil, err
	}
	// O_RDWR opens synchronously and keeps a read endpoint present even when
	// the creating client detaches before another client attaches.
	guard, err := openKnownProcessIOPath(ctx, path, unix.O_RDWR|unix.O_NONBLOCK, expected)
	if err != nil {
		return nil, nil, err
	}
	keeper, err := startOutputFIFOKeeper(guard, fifoKeeperGrace)
	if err != nil {
		_ = guard.Close()
		return nil, nil, err
	}
	w, err := openKnownProcessIOPath(ctx, path, unix.O_WRONLY|unix.O_NONBLOCK, expected)
	if err != nil {
		_ = keeper.Close()
		return nil, nil, err
	}
	// A later containerd attach can reopen the same FIFO and consume buffered
	// and future guest output.  The helper retains a duplicate of the guard for
	// a bounded handoff interval if this worker is killed, preventing the
	// existing containerd reader from observing EOF before a replacement worker
	// reopens the FIFO.  It never reads from the FIFO.
	return w, keeper, nil
}

type outputFIFOKeeper struct {
	guard   *os.File
	control *os.File
	command *exec.Cmd
	once    sync.Once
	err     error
}

func startOutputFIFOKeeper(guard *os.File, grace time.Duration) (*outputFIFOKeeper, error) {
	if guard == nil || grace < time.Millisecond || grace > time.Minute {
		return nil, errors.New("invalid output FIFO keeper configuration")
	}
	controlReader, controlWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create output FIFO keeper control: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		_ = controlReader.Close()
		_ = controlWriter.Close()
		return nil, fmt.Errorf("resolve output FIFO keeper executable: %w", err)
	}
	command := exec.Command(self, fifoKeeperMode, strconv.FormatInt(grace.Milliseconds(), 10))
	command.ExtraFiles = []*os.File{guard, controlReader}
	if err = command.Start(); err != nil {
		_ = controlReader.Close()
		_ = controlWriter.Close()
		return nil, fmt.Errorf("start output FIFO keeper: %w", err)
	}
	_ = controlReader.Close()
	return &outputFIFOKeeper{guard: guard, control: controlWriter, command: command}, nil
}

func (k *outputFIFOKeeper) Close() error {
	if k == nil {
		return nil
	}
	k.once.Do(func() {
		_, signalErr := k.control.Write([]byte{1})
		controlErr := k.control.Close()
		guardErr := k.guard.Close()
		waitErr := k.command.Wait()
		k.err = errors.Join(signalErr, controlErr, guardErr, waitErr)
	})
	return k.err
}

func runOutputFIFOKeeper(arguments []string) int {
	if len(arguments) != 1 {
		return 2
	}
	milliseconds, err := strconv.ParseInt(arguments[0], 10, 64)
	if err != nil || milliseconds < 1 || milliseconds > int64(time.Minute/time.Millisecond) {
		return 2
	}
	guard := os.NewFile(3, "output-fifo-guard")
	control := os.NewFile(4, "output-fifo-control")
	if guard == nil || control == nil {
		return 2
	}
	defer guard.Close()
	defer control.Close()
	var message [1]byte
	if count, _ := control.Read(message[:]); count == 1 {
		return 0
	}
	// EOF means the worker died without its explicit close notification.  Keep
	// the writer endpoint alive only for the bounded replacement interval.
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	return 0
}

func (s *service) openProcessIO(ctx context.Context, p *process) (err error) {
	p.stdoutWriter, p.stdoutGuard, err = openOutput(ctx, p.stdout, p.stdoutIdentity)
	if err != nil {
		return fmt.Errorf("open stdout: %w", err)
	}
	if !p.terminal {
		p.stderrWriter, p.stderrGuard, err = openOutput(ctx, p.stderr, p.stderrIdentity)
		if err != nil {
			closeProcessIO(p)
			return fmt.Errorf("open stderr: %w", err)
		}
	}
	if p.stdin != "" {
		p.stdinReader, err = openKnownProcessIOPath(ctx, p.stdin, unix.O_RDONLY|unix.O_NONBLOCK, p.stdinIdentity)
		if err != nil {
			closeProcessIO(p)
			return fmt.Errorf("open stdin: %w", err)
		}
	}
	return nil
}

func closeProcessIO(p *process) {
	if p.stdinReader != nil {
		_ = p.stdinReader.Close()
		p.stdinReader = nil
	}
	if p.stdoutWriter != nil {
		_ = p.stdoutWriter.Close()
		p.stdoutWriter = nil
	}
	if p.stderrWriter != nil {
		_ = p.stderrWriter.Close()
		p.stderrWriter = nil
	}
	if p.stdoutGuard != nil {
		_ = p.stdoutGuard.Close()
		p.stdoutGuard = nil
	}
	if p.stderrGuard != nil {
		_ = p.stderrGuard.Close()
		p.stderrGuard = nil
	}
}
