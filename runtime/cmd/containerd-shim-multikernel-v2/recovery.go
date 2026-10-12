//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	tasktypes "github.com/containerd/containerd/api/types/task"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type persisted struct {
	SchemaVersion  int                         `json:"schema_version"`
	BundleIdentity rootfspkg.DirectoryIdentity `json:"bundle_identity"`
	ID             string                      `json:"id"`
	Generation     string                      `json:"generation"`
	PID            uint32                      `json:"pid,omitempty"`
	Exit           uint32                      `json:"exit"`
	Exited         time.Time                   `json:"exited"`
	Network        mknetwork.Endpoint          `json:"network"`
	TaskIdentity   string                      `json:"task_identity"`
	StorageSHA256  string                      `json:"storage_sha256"`
	Processes      []persistedProcess          `json:"processes,omitempty"`
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
	PendingSignal     uint32            `json:"pending_signal,omitempty"`
	PendingSignalOp   string            `json:"pending_signal_operation,omitempty"`
	PendingSignalAck  bool              `json:"pending_signal_acknowledgement,omitempty"`
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
	if value.SchemaVersion != 3 || value.BundleIdentity.Device == 0 || value.BundleIdentity.Inode == 0 ||
		value.BundleIdentity.UID != uint32(os.Geteuid()) || value.ID != sandboxID(namespace, task) ||
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
		if process.PendingSignalOp == "" && (process.PendingSignal != 0 || process.PendingSignalAck) || process.PendingSignalOp != "" &&
			(!recoveryGeneration.MatchString(process.PendingSignalOp) || process.PendingSignal > 64 ||
				process.Status != tasktypes.Status_RUNNING && process.Status != tasktypes.Status_PAUSED &&
					!(process.PendingSignalAck && process.Status == tasktypes.Status_STOPPED)) {
			return errors.New("shim recovery contains an invalid pending signal")
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

func loadPersistedRecoveryFromDirectory(directory *safefile.Directory, namespace, task string) (persisted, bool, *safefile.Identity, error) {
	var value persisted
	data, present, identity, err := directory.ReadPrivateIdentity("sandbox.json", 8<<20)
	if err != nil || !present {
		return value, present, nil, err
	}
	if err = protocol.StrictDecode(data, &value); err != nil {
		return value, false, nil, fmt.Errorf("decode shim recovery: %w", err)
	}
	if err = validatePersistedRecovery(value, namespace, task); err != nil {
		return value, false, nil, err
	}
	return value, true, &identity, nil
}

func (s *service) persistRecovery() error {
	if s.sandbox.ID == "" {
		return nil
	}
	network := s.networkReport("READY")
	p := persisted{SchemaVersion: 3, BundleIdentity: s.bundleIdentity, ID: s.sandbox.ID, Generation: s.sandbox.Generation, Network: network,
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
			PendingSignal: process.pendingSignal, PendingSignalOp: process.pendingSignalOperation,
			PendingSignalAck: process.pendingSignalAcknowledgement,
		})
	}
	sort.Slice(p.Processes, func(i, j int) bool { return p.Processes[i].ID < p.Processes[j].ID })
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err = s.ensureBundleIdentity(); err != nil {
		return err
	}
	runtimeDirectory, err := s.ensureRuntimeDirectory()
	if err != nil {
		return err
	}
	published, err := runtimeDirectory.ReplaceIdentity("sandbox.json", b, 0600, s.recoveryIdentity)
	if err != nil {
		return err
	}
	s.recoveryIdentity = &published
	return nil
}

func (s *service) recoverExisting(ctx context.Context) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensureBundleIdentity(); err != nil {
		return fmt.Errorf("verify recovery bundle identity: %w", err)
	}
	runtimeDirectory, err := s.ensureRuntimeDirectory()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open recovery runtime directory: %w", err)
	}
	recovery, found, recoveryIdentity, err := loadPersistedRecoveryFromDirectory(runtimeDirectory, s.namespace, s.id)
	if err != nil {
		return fmt.Errorf("read shim recovery state: %w", err)
	}
	if !found {
		return nil
	}
	if recovery.BundleIdentity != s.bundleIdentity {
		return errors.New("persisted recovery bundle identity differs from the service handoff")
	}
	s.recoveryIdentity = recoveryIdentity
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
	if s.sandbox.Config.BundleIdentity != s.bundleIdentity {
		return errors.New("daemon sandbox bundle identity differs from the service handoff")
	}
	s.token, _, err = loadExistingTokenFromDirectory(runtimeDirectory)
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
	if err = s.ensureNamespaceHolder(); err != nil {
		return fmt.Errorf("recover network namespace holder: %w", err)
	}
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
		cleanupErrors = append(cleanupErrors, s.stopNamespaceHolder())
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
			err = s.acquireRelaySocketOwner(s.relaySocket)
			if errors.Is(err, os.ErrNotExist) && time.Now().Before(deadline) && ctx.Err() == nil {
				if err = waitContext(ctx, 50*time.Millisecond); err == nil {
					continue
				}
			}
			if err != nil {
				return fmt.Errorf("capture recovered agent relay socket: %w", err)
			}
		}
		var client agentClient
		client, err = s.dialAgent(ctx, s.relaySocket)
		if err == nil {
			s.agent = client
		}
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
	if err = s.negotiateAgentCapabilities(ctx); err != nil {
		return fmt.Errorf("negotiate recovered guest agent: %w", err)
	}
	type recoveredProcess struct {
		agentID string
		process *process
	}
	type recoveredSignalAcknowledgement struct {
		agentID, operationID string
		process              *process
		signal               uint32
	}
	var running []recoveredProcess
	var signalAcknowledgements []recoveredSignalAcknowledgement
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
			pendingSignal: saved.PendingSignal, pendingSignalOperation: saved.PendingSignalOp,
			pendingSignalAcknowledgement: saved.PendingSignalAck,
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
		if err = s.callAgentWithReconnectContext(ctx, "StateProcess", map[string]string{"ID": agentID}, &state); err != nil {
			return fmt.Errorf("recover process %q: %w", saved.ID, err)
		}
		if state.ID != agentID {
			return fmt.Errorf("recover process %q: guest returned a mismatched process identity", saved.ID)
		}
		if state.Status == "STOPPED" {
			if err = s.callAgentWithReconnectContext(ctx, "WaitProcess", map[string]string{"ID": agentID}, &state); err != nil {
				return fmt.Errorf("recover stopped process %q wait state: %w", saved.ID, err)
			}
			var pid, exit uint32
			pid, exit, err = stoppedGuestProcess(state, agentID, p.pid)
			if err != nil {
				return fmt.Errorf("recover stopped process %q wait state: %w", saved.ID, err)
			}
			p.status, p.pid, p.exit, p.stdinPending = tasktypes.Status_STOPPED, pid, exit, nil
			if p.pendingSignalOperation != "" {
				p.pendingSignalAcknowledgement = true
				signalAcknowledgements = append(signalAcknowledgements, recoveredSignalAcknowledgement{
					agentID: agentID, operationID: p.pendingSignalOperation, process: p, signal: p.pendingSignal,
				})
			}
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
		if p.pendingSignalOperation != "" {
			if !p.pendingSignalAcknowledgement {
				signalErr := s.signalGuestProcess(ctx, agentID, p.pendingSignal, p.pendingSignalOperation)
				var remoteError *agent.RemoteError
				if signalErr != nil && !errors.As(signalErr, &remoteError) {
					return fmt.Errorf("recover process %q pending signal: %w", saved.ID, signalErr)
				}
				p.pendingSignalAcknowledgement = true
			}
			signalAcknowledgements = append(signalAcknowledgements, recoveredSignalAcknowledgement{
				agentID: agentID, operationID: p.pendingSignalOperation, process: p, signal: p.pendingSignal,
			})
		}
		if p.terminal && p.sizeSet {
			if err = s.resizeGuestProcess(ctx, agentID, p.width, p.height); err != nil {
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
	for _, item := range signalAcknowledgements {
		if err = s.acknowledgeGuestSignal(ctx, item.agentID, item.operationID); err != nil {
			return fmt.Errorf("acknowledge recovered process %q signal: %w", item.process.id, err)
		}
		item.process.pendingSignal, item.process.pendingSignalOperation, item.process.pendingSignalAcknowledgement = 0, "", false
	}
	if len(signalAcknowledgements) != 0 {
		if err = s.persistRecovery(); err != nil {
			for _, item := range signalAcknowledgements {
				item.process.pendingSignal = item.signal
				item.process.pendingSignalOperation = item.operationID
				item.process.pendingSignalAcknowledgement = true
			}
			return fmt.Errorf("persist recovered signal acknowledgement: %w", err)
		}
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
