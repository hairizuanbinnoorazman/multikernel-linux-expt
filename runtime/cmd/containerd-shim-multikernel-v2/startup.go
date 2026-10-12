//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/runtime/v2/shim"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"golang.org/x/sys/unix"
)

func newCommand(ctx context.Context, id string, opts shim.StartOpts) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "-namespace", ns, "-id", id, "-address", opts.Address)
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

func bindCommandToBundle(cmd *exec.Cmd, directory *safefile.Directory) error {
	if cmd == nil || directory == nil {
		return errors.New("supervisor command or held bundle directory is unavailable")
	}
	cmd.Dir = directory.ProcPath()
	return nil
}

type shimSocketFile interface {
	File() (*os.File, error)
}

type publishedShimFile struct {
	directory *safefile.Directory
	name      string
	identity  safefile.Identity
}

func publishShimFile(path string, data []byte) (*publishedShimFile, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(absolute) != absolute || filepath.Base(absolute) == "." {
		return nil, errors.New("published shim file path must be canonical and absolute")
	}
	directory, err := safefile.OpenOwnedDirectory(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	name := filepath.Base(absolute)
	created, identity, err := directory.PublishExclusiveRegularIdentity(name, data, 0644)
	owner := &publishedShimFile{directory: directory, name: name, identity: identity}
	if err != nil {
		if created {
			return nil, errors.Join(err, owner.close(true))
		}
		return nil, errors.Join(err, directory.Close())
	}
	if !created {
		return nil, errors.Join(errors.New("published shim file already exists"), directory.Close())
	}
	return owner, nil
}

func (p *publishedShimFile) close(remove bool) error {
	if p == nil {
		return nil
	}
	var removeErr error
	if remove {
		_, removeErr = p.directory.RemoveIfIdentity(p.name, p.identity)
	}
	return errors.Join(removeErr, p.directory.Close())
}

func launchShimWorker(ctx context.Context, cmd *exec.Cmd, socket shimSocketFile, address, addressPath, pidPath string) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	var addressFile, pidFile *publishedShimFile
	started := false
	var inherited *os.File
	defer func() {
		var failures []error
		if retErr != nil && inherited != nil {
			failures = append(failures, inherited.Close())
		}
		if retErr != nil && started {
			failures = append(failures, terminateRelay(cmd))
		}
		if err := pidFile.close(retErr != nil); err != nil {
			failures = append(failures, fmt.Errorf("close partial shim PID ownership: %w", err))
		}
		if err := addressFile.close(retErr != nil); err != nil {
			failures = append(failures, fmt.Errorf("close partial shim address ownership: %w", err))
		}
		retErr = errors.Join(retErr, errors.Join(failures...))
	}()
	addressFile, retErr = publishShimFile(addressPath, []byte(address))
	if retErr != nil {
		return retErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	inherited, err = socket.File()
	if err != nil {
		return err
	}
	cmd.ExtraFiles = []*os.File{inherited}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	started = true
	if err = inherited.Close(); err != nil {
		inherited = nil
		return fmt.Errorf("close inherited shim socket: %w", err)
	}
	inherited = nil
	if err = ctx.Err(); err != nil {
		return err
	}
	pidFile, retErr = publishShimFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)))
	if retErr != nil {
		return retErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *service) StartShim(ctx context.Context, opts shim.StartOpts) (_ string, retErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd, err := newCommand(ctx, opts.ID, opts)
	if err != nil {
		return "", err
	}
	if err = bindCommandToBundle(cmd, s.bundleDirectory); err != nil {
		return "", err
	}
	address, err := shim.SocketAddress(ctx, opts.Address, opts.ID)
	if err != nil {
		return "", err
	}
	socket, existing, err := acquireStartShimSocket(ctx, address,
		func(address string) (startShimSocket, error) {
			path, pathErr := pathnameShimSocket(address)
			if pathErr != nil {
				return nil, pathErr
			}
			if pathErr = unixsocket.EnsureParent(path); pathErr != nil {
				return nil, pathErr
			}
			return unixsocket.ListenExclusive(path, 0600)
		},
		func(path string) (startShimSocketOwner, error) { return unixsocket.Capture(path) },
	)
	if err != nil {
		return "", err
	}
	if existing {
		if err = s.authenticateExistingShim(address, opts); err != nil {
			return "", err
		}
		return address, nil
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, socket.Close())
		}
	}()
	if err = launchShimWorker(ctx, cmd, socket, address, "address", "shim.pid"); err != nil {
		return "", err
	}
	return address, nil
}

func pathnameShimSocket(address string) (string, error) {
	const unixPrefix = "unix://"
	if !strings.HasPrefix(address, unixPrefix) {
		return "", errors.New("deterministic shim address is not a pathname Unix socket")
	}
	path := strings.TrimPrefix(address, unixPrefix)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("deterministic shim socket path is not canonical and absolute")
	}
	return path, nil
}

func acquireStartShimSocket(ctx context.Context, address string, create startShimSocketFactory, capture startShimSocketOwnerFactory) (startShimSocket, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	socket, err := create(address)
	if err == nil {
		return socket, false, nil
	}
	if !shim.SocketEaddrinuse(err) {
		return nil, false, err
	}
	path, err := pathnameShimSocket(address)
	if err != nil {
		return nil, false, err
	}
	owner, err := capture(path)
	if err != nil {
		return nil, false, fmt.Errorf("capture pre-existing shim socket: %w", err)
	}
	connection, dialErr := owner.DialContext(ctx)
	if dialErr == nil {
		return nil, true, errors.Join(connection.Close(), owner.Close())
	}
	if err = ctx.Err(); err != nil {
		return nil, false, errors.Join(err, owner.Close())
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		return nil, false, errors.Join(fmt.Errorf("probe pre-existing shim socket: %w", dialErr), owner.Close())
	}
	if err = owner.Remove(); err != nil {
		_ = owner.Close()
		return nil, false, errors.Join(fmt.Errorf("remove stale shim socket: %w", err), dialErr)
	}
	if err = ctx.Err(); err != nil {
		return nil, false, err
	}
	socket, err = create(address)
	if err != nil {
		return nil, false, fmt.Errorf("bind shim socket after stale cleanup: %w", err)
	}
	return socket, false, nil
}

func readProcEntry(directory *os.File, name string, limit int64) ([]byte, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("process metadata exceeds its bound")
	}
	return data, nil
}

func openedIdentity(file *os.File) (rootfspkg.DirectoryIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return rootfspkg.DirectoryIdentity{}, err
	}
	return rootfspkg.DirectoryIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, UID: stat.Uid}, nil
}

func openProcLink(directory *os.File, name string, directoryOnly bool) (*os.File, rootfspkg.DirectoryIdentity, error) {
	flags := unix.O_PATH | unix.O_CLOEXEC
	if directoryOnly {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(directory.Fd()), name, flags, 0)
	if err != nil {
		return nil, rootfspkg.DirectoryIdentity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	identity, err := openedIdentity(file)
	if err != nil {
		_ = file.Close()
		return nil, rootfspkg.DirectoryIdentity{}, err
	}
	return file, identity, nil
}

func processGroupFromStat(data []byte) (int, error) {
	close := bytes.LastIndex(data, []byte(") "))
	if close < 0 {
		return 0, errors.New("supervisor process stat is malformed")
	}
	fields := strings.Fields(string(data[close+2:]))
	if len(fields) < 3 || fields[0] == "Z" || fields[0] == "X" {
		return 0, errors.New("supervisor process is not live")
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil || group <= 1 {
		return 0, errors.Join(errors.New("supervisor process group is invalid"), err)
	}
	return group, nil
}

func commandLineArguments(data []byte) ([]string, error) {
	if len(data) == 0 || data[len(data)-1] != 0 {
		return nil, errors.New("supervisor command line is empty or unterminated")
	}
	parts := bytes.Split(data[:len(data)-1], []byte{0})
	arguments := make([]string, len(parts))
	for index, part := range parts {
		if len(part) == 0 {
			return nil, errors.New("supervisor command line contains an empty argument")
		}
		arguments[index] = string(part)
	}
	return arguments, nil
}

func (s *service) authenticateExistingShim(address string, opts shim.StartOpts) error {
	if s.bundleDirectory == nil {
		return errors.New("held bundle directory is unavailable for existing shim authentication")
	}
	publishedAddress, _, err := s.bundleDirectory.ReadRegularIdentity("address", 0644, 4096)
	if err != nil || string(publishedAddress) != address {
		return errors.Join(errors.New("existing shim address metadata does not match"), err)
	}
	pidBytes, _, err := s.bundleDirectory.ReadRegularIdentity("shim.pid", 0644, 64)
	if err != nil {
		return fmt.Errorf("read existing shim PID metadata: %w", err)
	}
	if len(pidBytes) == 0 || bytes.IndexFunc(pidBytes, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return errors.New("existing shim PID metadata is invalid")
	}
	pid64, err := strconv.ParseInt(string(pidBytes), 10, 32)
	if err != nil || pid64 <= 1 {
		return errors.Join(errors.New("existing shim PID metadata is invalid"), err)
	}
	pid := int(pid64)
	procFD, err := unix.Open(filepath.Join("/proc", strconv.Itoa(pid)), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open existing shim process identity: %w", err)
	}
	processDirectory := os.NewFile(uintptr(procFD), "shim-process")
	defer processDirectory.Close()
	cwd, cwdIdentity, err := openProcLink(processDirectory, "cwd", true)
	if err != nil {
		return fmt.Errorf("open existing shim cwd: %w", err)
	}
	defer cwd.Close()
	if cwdIdentity != s.bundleIdentity {
		return errors.New("existing shim cwd differs from the held bundle")
	}
	shimExecutable, shimExecutableIdentity, err := openProcLink(processDirectory, "exe", false)
	if err != nil {
		return fmt.Errorf("open existing shim executable: %w", err)
	}
	defer shimExecutable.Close()
	selfFD, err := unix.Open("/proc/self/exe", unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	selfExecutable := os.NewFile(uintptr(selfFD), "self-executable")
	defer selfExecutable.Close()
	selfIdentity, err := openedIdentity(selfExecutable)
	if err != nil || shimExecutableIdentity.Device != selfIdentity.Device || shimExecutableIdentity.Inode != selfIdentity.Inode {
		return errors.Join(errors.New("existing shim executable identity differs"), err)
	}
	statBytes, err := readProcEntry(processDirectory, "stat", 4096)
	if err != nil {
		return fmt.Errorf("read existing shim process stat: %w", err)
	}
	group, err := processGroupFromStat(statBytes)
	if err != nil || group != pid {
		return errors.Join(errors.New("existing shim is not its process-group leader"), err)
	}
	commandLine, err := readProcEntry(processDirectory, "cmdline", 16<<10)
	if err != nil {
		return fmt.Errorf("read existing shim command line: %w", err)
	}
	arguments, err := commandLineArguments(commandLine)
	if err != nil {
		return err
	}
	for name, expected := range map[string]string{"namespace": s.namespace, "id": s.id, "address": opts.Address} {
		observed, flagErr := invocationFlagValue(arguments, name)
		if flagErr != nil || observed != expected {
			return errors.Join(fmt.Errorf("existing shim %s invocation differs", name), flagErr)
		}
	}
	return nil
}
