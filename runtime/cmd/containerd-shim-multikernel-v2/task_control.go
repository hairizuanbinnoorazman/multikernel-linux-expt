//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"syscall"
	"time"

	cgroupstats "github.com/containerd/cgroups/stats/v1"
	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	return &taskapi.StateResponse{ID: s.id, Bundle: s.bundle, Pid: s.taskPID(r.ExecID, p), Status: p.status, Stdin: p.stdin, Stdout: p.stdout, Stderr: p.stderr, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited), ExecID: r.ExecID}, nil
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
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_RUNNING && p.status != tasktypes.Status_PAUSED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if s.agent == nil {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	operationID := p.pendingSignalOperation
	if operationID != "" && p.pendingSignal != r.Signal {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: a different signal remains unacknowledged", errdefs.ErrFailedPrecondition)
	}
	if operationID == "" {
		var err error
		operationID, err = newSignalOperationID()
		if err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("create signal operation identity: %w", err)
		}
		p.pendingSignal, p.pendingSignalOperation, p.pendingSignalAcknowledgement = r.Signal, operationID, false
		if err = s.persistRecovery(); err != nil {
			p.pendingSignal, p.pendingSignalOperation = 0, ""
			s.mu.Unlock()
			return nil, fmt.Errorf("persist signal intent: %w", err)
		}
	}
	resultObserved := p.pendingSignalAcknowledgement
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	var signalErr error
	if !resultObserved {
		signalErr = s.signalGuestProcess(ctx, id, r.Signal, operationID)
		var remoteError *agent.RemoteError
		if signalErr != nil && !errors.As(signalErr, &remoteError) {
			return nil, signalErr
		}
		s.mu.Lock()
		if current, exists := s.processes[r.ExecID]; exists && current == p && current.pendingSignalOperation == operationID {
			current.pendingSignalAcknowledgement = true
			if err := s.persistRecovery(); err != nil {
				current.pendingSignalAcknowledgement = false
				s.mu.Unlock()
				return nil, errors.Join(signalErr, fmt.Errorf("persist observed signal result: %w", err))
			}
		}
		s.mu.Unlock()
	}
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	ackErr := s.acknowledgeGuestSignal(ackCtx, id, operationID)
	cancel()
	if ackErr != nil {
		return nil, errors.Join(signalErr, fmt.Errorf("acknowledge guest signal result: %w", ackErr))
	}
	s.mu.Lock()
	var clearErr error
	if current, exists := s.processes[r.ExecID]; exists && current == p && current.pendingSignalOperation == operationID {
		current.pendingSignal, current.pendingSignalOperation, current.pendingSignalAcknowledgement = 0, "", false
		if clearErr = s.persistRecovery(); clearErr != nil {
			current.pendingSignal, current.pendingSignalOperation, current.pendingSignalAcknowledgement = r.Signal, operationID, true
		}
	}
	s.mu.Unlock()
	if clearErr != nil {
		clearErr = fmt.Errorf("persist retired signal intent: %w", clearErr)
	}
	if err := errors.Join(signalErr, clearErr); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
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
	for execID, process := range s.processes {
		if pid := s.taskPID(execID, process); pid != 0 {
			processes = append(processes, &tasktypes.ProcessInfo{Pid: pid})
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
		if err := s.ensureNamespaceHolder(); err != nil {
			return nil, err
		}
		taskPID = s.taskPID("", init)
	}
	return &taskapi.ConnectResponse{ShimPid: uint32(os.Getpid()), TaskPid: taskPID, Version: "multikernel-v2-host-netns-pid"}, nil
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
		s.closeBundleIdentity()
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
	if p.status == tasktypes.Status_RUNNING && s.agent == nil {
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
	if err := s.resizeGuestProcess(ctx, id, r.Width, r.Height); err != nil {
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

func (s *service) signalProcessTargets(ctx context.Context, targets []processSignalTarget, signal syscall.Signal) (int, error) {
	for index, target := range targets {
		if err := s.applyGuestSignal(ctx, target.id, uint32(signal)); err != nil {
			return index, err
		}
	}
	return len(targets), nil
}

func (s *service) rollbackProcessSignals(targets []processSignalTarget, signal syscall.Signal) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var failures []error
	for index := len(targets) - 1; index >= 0; index-- {
		err := s.applyGuestSignal(ctx, targets[index].id, uint32(signal))
		if err != nil {
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
	signaled, err := s.signalProcessTargets(ctx, targets, syscall.SIGSTOP)
	if err != nil {
		return nil, errors.Join(err, s.rollbackProcessSignals(targets[:signaled], syscall.SIGCONT))
	}
	for _, target := range targets {
		target.process.status = tasktypes.Status_PAUSED
	}
	if err := s.persistRecovery(); err != nil {
		rollbackErr := s.rollbackProcessSignals(targets, syscall.SIGCONT)
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
		rollbackErr := s.rollbackProcessSignals(targets, syscall.SIGCONT)
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
	signaled, err := s.signalProcessTargets(ctx, targets, syscall.SIGCONT)
	if err != nil {
		return nil, errors.Join(err, s.rollbackProcessSignals(targets[:signaled], syscall.SIGSTOP))
	}
	for _, target := range targets {
		target.process.status = tasktypes.Status_RUNNING
	}
	if err := s.persistRecovery(); err != nil {
		rollbackErr := s.rollbackProcessSignals(targets, syscall.SIGSTOP)
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
		rollbackErr := s.rollbackProcessSignals(targets, syscall.SIGSTOP)
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
		var buffered int
		if file, ok := p.stdinReader.(*os.File); ok {
			raw, err := file.SyscallConn()
			if err != nil {
				s.mu.Unlock()
				return nil, err
			}
			var inspectErr error
			if err = raw.Control(func(fd uintptr) { buffered, inspectErr = unix.IoctlGetInt(int(fd), unix.TIOCINQ) }); err != nil || inspectErr != nil {
				s.mu.Unlock()
				return nil, errors.Join(err, inspectErr)
			}
			if buffered < 0 || buffered > maxStdinCloseDrain {
				s.mu.Unlock()
				return nil, fmt.Errorf("buffered stdin exceeds the close-drain limit")
			}
		}
		p.stdinDrainRemaining = uint32(buffered)
		p.stdinClosed = true
		if err := s.persistRecovery(); err != nil {
			p.stdinClosed = false
			p.stdinDrainRemaining = 0
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
		if err := s.callAgentWithReconnectContext(ctx, "StatsProcess", map[string]string{"ID": id}, &observed); err != nil {
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
