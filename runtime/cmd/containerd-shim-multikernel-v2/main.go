//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	cgroupstats "github.com/containerd/cgroups/stats/v1"
	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const runtimeName = "io.containerd.multikernel.v2"

var runtimeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var guestProcessIdentifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var recoveryGeneration = regexp.MustCompile(`^[0-9a-f]{32}$`)
var recoverySHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

type process struct {
	id, stdin, stdout, stderr      string
	stdinIdentity                  processIOIdentity
	stdoutIdentity                 processIOIdentity
	stderrIdentity                 processIOIdentity
	terminal                       bool
	width, height                  uint32
	sizeSet                        bool
	status                         tasktypes.Status
	pid                            uint32
	exit                           uint32
	exited                         time.Time
	done                           chan struct{}
	stdinReader                    io.ReadWriteCloser
	stdoutWriter, stderrWriter     io.WriteCloser
	stdoutGuard, stderrGuard       io.Closer
	stdinClosed, stdinCloseAcked   bool
	stdinOffset                    uint64
	stdinPending                   []byte
	stdoutOffset, stderrOffset     uint64
	stdoutPressure, stderrPressure time.Time
	exitEventQueued                bool
	deleteEventQueued              bool
	deleting                       bool
}

const processOutputChunk = 4096

type processOutput struct {
	Stdout          []byte `json:"stdout"`
	Stderr          []byte `json:"stderr"`
	StdoutOffset    uint64 `json:"stdout_offset"`
	StderrOffset    uint64 `json:"stderr_offset"`
	Status          string `json:"status"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}

type guestQuiesceResponse struct {
	Status string `json:"status"`
}

type agentClient interface {
	Call(string, any, any) error
	CallContext(context.Context, string, any, any) error
	Close() error
	Reconnect(string) error
	ReconnectContext(context.Context, string) error
}

type networkClient interface {
	Call(context.Context, mknetwork.Request) (mknetwork.Response, error)
	Attach(context.Context, mknetwork.Request) (mknetwork.Response, *os.File, error)
}

type agentDialer func(context.Context, string, string, string, uint32, []byte) (agentClient, error)
type relayFactory func(uint32, string) *exec.Cmd
type relayPathFactory func(uint32, string) string
type relayPathOwner interface{ Remove() error }
type relayOwnerFactory func(string) (relayPathOwner, error)

type stateFileIdentity struct {
	device uint64
	inode  uint64
	uid    uint32
	mode   uint32
	links  uint64
}

type service struct {
	mu                    sync.Mutex
	eventMu               sync.Mutex
	eventRetryStop        sync.Once
	id, namespace, bundle string
	publisher             shim.Publisher
	shutdown              func()
	daemon                daemon.Caller
	sandbox               protocol.Sandbox
	token                 []byte
	agent                 agentClient
	relay                 *exec.Cmd
	relaySocket           string
	relayOwner            relayPathOwner
	netDevice             *os.File
	netDone               chan struct{}
	netReports            chan string
	netWG                 sync.WaitGroup
	netEndpoint           mknetwork.Endpoint
	netClient             networkClient
	agentDial             agentDialer
	newRelay              relayFactory
	relayPath             relayPathFactory
	newRelayOwner         relayOwnerFactory
	ioCallTimeout         time.Duration
	networkCloseTimeout   time.Duration
	netRXPackets          atomic.Uint64
	netTXPackets          atomic.Uint64
	netRXDrops            atomic.Uint64
	netTXDrops            atomic.Uint64
	netErrors             atomic.Uint64
	processes             map[string]*process
	events                eventJournal
	eventJournalIdentity  *stateFileIdentity
	eventRetryCancel      context.CancelFunc
	eventRetryDone        chan struct{}
	shuttingDown          bool
}

func getenv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func lockContext(ctx context.Context, mutex *sync.Mutex) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for !mutex.TryLock() {
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		mutex.Unlock()
		return err
	}
	return nil
}

func newService(ctx context.Context, id string, publisher shim.Publisher, shutdown func()) (shim.Shim, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	bundle, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err = validateServiceIdentity(id, ns, bundle); err != nil {
		return nil, err
	}
	s := &service{id: id, namespace: ns, bundle: bundle, publisher: publisher, shutdown: shutdown,
		daemon:    daemon.Client{Path: getenv("MK_DAEMON_SOCKET", "/run/mkruntimed.sock")},
		netClient: mknetwork.Client{Path: getenv("MK_NETWORK_SOCKET", "/run/mknetd.sock")}, processes: map[string]*process{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err = s.loadEventJournal(); err != nil {
		return nil, err
	}
	if err = s.recoverExisting(ctx); err != nil {
		return nil, err
	}
	if err = s.flushEvents(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel event replay deferred: %v\n", err)
	}
	s.startEventRetry(time.Second)
	return s, nil
}

func (s *service) dialAgent(ctx context.Context, path string) (agentClient, error) {
	if s.agentDial != nil {
		return s.agentDial(ctx, path, s.sandbox.ID, s.sandbox.Generation, s.sandbox.Config.AgentPort, s.token)
	}
	return agent.DialContext(ctx, path, s.sandbox.ID, s.sandbox.Generation, s.sandbox.Config.AgentPort, s.token)
}

func (s *service) relayCommand(port uint32, socket string) *exec.Cmd {
	if s.newRelay != nil {
		return s.newRelay(port, socket)
	}
	return newRelayCommand(getenv("MK_RELAY", "/usr/local/libexec/multikernel/mkvsock-relay"), port, socket)
}

func (s *service) agentRelaySocketPath(port uint32, generation string) string {
	if s.relayPath != nil {
		return s.relayPath(port, generation)
	}
	return relaySocketPath(port, generation)
}

func (s *service) captureRelaySocket(path string) (relayPathOwner, error) {
	if s.newRelayOwner != nil {
		return s.newRelayOwner(path)
	}
	return unixsocket.Capture(path)
}

func validateServiceIdentity(id, namespace, bundle string) error {
	if !runtimeIdentifier.MatchString(id) || !runtimeIdentifier.MatchString(namespace) {
		return fmt.Errorf("%w: invalid task ID or containerd namespace", errdefs.ErrInvalidArgument)
	}
	if !filepath.IsAbs(bundle) || filepath.Clean(bundle) != bundle {
		return fmt.Errorf("%w: bundle must be an absolute canonical path", errdefs.ErrInvalidArgument)
	}
	resolved, err := filepath.EvalSymlinks(bundle)
	if err != nil {
		return fmt.Errorf("%w: resolve bundle: %v", errdefs.ErrInvalidArgument, err)
	}
	if resolved != bundle {
		return fmt.Errorf("%w: symlinked bundle paths are unsupported", errdefs.ErrInvalidArgument)
	}
	info, err := os.Stat(bundle)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: bundle is not a directory", errdefs.ErrInvalidArgument)
	}
	return nil
}

func newCommand(ctx context.Context, id string, opts shim.StartOpts) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "-namespace", ns, "-id", id, "-address", opts.Address)
	cmd.Dir, cmd.Env = cwd, append(os.Environ(), "GOMAXPROCS=2")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

type shimSocketFile interface {
	File() (*os.File, error)
}

func launchShimWorker(ctx context.Context, cmd *exec.Cmd, socket shimSocketFile, address, addressPath, pidPath string) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	addressWritten := false
	pidWritten := false
	started := false
	var inherited *os.File
	defer func() {
		if retErr == nil {
			return
		}
		var failures []error
		if inherited != nil {
			failures = append(failures, inherited.Close())
		}
		if started {
			failures = append(failures, terminateRelay(cmd))
		}
		if pidWritten {
			if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove partial shim PID file: %w", err))
			}
		}
		if addressWritten {
			if err := os.Remove(addressPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove partial shim address file: %w", err))
			}
		}
		retErr = errors.Join(retErr, errors.Join(failures...))
	}()
	if err := shim.WriteAddress(addressPath, address); err != nil {
		return err
	}
	addressWritten = true
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
	if err = shim.WritePidFile(pidPath, cmd.Process.Pid); err != nil {
		return err
	}
	pidWritten = true
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
	address, err := shim.SocketAddress(ctx, opts.Address, opts.ID)
	if err != nil {
		return "", err
	}
	socket, err := shim.NewSocket(address)
	if err != nil {
		if !shim.SocketEaddrinuse(err) {
			return "", err
		}
		_ = shim.RemoveSocket(address)
		socket, err = shim.NewSocket(address)
		if err != nil {
			return "", err
		}
	}
	defer func() {
		if retErr != nil {
			socket.Close()
			_ = shim.RemoveSocket(address)
		}
	}()
	if err = launchShimWorker(ctx, cmd, socket, address, "address", "shim.pid"); err != nil {
		return "", err
	}
	return address, nil
}

type persisted struct {
	SchemaVersion int                `json:"schema_version"`
	ID            string             `json:"id"`
	Generation    string             `json:"generation"`
	PID           uint32             `json:"pid,omitempty"`
	Exit          uint32             `json:"exit"`
	Exited        time.Time          `json:"exited"`
	Network       mknetwork.Endpoint `json:"network"`
	TaskIdentity  string             `json:"task_identity"`
	StorageSHA256 string             `json:"storage_sha256"`
	Processes     []persistedProcess `json:"processes,omitempty"`
}

type persistedProcess struct {
	ID                string            `json:"id"`
	Stdin             string            `json:"stdin,omitempty"`
	Stdout            string            `json:"stdout,omitempty"`
	Stderr            string            `json:"stderr,omitempty"`
	StdinIdentity     processIOIdentity `json:"stdin_identity,omitempty"`
	StdoutIdentity    processIOIdentity `json:"stdout_identity,omitempty"`
	StderrIdentity    processIOIdentity `json:"stderr_identity,omitempty"`
	Terminal          bool              `json:"terminal,omitempty"`
	Width             uint32            `json:"width,omitempty"`
	Height            uint32            `json:"height,omitempty"`
	SizeSet           bool              `json:"size_set,omitempty"`
	StdinClosed       bool              `json:"stdin_closed,omitempty"`
	StdinCloseAcked   bool              `json:"stdin_close_acked,omitempty"`
	StdinOffset       uint64            `json:"stdin_offset,omitempty"`
	StdinPending      []byte            `json:"stdin_pending,omitempty"`
	Status            tasktypes.Status  `json:"status"`
	PID               uint32            `json:"pid,omitempty"`
	Exit              uint32            `json:"exit,omitempty"`
	Exited            time.Time         `json:"exited,omitempty"`
	StdoutOffset      uint64            `json:"stdout_offset,omitempty"`
	StderrOffset      uint64            `json:"stderr_offset,omitempty"`
	ExitEventQueued   bool              `json:"exit_event_queued,omitempty"`
	DeleteEventQueued bool              `json:"delete_event_queued,omitempty"`
}

type processIOIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Mode   uint32 `json:"mode"`
	Links  uint64 `json:"links"`
}

func processIOIdentityFromInfo(info os.FileInfo) (processIOIdentity, bool) {
	value, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return processIOIdentity{}, false
	}
	return processIOIdentity{Device: uint64(value.Dev), Inode: value.Ino, UID: value.Uid,
		GID: value.Gid, Mode: value.Mode, Links: value.Nlink}, true
}

func validProcessIOIdentity(path string, value processIOIdentity, stdin bool) bool {
	if path == "" {
		return value == (processIOIdentity{})
	}
	typeBits := value.Mode & unix.S_IFMT
	typeOK := typeBits == unix.S_IFIFO || !stdin && typeBits == unix.S_IFREG
	return value.Inode != 0 && value.UID == uint32(os.Geteuid()) && value.Links == 1 &&
		value.Mode&0077 == 0 && typeOK
}

func validatePersistedRecovery(value persisted, namespace, task string) error {
	if value.SchemaVersion != 2 || value.ID != sandboxID(namespace, task) ||
		!recoveryGeneration.MatchString(value.Generation) ||
		value.TaskIdentity != storageTaskIdentity(namespace, task) ||
		(value.StorageSHA256 != "" && !recoverySHA256.MatchString(value.StorageSHA256)) ||
		len(value.Processes) == 0 || len(value.Processes) > 1024 {
		return errors.New("shim recovery identity or bounds are invalid")
	}
	networkIdentityPresent := value.Network.Generation != "" || value.Network.SandboxID != "" || value.Network.SandboxGeneration != ""
	if networkIdentityPresent {
		if value.Network.SandboxID != value.ID || value.Network.SandboxGeneration != value.Generation ||
			!recoveryGeneration.MatchString(value.Network.Generation) {
			return errors.New("shim recovery network ownership is invalid")
		}
		if err := mknetwork.ValidateEndpoint(value.Network); err != nil {
			return fmt.Errorf("shim recovery network endpoint is invalid: %w", err)
		}
	}
	seen := make(map[string]bool, len(value.Processes))
	initSeen := false
	for _, process := range value.Processes {
		if seen[process.ID] || (process.ID != "" && !guestProcessIdentifier.MatchString(process.ID)) {
			return errors.New("shim recovery contains an invalid or duplicate process identity")
		}
		seen[process.ID] = true
		if process.ID == "" {
			if initSeen {
				return errors.New("shim recovery contains multiple init processes")
			}
			initSeen = true
		}
		if process.Status != tasktypes.Status_CREATED && process.Status != tasktypes.Status_RUNNING &&
			process.Status != tasktypes.Status_PAUSED && process.Status != tasktypes.Status_STOPPED {
			return errors.New("shim recovery contains an invalid process state")
		}
		if process.Width > 65535 || process.Height > 65535 || process.StdinCloseAcked &&
			(!process.StdinClosed || len(process.StdinPending) != 0) {
			return errors.New("shim recovery contains invalid terminal or stdin state")
		}
		if len(process.StdinPending) > 32<<10 || len(process.StdinPending) != 0 &&
			(process.Stdin == "" || process.Status != tasktypes.Status_RUNNING && process.Status != tasktypes.Status_PAUSED) {
			return errors.New("shim recovery contains invalid pending stdin")
		}
		if uint64(len(process.StdinPending)) > ^uint64(0)-process.StdinOffset {
			return errors.New("shim recovery pending stdin offset overflows")
		}
		if process.Stdin == "" && process.StdinOffset != 0 {
			return errors.New("shim recovery contains stdin offset without a stream")
		}
		for _, path := range []string{process.Stdin, process.Stdout, process.Stderr} {
			if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 || strings.ContainsRune(path, 0)) {
				return errors.New("shim recovery contains an invalid stdio path")
			}
		}
		if !validProcessIOIdentity(process.Stdin, process.StdinIdentity, true) ||
			!validProcessIOIdentity(process.Stdout, process.StdoutIdentity, false) ||
			!validProcessIOIdentity(process.Stderr, process.StderrIdentity, false) {
			return errors.New("shim recovery contains invalid stdio ownership")
		}
	}
	if !initSeen {
		return errors.New("shim recovery has no init process")
	}
	return nil
}

func loadPersistedRecovery(path, namespace, task string) (persisted, bool, error) {
	var value persisted
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || identity.Uid != uint32(os.Geteuid()) ||
		identity.Nlink != 1 || info.Size() <= 0 || info.Size() > 8<<20 {
		return value, false, errors.New("shim recovery must be a private caller-owned bounded single-link regular file")
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return value, false, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !sameProcessIOIdentity(info, opened) {
		return value, false, errors.New("shim recovery identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return value, false, errors.New("shim recovery changed or exceeded its read bound")
	}
	after, err := file.Stat()
	if err != nil || !sameProcessIOIdentity(opened, after) {
		return value, false, errors.New("shim recovery identity changed while reading")
	}
	if err = protocol.StrictDecode(data, &value); err != nil {
		return value, false, fmt.Errorf("decode shim recovery: %w", err)
	}
	if err = validatePersistedRecovery(value, namespace, task); err != nil {
		return value, false, err
	}
	return value, true, nil
}

func (s *service) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var failures []error
	if address, err := shim.ReadAddress("address"); err == nil {
		if err = shim.RemoveSocket(address); err != nil {
			failures = append(failures, fmt.Errorf("remove shim socket: %w", err))
		}
	} else if _, statErr := os.Lstat("address"); statErr == nil {
		failures = append(failures, fmt.Errorf("read existing shim address: %w", err))
	} else if !errors.Is(statErr, os.ErrNotExist) {
		failures = append(failures, fmt.Errorf("inspect shim address: %w", statErr))
	}
	p, found, err := loadPersistedRecovery(filepath.Join(".multikernel", "sandbox.json"), s.namespace, s.id)
	if err != nil {
		return nil, fmt.Errorf("load shim cleanup ownership: %w", err)
	}
	if !found {
		now := timestamppb.Now()
		return &taskapi.DeleteResponse{ExitedAt: now}, errors.Join(failures...)
	}
	s.netEndpoint = p.Network
	if err = s.stopNetwork(); err != nil {
		failures = append(failures, fmt.Errorf("stop recovered network: %w", err))
	}
	if err = s.stopRelay(); err != nil {
		failures = append(failures, fmt.Errorf("stop recovered relay: %w", err))
	}
	if err = s.releaseNetwork(ctx); err != nil {
		failures = append(failures, fmt.Errorf("release recovered network: %w", err))
	}
	if p.ID != "" {
		if _, stopErr := daemon.Mutation(ctx, s.daemon, "StopSandbox", p.ID, p.Generation, "cleanup-stop-"+p.Generation, nil); stopErr != nil {
			failures = append(failures, fmt.Errorf("stop recovered sandbox: %w", stopErr))
		} else if _, deleteErr := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", p.ID, p.Generation, "cleanup-delete-"+p.Generation, nil); deleteErr != nil {
			failures = append(failures, fmt.Errorf("delete recovered sandbox: %w", deleteErr))
		} else if p.StorageSHA256 != "" {
			if cleanupErr := s.cleanupRootfs(ctx, rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: s.bundle, TaskIdentity: p.TaskIdentity, StorageSHA256: p.StorageSHA256}); cleanupErr != nil {
				failures = append(failures, fmt.Errorf("cleanup recovered rootfs: %w", cleanupErr))
			}
		}
	}
	if p.Exited.IsZero() {
		p.Exited = time.Now().UTC()
	}
	return &taskapi.DeleteResponse{Pid: p.PID, ExitStatus: p.Exit, ExitedAt: timestamppb.New(p.Exited)}, errors.Join(failures...)
}

func randomToken() ([]byte, string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, "", err
	}
	return b, hex.EncodeToString(b), nil
}

func loadExistingToken(runtimeDir string) ([]byte, string, error) {
	path := filepath.Join(runtimeDir, "token")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 65 ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return nil, "", errors.New("existing runtime token is unsafe")
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	opened, statErr := file.Stat()
	if statErr != nil || !sameProcessIOIdentity(info, opened) {
		_ = file.Close()
		return nil, "", errors.New("runtime token identity changed while opening")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 66))
	after, afterErr := file.Stat()
	closeErr := file.Close()
	if err = errors.Join(readErr, afterErr, closeErr); err != nil {
		return nil, "", err
	}
	if !sameProcessIOIdentity(opened, after) {
		return nil, "", errors.New("runtime token identity changed while reading")
	}
	if len(data) != 65 || data[64] != '\n' {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	token, err := hex.DecodeString(string(data[:64]))
	if err != nil || len(token) != 32 {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	return token, string(data[:64]), nil
}

func loadOrCreateToken(runtimeDir string) (_ []byte, _ string, retErr error) {
	if token, encoded, err := loadExistingToken(runtimeDir); err == nil {
		return token, encoded, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	token, encoded, err := randomToken()
	if err != nil {
		return nil, "", err
	}
	directory, err := openStateDirectory(runtimeDir)
	if err != nil {
		return nil, "", err
	}
	defer func() { retErr = errors.Join(retErr, directory.Close()) }()
	descriptor, err := unix.Openat(int(directory.Fd()), "token",
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(descriptor), "token")
	createdInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	createdIdentity, ok := createdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		_ = file.Close()
		return nil, "", errors.New("created runtime token identity is unavailable")
	}
	remove := true
	defer func() {
		if remove {
			var current unix.Stat_t
			if err := unix.Fstatat(int(directory.Fd()), "token", &current, unix.AT_SYMLINK_NOFOLLOW); err == nil &&
				current.Dev == createdIdentity.Dev && current.Ino == createdIdentity.Ino {
				_ = unix.Unlinkat(int(directory.Fd()), "token", 0)
			}
		}
	}()
	written, err := file.WriteString(encoded + "\n")
	if err == nil && written != 65 {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return nil, "", err
	}
	if closeErr != nil {
		return nil, "", closeErr
	}
	err = directory.Sync()
	if err != nil {
		return nil, "", err
	}
	remove = false
	return token, encoded, nil
}

func (s *service) persistRecovery() error {
	if s.sandbox.ID == "" {
		return nil
	}
	network := s.networkReport("READY")
	p := persisted{SchemaVersion: 2, ID: s.sandbox.ID, Generation: s.sandbox.Generation, Network: network,
		TaskIdentity: storageTaskIdentity(s.namespace, s.id)}
	if s.sandbox.Config.Storage != nil {
		p.StorageSHA256 = s.sandbox.Config.Storage.SHA256
	}
	for _, process := range s.processes {
		if process.id == "" {
			p.PID, p.Exit, p.Exited = process.pid, process.exit, process.exited
		}
		p.Processes = append(p.Processes, persistedProcess{
			ID: process.id, Stdin: process.stdin, Stdout: process.stdout, Stderr: process.stderr,
			StdinIdentity: process.stdinIdentity, StdoutIdentity: process.stdoutIdentity, StderrIdentity: process.stderrIdentity,
			Terminal: process.terminal, Width: process.width, Height: process.height,
			SizeSet: process.sizeSet, StdinClosed: process.stdinClosed, StdinCloseAcked: process.stdinCloseAcked,
			StdinOffset: process.stdinOffset, StdinPending: append([]byte(nil), process.stdinPending...), Status: process.status,
			PID: process.pid, Exit: process.exit, Exited: process.exited,
			StdoutOffset: process.stdoutOffset, StderrOffset: process.stderrOffset,
			ExitEventQueued: process.exitEventQueued, DeleteEventQueued: process.deleteEventQueued,
		})
	}
	sort.Slice(p.Processes, func(i, j int) bool { return p.Processes[i].ID < p.Processes[j].ID })
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(s.bundle, ".multikernel", "sandbox.json"), b, 0600)
}

func taskGuestPID(pid int) (uint32, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return 0, errors.New("guest returned a process ID outside the Task v2 range")
	}
	return uint32(pid), nil
}

func runningGuestProcess(state agent.ProcessState, processID string) (uint32, error) {
	if state.ID != processID || state.Status != "RUNNING" {
		return 0, errors.New("guest returned a running process with mismatched identity or state")
	}
	return taskGuestPID(state.PID)
}

func startedGuestProcess(state agent.ProcessState, processID string) (uint32, error) {
	if state.Status == "RUNNING" {
		return runningGuestProcess(state, processID)
	}
	if state.Status == "STOPPED" {
		pid, _, err := stoppedGuestProcess(state, processID, 0)
		return pid, err
	}
	return 0, errors.New("guest returned a started process with mismatched identity or state")
}

func createdGuestProcess(state agent.ProcessState, processID string) error {
	if state.ID != processID || state.Status != "CREATED" || state.PID != 0 || state.ExitCode != 0 {
		return errors.New("guest returned a created process with mismatched identity or state")
	}
	return nil
}

func stoppedGuestProcess(state agent.ProcessState, processID string, expectedPID uint32) (uint32, uint32, error) {
	if state.ID != processID || state.Status != "STOPPED" {
		return 0, 0, errors.New("guest returned a stopped process with mismatched identity or state")
	}
	pid, err := taskGuestPID(state.PID)
	if err != nil {
		return 0, 0, err
	}
	if expectedPID != 0 && pid != expectedPID {
		return 0, 0, errors.New("guest returned a stopped process with a different PID")
	}
	if state.ExitCode < 0 || state.ExitCode > 255 {
		return 0, 0, errors.New("guest returned an exit status outside the agent contract")
	}
	return pid, uint32(state.ExitCode), nil
}

func (s *service) networkReport(state string) mknetwork.Endpoint {
	endpoint := s.netEndpoint
	endpoint.State = state
	endpoint.RXPackets = s.netRXPackets.Load()
	endpoint.TXPackets = s.netTXPackets.Load()
	endpoint.RXDrops = s.netRXDrops.Load()
	endpoint.TXDrops = s.netTXDrops.Load()
	endpoint.Errors = s.netErrors.Load()
	return endpoint
}

func (s *service) reportNetwork(state string) error {
	endpoint := s.networkReport(state)
	if endpoint.Generation == "" {
		return nil
	}
	if err := mknetwork.ValidateEndpoint(endpoint); err != nil {
		return fmt.Errorf("validate network report endpoint: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-report-" + endpoint.Generation[:12], Method: "REPORT", Endpoint: &endpoint}
	_, err := s.netClient.Call(ctx, request)
	return err
}

func (s *service) queueNetworkReport(state string) {
	if s.netReports == nil {
		return
	}
	select {
	case s.netReports <- state:
		return
	default:
	}
	select {
	case <-s.netReports:
	default:
	}
	select {
	case s.netReports <- state:
	default:
	}
}

func (s *service) startNetworkReporter() {
	s.netWG.Add(1)
	go func() {
		defer s.netWG.Done()
		for {
			var state string
			select {
			case <-s.netDone:
				return
			case state = <-s.netReports:
			}
			for {
				if err := s.reportNetwork(state); err == nil {
					break
				} else {
					fmt.Fprintf(os.Stderr, "multikernel network report %s: %v\n", state, err)
				}
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-s.netDone:
					timer.Stop()
					return
				case state = <-s.netReports:
					timer.Stop()
				case <-timer.C:
				}
				select {
				case state = <-s.netReports:
				default:
				}
			}
		}
	}()
}

func openStateDirectory(directory string) (*os.File, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, errors.New("state directory must be canonical and absolute")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("state directory identity is unavailable")
	}
	if !info.IsDir() || info.Mode().Perm()&0002 != 0 || identity.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("state directory must be caller-owned with safe mode (mode=%#o owner=%d caller=%d)",
			info.Mode().Perm(), identity.Uid, os.Geteuid())
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, directory, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(descriptor), directory)
	opened, err := dir.Stat()
	if err != nil || !sameProcessIOIdentity(info, opened) {
		_ = dir.Close()
		return nil, errors.New("state directory identity changed while opening")
	}
	return dir, nil
}

func stateIdentity(info os.FileInfo) (stateFileIdentity, bool) {
	value, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return stateFileIdentity{}, false
	}
	return stateFileIdentity{device: uint64(value.Dev), inode: value.Ino, uid: value.Uid,
		mode: value.Mode, links: value.Nlink}, true
}

func inspectStateFileAt(dir *os.File, base string, mode os.FileMode) (stateFileIdentity, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return stateFileIdentity{}, false, nil
	}
	if err != nil {
		return stateFileIdentity{}, false, err
	}
	identity := stateFileIdentity{device: uint64(stat.Dev), inode: stat.Ino, uid: stat.Uid,
		mode: stat.Mode, links: stat.Nlink}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		os.FileMode(stat.Mode).Perm() != mode.Perm() {
		return stateFileIdentity{}, true, errors.New("state file must be a private caller-owned single-link regular file")
	}
	return identity, true, nil
}

func removeStateFile(path string, expected *stateFileIdentity, mode os.FileMode) (retErr error) {
	dir, err := openStateDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, dir.Close()) }()
	current, found, err := inspectStateFileAt(dir, filepath.Base(path), mode)
	if err != nil {
		return err
	}
	if !found {
		if expected != nil {
			return errors.New("owned state file disappeared before removal")
		}
		return nil
	}
	if expected == nil || current != *expected {
		return errors.New("refusing to remove replaced or unowned state file")
	}
	if err = unix.Unlinkat(int(dir.Fd()), filepath.Base(path), 0); err != nil {
		return err
	}
	return dir.Sync()
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) (retErr error) {
	return atomicWriteFileOwned(path, data, mode, nil, nil)
}

func atomicWriteFileOwned(path string, data []byte, mode os.FileMode, expected, published *stateFileIdentity) (retErr error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || mode.Perm()&0077 != 0 {
		return errors.New("atomic state path and mode must be canonical, absolute, and private")
	}
	directory := filepath.Dir(path)
	dir, err := openStateDirectory(directory)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, dir.Close()) }()
	base := filepath.Base(path)
	current, found, err := inspectStateFileAt(dir, base, mode)
	if err != nil {
		return err
	}
	if published != nil && (expected == nil && found || expected != nil && (!found || current != *expected)) {
		return errors.New("refusing to replace an unowned or changed state file")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, err = io.ReadFull(rand.Reader, random); err != nil {
			return err
		}
		temporaryName = "." + base + "." + hex.EncodeToString(random)
		temporaryDescriptor, openErr := unix.Openat(int(dir.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(openErr, syscall.EEXIST) {
			continue
		}
		if openErr != nil {
			return openErr
		}
		temporary = os.NewFile(uintptr(temporaryDescriptor), temporaryName)
		break
	}
	if temporary == nil {
		return errors.New("could not allocate a unique atomic state temporary file")
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = unix.Unlinkat(int(dir.Fd()), temporaryName, 0)
		}
	}()
	if err = temporary.Chmod(mode); err == nil {
		var written int
		written, err = temporary.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = temporary.Sync()
	}
	var temporaryIdentity stateFileIdentity
	if err == nil {
		var info os.FileInfo
		info, err = temporary.Stat()
		var ok bool
		if err == nil {
			temporaryIdentity, ok = stateIdentity(info)
			if !ok {
				err = errors.New("temporary state identity is unavailable")
			}
		}
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = unix.Renameat(int(dir.Fd()), temporaryName, int(dir.Fd()), base); err != nil {
		return err
	}
	renamed = true
	err = dir.Sync()
	if err == nil && published != nil {
		*published = temporaryIdentity
	}
	return err
}

func (s *service) recoverExisting(ctx context.Context) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtimeDir := filepath.Join(s.bundle, ".multikernel")
	recovery, found, err := loadPersistedRecovery(filepath.Join(runtimeDir, "sandbox.json"), s.namespace, s.id)
	if err != nil {
		return fmt.Errorf("read shim recovery state: %w", err)
	}
	if !found {
		return nil
	}
	var sandboxes []protocol.Sandbox
	if apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "shim-recover-list-" + s.id, Method: "ListSandboxes"}, &sandboxes); apiErr != nil {
		return fmt.Errorf("list sandboxes for shim recovery: %s", apiErr.Message)
	}
	for _, sandbox := range sandboxes {
		if sandbox.ID == recovery.ID && sandbox.Generation == recovery.Generation {
			s.sandbox = sandbox
			break
		}
	}
	if s.sandbox.ID == "" {
		return errors.New("persisted sandbox generation is not owned by mkruntimed")
	}
	s.token, _, err = loadExistingToken(runtimeDir)
	if err != nil {
		return fmt.Errorf("read recovery token: %w", err)
	}
	s.netEndpoint = recovery.Network
	if s.netEndpoint.SandboxID != s.sandbox.ID || s.netEndpoint.SandboxGeneration != s.sandbox.Generation {
		return errors.New("persisted network binding does not match the sandbox generation")
	}
	bound, device, bindErr := s.attachNetwork(ctx, s.netEndpoint.NetNS, s.netEndpoint.Generation)
	if bindErr != nil {
		return fmt.Errorf("recover CNI endpoint binding: %w", bindErr)
	}
	s.netEndpoint = bound
	s.netDevice = device
	recovered := false
	defer func() {
		if recovered {
			return
		}
		var cleanupErrors []error
		for _, process := range s.processes {
			closeProcessIO(process)
		}
		if s.agent != nil {
			cleanupErrors = append(cleanupErrors, s.agent.Close())
			s.agent = nil
		}
		if s.netDevice != nil {
			cleanupErrors = append(cleanupErrors, s.netDevice.Close())
			s.netDevice = nil
		}
		cleanupErrors = append(cleanupErrors, s.stopRelay())
		retErr = errors.Join(retErr, errors.Join(cleanupErrors...))
	}()
	s.netRXPackets.Store(bound.RXPackets)
	s.netTXPackets.Store(bound.TXPackets)
	s.netRXDrops.Store(bound.RXDrops)
	s.netTXDrops.Store(bound.TXDrops)
	s.netErrors.Store(bound.Errors)
	s.relaySocket = s.agentRelaySocketPath(s.sandbox.Config.AgentPort, s.sandbox.Generation)
	if err = removeStaleRelaySocket(s.relaySocket); err != nil {
		s.relaySocket = ""
		return fmt.Errorf("remove stale recovered agent relay socket: %w", err)
	}
	s.relay = s.relayCommand(s.sandbox.Config.AgentPort, s.relaySocket)
	if err = s.relay.Start(); err != nil {
		return fmt.Errorf("restart recovered agent relay: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s.relayOwner == nil {
			s.relayOwner, err = s.captureRelaySocket(s.relaySocket)
			if errors.Is(err, os.ErrNotExist) && time.Now().Before(deadline) && ctx.Err() == nil {
				if err = waitContext(ctx, 50*time.Millisecond); err == nil {
					continue
				}
			}
			if err != nil {
				return fmt.Errorf("capture recovered agent relay socket: %w", err)
			}
		}
		s.agent, err = s.dialAgent(ctx, s.relaySocket)
		if err == nil || time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		if err = waitContext(ctx, 50*time.Millisecond); err != nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("reconnect recovered guest agent: %w", err)
	}
	type recoveredProcess struct {
		agentID string
		process *process
	}
	var running []recoveredProcess
	for _, saved := range recovery.Processes {
		p := &process{
			id: saved.ID, stdin: saved.Stdin, stdout: saved.Stdout, stderr: saved.Stderr,
			stdinIdentity: saved.StdinIdentity, stdoutIdentity: saved.StdoutIdentity, stderrIdentity: saved.StderrIdentity,
			terminal: saved.Terminal, width: saved.Width, height: saved.Height,
			sizeSet: saved.SizeSet, stdinClosed: saved.StdinClosed, stdinCloseAcked: saved.StdinCloseAcked,
			stdinOffset: saved.StdinOffset, stdinPending: append([]byte(nil), saved.StdinPending...), status: saved.Status,
			pid: saved.PID, exit: saved.Exit, exited: saved.Exited,
			stdoutOffset: saved.StdoutOffset, stderrOffset: saved.StderrOffset,
			exitEventQueued: saved.ExitEventQueued, deleteEventQueued: saved.DeleteEventQueued, done: make(chan struct{}),
		}
		s.processes[saved.ID] = p
		if p.status == tasktypes.Status_STOPPED {
			if !p.exitEventQueued {
				if err = s.publishExit(ctx, p.id, p); err != nil {
					return fmt.Errorf("recover process %q exit event: %w", saved.ID, err)
				}
				p.exitEventQueued = true
			}
			close(p.done)
			continue
		}
		if p.status == tasktypes.Status_CREATED && p.id != "" {
			var exists bool
			exists, err = s.reconcileRecoveredCreatedExec(ctx, p)
			if err != nil {
				return err
			}
			if !exists {
				delete(s.processes, p.id)
			}
			continue
		}
		if p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED {
			continue
		}
		agentID := p.id
		if agentID == "" {
			agentID = "init"
		}
		var state agent.ProcessState
		if err = s.agent.CallContext(ctx, "StateProcess", map[string]string{"ID": agentID}, &state); err != nil {
			return fmt.Errorf("recover process %q: %w", saved.ID, err)
		}
		if state.ID != agentID {
			return fmt.Errorf("recover process %q: guest returned a mismatched process identity", saved.ID)
		}
		if state.Status == "STOPPED" {
			if err = s.agent.CallContext(ctx, "WaitProcess", map[string]string{"ID": agentID}, &state); err != nil {
				return fmt.Errorf("recover stopped process %q wait state: %w", saved.ID, err)
			}
			var pid, exit uint32
			pid, exit, err = stoppedGuestProcess(state, agentID, p.pid)
			if err != nil {
				return fmt.Errorf("recover stopped process %q wait state: %w", saved.ID, err)
			}
			p.status, p.pid, p.exit, p.stdinPending = tasktypes.Status_STOPPED, pid, exit, nil
			p.exited = time.Now().UTC()
			if err = s.publishExit(ctx, p.id, p); err != nil {
				return fmt.Errorf("recover stopped process %q exit event: %w", saved.ID, err)
			}
			p.exitEventQueued = true
			close(p.done)
			continue
		}
		guestPID, pidErr := runningGuestProcess(state, agentID)
		if pidErr != nil {
			return fmt.Errorf("recover process %q: %w", saved.ID, pidErr)
		}
		p.pid = guestPID
		if p.terminal && p.sizeSet {
			if err = s.agent.CallContext(ctx, "ResizeProcess", map[string]any{"id": agentID, "width": p.width, "height": p.height}, nil); err != nil {
				return fmt.Errorf("recover process %q terminal size: %w", saved.ID, err)
			}
		}
		if err = s.openProcessIO(ctx, p); err != nil {
			return fmt.Errorf("recover process %q I/O: %w", saved.ID, err)
		}
		running = append(running, recoveredProcess{agentID: agentID, process: p})
	}
	if s.netEndpoint.Generation != "" {
		s.startNetworkPump()
	}
	for _, item := range running {
		go s.pumpStdin(item.agentID, item.process)
		go s.waitProcess(item.agentID, item.process.id, item.process)
	}
	if err = s.persistRecovery(); err != nil {
		return fmt.Errorf("persist reconstructed process state: %w", err)
	}
	recovered = true
	return nil
}

func (s *service) reconcileRecoveredCreatedExec(ctx context.Context, p *process) (bool, error) {
	var state agent.ProcessState
	err := s.callAgentWithReconnectContext(ctx, "StateProcess", map[string]string{"ID": p.id}, &state)
	if agentNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("recover created exec process %q: %w", p.id, err)
	}
	if err = createdGuestProcess(state, p.id); err != nil {
		return false, fmt.Errorf("recover created exec process %q: %w", p.id, err)
	}
	if err = s.publish(ctx, ctruntime.TaskExecAddedEventTopic, &eventstypes.TaskExecAdded{ContainerID: s.id, ExecID: p.id}); err != nil {
		return false, fmt.Errorf("recover created exec process %q event: %w", p.id, err)
	}
	return true, nil
}

func relaySocketPath(port uint32, generation string) string {
	if len(generation) > 12 {
		generation = generation[:12]
	}
	return filepath.Join("/run", "mk-agent-"+strconv.Itoa(int(port))+"-"+generation+".sock")
}

func newRelayCommand(binary string, port uint32, socket string) *exec.Cmd {
	command := exec.Command(binary, "server", strconv.Itoa(int(port)), socket)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return command
}

func removeStaleRelaySocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0022 != 0 ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return errors.New("stale relay path is not a caller-owned single-link socket with safe mode")
	}
	return os.Remove(path)
}

func terminateRelay(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	waitErr := command.Wait()
	var exitErr *exec.ExitError
	if waitErr == nil || errors.As(waitErr, &exitErr) {
		return nil
	}
	return waitErr
}

func (s *service) stopRelay() error {
	if s.relay != nil {
		if err := terminateRelay(s.relay); err != nil {
			return err
		}
		s.relay = nil
	}
	if s.relaySocket != "" {
		var err error
		if s.relayOwner != nil {
			err = s.relayOwner.Remove()
		} else {
			err = removeStaleRelaySocket(s.relaySocket)
		}
		if err != nil {
			return err
		}
		s.relayOwner = nil
		s.relaySocket = ""
	}
	return nil
}

// sandboxID maps the containerd namespace/task tuple into the daemon's global
// identifier space. The readable prefix is diagnostic only; the digest binds
// the complete, unsanitized tuple so truncation and punctuation cannot alias.
func sandboxID(namespace, id string) string {
	var b strings.Builder
	b.WriteString("mk-")
	for _, r := range strings.ToLower(id) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	prefix := strings.Trim(strings.TrimPrefix(b.String(), "mk-"), "-")
	if prefix == "" {
		prefix = "task"
	}
	if len(prefix) > 32 {
		prefix = prefix[:32]
	}
	digest := sha256.Sum256([]byte(namespace + "\x00" + id))
	return "mk-" + prefix + "-" + hex.EncodeToString(digest[:8])
}

func storageTaskIdentity(namespace, id string) string {
	digest := sha256.Sum256([]byte(namespace + "\x00" + id))
	return "task-" + hex.EncodeToString(digest[:16])
}

func parseCPUSet(s string) ([][]int, error) {
	var sets [][]int
	for _, group := range strings.Split(s, ";") {
		var set []int
		for _, raw := range strings.Split(group, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				return nil, err
			}
			set = append(set, n)
		}
		if len(set) > 0 {
			sets = append(sets, set)
		}
	}
	return sets, nil
}

func (s *service) allocate(ctx context.Context, bundle string) (protocol.SandboxConfig, *os.File, error) {
	lock, err := os.OpenFile(getenv("MK_SHIM_LOCK", "/run/multikernel-shim.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return protocol.SandboxConfig{}, nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, err
	}
	var existing []protocol.Sandbox
	req := protocol.Request{Version: 1, RequestID: "list-" + s.id, Method: "ListSandboxes"}
	if apiErr := s.daemon.Call(ctx, req, &existing); apiErr != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, errors.New(apiErr.Message)
	}
	sets, err := parseCPUSet(getenv("MK_SANDBOX_CPU_SETS", "8,10;12,14;9,11;13,15"))
	if err != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, err
	}
	used := map[int]bool{}
	for _, sb := range existing {
		for _, cpu := range sb.Config.CPUs {
			used[cpu] = true
		}
	}
	for i, set := range sets {
		free := true
		for _, cpu := range set {
			if used[cpu] {
				free = false
			}
		}
		if free {
			return protocol.SandboxConfig{SchemaVersion: 1, ID: sandboxID(s.namespace, s.id), CPUs: set, MemoryBytes: 3 << 30, KernelManifest: "gce-mk2", Bundle: bundle, AgentPort: uint32(7200 + i), ChildCID: uint32(40 + i), Storage: &protocol.StorageConfig{Port: uint32(4061 + i)}}, lock, nil
		}
	}
	lock.Close()
	return protocol.SandboxConfig{}, nil, errors.New("no disjoint Multikernel CPU set is available")
}

func (s *service) rollbackCreate(ctx context.Context, prepared *rootfspkg.CleanupRequest, lifecycleAttempted bool) error {
	var failures []error
	canCleanupPrepared := !lifecycleAttempted
	if err := s.releaseNetwork(ctx); err != nil {
		failures = append(failures, err)
	}
	if s.sandbox.ID != "" {
		if _, err := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-create-rollback-delete-"+s.sandbox.Generation, nil); err != nil {
			failures = append(failures, fmt.Errorf("delete allocated sandbox: %w", err))
		} else {
			canCleanupPrepared = true
			s.sandbox = protocol.Sandbox{}
		}
	}
	if prepared != nil && canCleanupPrepared {
		if err := s.cleanupRootfs(ctx, *prepared); err != nil {
			failures = append(failures, fmt.Errorf("cleanup prepared rootfs: %w", err))
		}
	}
	s.token = nil
	delete(s.processes, "")
	return errors.Join(failures...)
}

func bundleNetworkNamespace(bundle string) (string, error) {
	path := filepath.Join(bundle, "config.json")
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 1<<20 ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return "", errors.New("OCI config.json must be a bounded private caller-owned single-link regular file")
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !sameProcessIOIdentity(info, opened) {
		return "", errors.New("OCI config.json identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return "", errors.New("OCI config.json changed or exceeded its read bound")
	}
	after, err := file.Stat()
	if err != nil || !sameProcessIOIdentity(opened, after) {
		return "", errors.New("OCI config.json identity changed while reading")
	}
	var spec specs.Spec
	if err = protocol.StrictDecode(data, &spec); err != nil {
		return "", fmt.Errorf("decode OCI config for network ownership: %w", err)
	}
	if spec.Linux == nil {
		return "", errors.New("OCI Linux configuration is required")
	}
	var result string
	count := 0
	for _, namespace := range spec.Linux.Namespaces {
		if namespace.Type != specs.NetworkNamespace {
			continue
		}
		count++
		if count > 1 {
			return "", errors.New("OCI config contains multiple network namespaces")
		}
		result = namespace.Path
	}
	if count != 1 {
		return "", errors.New("OCI config must contain exactly one network namespace")
	}
	if result != "" && (!filepath.IsAbs(result) || filepath.Clean(result) != result) {
		return "", errors.New("OCI network namespace path must be absolute and canonical")
	}
	return result, nil
}

func (s *service) provisionNetwork(ctx context.Context, netns string) error {
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-provision-" + s.sandbox.Generation[:16], Method: "PROVISION", Endpoint: &mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}}
	response, err := s.netClient.Call(ctx, request)
	if err != nil {
		return err
	}
	if response.Endpoint == nil {
		return errors.New("mknetd PROVISION returned no endpoint")
	}
	if err = validateShimNetworkEndpoint(*response.Endpoint, mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}); err != nil {
		return fmt.Errorf("validate mknetd PROVISION endpoint: %w", err)
	}
	s.netEndpoint = *response.Endpoint
	return nil
}

func validateShimNetworkEndpoint(endpoint, expected mknetwork.Endpoint) error {
	if err := mknetwork.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	if endpoint.ContainerID != expected.ContainerID || endpoint.NetworkName != expected.NetworkName ||
		endpoint.IfName != expected.IfName || endpoint.SandboxID != expected.SandboxID ||
		endpoint.SandboxGeneration != expected.SandboxGeneration ||
		expected.NetNS != "" && endpoint.NetNS != expected.NetNS ||
		expected.Generation != "" && endpoint.Generation != expected.Generation {
		return errors.New("network endpoint differs from the requested ownership identity")
	}
	if expected.NetNS == "" && (endpoint.Owner != "runtime" || !endpoint.ManagedNamespace) {
		return errors.New("network endpoint differs from the requested managed-namespace ownership")
	}
	return nil
}

func (s *service) attachNetwork(ctx context.Context, netns, endpointGeneration string) (mknetwork.Endpoint, *os.File, error) {
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-attach-" + s.sandbox.Generation[:16], Method: "ATTACH", Endpoint: &mknetwork.Endpoint{
		NetNS: netns, Generation: endpointGeneration, SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}}
	response, descriptor, err := s.netClient.Attach(ctx, request)
	if err != nil {
		return mknetwork.Endpoint{}, nil, err
	}
	if response.Endpoint == nil || descriptor == nil {
		if descriptor != nil {
			_ = descriptor.Close()
		}
		return mknetwork.Endpoint{}, nil, errors.New("mknetd ATTACH returned no endpoint descriptor")
	}
	if err = validateShimNetworkEndpoint(*response.Endpoint, mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		Generation: endpointGeneration, SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}); err != nil {
		_ = descriptor.Close()
		return mknetwork.Endpoint{}, nil, fmt.Errorf("validate mknetd ATTACH endpoint: %w", err)
	}
	return *response.Endpoint, descriptor, nil
}

func (s *service) releaseNetwork(ctx context.Context) error {
	if s.netEndpoint.SandboxID == "" {
		return nil
	}
	if err := mknetwork.ValidateEndpoint(s.netEndpoint); err != nil {
		return fmt.Errorf("validate network release endpoint: %w", err)
	}
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-release-" + s.netEndpoint.SandboxGeneration[:16], Method: "RELEASE", Endpoint: &mknetwork.Endpoint{
		SandboxID: s.netEndpoint.SandboxID, SandboxGeneration: s.netEndpoint.SandboxGeneration,
	}}
	_, err := s.netClient.Call(ctx, request)
	if err == nil {
		s.netEndpoint = mknetwork.Endpoint{}
	}
	return err
}

func rootfsMounts(input []*types.Mount) ([]rootfspkg.Mount, error) {
	if len(input) > 8 {
		return nil, fmt.Errorf("%w: at most eight rootfs mounts are supported", errdefs.ErrInvalidArgument)
	}
	result := make([]rootfspkg.Mount, len(input))
	for index, item := range input {
		if item == nil {
			return nil, fmt.Errorf("%w: nil rootfs mount", errdefs.ErrInvalidArgument)
		}
		if item.Type != "overlay" && item.Type != "bind" && item.Type != "none" {
			return nil, fmt.Errorf("%w: unsupported rootfs mount type %q", errdefs.ErrNotImplemented, item.Type)
		}
		if (item.Type == "bind" || item.Type == "none") && !filepath.IsAbs(item.Source) {
			return nil, fmt.Errorf("%w: bind rootfs source must be absolute", errdefs.ErrInvalidArgument)
		}
		options := make([]string, 0, len(item.Options))
		for _, option := range item.Options {
			if option == "rw" {
				continue
			}
			if strings.ContainsAny(option, "\x00\n\r") {
				return nil, fmt.Errorf("%w: unsafe rootfs mount option", errdefs.ErrInvalidArgument)
			}
			options = append(options, option)
		}
		result[index] = rootfspkg.Mount{Type: item.Type, Source: item.Source, Options: options}
	}
	if err := rootfspkg.ValidateMounts(result); err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrInvalidArgument, err)
	}
	return result, nil
}

func (s *service) prepareRootfs(ctx context.Context, request rootfspkg.PrepareRequest) (rootfspkg.PrepareResult, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return rootfspkg.PrepareResult{}, err
	}
	var result rootfspkg.PrepareResult
	apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "prepare-rootfs-" + request.TaskIdentity, Method: "PrepareRootfs", Body: body}, &result)
	if apiErr != nil {
		return result, errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return result, nil
}

func (s *service) cleanupRootfs(ctx context.Context, request rootfspkg.CleanupRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "cleanup-rootfs-" + request.TaskIdentity, Method: "CleanupRootfs", Body: body}, nil)
	if apiErr != nil {
		return errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return nil
}

func (s *service) cancelCreate(ctx context.Context, config protocol.SandboxConfig, key string) error {
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	var result struct {
		SafeToCleanup bool `json:"safe_to_cleanup"`
	}
	apiErr := s.daemon.Call(ctx, protocol.Request{
		Version: 1, RequestID: "cancel-" + key, Method: "CancelCreateSandbox",
		IdempotencyKey: key, Body: body,
	}, &result)
	if apiErr != nil {
		return errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	if !result.SafeToCleanup {
		return errors.New("create cancellation did not prove cleanup safety")
	}
	return nil
}

func (s *service) validateTaskRequest(id string) error {
	if id != s.id {
		return fmt.Errorf("%w: request task ID does not match shim task", errdefs.ErrInvalidArgument)
	}
	return nil
}

func (s *service) Create(ctx context.Context, r *taskapi.CreateTaskRequest) (_ *taskapi.CreateTaskResponse, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil create request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil || r.Bundle != s.bundle {
		return nil, fmt.Errorf("%w: invalid task", errdefs.ErrInvalidArgument)
	}
	stdinIdentity, stdoutIdentity, stderrIdentity, err := inspectBoundProcessIOPaths(r.Stdin, r.Stdout, r.Stderr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrInvalidArgument, err)
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	if s.shuttingDown {
		return nil, errdefs.ErrFailedPrecondition
	}
	if len(s.processes) != 0 {
		return nil, errdefs.ErrAlreadyExists
	}
	netns, err := bundleNetworkNamespace(r.Bundle)
	if err != nil {
		return nil, err
	}
	mounts, err := rootfsMounts(r.Rootfs)
	if err != nil {
		return nil, err
	}
	runtimeDir := filepath.Join(r.Bundle, ".multikernel")
	var prepared *rootfspkg.CleanupRequest
	lifecycleAttempted := false
	fail := true
	defer func() {
		if fail {
			if cleanupErr := s.rollbackCreate(context.WithoutCancel(ctx), prepared, lifecycleAttempted); cleanupErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("create rollback: %w", cleanupErr))
			}
		}
	}()
	config, lock, err := s.allocate(ctx, r.Bundle)
	if err != nil {
		return nil, err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); lock.Close() }()
	identity := storageTaskIdentity(s.namespace, s.id)
	result, err := s.prepareRootfs(ctx, rootfspkg.PrepareRequest{Version: rootfspkg.Version, Bundle: r.Bundle,
		TaskIdentity: identity, StoragePort: config.Storage.Port, Mounts: mounts})
	if err != nil {
		return nil, err
	}
	config.Storage = &result.Storage
	prepared = &rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: r.Bundle, TaskIdentity: identity, StorageSHA256: result.Storage.SHA256}
	token, tokenHex, err := loadOrCreateToken(runtimeDir)
	if err != nil {
		return nil, err
	}
	s.token = token
	lifecycleAttempted = true
	createKey := "shim-create-" + s.id + "-" + tokenHex[:12]
	created, err := daemon.Mutation(ctx, s.daemon, "CreateSandbox", "", "", createKey, &config)
	if err != nil {
		cancelContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		cancelErr := s.cancelCreate(cancelContext, config, createKey)
		cancel()
		if cancelErr == nil {
			lifecycleAttempted = false
		}
		return nil, errors.Join(err, cancelErr)
	}
	s.sandbox = created.Sandbox
	if _, err = daemon.Mutation(ctx, s.daemon, "LoadSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-load-"+s.sandbox.Generation, nil); err != nil {
		return nil, err
	}
	if err = s.provisionNetwork(ctx, netns); err != nil {
		return nil, fmt.Errorf("provision primary network endpoint: %w", err)
	}
	s.bundle = r.Bundle
	s.processes[""] = &process{id: "", stdin: r.Stdin, stdout: r.Stdout, stderr: r.Stderr,
		stdinIdentity: stdinIdentity, stdoutIdentity: stdoutIdentity, stderrIdentity: stderrIdentity,
		terminal: r.Terminal, status: tasktypes.Status_CREATED, done: make(chan struct{})}
	if err = s.persistRecovery(); err != nil {
		return nil, fmt.Errorf("persist recovery state: %w", err)
	}
	pid := uint32(os.Getpid())
	if err = s.publish(ctx, ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: s.id, Bundle: r.Bundle, Rootfs: r.Rootfs, Pid: pid}); err != nil {
		return nil, err
	}
	fail = false
	return &taskapi.CreateTaskResponse{Pid: pid}, nil
}

func (s *service) connectAgent(ctx context.Context) error {
	// Docker uses 64-byte container IDs and deeply nested runtime bundles;
	// placing the relay socket in the bundle can exceed sockaddr_un.sun_path.
	// Use a generation-qualified short path under /run instead.
	sock := s.agentRelaySocketPath(s.sandbox.Config.AgentPort, s.sandbox.Generation)
	s.relaySocket = sock
	if err := removeStaleRelaySocket(sock); err != nil {
		s.relaySocket = ""
		return fmt.Errorf("remove stale agent relay socket: %w", err)
	}
	if s.netEndpoint.Generation == "" {
		return errors.New("CNI endpoint is not bound")
	}
	endpoint, device, err := s.attachNetwork(ctx, s.netEndpoint.NetNS, s.netEndpoint.Generation)
	if err != nil {
		return err
	}
	s.netEndpoint, s.netDevice = endpoint, device
	if err := s.persistRecovery(); err != nil {
		return errors.Join(fmt.Errorf("persist network recovery state: %w", err), s.stopNetwork(), s.stopRelay())
	}
	s.relay = s.relayCommand(s.sandbox.Config.AgentPort, sock)
	if err := s.relay.Start(); err != nil {
		return errors.Join(err, s.stopNetwork(), s.stopRelay())
	}
	if _, err := daemon.Mutation(ctx, s.daemon, "StartSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-start-"+s.sandbox.Generation, nil); err != nil {
		return errors.Join(err, s.stopNetwork(), s.stopRelay())
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if s.relayOwner == nil {
			s.relayOwner, err = s.captureRelaySocket(sock)
			if errors.Is(err, os.ErrNotExist) {
				if err = waitContext(ctx, 100*time.Millisecond); err != nil {
					return errors.Join(err, s.stopNetwork(), s.stopRelay())
				}
				continue
			}
			if err != nil {
				return errors.Join(fmt.Errorf("capture agent relay socket: %w", err), s.stopNetwork(), s.stopRelay())
			}
		}
		client, err := s.dialAgent(ctx, sock)
		if err == nil {
			s.agent = client
			if err = s.agent.CallContext(ctx, "ConfigureNetwork", agent.NetworkConfig{
				Name: "mkn0", Address: s.netEndpoint.Address, Gateway: s.netEndpoint.Gateway,
				MTU: s.netEndpoint.MTU, Nameservers: s.netEndpoint.DNS.Nameservers,
			}, nil); err != nil {
				_ = s.agent.Close()
				s.agent = nil
				return errors.Join(fmt.Errorf("configure child network: %w", err), s.stopNetwork(), s.stopRelay())
			}
			s.startNetworkPump()
			return nil
		}
		if err := waitContext(ctx, 100*time.Millisecond); err != nil {
			return errors.Join(err, s.stopNetwork(), s.stopRelay())
		}
	}
	return errors.Join(errors.New("timed out connecting to child agent"), s.stopNetwork(), s.stopRelay())
}

func (s *service) startNetworkPump() {
	s.netDone = make(chan struct{})
	s.netReports = make(chan string, 1)
	s.startNetworkReporter()
	s.queueNetworkReport("READY")
	s.netWG.Add(1)
	go func() {
		defer s.netWG.Done()
		buffer := make([]byte, s.netEndpoint.MTU+1)
		lastReported := s.netRXPackets.Load() + s.netTXPackets.Load()
		for {
			select {
			case <-s.netDone:
				return
			default:
			}
			var packet []byte
			n, err := unix.Read(int(s.netDevice.Fd()), buffer)
			if err == nil && n > 0 {
				if n > s.netEndpoint.MTU {
					s.netRXDrops.Add(1)
					continue
				}
				packet = append([]byte(nil), buffer[:n]...)
			} else if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				s.netErrors.Add(1)
				s.queueNetworkReport("DEGRADED")
				fmt.Fprintf(os.Stderr, "multikernel network: host TUN read: %v\n", err)
				return
			}
			var response struct {
				Packet []byte `json:"packet"`
			}
			exchangeCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			err = s.agent.CallContext(exchangeCtx, "ExchangeNetwork", map[string][]byte{"packet": packet}, &response)
			cancel()
			if err != nil {
				s.netErrors.Add(1)
				if len(packet) > 0 {
					s.netRXDrops.Add(1)
				}
				s.queueNetworkReport("DISCONNECTED")
				for {
					select {
					case <-s.netDone:
						return
					case <-time.After(100 * time.Millisecond):
					}
					reconnectCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
					reconnectErr := s.agent.ReconnectContext(reconnectCtx, s.relaySocket)
					cancel()
					if reconnectErr == nil {
						s.queueNetworkReport("READY")
						break
					}
				}
				continue
			}
			if len(packet) > 0 {
				s.netRXPackets.Add(1)
			}
			if len(response.Packet) > 0 {
				if len(response.Packet) > s.netEndpoint.MTU {
					s.netTXDrops.Add(1)
					continue
				}
				if _, err = unix.Write(int(s.netDevice.Fd()), response.Packet); err == nil {
					s.netTXPackets.Add(1)
				} else if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					s.netTXDrops.Add(1)
				} else {
					s.netErrors.Add(1)
					s.queueNetworkReport("DEGRADED")
					fmt.Fprintf(os.Stderr, "multikernel network: host TUN write: %v\n", err)
					return
				}
			}
			total := s.netRXPackets.Load() + s.netTXPackets.Load()
			if total-lastReported >= 256 {
				s.queueNetworkReport("READY")
				lastReported = total
			}
			if len(packet) == 0 && len(response.Packet) == 0 {
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
}

func (s *service) stopNetwork() error {
	var failures []error
	if s.netDone != nil {
		close(s.netDone)
		s.netWG.Wait()
		s.netDone = nil
		s.netReports = nil
	}
	if s.agent != nil {
		timeout := s.networkCloseTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), timeout)
		err := s.callAgentWithReconnectContext(closeCtx, "CloseNetwork", map[string]any{}, nil)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("close guest network: %w", err))
		}
	}
	if s.netDevice != nil {
		if err := s.netDevice.Close(); err != nil {
			failures = append(failures, fmt.Errorf("close TUN: %w", err))
		}
		s.netDevice = nil
	}
	if err := s.reportNetwork("READY"); err != nil {
		failures = append(failures, fmt.Errorf("persist network counters: %w", err))
	}
	return errors.Join(failures...)
}

func (s *service) Start(ctx context.Context, r *taskapi.StartRequest) (*taskapi.StartResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil start request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_CREATED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if r.ExecID != "" {
		init, exists := s.processes[""]
		if !exists || init.status != tasktypes.Status_RUNNING {
			s.mu.Unlock()
			return nil, errdefs.ErrFailedPrecondition
		}
	}
	if r.ExecID == "" {
		if err := s.connectAgent(ctx); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if err := ensureGuestProcessCreated(ctx, s.agent, "init", "/bundle"); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	processID := r.ExecID
	if processID == "" {
		processID = "init"
	}
	startRequest := map[string]any{"id": processID}
	if p.terminal && p.sizeSet {
		startRequest["width"] = p.width
		startRequest["height"] = p.height
		startRequest["initial_size"] = true
	}
	if err := s.openProcessIO(ctx, p); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	startErr := s.agent.CallContext(ctx, "StartProcess", startRequest, nil)
	var guestState agent.ProcessState
	stateCtx := ctx
	var cancelState context.CancelFunc
	if startErr != nil {
		stateCtx, cancelState = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	stateErr := s.agent.CallContext(stateCtx, "StateProcess", map[string]string{"ID": processID}, &guestState)
	if cancelState != nil {
		cancelState()
	}
	if startErr != nil && stateErr == nil && createdGuestProcess(guestState, processID) == nil {
		closeProcessIO(p)
		s.mu.Unlock()
		return nil, startErr
	}
	guestPID, pidErr := startedGuestProcess(guestState, processID)
	if startErr != nil && (stateErr != nil || pidErr != nil) {
		stateErr = errors.Join(startErr, fmt.Errorf("reconcile failed guest start: %w", errors.Join(stateErr, pidErr)))
	}
	if stateErr != nil || pidErr != nil {
		p.status = tasktypes.Status_RUNNING
		go s.pumpStdin(processID, p)
		go s.waitProcess(processID, r.ExecID, p)
		persistErr := s.persistRecovery()
		killErr := s.killStartedProcess(ctx, processID)
		if stateErr == nil {
			stateErr = pidErr
		}
		s.mu.Unlock()
		if persistErr != nil {
			persistErr = fmt.Errorf("persist unverified started process ownership: %w", persistErr)
		}
		return nil, errors.Join(fmt.Errorf("read started guest process identity: %w", stateErr), persistErr, killErr)
	}
	p.status = tasktypes.Status_RUNNING
	p.pid = guestPID
	pid := p.pid
	go s.pumpStdin(processID, p)
	go s.waitProcess(processID, r.ExecID, p)
	if err := s.persistRecovery(); err != nil {
		killErr := s.killStartedProcess(ctx, processID)
		s.mu.Unlock()
		return nil, errors.Join(fmt.Errorf("persist started process: %w", err), killErr)
	}
	var topic string
	var event any
	if r.ExecID == "" {
		topic, event = ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: s.id, Pid: pid}
	} else {
		topic, event = ctruntime.TaskExecStartedEventTopic, &eventstypes.TaskExecStarted{ContainerID: s.id, ExecID: r.ExecID, Pid: pid}
	}
	if err := s.publish(ctx, topic, event); err != nil {
		killErr := s.killStartedProcess(ctx, processID)
		s.mu.Unlock()
		return nil, errors.Join(err, killErr)
	}
	s.mu.Unlock()
	return &taskapi.StartResponse{Pid: pid}, nil
}

func (s *service) killStartedProcess(ctx context.Context, processID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := s.agent.CallContext(cleanupCtx, "SignalProcess", map[string]any{
		"ID": processID, "Signal": strconv.Itoa(int(syscall.SIGKILL)),
	}, nil)
	if agentNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kill started guest process: %w", err)
	}
	return nil
}

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

func inspectBoundProcessIOPaths(stdin, stdout, stderr string) (processIOIdentity, processIOIdentity, processIOIdentity, error) {
	stdinIdentity, err := inspectBoundProcessIOPath(stdin, true)
	if err != nil {
		return processIOIdentity{}, processIOIdentity{}, processIOIdentity{}, fmt.Errorf("inspect stdin: %w", err)
	}
	stdoutIdentity, err := inspectBoundProcessIOPath(stdout, false)
	if err != nil {
		return processIOIdentity{}, processIOIdentity{}, processIOIdentity{}, fmt.Errorf("inspect stdout: %w", err)
	}
	stderrIdentity, err := inspectBoundProcessIOPath(stderr, false)
	if err != nil {
		return processIOIdentity{}, processIOIdentity{}, processIOIdentity{}, fmt.Errorf("inspect stderr: %w", err)
	}
	return stdinIdentity, stdoutIdentity, stderrIdentity, nil
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
	w, err := openKnownProcessIOPath(ctx, path, unix.O_WRONLY|unix.O_NONBLOCK, expected)
	if err != nil {
		guard.Close()
		return nil, nil, err
	}
	// A later containerd attach can reopen the same FIFO and consume buffered
	// and future guest output; this guard never reads from the FIFO.
	return w, guard, nil
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

func (s *service) pumpStdin(agentID string, p *process) {
	reader := p.stdinReader
	if reader != nil {
		defer func() {
			s.mu.Lock()
			_ = reader.Close()
			p.stdinReader = nil
			pending := len(p.stdinPending) != 0
			s.mu.Unlock()
			if !pending {
				s.retryPendingStdinClose(agentID, p)
			}
		}()
	}
	if err := s.deliverPendingStdin(agentID, p); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel stdin: %v\n", err)
		return
	}
	if reader == nil {
		s.retryPendingStdinClose(agentID, p)
		return
	}
	buffer := make([]byte, 32<<10)
	for {
		s.mu.Lock()
		closeRequested := p.stdinClosed
		closeAcknowledged := p.stdinCloseAcked
		s.mu.Unlock()
		if closeRequested {
			if !closeAcknowledged {
				s.retryPendingStdinClose(agentID, p)
			}
			return
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			s.mu.Lock()
			stopped := p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED
			if !stopped {
				p.stdinPending = append(p.stdinPending[:0], buffer[:n]...)
				if persistErr := s.persistRecovery(); persistErr != nil {
					stopped = true
					fmt.Fprintf(os.Stderr, "multikernel stdin intent: %v\n", persistErr)
				}
			}
			s.mu.Unlock()
			if stopped {
				return
			}
			if callErr := s.deliverPendingStdin(agentID, p); callErr != nil {
				fmt.Fprintf(os.Stderr, "multikernel stdin: %v\n", callErr)
				return
			}
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return
		}
		if n == 0 {
			s.mu.Lock()
			closeRequested := p.stdinClosed
			closeAcknowledged := p.stdinCloseAcked
			s.mu.Unlock()
			if closeRequested && !closeAcknowledged {
				s.retryPendingStdinClose(agentID, p)
				return
			}
			// FIFO EOF can also mean that an attaching client disconnected.
			// Keep the guest side open until CloseIO explicitly requests EOF.
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *service) deliverPendingStdin(agentID string, p *process) error {
	s.mu.Lock()
	offset := p.stdinOffset
	data := append([]byte(nil), p.stdinPending...)
	s.mu.Unlock()
	if len(data) == 0 {
		return nil
	}
	var response struct {
		Offset uint64 `json:"offset"`
	}
	if err := s.callAgentWithReconnect("WriteProcess", map[string]any{
		"id": agentID, "offset": offset, "data": data,
	}, &response); err != nil {
		return err
	}
	if uint64(len(data)) > ^uint64(0)-offset || response.Offset != offset+uint64(len(data)) {
		return errors.New("guest acknowledged an invalid stdin offset")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.stdinOffset != offset || !bytes.Equal(p.stdinPending, data) {
		return errors.New("pending stdin identity changed during guest acknowledgement")
	}
	p.stdinOffset, p.stdinPending = response.Offset, nil
	if err := s.persistRecovery(); err != nil {
		p.stdinOffset, p.stdinPending = offset, data
		return fmt.Errorf("persist acknowledged stdin offset: %w", err)
	}
	return nil
}

func (s *service) retryPendingStdinClose(agentID string, p *process) {
	for {
		s.mu.Lock()
		requested := p.stdinClosed && !p.stdinCloseAcked
		stopped := p.status == tasktypes.Status_STOPPED
		s.mu.Unlock()
		if !requested || stopped {
			return
		}
		if err := s.acknowledgeStdinClose(context.Background(), agentID, p); err == nil {
			return
		} else {
			fmt.Fprintf(os.Stderr, "multikernel close stdin: %v\n", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *service) acknowledgeStdinClose(ctx context.Context, agentID string, p *process) error {
	s.mu.Lock()
	if p.stdinCloseAcked {
		s.mu.Unlock()
		return nil
	}
	connected := s.agent != nil
	s.mu.Unlock()
	if !connected {
		return errdefs.ErrFailedPrecondition
	}
	if err := s.callAgentWithReconnectContext(ctx, "CloseProcessStdin", map[string]string{"id": agentID}, nil); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.stdinCloseAcked {
		return nil
	}
	p.stdinCloseAcked = true
	if err := s.persistRecovery(); err != nil {
		p.stdinCloseAcked = false
		return fmt.Errorf("persist acknowledged stdin close: %w", err)
	}
	return nil
}

func (s *service) waitProcess(agentID, execID string, p *process) {
	var state agent.ProcessState
	var pid, exit uint32
	var err error
	stdoutTruncationReported, stderrTruncationReported := false, false
	for {
		var output processOutput
		err = s.readProcessOutput(agentID, p.stdoutOffset, p.stderrOffset, &output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "multikernel output monitor: %v\n", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		now := time.Now()
		stdoutAdvance, stdoutDropped := deliverOutput(p.stdoutWriter, output.Stdout, &p.stdoutPressure, now, 30*time.Second)
		stderrAdvance, stderrDropped := deliverOutput(p.stderrWriter, output.Stderr, &p.stderrPressure, now, 30*time.Second)
		s.mu.Lock()
		oldStdoutOffset, oldStderrOffset := p.stdoutOffset, p.stderrOffset
		if stdoutAdvance {
			p.stdoutOffset = output.StdoutOffset
		}
		if stderrAdvance {
			p.stderrOffset = output.StderrOffset
		}
		stdoutChanged := p.stdoutOffset != oldStdoutOffset
		stderrChanged := p.stderrOffset != oldStderrOffset
		var persistErr error
		if stdoutChanged || stderrChanged {
			persistErr = s.persistRecovery()
			if persistErr != nil {
				p.stdoutOffset, p.stderrOffset = oldStdoutOffset, oldStderrOffset
				if stdoutChanged {
					stdoutAdvance = false
				}
				if stderrChanged {
					stderrAdvance = false
				}
			}
		}
		s.mu.Unlock()
		if persistErr != nil {
			fmt.Fprintf(os.Stderr, "multikernel output acknowledgement: %v\n", persistErr)
		}
		if stdoutDropped {
			fmt.Fprintf(os.Stderr, "multikernel stdout: discarded %d bytes after 30s output pressure\n", len(output.Stdout))
		}
		if stderrDropped {
			fmt.Fprintf(os.Stderr, "multikernel stderr: discarded %d bytes after 30s output pressure\n", len(output.Stderr))
		}
		if output.StdoutTruncated && !stdoutTruncationReported {
			fmt.Fprintln(os.Stderr, "multikernel stdout: guest retention limit discarded output before delivery")
			stdoutTruncationReported = true
		}
		if output.StderrTruncated && !stderrTruncationReported {
			fmt.Fprintln(os.Stderr, "multikernel stderr: guest retention limit discarded output before delivery")
			stderrTruncationReported = true
		}
		if !stdoutAdvance || !stderrAdvance {
			time.Sleep(10 * time.Millisecond)
		}
		if output.Status == "STOPPED" && len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			err = s.callAgentWithReconnect("WaitProcess", map[string]string{"ID": agentID}, &state)
			if err != nil {
				fmt.Fprintf(os.Stderr, "multikernel process wait: %v\n", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			pid, exit, err = stoppedGuestProcess(state, agentID, p.pid)
			if err != nil {
				fmt.Fprintf(os.Stderr, "multikernel process wait validation: %v\n", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
			break
		}
		if len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	now := time.Now().UTC()
	for {
		s.mu.Lock()
		closeProcessIO(p)
		oldStatus, oldPID, oldExit, oldExited := p.status, p.pid, p.exit, p.exited
		oldPending := append([]byte(nil), p.stdinPending...)
		p.status, p.pid, p.exit, p.exited, p.stdinPending = tasktypes.Status_STOPPED, pid, exit, now, nil
		if err = s.persistRecovery(); err == nil {
			break
		}
		p.status, p.pid, p.exit, p.exited, p.stdinPending = oldStatus, oldPID, oldExit, oldExited, oldPending
		s.mu.Unlock()
		fmt.Fprintf(os.Stderr, "multikernel exit state persistence: %v\n", err)
		time.Sleep(100 * time.Millisecond)
	}
	if err := s.publishExit(context.Background(), execID, p); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel exit event queue: %v\n", err)
	} else {
		p.exitEventQueued = true
		if err := s.persistRecovery(); err != nil {
			p.exitEventQueued = false
			fmt.Fprintf(os.Stderr, "multikernel exit event acknowledgement: %v\n", err)
		}
	}
	close(p.done)
	s.mu.Unlock()
}

func (s *service) readProcessOutput(agentID string, stdoutOffset, stderrOffset uint64, output *processOutput) error {
	if err := s.callAgentWithReconnect("ReadProcessOutput", map[string]any{
		"id": agentID, "stdout_offset": stdoutOffset, "stderr_offset": stderrOffset, "limit": uint64(processOutputChunk),
	}, output); err != nil {
		return err
	}
	return validateProcessOutput(stdoutOffset, stderrOffset, *output)
}

func validateProcessOutput(stdoutOffset, stderrOffset uint64, output processOutput) error {
	if len(output.Stdout) > processOutputChunk || len(output.Stderr) > processOutputChunk {
		return errors.New("guest output exceeded the requested atomic chunk")
	}
	exactOffset := func(current, next uint64, data []byte) bool {
		return uint64(len(data)) <= ^uint64(0)-current && next == current+uint64(len(data))
	}
	if !exactOffset(stdoutOffset, output.StdoutOffset, output.Stdout) ||
		!exactOffset(stderrOffset, output.StderrOffset, output.Stderr) {
		return errors.New("guest output returned a non-contiguous offset")
	}
	if output.Status != "RUNNING" && output.Status != "STOPPED" {
		return errors.New("guest output returned an invalid process state")
	}
	return nil
}

func (s *service) callAgentWithReconnect(method string, request, output any) error {
	return s.callAgentWithReconnectContext(context.Background(), method, request, output)
}

func (s *service) callAgentWithReconnectContext(parent context.Context, method string, request, output any) error {
	timeout := s.ioCallTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var lastErr error
	for {
		if err := s.agent.CallContext(ctx, method, request, output); err == nil {
			return nil
		} else {
			lastErr = err
		}
		var remoteError *agent.RemoteError
		if errors.As(lastErr, &remoteError) || s.relaySocket == "" || ctx.Err() != nil {
			return lastErr
		}
		if err := s.agent.ReconnectContext(ctx, s.relaySocket); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("reconnect guest agent transport: %w", err))
		}
		if err := waitContext(ctx, 50*time.Millisecond); err != nil {
			return errors.Join(lastErr, err)
		}
	}
}

func (s *service) quiesceAndShutdownGuest(ctx context.Context) error {
	var quiesced guestQuiesceResponse
	if err := s.callAgentWithReconnectContext(ctx, "Quiesce", map[string]any{}, &quiesced); err != nil {
		return fmt.Errorf("quiesce guest agent: %w", err)
	}
	if quiesced.Status != "quiesced" {
		return fmt.Errorf("quiesce guest agent: unexpected status %q", quiesced.Status)
	}
	var shutdown guestQuiesceResponse
	err := s.agent.CallContext(ctx, "Shutdown", map[string]any{}, &shutdown)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		var remote *agent.RemoteError
		if errors.As(err, &remote) {
			return fmt.Errorf("shutdown guest agent: %w", err)
		}
		// Quiesce is the authenticated commit point. After it succeeds, a
		// transport loss during terminal Shutdown is safe to complete locally:
		// the guest is sealed against new mutations and storage is already quiet.
		return nil
	}
	if shutdown.Status != "quiesced" {
		return fmt.Errorf("shutdown guest agent: unexpected status %q", shutdown.Status)
	}
	return nil
}

func (s *service) deleteGuestProcess(ctx context.Context, id string) error {
	err := s.callAgentWithReconnectContext(ctx, "DeleteProcess", map[string]string{"ID": id}, nil)
	if err != nil && !agentNotFound(err) {
		return fmt.Errorf("delete guest process: %w", err)
	}
	return nil
}

// deliverOutput advances a guest offset only after the complete chunk reaches
// its destination. Chunks are capped at Linux PIPE_BUF by waitProcess, making
// nonblocking FIFO writes all-or-nothing. A permanently absent/slow consumer
// cannot retain the shim forever: after the documented bound the chunk is
// deliberately discarded and acknowledged with a diagnostic.
func deliverOutput(writer io.Writer, data []byte, pressure *time.Time, now time.Time, timeout time.Duration) (advance, dropped bool) {
	if len(data) == 0 || writer == nil {
		*pressure = time.Time{}
		return true, false
	}
	n, err := writer.Write(data)
	if err == nil && n == len(data) {
		*pressure = time.Time{}
		return true, false
	}
	if pressure.IsZero() {
		*pressure = now
	}
	if timeout > 0 && now.Sub(*pressure) >= timeout {
		*pressure = time.Time{}
		return true, true
	}
	return false, false
}

func (s *service) publishExit(ctx context.Context, execID string, p *process) error {
	eventID := execID
	if eventID == "" {
		eventID = s.id
	}
	return s.publish(ctx, ctruntime.TaskExitEventTopic, &eventstypes.TaskExit{ContainerID: s.id, ID: eventID,
		Pid: p.pid, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)})
}

func (s *service) State(ctx context.Context, r *taskapi.StateRequest) (*taskapi.StateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil state request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		return nil, errdefs.ErrNotFound
	}
	return &taskapi.StateResponse{ID: s.id, Bundle: s.bundle, Pid: p.pid, Status: p.status, Stdin: p.stdin, Stdout: p.stdout, Stderr: p.stderr, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited), ExecID: r.ExecID}, nil
}

func (s *service) Wait(ctx context.Context, r *taskapi.WaitRequest) (*taskapi.WaitResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil wait request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	done := p.done
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	return &taskapi.WaitResponse{ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}, nil
}

func (s *service) Kill(ctx context.Context, r *taskapi.KillRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil kill request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	client := s.agent
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if client == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	if err := client.CallContext(ctx, "SignalProcess", map[string]any{"ID": id, "Signal": strconv.FormatUint(uint64(r.Signal), 10)}, nil); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func processSpec(p *specs.Process) agent.ProcessSpec {
	gids := make([]uint32, len(p.User.AdditionalGids))
	copy(gids, p.User.AdditionalGids)
	result := agent.ProcessSpec{Terminal: p.Terminal, User: agent.User{UID: p.User.UID, GID: p.User.GID, AdditionalGids: gids}, Args: p.Args, Env: p.Env, Cwd: p.Cwd}
	if p.NoNewPrivileges {
		value := true
		result.NoNewPrivileges = &value
	}
	for _, limit := range p.Rlimits {
		result.Rlimits = append(result.Rlimits, agent.Rlimit{Type: limit.Type, Hard: limit.Hard, Soft: limit.Soft})
	}
	if p.Capabilities != nil {
		result.Capabilities = map[string][]string{
			"bounding": append([]string(nil), p.Capabilities.Bounding...), "effective": append([]string(nil), p.Capabilities.Effective...),
			"inheritable": append([]string(nil), p.Capabilities.Inheritable...), "permitted": append([]string(nil), p.Capabilities.Permitted...),
			"ambient": append([]string(nil), p.Capabilities.Ambient...),
		}
	}
	return result
}

func validateExecProcess(p *specs.Process) error {
	if p == nil || len(p.Args) == 0 || p.Cwd == "" || !filepath.IsAbs(p.Cwd) {
		return fmt.Errorf("%w: exec args and absolute cwd are required", errdefs.ErrInvalidArgument)
	}
	if p.ConsoleSize != nil || p.CommandLine != "" || p.ApparmorProfile != "" ||
		p.OOMScoreAdj != nil || p.Scheduler != nil || p.SelinuxLabel != "" ||
		p.IOPriority != nil || p.User.Umask != nil || p.User.Username != "" {
		return fmt.Errorf("%w: unsupported exec process field", errdefs.ErrNotImplemented)
	}
	return nil
}

func agentNotFound(err error) bool {
	var remote *agent.RemoteError
	return errors.As(err, &remote) && remote.Failure.Code == "NOT_FOUND"
}

func ensureGuestProcessCreated(ctx context.Context, client agentClient, processID, bundle string) error {
	var state agent.ProcessState
	err := client.CallContext(ctx, "StateProcess", map[string]string{"ID": processID}, &state)
	if err == nil {
		if err = createdGuestProcess(state, processID); err != nil {
			return fmt.Errorf("reconcile created guest process: %w", err)
		}
		return nil
	}
	if !agentNotFound(err) {
		return fmt.Errorf("inspect created guest process: %w", err)
	}
	if err = client.CallContext(ctx, "CreateProcess", map[string]any{"ID": processID, "Bundle": bundle}, nil); err != nil {
		return fmt.Errorf("create guest process: %w", err)
	}
	return nil
}

func (s *service) rollbackExec(ctx context.Context, execID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	deleteErr := s.deleteGuestProcess(cleanupCtx, execID)
	s.mu.Lock()
	if deleteErr == nil {
		delete(s.processes, execID)
	}
	persistErr := s.persistRecovery()
	s.mu.Unlock()
	return errors.Join(deleteErr, persistErr)
}

func (s *service) Exec(ctx context.Context, r *taskapi.ExecProcessRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil exec request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if !guestProcessIdentifier.MatchString(r.ExecID) {
		return nil, errdefs.ErrInvalidArgument
	}
	stdinIdentity, stdoutIdentity, stderrIdentity, err := inspectBoundProcessIOPaths(r.Stdin, r.Stdout, r.Stderr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrInvalidArgument, err)
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	client := s.agent
	s.mu.Unlock()
	if client == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	v, err := typeurl.UnmarshalAny(r.Spec)
	if err != nil {
		return nil, err
	}
	spec, ok := v.(*specs.Process)
	if !ok {
		return nil, errdefs.ErrInvalidArgument
	}
	if err = validateExecProcess(spec); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	init, initExists := s.processes[""]
	if !initExists || init.status != tasktypes.Status_RUNNING {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if _, exists := s.processes[r.ExecID]; exists {
		s.mu.Unlock()
		return nil, errdefs.ErrAlreadyExists
	}
	p := &process{id: r.ExecID, stdin: r.Stdin, stdout: r.Stdout, stderr: r.Stderr,
		stdinIdentity: stdinIdentity, stdoutIdentity: stdoutIdentity, stderrIdentity: stderrIdentity,
		terminal: r.Terminal, status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s.processes[r.ExecID] = p
	if err = s.persistRecovery(); err != nil {
		delete(s.processes, r.ExecID)
		s.mu.Unlock()
		return nil, fmt.Errorf("persist exec creation intent: %w", err)
	}
	s.mu.Unlock()
	if createErr := client.CallContext(ctx, "ExecProcess", map[string]any{"id": r.ExecID, "parent_id": "init", "spec": processSpec(spec)}, nil); createErr != nil {
		reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		var state agent.ProcessState
		stateErr := s.callAgentWithReconnectContext(reconcileCtx, "StateProcess", map[string]string{"ID": r.ExecID}, &state)
		cancel()
		stateValidationErr := stateErr
		if stateValidationErr == nil {
			stateValidationErr = createdGuestProcess(state, r.ExecID)
		}
		if stateValidationErr != nil {
			rollbackErr := s.rollbackExec(ctx, r.ExecID)
			return nil, errors.Join(fmt.Errorf("create guest exec: %w", createErr),
				fmt.Errorf("reconcile guest exec creation: %w", stateValidationErr), rollbackErr)
		}
	}
	s.mu.Lock()
	err = s.persistRecovery()
	s.mu.Unlock()
	if err != nil {
		rollbackErr := s.rollbackExec(ctx, r.ExecID)
		return nil, errors.Join(fmt.Errorf("persist exec process: %w", err), rollbackErr)
	}
	if err = s.publish(ctx, ctruntime.TaskExecAddedEventTopic, &eventstypes.TaskExecAdded{ContainerID: s.id, ExecID: r.ExecID}); err != nil {
		return nil, errors.Join(err, s.rollbackExec(ctx, r.ExecID))
	}
	return &emptypb.Empty{}, nil
}

func (s *service) Delete(ctx context.Context, r *taskapi.DeleteRequest) (*taskapi.DeleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil delete request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status == tasktypes.Status_RUNNING || p.status == tasktypes.Status_PAUSED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if p.deleting {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: process deletion is already in progress", errdefs.ErrFailedPrecondition)
	}
	if r.ExecID == "" && len(s.processes) != 1 {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: delete exec processes before deleting init", errdefs.ErrFailedPrecondition)
	}
	if p.status == tasktypes.Status_STOPPED && !p.exitEventQueued {
		if err := s.publishExit(ctx, r.ExecID, p); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("queue missing exit event before delete: %w", err)
		}
		p.exitEventQueued = true
		if err := s.persistRecovery(); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("persist exit event ordering before delete: %w", err)
		}
	}
	p.deleting = true
	client := s.agent
	s.mu.Unlock()
	abort := func(err error) (*taskapi.DeleteResponse, error) {
		s.mu.Lock()
		p.deleting = false
		s.mu.Unlock()
		return nil, err
	}
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if client != nil {
		if err := s.deleteGuestProcess(ctx, id); err != nil {
			return abort(err)
		}
	}
	if r.ExecID == "" {
		if client != nil {
			if err := s.stopNetwork(); err != nil {
				return abort(err)
			}
			if err := s.quiesceAndShutdownGuest(ctx); err != nil {
				return abort(err)
			}
			closeErr := client.Close()
			s.mu.Lock()
			s.agent = nil
			s.mu.Unlock()
			if closeErr != nil {
				return abort(fmt.Errorf("close guest agent: %w", closeErr))
			}
		}
		if err := s.stopRelay(); err != nil {
			return abort(fmt.Errorf("stop agent relay: %w", err))
		}
		if err := s.releaseNetwork(ctx); err != nil {
			return abort(fmt.Errorf("release primary network endpoint: %w", err))
		}
		if _, err := daemon.Mutation(ctx, s.daemon, "StopSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-stop-"+s.sandbox.Generation, nil); err != nil {
			return abort(fmt.Errorf("stop sandbox: %w", err))
		}
		if _, err := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-delete-"+s.sandbox.Generation, nil); err != nil {
			return abort(fmt.Errorf("delete sandbox: %w", err))
		}
	}
	s.mu.Lock()
	deleteQueued := p.deleteEventQueued
	s.mu.Unlock()
	if !deleteQueued {
		response := &taskapi.DeleteResponse{Pid: p.pid, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}
		if err := s.publish(ctx, ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: s.id, ID: r.ExecID, Pid: response.Pid, ExitStatus: p.exit, ExitedAt: response.ExitedAt}); err != nil {
			return abort(fmt.Errorf("queue task delete event: %w", err))
		}
		s.mu.Lock()
		p.deleteEventQueued = true
		persistErr := s.persistRecovery()
		s.mu.Unlock()
		if persistErr != nil {
			return abort(fmt.Errorf("persist queued delete event: %w", persistErr))
		}
	}
	if err := s.flushEvents(ctx); err != nil {
		return abort(fmt.Errorf("flush task delete event: %w", err))
	}
	if r.ExecID == "" && s.sandbox.Config.Storage != nil {
		request := rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: s.bundle,
			TaskIdentity: storageTaskIdentity(s.namespace, s.id), StorageSHA256: s.sandbox.Config.Storage.SHA256}
		if err := s.cleanupRootfs(ctx, request); err != nil {
			return abort(fmt.Errorf("cleanup prepared rootfs: %w", err))
		}
	}
	s.mu.Lock()
	delete(s.processes, r.ExecID)
	if r.ExecID != "" {
		if err := s.persistRecovery(); err != nil {
			s.processes[r.ExecID] = p
			p.deleting = false
			s.mu.Unlock()
			return nil, fmt.Errorf("persist process deletion: %w", err)
		}
	}
	s.mu.Unlock()
	resp := &taskapi.DeleteResponse{Pid: p.pid, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}
	return resp, nil
}

func (s *service) Pids(ctx context.Context, r *taskapi.PidsRequest) (*taskapi.PidsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil pids request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	processes := make([]*tasktypes.ProcessInfo, 0, len(s.processes))
	for _, process := range s.processes {
		if process.pid != 0 {
			processes = append(processes, &tasktypes.ProcessInfo{Pid: process.pid})
		}
	}
	sort.Slice(processes, func(i, j int) bool { return processes[i].Pid < processes[j].Pid })
	return &taskapi.PidsResponse{Processes: processes}, nil
}
func (s *service) Connect(ctx context.Context, r *taskapi.ConnectRequest) (*taskapi.ConnectResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil connect request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	var taskPID uint32
	if init, ok := s.processes[""]; ok {
		taskPID = init.pid
	}
	return &taskapi.ConnectResponse{ShimPid: uint32(os.Getpid()), TaskPid: taskPID, Version: "multikernel-v1-guest-pid"}, nil
}
func (s *service) Shutdown(ctx context.Context, r *taskapi.ShutdownRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil shutdown request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	if len(s.processes) != 0 || s.shuttingDown {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	s.shuttingDown = true
	shutdown := s.shutdown
	s.mu.Unlock()
	if err := s.flushEvents(ctx); err != nil {
		s.mu.Lock()
		s.shuttingDown = false
		s.mu.Unlock()
		return nil, fmt.Errorf("flush durable events before shutdown: %w", err)
	}
	go func() {
		s.stopEventRetry()
		if shutdown != nil {
			shutdown()
		}
	}()
	return &emptypb.Empty{}, nil
}
func (s *service) ResizePty(ctx context.Context, r *taskapi.ResizePtyRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil resize request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if r.Width > 65535 || r.Height > 65535 {
		return nil, fmt.Errorf("%w: terminal dimensions exceed the Linux PTY limit", errdefs.ErrInvalidArgument)
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if !p.terminal {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if p.status != tasktypes.Status_CREATED && p.status != tasktypes.Status_RUNNING {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	client := s.agent
	if p.status == tasktypes.Status_RUNNING && client == nil {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	oldWidth, oldHeight, oldSizeSet := p.width, p.height, p.sizeSet
	p.width, p.height, p.sizeSet = r.Width, r.Height, true
	if err := s.persistRecovery(); err != nil {
		p.width, p.height, p.sizeSet = oldWidth, oldHeight, oldSizeSet
		s.mu.Unlock()
		return nil, fmt.Errorf("persist terminal size: %w", err)
	}
	if p.status == tasktypes.Status_CREATED {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if err := client.CallContext(ctx, "ResizeProcess", map[string]any{"id": id, "width": r.Width, "height": r.Height}, nil); err != nil {
		p.width, p.height, p.sizeSet = oldWidth, oldHeight, oldSizeSet
		rollbackErr := s.persistRecovery()
		if rollbackErr != nil {
			// The durable new intent won the failure race. Keep memory aligned
			// with it so a caller retry or reconstruction reapplies that intent.
			p.width, p.height, p.sizeSet = r.Width, r.Height, true
		}
		s.mu.Unlock()
		return nil, errors.Join(err, rollbackErr)
	}
	s.mu.Unlock()
	return &emptypb.Empty{}, nil
}

type processSignalTarget struct {
	id      string
	process *process
}

func (s *service) signalTargets(status tasktypes.Status) []processSignalTarget {
	result := make([]processSignalTarget, 0, len(s.processes))
	for id, process := range s.processes {
		if process.status != status {
			continue
		}
		agentID := id
		if agentID == "" {
			agentID = "init"
		}
		result = append(result, processSignalTarget{id: agentID, process: process})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].id == "init" || result[j].id == "init" {
			return result[i].id == "init"
		}
		return result[i].id < result[j].id
	})
	return result
}

func signalProcessTargets(ctx context.Context, client agentClient, targets []processSignalTarget, signal syscall.Signal) (int, error) {
	for index, target := range targets {
		if err := client.CallContext(ctx, "SignalProcess", map[string]any{"ID": target.id, "Signal": strconv.Itoa(int(signal))}, nil); err != nil {
			return index, err
		}
	}
	return len(targets), nil
}

func rollbackProcessSignals(client agentClient, targets []processSignalTarget, signal syscall.Signal) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var failures []error
	for index := len(targets) - 1; index >= 0; index-- {
		if err := client.CallContext(ctx, "SignalProcess", map[string]any{"ID": targets[index].id, "Signal": strconv.Itoa(int(signal))}, nil); err != nil {
			failures = append(failures, fmt.Errorf("rollback process %s: %w", targets[index].id, err))
		}
	}
	return errors.Join(failures...)
}

func (s *service) Pause(ctx context.Context, r *taskapi.PauseRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil pause request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	p, ok := s.processes[""]
	if !ok {
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_RUNNING || s.agent == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	targets := s.signalTargets(tasktypes.Status_RUNNING)
	signaled, err := signalProcessTargets(ctx, s.agent, targets, syscall.SIGSTOP)
	if err != nil {
		return nil, errors.Join(err, rollbackProcessSignals(s.agent, targets[:signaled], syscall.SIGCONT))
	}
	for _, target := range targets {
		target.process.status = tasktypes.Status_PAUSED
	}
	if err := s.persistRecovery(); err != nil {
		rollbackErr := rollbackProcessSignals(s.agent, targets, syscall.SIGCONT)
		for _, target := range targets {
			target.process.status = tasktypes.Status_RUNNING
		}
		persistErr := s.persistRecovery()
		if persistErr != nil {
			persistErr = fmt.Errorf("persist pause rollback: %w", persistErr)
		}
		return nil, errors.Join(fmt.Errorf("persist paused state: %w", err), rollbackErr, persistErr)
	}
	if err := s.publish(ctx, ctruntime.TaskPausedEventTopic, &eventstypes.TaskPaused{ContainerID: s.id}); err != nil {
		rollbackErr := rollbackProcessSignals(s.agent, targets, syscall.SIGCONT)
		for _, target := range targets {
			target.process.status = tasktypes.Status_RUNNING
		}
		persistErr := s.persistRecovery()
		if persistErr != nil {
			persistErr = fmt.Errorf("persist pause rollback: %w", persistErr)
		}
		return nil, errors.Join(err, rollbackErr, persistErr)
	}
	return &emptypb.Empty{}, nil
}
func (s *service) Resume(ctx context.Context, r *taskapi.ResumeRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil resume request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	p, ok := s.processes[""]
	if !ok {
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_PAUSED || s.agent == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	targets := s.signalTargets(tasktypes.Status_PAUSED)
	signaled, err := signalProcessTargets(ctx, s.agent, targets, syscall.SIGCONT)
	if err != nil {
		return nil, errors.Join(err, rollbackProcessSignals(s.agent, targets[:signaled], syscall.SIGSTOP))
	}
	for _, target := range targets {
		target.process.status = tasktypes.Status_RUNNING
	}
	if err := s.persistRecovery(); err != nil {
		rollbackErr := rollbackProcessSignals(s.agent, targets, syscall.SIGSTOP)
		for _, target := range targets {
			target.process.status = tasktypes.Status_PAUSED
		}
		persistErr := s.persistRecovery()
		if persistErr != nil {
			persistErr = fmt.Errorf("persist resume rollback: %w", persistErr)
		}
		return nil, errors.Join(fmt.Errorf("persist resumed state: %w", err), rollbackErr, persistErr)
	}
	if err := s.publish(ctx, ctruntime.TaskResumedEventTopic, &eventstypes.TaskResumed{ContainerID: s.id}); err != nil {
		rollbackErr := rollbackProcessSignals(s.agent, targets, syscall.SIGSTOP)
		for _, target := range targets {
			target.process.status = tasktypes.Status_PAUSED
		}
		persistErr := s.persistRecovery()
		if persistErr != nil {
			persistErr = fmt.Errorf("persist resume rollback: %w", persistErr)
		}
		return nil, errors.Join(err, rollbackErr, persistErr)
	}
	return &emptypb.Empty{}, nil
}
func (s *service) Checkpoint(context.Context, *taskapi.CheckpointTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) CloseIO(ctx context.Context, r *taskapi.CloseIORequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil close I/O request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if !r.Stdin {
		return &emptypb.Empty{}, nil
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.stdinClosed && p.stdinCloseAcked {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	if p.status == tasktypes.Status_STOPPED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if p.status != tasktypes.Status_CREATED && p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if !p.stdinClosed {
		p.stdinClosed = true
		if err := s.persistRecovery(); err != nil {
			p.stdinClosed = false
			s.mu.Unlock()
			return nil, fmt.Errorf("persist closed stdin: %w", err)
		}
	}
	if p.status == tasktypes.Status_CREATED {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	hasReader := p.stdinReader != nil
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if hasReader {
		// The pump must forward bytes already buffered in the FIFO before it
		// closes guest stdin. It observes stdinClosed after reaching FIFO EOF.
		return &emptypb.Empty{}, nil
	}
	if err := s.acknowledgeStdinClose(ctx, id, p); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
func (s *service) Update(context.Context, *taskapi.UpdateTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) Stats(ctx context.Context, r *taskapi.StatsRequest) (*taskapi.StatsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w: nil stats request", errdefs.ErrInvalidArgument)
	}
	if err := s.validateTaskRequest(r.ID); err != nil {
		return nil, err
	}
	if err := lockContext(ctx, &s.mu); err != nil {
		return nil, err
	}
	p, ok := s.processes[""]
	client := s.agent
	memoryLimit := s.sandbox.Config.MemoryBytes
	var processIDs []string
	for id, process := range s.processes {
		if process.status != tasktypes.Status_RUNNING && process.status != tasktypes.Status_PAUSED {
			continue
		}
		if id == "" {
			id = "init"
		}
		processIDs = append(processIDs, id)
	}
	sort.Strings(processIDs)
	var status tasktypes.Status
	if ok {
		status = p.status
	}
	s.mu.Unlock()
	if !ok || client == nil {
		return nil, errdefs.ErrNotFound
	}
	if status != tasktypes.Status_RUNNING && status != tasktypes.Status_PAUSED {
		return nil, errdefs.ErrFailedPrecondition
	}
	var guest agent.ProcessStats
	for _, id := range processIDs {
		var observed agent.ProcessStats
		if err := client.CallContext(ctx, "StatsProcess", map[string]string{"ID": id}, &observed); err != nil {
			return nil, fmt.Errorf("stats process %s: %w", id, err)
		}
		if ^uint64(0)-guest.CPUUserNS < observed.CPUUserNS ||
			^uint64(0)-guest.CPUSystemNS < observed.CPUSystemNS ||
			^uint64(0)-guest.RSSBytes < observed.RSSBytes ||
			^uint64(0)-guest.PIDs < observed.PIDs {
			return nil, errors.New("guest process metrics overflow task aggregate")
		}
		guest.CPUUserNS += observed.CPUUserNS
		guest.CPUSystemNS += observed.CPUSystemNS
		guest.RSSBytes += observed.RSSBytes
		guest.PIDs += observed.PIDs
	}
	if ^uint64(0)-guest.CPUUserNS < guest.CPUSystemNS {
		return nil, errors.New("guest CPU metrics overflow total usage")
	}
	metrics := &cgroupstats.Metrics{
		Pids: &cgroupstats.PidsStat{Current: guest.PIDs, Limit: 1024},
		CPU: &cgroupstats.CPUStat{Usage: &cgroupstats.CPUUsage{
			Total: guest.CPUUserNS + guest.CPUSystemNS, User: guest.CPUUserNS, Kernel: guest.CPUSystemNS,
		}},
		Memory: &cgroupstats.MemoryStat{RSS: guest.RSSBytes, TotalRSS: guest.RSSBytes,
			Usage: &cgroupstats.MemoryEntry{Usage: guest.RSSBytes, Limit: memoryLimit}},
	}
	encoded, err := typeurl.MarshalAny(metrics)
	if err != nil {
		return nil, err
	}
	return &taskapi.StatsResponse{Stats: &anypb.Any{TypeUrl: encoded.GetTypeUrl(), Value: encoded.GetValue()}}, nil
}

var _ taskapi.TaskService = (*service)(nil)

func serverInvocation() bool {
	if len(os.Args) > 1 {
		action := os.Args[len(os.Args)-1]
		if action == "start" || action == "delete" {
			return false
		}
	}
	var info unix.Stat_t
	return unix.Fstat(3, &info) == nil && info.Mode&unix.S_IFMT == unix.S_IFSOCK
}

func superviseShimWorker() int {
	listener := os.NewFile(3, "shim-listener")
	if listener == nil {
		fmt.Fprintln(os.Stderr, "multikernel shim supervisor: inherited listener is missing")
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: executable: %v\n", err)
		return 1
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: working directory: %v\n", err)
		return 1
	}
	return superviseShimWorkerWith(listener, self, os.Args[1:], workingDirectory, os.Environ())
}

func superviseShimWorkerWith(listener *os.File, self string, arguments []string, workingDirectory string, environment []string) int {
	var err error
	pidPath := filepath.Join(workingDirectory, ".multikernel-worker.pid")
	defer os.Remove(pidPath)
	for attempt := 0; attempt < 10; attempt++ {
		cmd := exec.Command(self, arguments...)
		cmd.Dir = workingDirectory
		cmd.Env = append(environment, "MK_SHIM_WORKER=1")
		cmd.ExtraFiles = []*os.File{listener}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
		if err = cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: start worker: %v\n", err)
			return 1
		}
		_ = os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0600)
		err = cmd.Wait()
		if err == nil {
			return 0
		}
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: wait worker: %v\n", err)
			return 1
		}
		status, ok := exitError.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() {
			fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker exited without a recoverable signal: %v\n", err)
			return exitError.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "multikernel shim supervisor: worker pid=%d signal=%s restart_attempt=%d\n", cmd.Process.Pid, status.Signal(), attempt+1)
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "multikernel shim supervisor: restart budget exhausted")
	return 1
}

func main() {
	if buildinfo.PrintRequested(os.Stdout, "containerd-shim-multikernel-v2", os.Args[1:]) {
		return
	}
	if serverInvocation() && os.Getenv("MK_SHIM_WORKER") != "1" {
		os.Exit(superviseShimWorker())
	}
	shim.Run(runtimeName, newService)
}
