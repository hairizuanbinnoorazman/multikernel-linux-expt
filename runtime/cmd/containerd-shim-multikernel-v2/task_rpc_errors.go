//go:build linux

package main

import (
	"context"
	"errors"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/runtime/v2/shim"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"os"
)

// taskRPCShim changes only the Task API error boundary. Manager startup/cleanup
// and internal service calls retain their original errors and recovery behavior.
// shim.Run embeds the returned Shim's TaskService, so this facade survives that
// registration without relying on an optional interceptor interface being kept.
type taskRPCShim struct{ delegate shim.Shim }

var _ shim.Shim = (*taskRPCShim)(nil)

func newTaskRPCShim(delegate shim.Shim) shim.Shim { return &taskRPCShim{delegate: delegate} }

type taskRPCOperation uint8

const (
	taskRPCState taskRPCOperation = iota + 1
	taskRPCCreate
	taskRPCStart
	taskRPCDelete
	taskRPCPids
	taskRPCPause
	taskRPCResume
	taskRPCCheckpoint
	taskRPCKill
	taskRPCExec
	taskRPCResizePty
	taskRPCCloseIO
	taskRPCUpdate
	taskRPCWait
	taskRPCStats
	taskRPCConnect
	taskRPCShutdown
)

// taskRPCFailure stores no cause, status details, request, paths or guest data.
// Even a malformed upstream status with code OK remains a non-nil Go error;
// GRPCStatus preserves the pinned transport's existing wire code in that case.
type taskRPCFailure struct {
	operation taskRPCOperation
	code      codes.Code
}

func (e *taskRPCFailure) Error() string {
	messages := [...]string{"task operation failed",
		"task State failed",
		"task Create failed",
		"task Start failed",
		"task Delete failed",
		"task Pids failed",
		"task Pause failed",
		"task Resume failed",
		"task Checkpoint failed",
		"task Kill failed",
		"task Exec failed",
		"task ResizePty failed",
		"task CloseIO failed",
		"task Update failed",
		"task Wait failed",
		"task Stats failed",
		"task Connect failed",
		"task Shutdown failed",
	}
	if int(e.operation) >= len(messages) {
		return messages[0]
	}
	return messages[e.operation]
}
func (e *taskRPCFailure) GRPCStatus() *status.Status { return status.New(e.code, e.Error()) }

// Match code selection in pinned ttrpc v1.2.7 services.go. In particular, its
// context/I/O switch uses direct sentinels, and raw containerd errdefs are not
// automatically translated. A privacy change must not silently change retry or
// not-found handling. Real wire oracle tests compare both registrations.
func taskRPCWireCode(err error) codes.Code {
	var grpcError interface{ GRPCStatus() *status.Status }
	if errors.As(err, &grpcError) {
		if observed := grpcError.GRPCStatus(); observed != nil {
			return observed.Code()
		}
	}
	switch err {
	case io.EOF:
		return codes.OutOfRange
	case io.ErrClosedPipe, io.ErrNoProgress, io.ErrShortBuffer, io.ErrShortWrite, io.ErrUnexpectedEOF:
		return codes.FailedPrecondition
	case os.ErrInvalid:
		return codes.InvalidArgument
	case context.Canceled:
		return codes.Canceled
	case context.DeadlineExceeded:
		return codes.DeadlineExceeded
	}
	switch {
	case os.IsExist(err):
		return codes.AlreadyExists
	case os.IsNotExist(err):
		return codes.NotFound
	case os.IsPermission(err):
		return codes.PermissionDenied
	default:
		return codes.Unknown
	}
}
func taskRPCResult[T any](operation taskRPCOperation, response *T, err error) (*T, error) {
	if err == nil {
		return response, nil
	}
	return nil, &taskRPCFailure{operation: operation, code: taskRPCWireCode(err)}
}
func (s *taskRPCShim) State(ctx context.Context, r *taskapi.StateRequest) (*taskapi.StateResponse, error) {
	response, err := s.delegate.State(ctx, r)
	return taskRPCResult(taskRPCState, response, err)
}
func (s *taskRPCShim) Create(ctx context.Context, r *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	response, err := s.delegate.Create(ctx, r)
	return taskRPCResult(taskRPCCreate, response, err)
}
func (s *taskRPCShim) Start(ctx context.Context, r *taskapi.StartRequest) (*taskapi.StartResponse, error) {
	response, err := s.delegate.Start(ctx, r)
	return taskRPCResult(taskRPCStart, response, err)
}
func (s *taskRPCShim) Delete(ctx context.Context, r *taskapi.DeleteRequest) (*taskapi.DeleteResponse, error) {
	response, err := s.delegate.Delete(ctx, r)
	return taskRPCResult(taskRPCDelete, response, err)
}
func (s *taskRPCShim) Pids(ctx context.Context, r *taskapi.PidsRequest) (*taskapi.PidsResponse, error) {
	response, err := s.delegate.Pids(ctx, r)
	return taskRPCResult(taskRPCPids, response, err)
}
func (s *taskRPCShim) Pause(ctx context.Context, r *taskapi.PauseRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Pause(ctx, r)
	return taskRPCResult(taskRPCPause, response, err)
}
func (s *taskRPCShim) Resume(ctx context.Context, r *taskapi.ResumeRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Resume(ctx, r)
	return taskRPCResult(taskRPCResume, response, err)
}
func (s *taskRPCShim) Checkpoint(ctx context.Context, r *taskapi.CheckpointTaskRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Checkpoint(ctx, r)
	return taskRPCResult(taskRPCCheckpoint, response, err)
}
func (s *taskRPCShim) Kill(ctx context.Context, r *taskapi.KillRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Kill(ctx, r)
	return taskRPCResult(taskRPCKill, response, err)
}
func (s *taskRPCShim) Exec(ctx context.Context, r *taskapi.ExecProcessRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Exec(ctx, r)
	return taskRPCResult(taskRPCExec, response, err)
}
func (s *taskRPCShim) ResizePty(ctx context.Context, r *taskapi.ResizePtyRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.ResizePty(ctx, r)
	return taskRPCResult(taskRPCResizePty, response, err)
}
func (s *taskRPCShim) CloseIO(ctx context.Context, r *taskapi.CloseIORequest) (*emptypb.Empty, error) {
	response, err := s.delegate.CloseIO(ctx, r)
	return taskRPCResult(taskRPCCloseIO, response, err)
}
func (s *taskRPCShim) Update(ctx context.Context, r *taskapi.UpdateTaskRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Update(ctx, r)
	return taskRPCResult(taskRPCUpdate, response, err)
}
func (s *taskRPCShim) Wait(ctx context.Context, r *taskapi.WaitRequest) (*taskapi.WaitResponse, error) {
	response, err := s.delegate.Wait(ctx, r)
	return taskRPCResult(taskRPCWait, response, err)
}
func (s *taskRPCShim) Stats(ctx context.Context, r *taskapi.StatsRequest) (*taskapi.StatsResponse, error) {
	response, err := s.delegate.Stats(ctx, r)
	return taskRPCResult(taskRPCStats, response, err)
}
func (s *taskRPCShim) Connect(ctx context.Context, r *taskapi.ConnectRequest) (*taskapi.ConnectResponse, error) {
	response, err := s.delegate.Connect(ctx, r)
	return taskRPCResult(taskRPCConnect, response, err)
}
func (s *taskRPCShim) Shutdown(ctx context.Context, r *taskapi.ShutdownRequest) (*emptypb.Empty, error) {
	response, err := s.delegate.Shutdown(ctx, r)
	return taskRPCResult(taskRPCShutdown, response, err)
}

// These manager operations are outside the Task RPC interface.
func (s *taskRPCShim) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	return s.delegate.Cleanup(ctx)
}
func (s *taskRPCShim) StartShim(ctx context.Context, opts shim.StartOpts) (string, error) {
	return s.delegate.StartShim(ctx, opts)
}
