//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/ttrpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var privateTaskRPCPayload = strings.Repeat("SYNTHETIC_TASK_RPC_TOKEN /private/task/credential\n", 256)

type taskRPCBackend struct {
	shim.Shim
	mu       sync.Mutex
	failure  error
	results  map[string]proto.Message
	requests map[string]any
	contexts map[string]context.Context
	calls    map[string]int
}

func (b *taskRPCBackend) record(method string, ctx context.Context, request any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.requests == nil {
		b.requests = map[string]any{}
		b.contexts = map[string]context.Context{}
	}
	b.calls[method]++
	b.requests[method] = request
	b.contexts[method] = ctx
	return b.failure
}

// Exercise the same facade factory used by newService before shim.Run registration.
func taskRPCUnderTest(s shim.Shim) shim.Shim { return newTaskRPCShim(s) }
func (b *taskRPCBackend) State(ctx context.Context, r *taskapi.StateRequest) (*taskapi.StateResponse, error) {
	err := b.record("State", ctx, r)
	if result := b.results["State"]; result != nil {
		return result.(*taskapi.StateResponse), err
	}
	return &taskapi.StateResponse{}, err
}
func (b *taskRPCBackend) Create(ctx context.Context, r *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	err := b.record("Create", ctx, r)
	if result := b.results["Create"]; result != nil {
		return result.(*taskapi.CreateTaskResponse), err
	}
	return &taskapi.CreateTaskResponse{}, err
}
func (b *taskRPCBackend) Start(ctx context.Context, r *taskapi.StartRequest) (*taskapi.StartResponse, error) {
	err := b.record("Start", ctx, r)
	if result := b.results["Start"]; result != nil {
		return result.(*taskapi.StartResponse), err
	}
	return &taskapi.StartResponse{}, err
}
func (b *taskRPCBackend) Delete(ctx context.Context, r *taskapi.DeleteRequest) (*taskapi.DeleteResponse, error) {
	err := b.record("Delete", ctx, r)
	if result := b.results["Delete"]; result != nil {
		return result.(*taskapi.DeleteResponse), err
	}
	return &taskapi.DeleteResponse{}, err
}
func (b *taskRPCBackend) Pids(ctx context.Context, r *taskapi.PidsRequest) (*taskapi.PidsResponse, error) {
	err := b.record("Pids", ctx, r)
	if result := b.results["Pids"]; result != nil {
		return result.(*taskapi.PidsResponse), err
	}
	return &taskapi.PidsResponse{}, err
}
func (b *taskRPCBackend) Pause(ctx context.Context, r *taskapi.PauseRequest) (*emptypb.Empty, error) {
	err := b.record("Pause", ctx, r)
	if result := b.results["Pause"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Resume(ctx context.Context, r *taskapi.ResumeRequest) (*emptypb.Empty, error) {
	err := b.record("Resume", ctx, r)
	if result := b.results["Resume"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Checkpoint(ctx context.Context, r *taskapi.CheckpointTaskRequest) (*emptypb.Empty, error) {
	err := b.record("Checkpoint", ctx, r)
	if result := b.results["Checkpoint"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Kill(ctx context.Context, r *taskapi.KillRequest) (*emptypb.Empty, error) {
	err := b.record("Kill", ctx, r)
	if result := b.results["Kill"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Exec(ctx context.Context, r *taskapi.ExecProcessRequest) (*emptypb.Empty, error) {
	err := b.record("Exec", ctx, r)
	if result := b.results["Exec"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) ResizePty(ctx context.Context, r *taskapi.ResizePtyRequest) (*emptypb.Empty, error) {
	err := b.record("ResizePty", ctx, r)
	if result := b.results["ResizePty"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) CloseIO(ctx context.Context, r *taskapi.CloseIORequest) (*emptypb.Empty, error) {
	err := b.record("CloseIO", ctx, r)
	if result := b.results["CloseIO"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Update(ctx context.Context, r *taskapi.UpdateTaskRequest) (*emptypb.Empty, error) {
	err := b.record("Update", ctx, r)
	if result := b.results["Update"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}
func (b *taskRPCBackend) Wait(ctx context.Context, r *taskapi.WaitRequest) (*taskapi.WaitResponse, error) {
	err := b.record("Wait", ctx, r)
	if result := b.results["Wait"]; result != nil {
		return result.(*taskapi.WaitResponse), err
	}
	return &taskapi.WaitResponse{}, err
}
func (b *taskRPCBackend) Stats(ctx context.Context, r *taskapi.StatsRequest) (*taskapi.StatsResponse, error) {
	err := b.record("Stats", ctx, r)
	if result := b.results["Stats"]; result != nil {
		return result.(*taskapi.StatsResponse), err
	}
	return &taskapi.StatsResponse{}, err
}
func (b *taskRPCBackend) Connect(ctx context.Context, r *taskapi.ConnectRequest) (*taskapi.ConnectResponse, error) {
	err := b.record("Connect", ctx, r)
	if result := b.results["Connect"]; result != nil {
		return result.(*taskapi.ConnectResponse), err
	}
	return &taskapi.ConnectResponse{}, err
}
func (b *taskRPCBackend) Shutdown(ctx context.Context, r *taskapi.ShutdownRequest) (*emptypb.Empty, error) {
	err := b.record("Shutdown", ctx, r)
	if result := b.results["Shutdown"]; result != nil {
		return result.(*emptypb.Empty), err
	}
	return &emptypb.Empty{}, err
}

func taskWireClient(t *testing.T, backend shim.Shim, protect bool) *ttrpc.Client {
	t.Helper()
	directory, err := os.MkdirTemp("", "task-rpc-")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(directory, "rpc.sock"))
	if err != nil {
		os.RemoveAll(directory)
		t.Fatal(err)
	}
	server, err := ttrpc.NewServer()
	if err != nil {
		listener.Close()
		os.RemoveAll(directory)
		t.Fatal(err)
	}
	if protect {
		backend = taskRPCUnderTest(backend)
	}
	taskapi.RegisterTaskService(server, backend)
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(context.Background(), listener) }()
	connection, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	client := ttrpc.NewClient(connection)
	t.Cleanup(func() {
		client.Close()
		server.Close()
		listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("Task RPC server retained")
		}
		os.RemoveAll(directory)
	})
	return client
}
func TestTaskRPCAllMethodsOmitPrivateFailuresOnWire(t *testing.T) {
	detailed, err := status.New(codes.PermissionDenied, privateTaskRPCPayload).WithDetails(wrapperspb.String(privateTaskRPCPayload))
	if err != nil {
		t.Fatal("build private status control")
	}
	backend := &taskRPCBackend{failure: detailed.Err(), calls: map[string]int{}}
	client := taskWireClient(t, backend, true)
	for _, test := range []struct {
		name              string
		request, response proto.Message
	}{
		{"State", &taskapi.StateRequest{}, &taskapi.StateResponse{}},
		{"Create", &taskapi.CreateTaskRequest{}, &taskapi.CreateTaskResponse{}},
		{"Start", &taskapi.StartRequest{}, &taskapi.StartResponse{}},
		{"Delete", &taskapi.DeleteRequest{}, &taskapi.DeleteResponse{}},
		{"Pids", &taskapi.PidsRequest{}, &taskapi.PidsResponse{}},
		{"Pause", &taskapi.PauseRequest{}, &emptypb.Empty{}},
		{"Resume", &taskapi.ResumeRequest{}, &emptypb.Empty{}},
		{"Checkpoint", &taskapi.CheckpointTaskRequest{}, &emptypb.Empty{}},
		{"Kill", &taskapi.KillRequest{}, &emptypb.Empty{}},
		{"Exec", &taskapi.ExecProcessRequest{}, &emptypb.Empty{}},
		{"ResizePty", &taskapi.ResizePtyRequest{}, &emptypb.Empty{}},
		{"CloseIO", &taskapi.CloseIORequest{}, &emptypb.Empty{}},
		{"Update", &taskapi.UpdateTaskRequest{}, &emptypb.Empty{}},
		{"Wait", &taskapi.WaitRequest{}, &taskapi.WaitResponse{}},
		{"Stats", &taskapi.StatsRequest{}, &taskapi.StatsResponse{}},
		{"Connect", &taskapi.ConnectRequest{}, &taskapi.ConnectResponse{}},
		{"Shutdown", &taskapi.ShutdownRequest{}, &emptypb.Empty{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			issue := client.Call(ctx, "containerd.task.v2.Task", test.name, test.request, test.response)
			result := status.Convert(issue)
			if result.Code() != codes.PermissionDenied {
				t.Fatal("Task wire error code changed")
			}
			if len(result.Message()) > 128 || strings.Contains(result.Message(), "SYNTHETIC_TASK_RPC_TOKEN") || strings.Contains(result.Message(), "/private/task/credential") || len(result.Details()) != 0 {
				t.Fatal("Task wire error exposes private message/details")
			}
		})
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.calls) != 17 {
		t.Fatal("Task method coverage incomplete")
	}
	for _, count := range backend.calls {
		if count != 1 {
			t.Fatal("Task boundary retried or skipped delegate")
		}
	}
}
func TestTaskRPCStatusCodesMatchPinnedTransport(t *testing.T) {
	failures := []error{taskRPCStatusCarrier{code: codes.OK}, taskRPCStatusCarrier{nilStatus: true}, taskRPCStatusCarrier{code: codes.Code(87)}, errdefs.ErrNotFound, errors.New(privateTaskRPCPayload), fmt.Errorf("%s: %w", privateTaskRPCPayload, errdefs.ErrNotFound), context.Canceled, context.DeadlineExceeded, fmt.Errorf("%s: %w", privateTaskRPCPayload, context.Canceled), io.EOF, io.ErrClosedPipe, io.ErrNoProgress, io.ErrShortBuffer, io.ErrShortWrite, io.ErrUnexpectedEOF, os.ErrInvalid, &os.PathError{Op: "open", Path: privateTaskRPCPayload, Err: os.ErrNotExist}, &os.PathError{Op: "open", Path: privateTaskRPCPayload, Err: os.ErrPermission}, &os.PathError{Op: "open", Path: privateTaskRPCPayload, Err: os.ErrExist}, fmt.Errorf("%s: %w", privateTaskRPCPayload, status.Error(codes.ResourceExhausted, privateTaskRPCPayload))}
	for index, failure := range failures {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			backend := &taskRPCBackend{failure: failure, calls: map[string]int{}}
			raw := taskWireClient(t, backend, false)
			protected := taskWireClient(t, backend, true)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			first := raw.Call(ctx, "containerd.task.v2.Task", "State", &taskapi.StateRequest{}, &taskapi.StateResponse{})
			second := protected.Call(ctx, "containerd.task.v2.Task", "State", &taskapi.StateRequest{}, &taskapi.StateResponse{})
			if status.Code(first) != status.Code(second) {
				t.Fatal("Task boundary changed pinned transport status mapping")
			}
			if strings.Contains(status.Convert(second).Message(), "SYNTHETIC_TASK_RPC_TOKEN") || len(status.Convert(second).Message()) > 128 {
				t.Fatal("Task boundary leaked wrapped cause")
			}
		})
	}
}

func TestTaskRPCSuccessAndPartialFailureDelegateExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		name              string
		request, response proto.Message
	}{
		{"State", &taskapi.StateRequest{}, &taskapi.StateResponse{}},
		{"Create", &taskapi.CreateTaskRequest{}, &taskapi.CreateTaskResponse{}},
		{"Start", &taskapi.StartRequest{}, &taskapi.StartResponse{}},
		{"Delete", &taskapi.DeleteRequest{}, &taskapi.DeleteResponse{}},
		{"Pids", &taskapi.PidsRequest{}, &taskapi.PidsResponse{}},
		{"Pause", &taskapi.PauseRequest{}, &emptypb.Empty{}},
		{"Resume", &taskapi.ResumeRequest{}, &emptypb.Empty{}},
		{"Checkpoint", &taskapi.CheckpointTaskRequest{}, &emptypb.Empty{}},
		{"Kill", &taskapi.KillRequest{}, &emptypb.Empty{}},
		{"Exec", &taskapi.ExecProcessRequest{}, &emptypb.Empty{}},
		{"ResizePty", &taskapi.ResizePtyRequest{}, &emptypb.Empty{}},
		{"CloseIO", &taskapi.CloseIORequest{}, &emptypb.Empty{}},
		{"Update", &taskapi.UpdateTaskRequest{}, &emptypb.Empty{}},
		{"Wait", &taskapi.WaitRequest{}, &taskapi.WaitResponse{}},
		{"Stats", &taskapi.StatsRequest{}, &taskapi.StatsResponse{}},
		{"Connect", &taskapi.ConnectRequest{}, &taskapi.ConnectResponse{}},
		{"Shutdown", &taskapi.ShutdownRequest{}, &emptypb.Empty{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, failure := range []error{nil, errors.New(privateTaskRPCPayload)} {
				backend := &taskRPCBackend{failure: failure, calls: map[string]int{}, results: map[string]proto.Message{test.name: test.response}}
				facade := newTaskRPCShim(backend)
				ctx, cancel := context.WithCancel(context.Background())
				returned := reflect.ValueOf(facade).MethodByName(test.name).Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(test.request)})
				cancel()
				if failure == nil {
					if returned[0].Interface() != test.response || !returned[1].IsNil() {
						t.Fatal("successful Task result changed")
					}
				} else {
					if !returned[0].IsNil() || returned[1].IsNil() {
						t.Fatal("Task failure retained partial response")
					}
					issue := returned[1].Interface().(error)
					if errors.Unwrap(issue) != nil || strings.Contains(issue.Error(), "SYNTHETIC_TASK_RPC_TOKEN") {
						t.Fatal("Task error retained private cause")
					}
				}
				if backend.calls[test.name] != 1 || backend.requests[test.name] != test.request || backend.contexts[test.name] != ctx {
					t.Fatal("Task facade changed request/context or retried delegate")
				}
			}
		})
	}
}

type taskRPCUnprintableError struct{}

func (taskRPCUnprintableError) Error() string { panic("Task cause formatted") }
func TestTaskRPCFailureDoesNotFormatPrivateCause(t *testing.T) {
	result, issue := taskRPCResult(taskRPCState, &taskapi.StateResponse{Bundle: privateTaskRPCPayload}, taskRPCUnprintableError{})
	if result != nil || issue == nil || errors.Unwrap(issue) != nil || status.Code(issue) != codes.Unknown || len(issue.Error()) > 128 {
		t.Fatal("Task boundary retained or formatted private cause")
	}
}

func TestTaskRPCNewServiceReturnsProtectedFacade(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Chmod(bundle, 0700); err != nil {
		t.Fatal("protect constructor fixture bundle")
	}
	t.Chdir(bundle)
	for _, name := range []string{"MK_SHIM_BUNDLE_DEVICE", "MK_SHIM_BUNDLE_INODE", "MK_SHIM_BUNDLE_UID"} {
		t.Setenv(name, "")
	}
	ctx := namespaces.WithNamespace(context.Background(), "task-rpc-test")
	created, err := newService(ctx, "task", nil, func() {})
	if err != nil {
		t.Fatal("service constructor failed with protected fixture")
	}
	facade, ok := created.(*taskRPCShim)
	if !ok {
		t.Fatal("service constructor bypasses Task facade")
	}
	underlying, ok := facade.delegate.(*service)
	if !ok {
		t.Fatal("Task facade lost concrete service")
	}
	defer underlying.closeBundleIdentity()
	defer underlying.stopEventRetry()
}

type taskRPCStatusCarrier struct {
	code      codes.Code
	nilStatus bool
}

func (e taskRPCStatusCarrier) Error() string { return privateTaskRPCPayload }
func (e taskRPCStatusCarrier) GRPCStatus() *status.Status {
	if e.nilStatus {
		return nil
	}
	return status.New(e.code, privateTaskRPCPayload)
}

func (b *taskRPCBackend) StartShim(ctx context.Context, opts shim.StartOpts) (string, error) {
	return "manager-address", b.record("ManagerStartShim", ctx, opts)
}
func (b *taskRPCBackend) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	return &taskapi.DeleteResponse{Pid: 42}, b.record("ManagerCleanup", ctx, nil)
}
func TestTaskRPCManagerOperationsRetainDelegateContract(t *testing.T) {
	failure := errors.New(privateTaskRPCPayload)
	backend := &taskRPCBackend{failure: failure, calls: map[string]int{}}
	facade := newTaskRPCShim(backend)
	ctx := context.Background()
	opts := shim.StartOpts{ID: "task", Address: "private-control-address"}
	address, err := facade.StartShim(ctx, opts)
	if address != "manager-address" || err != failure || backend.requests["ManagerStartShim"] != opts {
		t.Fatal("manager startup delegate changed")
	}
	result, err := facade.Cleanup(ctx)
	if result == nil || result.Pid != 42 || err != failure || backend.calls["ManagerCleanup"] != 1 || backend.calls["ManagerStartShim"] != 1 {
		t.Fatal("manager cleanup delegate changed")
	}
}
