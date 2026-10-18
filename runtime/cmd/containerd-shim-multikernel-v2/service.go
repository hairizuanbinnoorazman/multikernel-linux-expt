//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const fifoKeeperMode = "--multikernel-fifo-keeper"

const defaultAgentIOCallTimeout = 30 * time.Second

// A replacement worker can spend one complete agent I/O timeout proving that
// the inherited guest transport is stale before it replaces the relay. Keep
// the FIFO writer guard beyond that bounded recovery window so an existing
// containerd attach does not observe a transient EOF during reconstruction.
const fifoKeeperGrace = defaultAgentIOCallTimeout + 15*time.Second

var shimWorkerDisconnectGrace = 30 * time.Second

var runtimeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

var guestProcessIdentifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

var recoveryGeneration = regexp.MustCompile(`^[0-9a-f]{32}$`)

var recoverySHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var capabilityName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)

var requiredAgentProtocolFeatures = []string{"signal-operation-id-v1", "stdin-offset-v1", "two-phase-shutdown-v1"}

var requiredAgentOCIFeatures = []string{
	"argv", "attach", "cwd", "environment", "exit-code", "hostname", "linux-capabilities",
	"masked-paths", "no-new-privileges", "readonly-bind-inputs-v1", "readonly-paths", "readonly-root",
	"rlimits", "split-stdio", "standard-mounts", "stdin", "terminal", "terminal-resize",
}

type guestCapabilities struct {
	Protocol         int            `json:"protocol"`
	ProtocolFeatures []string       `json:"protocol_features"`
	OCIFeatures      []string       `json:"oci_features"`
	Kernel           map[string]any `json:"kernel"`
	Agent            map[string]any `json:"agent"`
}

func requireCapabilitySet(observed, required []string, name string) error {
	if len(observed) > 128 {
		return fmt.Errorf("guest %s capability set exceeds its bound", name)
	}
	available := make(map[string]struct{}, len(observed))
	for _, feature := range observed {
		if !capabilityName.MatchString(feature) {
			return fmt.Errorf("guest %s capability set contains an invalid feature", name)
		}
		if _, duplicate := available[feature]; duplicate {
			return fmt.Errorf("guest %s capability set contains duplicate %q", name, feature)
		}
		available[feature] = struct{}{}
	}
	for _, feature := range required {
		if _, ok := available[feature]; !ok {
			return fmt.Errorf("guest does not advertise required %s capability %q", name, feature)
		}
	}
	return nil
}

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
	stdinDrainRemaining            uint32
	stdinPending                   []byte
	stdoutOffset, stderrOffset     uint64
	stdoutPressure, stderrPressure time.Time
	exitEventQueued                bool
	deleteEventQueued              bool
	deleting                       bool
	pendingSignal                  uint32
	pendingSignalOperation         string
	pendingSignalAcknowledgement   bool
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

type namespaceHolder interface {
	PID() uint32
	Validate(string) error
	Stop() error
}

type namespaceHolderFactory func(string) (namespaceHolder, error)

type startShimSocket interface {
	shimSocketFile
	Close() error
}

type startShimSocketOwner interface {
	DialContext(context.Context) (net.Conn, error)
	Remove() error
	Close() error
}

type startShimSocketFactory func(string) (startShimSocket, error)

type startShimSocketOwnerFactory func(string) (startShimSocketOwner, error)

type stateFileIdentity struct {
	device uint64
	inode  uint64
	uid    uint32
	mode   uint32
	links  uint64
}

type service struct {
	mu                    sync.Mutex
	relayMu               sync.Mutex
	eventMu               sync.Mutex
	runtimeMu             sync.Mutex
	eventRetryStop        sync.Once
	bundleClose           sync.Once
	id, namespace, bundle string
	bundleDirectory       *safefile.Directory
	bundleIdentity        rootfspkg.DirectoryIdentity
	runtimeDirectory      *safefile.Directory
	publisher             shim.Publisher
	shutdown              func()
	daemon                daemon.Caller
	sandbox               protocol.Sandbox
	token                 []byte
	agent                 agentClient
	relay                 *exec.Cmd
	relaySocket           string
	relayOwner            relayPathOwner
	relayGeneration       uint64
	netDevice             *os.File
	netDone               chan struct{}
	netReports            chan string
	netWG                 sync.WaitGroup
	netEndpoint           mknetwork.Endpoint
	namespaceHolder       namespaceHolder
	netClient             networkClient
	newNamespaceHolder    namespaceHolderFactory
	agentDial             agentDialer
	newRelay              relayFactory
	relayPath             relayPathFactory
	newRelayOwner         relayOwnerFactory
	newShimSocketOwner    relayOwnerFactory
	shimArguments         []string
	ioCallTimeout         time.Duration
	networkCloseTimeout   time.Duration
	netRXPackets          atomic.Uint64
	netTXPackets          atomic.Uint64
	netRXDrops            atomic.Uint64
	netTXDrops            atomic.Uint64
	netErrors             atomic.Uint64
	processes             map[string]*process
	events                eventJournal
	eventJournalIdentity  *safefile.Identity
	recoveryIdentity      *safefile.Identity
	eventRetryCancel      context.CancelFunc
	eventRetryDone        chan struct{}
	shuttingDown          bool
}

const (
	rootfsPrepareDaemonTimeout = 11 * time.Minute
	createSandboxDaemonTimeout = 61 * time.Minute
)

func daemonCallerWithTimeout(caller daemon.Caller, timeout time.Duration) daemon.Caller {
	client, ok := caller.(daemon.Client)
	if !ok {
		return caller
	}
	client.Timeout = timeout
	return client
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
	bundleDirectory, bundleIdentity, err := openServiceIdentity(id, ns, bundle)
	if err != nil {
		return nil, err
	}
	s := &service{id: id, namespace: ns, bundle: bundle, publisher: publisher, shutdown: shutdown,
		bundleDirectory: bundleDirectory, bundleIdentity: bundleIdentity,
		daemon:    daemon.Client{Path: getenv("MK_DAEMON_SOCKET", "/run/mkruntimed.sock")},
		netClient: mknetwork.Client{Path: getenv("MK_NETWORK_SOCKET", "/run/mknetd.sock")}, processes: map[string]*process{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if !deleteInvocation(os.Args[1:]) {
		if err = s.loadEventJournal(); err != nil {
			_ = bundleDirectory.Close()
			return nil, err
		}
		if err = s.recoverExisting(ctx); err != nil {
			_ = bundleDirectory.Close()
			return nil, err
		}
		if err = s.flushEvents(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "multikernel event replay deferred: %v\n", err)
		}
		s.startEventRetry(time.Second)
	}
	return s, nil
}

func deleteInvocation(arguments []string) bool {
	valueFlags := map[string]bool{
		"-address": true, "-bundle": true, "-id": true, "-namespace": true,
		"-publish-binary": true, "-socket": true,
	}
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if valueFlags[argument] {
			index++
			if index >= len(arguments) {
				return false
			}
			continue
		}
		if argument == "-debug" || strings.HasPrefix(argument, "-debug=") {
			continue
		}
		matchedValueFlag := false
		for name := range valueFlags {
			if strings.HasPrefix(argument, name+"=") {
				matchedValueFlag = true
				break
			}
		}
		if matchedValueFlag {
			continue
		}
		return argument == "delete" && index == len(arguments)-1
	}
	return false
}

func (s *service) taskPID(execID string, process *process) uint32 {
	if execID == "" && s.namespaceHolder != nil {
		return s.namespaceHolder.PID()
	}
	if process == nil {
		return 0
	}
	return process.pid
}
