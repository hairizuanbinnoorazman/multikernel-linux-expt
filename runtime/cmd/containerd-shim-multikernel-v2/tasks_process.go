//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func processSpec(p *specs.Process) agent.ProcessSpec {
	gids := make([]uint32, len(p.User.AdditionalGids))
	copy(gids, p.User.AdditionalGids)
	result := agent.ProcessSpec{Terminal: p.Terminal, User: agent.User{UID: p.User.UID, GID: p.User.GID, AdditionalGids: gids}, Args: p.Args, Env: p.Env, Cwd: p.Cwd}
	if p.ConsoleSize != nil {
		result.ConsoleSize = &agent.ConsoleSize{Width: uint32(p.ConsoleSize.Width), Height: uint32(p.ConsoleSize.Height)}
	}
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
	var unsupported []string
	if p.ConsoleSize != nil && (!p.Terminal || p.ConsoleSize.Width > 65535 || p.ConsoleSize.Height > 65535) {
		return fmt.Errorf("%w: console size requires a terminal and dimensions within the Linux PTY limit", errdefs.ErrInvalidArgument)
	}
	if p.CommandLine != "" {
		unsupported = append(unsupported, "commandLine")
	}
	// Docker copies these two container-level opt-out values into exec specs.
	// They request no child policy: AppArmor is explicitly unconfined and a
	// zero OOM adjustment is the kernel default.  Omit only those exact values
	// from the guest projection, matching the validated init-process contract.
	if p.ApparmorProfile != "" && p.ApparmorProfile != "unconfined" {
		unsupported = append(unsupported, "apparmorProfile")
	}
	if p.OOMScoreAdj != nil && *p.OOMScoreAdj != 0 {
		unsupported = append(unsupported, "oomScoreAdj")
	}
	if p.Scheduler != nil {
		unsupported = append(unsupported, "scheduler")
	}
	if p.SelinuxLabel != "" {
		unsupported = append(unsupported, "selinuxLabel")
	}
	if p.IOPriority != nil {
		unsupported = append(unsupported, "ioPriority")
	}
	if p.User.Umask != nil {
		unsupported = append(unsupported, "umask")
	}
	if p.User.Username != "" {
		unsupported = append(unsupported, "username")
	}
	if len(unsupported) != 0 {
		return fmt.Errorf("%w: unsupported exec process field(s): %s", errdefs.ErrNotImplemented, strings.Join(unsupported, ", "))
	}
	return nil
}

func agentNotFound(err error) bool {
	var remote *agent.RemoteError
	return errors.As(err, &remote) && remote.Failure.Code == "NOT_FOUND"
}

func (s *service) ensureGuestProcessCreated(ctx context.Context, processID, bundle string) error {
	var state agent.ProcessState
	err := s.callAgentWithReconnectContext(ctx, "StateProcess", map[string]string{"ID": processID}, &state)
	if err == nil {
		if err = createdGuestProcess(state, processID); err != nil {
			return fmt.Errorf("reconcile created guest process: %w", err)
		}
		return nil
	}
	if !agentNotFound(err) {
		return fmt.Errorf("inspect created guest process: %w", err)
	}
	createErr := s.agent.CallContext(ctx, "CreateProcess", map[string]any{"ID": processID, "Bundle": bundle}, nil)
	if createErr == nil {
		return nil
	}
	var remoteError *agent.RemoteError
	if errors.As(createErr, &remoteError) {
		return fmt.Errorf("create guest process: %w", createErr)
	}
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	state = agent.ProcessState{}
	reconcileErr := s.callAgentWithReconnectContext(reconcileCtx, "StateProcess", map[string]string{"ID": processID}, &state)
	if reconcileErr == nil {
		reconcileErr = createdGuestProcess(state, processID)
	}
	if reconcileErr != nil {
		return errors.Join(fmt.Errorf("create guest process: %w", createErr),
			fmt.Errorf("reconcile guest process creation: %w", reconcileErr))
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
	boundIO, err := inspectBoundProcessIOPaths(r.Stdin, r.Stdout, r.Stderr)
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
	p := &process{id: r.ExecID, stdin: boundIO.stdin, stdout: boundIO.stdout, stderr: boundIO.stderr,
		stdinIdentity: boundIO.stdinIdentity, stdoutIdentity: boundIO.stdoutIdentity, stderrIdentity: boundIO.stderrIdentity,
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
	responsePID := s.taskPID(r.ExecID, p)
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
		var teardownNotes []error
		var hostFailures []error
		if client != nil {
			if err := s.stopNetwork(); err != nil {
				teardownNotes = append(teardownNotes, err)
			}
			if err := s.quiesceAndShutdownGuest(ctx); err != nil {
				teardownNotes = append(teardownNotes, err)
			}
			closeErr := client.Close()
			s.mu.Lock()
			s.agent = nil
			s.mu.Unlock()
			if closeErr != nil {
				teardownNotes = append(teardownNotes, fmt.Errorf("close guest agent: %w", closeErr))
			}
		}
		if err := s.stopRelay(); err != nil {
			hostFailures = append(hostFailures, fmt.Errorf("stop agent relay: %w", err))
		}
		if err := s.releaseNetwork(ctx); err != nil {
			hostFailures = append(hostFailures, fmt.Errorf("release primary network endpoint: %w", err))
		}
		if _, err := daemon.Mutation(ctx, s.daemon, "StopSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-stop-"+s.sandbox.Generation, nil); err != nil && !daemonErrorCode(err, "NOT_FOUND") {
			hostFailures = append(hostFailures, fmt.Errorf("stop sandbox: %w", err))
		}
		if _, err := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-delete-"+s.sandbox.Generation, nil); err != nil && !daemonErrorCode(err, "NOT_FOUND") {
			hostFailures = append(hostFailures, fmt.Errorf("delete sandbox: %w", err))
		}
		if len(hostFailures) != 0 {
			return abort(errors.Join(append(hostFailures, teardownNotes...)...))
		}
		if note := errors.Join(teardownNotes...); note != nil {
			diagnostics := s.processDiagnostics("init")
			diagnostics.emit(diagnosticGuestTeardownWarnings, 0)
		}
	}
	s.mu.Lock()
	deleteQueued := p.deleteEventQueued
	s.mu.Unlock()
	if !deleteQueued {
		response := &taskapi.DeleteResponse{Pid: responsePID, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}
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
		request := rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: s.bundle, BundleIdentity: s.bundleIdentity,
			TaskIdentity: storageTaskIdentity(s.namespace, s.id), StorageSHA256: s.sandbox.Config.Storage.SHA256}
		if err := s.cleanupRootfs(ctx, request); err != nil {
			return abort(fmt.Errorf("cleanup prepared rootfs: %w", err))
		}
	}
	// Preserve the host task PID through every fallible host cleanup and
	// delete-event/rootfs step. A failed Delete retains ownership for retry;
	// it must not point at a holder that this call already killed or reaped.
	if r.ExecID == "" {
		if err := s.stopNamespaceHolder(); err != nil {
			return abort(fmt.Errorf("stop network namespace holder: %w", err))
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
	resp := &taskapi.DeleteResponse{Pid: responsePID, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}
	return resp, nil
}
