//go:build linux

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// createOperationKey remains stable across retries without copying credential
// bytes into public operation metadata or the durable lifecycle journal. The
// domain-separated 128-bit pseudonym fits the protocol's bounded key field.
func createOperationKey(namespace, id string, token []byte) string {
	digest := hmac.New(sha256.New, token)
	digest.Write([]byte("mkruntime/shim-create/v1\x00"))
	digest.Write([]byte(namespace))
	digest.Write([]byte{0})
	digest.Write([]byte(id))
	displayID := id
	if len(displayID) > 64 {
		displayID = displayID[:64]
	}
	return "shim-create-" + displayID + "-" + hex.EncodeToString(digest.Sum(nil)[:16])
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

func openAllocationLock(ctx context.Context, path string) (*safefile.Directory, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != absolute || filepath.Base(absolute) == "." || filepath.Base(absolute) == string(filepath.Separator) {
		return nil, errors.Join(errors.New("allocation lock path must be canonical and below the filesystem root"), err)
	}
	lock, err := safefile.OpenOwnedDirectory(filepath.Dir(absolute))
	if err != nil {
		return nil, err
	}
	for {
		acquired, lockErr := lock.TryLockExclusive()
		if lockErr != nil {
			_ = lock.Close()
			return nil, lockErr
		}
		if acquired {
			return lock, nil
		}
		select {
		case <-ctx.Done():
			_ = lock.Close()
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (s *service) allocate(ctx context.Context, bundle string) (protocol.SandboxConfig, *safefile.Directory, error) {
	lock, err := openAllocationLock(ctx, getenv("MK_SHIM_LOCK", "/run/multikernel-shim.lock"))
	if err != nil {
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
			return protocol.SandboxConfig{SchemaVersion: 1, ID: sandboxID(s.namespace, s.id), CPUs: set, MemoryBytes: 3 << 30, KernelManifest: "gce-mk2", Bundle: bundle,
				Labels: map[string]string{protocol.NetworkExpectationLabel: "required"}, BundleIdentity: s.bundleIdentity, AgentPort: uint32(7200 + i), ChildCID: uint32(40 + i), Storage: &protocol.StorageConfig{Port: uint32(4061 + i)}}, lock, nil
		}
	}
	lock.Close()
	return protocol.SandboxConfig{}, nil, errors.New("no disjoint Multikernel CPU set is available")
}

func (s *service) rollbackCreate(ctx context.Context, prepared *rootfspkg.CleanupRequest, lifecycleAttempted bool, config protocol.SandboxConfig, createKey string) error {
	var failures []error
	canCleanupPrepared := !lifecycleAttempted
	if err := s.stopNamespaceHolder(); err != nil {
		failures = append(failures, fmt.Errorf("stop network namespace holder: %w", err))
	}
	if err := s.releaseNetwork(ctx); err != nil {
		failures = append(failures, err)
	}
	if lifecycleAttempted {
		if err := s.cancelCreate(ctx, config, createKey); err != nil {
			failures = append(failures, fmt.Errorf("cancel allocated sandbox: %w", err))
		} else {
			canCleanupPrepared = true
			s.sandbox = protocol.Sandbox{}
		}
	} else if s.sandbox.ID != "" {
		if _, err := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-create-rollback-delete-"+s.sandbox.Generation, nil); err != nil && !daemonErrorCode(err, "NOT_FOUND") {
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
	boundIO, err := inspectBoundProcessIOPaths(r.Stdin, r.Stdout, r.Stderr)
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
	if err := s.ensureBundleIdentity(); err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrInvalidArgument, err)
	}
	netns, err := bundleNetworkNamespaceFrom(s.bundleDirectory)
	if err != nil {
		return nil, err
	}
	mounts, err := rootfsMounts(r.Rootfs)
	if err != nil {
		return nil, err
	}
	if err = s.validateRootfs(ctx, rootfspkg.ValidateRequest{Version: rootfspkg.Version, Bundle: r.Bundle,
		BundleIdentity: s.bundleIdentity}); err != nil {
		return nil, fmt.Errorf("validate OCI bundle before allocation: %w", err)
	}
	var prepared *rootfspkg.CleanupRequest
	lifecycleAttempted := false
	var config protocol.SandboxConfig
	var createKey string
	fail := true
	defer func() {
		if fail {
			if cleanupErr := s.rollbackCreate(context.WithoutCancel(ctx), prepared, lifecycleAttempted, config, createKey); cleanupErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("create rollback: %w", cleanupErr))
			}
		}
	}()
	config, lock, err := s.allocate(ctx, r.Bundle)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	identity := storageTaskIdentity(s.namespace, s.id)
	result, err := s.prepareRootfs(ctx, rootfspkg.PrepareRequest{Version: rootfspkg.Version, Bundle: r.Bundle, BundleIdentity: s.bundleIdentity,
		TaskIdentity: identity, StoragePort: config.Storage.Port, Mounts: mounts})
	if err != nil {
		return nil, err
	}
	config.Storage = &result.Storage
	prepared = &rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: r.Bundle, BundleIdentity: s.bundleIdentity, TaskIdentity: identity, StorageSHA256: result.Storage.SHA256}
	runtimeDirectory, err := s.ensureRuntimeDirectory()
	if err != nil {
		return nil, err
	}
	if err = verifyRuntimeDirectoryHandoff(runtimeDirectory, result.RuntimeIdentity); err != nil {
		return nil, err
	}
	token, _, err := loadOrCreateTokenFromDirectory(runtimeDirectory)
	if err != nil {
		return nil, err
	}
	s.token = token
	lifecycleAttempted = true
	createKey = createOperationKey(s.namespace, s.id, token)
	createClient := daemonCallerWithTimeout(s.daemon, createSandboxDaemonTimeout)
	created, err := daemon.Mutation(ctx, createClient, "CreateSandbox", "", "", createKey, &config)
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
	if err = s.ensureNamespaceHolder(); err != nil {
		return nil, err
	}
	s.bundle = r.Bundle
	s.processes[""] = &process{id: "", stdin: boundIO.stdin, stdout: boundIO.stdout, stderr: boundIO.stderr,
		stdinIdentity: boundIO.stdinIdentity, stdoutIdentity: boundIO.stdoutIdentity, stderrIdentity: boundIO.stderrIdentity,
		terminal: r.Terminal, status: tasktypes.Status_CREATED, done: make(chan struct{})}
	if err = s.persistRecovery(); err != nil {
		return nil, fmt.Errorf("persist recovery state: %w", err)
	}
	pid := s.taskPID("", s.processes[""])
	if err = s.publish(ctx, ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: s.id, Bundle: r.Bundle, Rootfs: r.Rootfs, Pid: pid}); err != nil {
		return nil, err
	}
	fail = false
	return &taskapi.CreateTaskResponse{Pid: pid}, nil
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
		if err := s.ensureNamespaceHolder(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if err := s.connectAgent(ctx); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if err := s.ensureGuestProcessCreated(ctx, "init", "/bundle"); err != nil {
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
	stateErr := s.callAgentWithReconnectContext(stateCtx, "StateProcess", map[string]string{"ID": processID}, &guestState)
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
	pid := s.taskPID(r.ExecID, p)
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
	operationID, err := newSignalOperationID()
	if err != nil {
		return fmt.Errorf("create cleanup signal identity: %w", err)
	}
	err = s.signalGuestProcess(cleanupCtx, processID, uint32(syscall.SIGKILL), operationID)
	if err == nil {
		err = s.acknowledgeGuestSignal(cleanupCtx, processID, operationID)
	} else {
		var remoteError *agent.RemoteError
		if errors.As(err, &remoteError) {
			err = errors.Join(err, s.acknowledgeGuestSignal(cleanupCtx, processID, operationID))
		}
	}
	if agentNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("kill started guest process: %w", err)
	}
	return nil
}
