//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	cgroupstats "github.com/containerd/cgroups/stats/v1"
	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/events"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/fifo"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestMain(m *testing.M) {
	if marker := os.Getenv("MK_SHIM_SUPERVISOR_TEST_MARKER"); marker != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("first-worker-signaled\n"), 0600)
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			os.Exit(99)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fakeAgentClient struct {
	calls     []string
	fail      map[string]error
	stats     agent.ProcessStats
	statsByID map[string]agent.ProcessStats
}

type shutdownBoundaryAgent struct {
	calls               []string
	closeCalls          int
	configureCalls      int
	deleteCalls         int
	quiesceCalls        int
	reconnects          int
	loseFirstClose      bool
	loseFirstConfigure  bool
	loseFirstDelete     bool
	loseFirstQuiesce    bool
	loseShutdown        bool
	quiesceStatus       string
	shutdownRemoteError error
	afterQuiesce        func()
}

type execCreationAgent struct {
	bundle       string
	calls        []string
	createErr    error
	state        agent.ProcessState
	stateErr     error
	deleteErr    error
	durableOwner bool
}

func (f *execCreationAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *execCreationAgent) CallContext(_ context.Context, method string, request, response any) error {
	f.calls = append(f.calls, method)
	switch method {
	case "ExecProcess":
		data, err := os.ReadFile(filepath.Join(f.bundle, ".multikernel", "sandbox.json"))
		if err == nil {
			var saved persisted
			if err = json.Unmarshal(data, &saved); err == nil {
				id := request.(map[string]any)["id"].(string)
				for _, process := range saved.Processes {
					f.durableOwner = f.durableOwner || process.ID == id && process.Status == tasktypes.Status_CREATED
				}
			}
		}
		return f.createErr
	case "StateProcess":
		if f.stateErr != nil {
			return f.stateErr
		}
		return setJSONResponse(response, f.state)
	case "DeleteProcess":
		return f.deleteErr
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*execCreationAgent) Close() error                                   { return nil }
func (*execCreationAgent) Reconnect(string) error                         { return nil }
func (*execCreationAgent) ReconnectContext(context.Context, string) error { return nil }

func (f *shutdownBoundaryAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *shutdownBoundaryAgent) CallContext(ctx context.Context, method string, _ any, response any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.calls = append(f.calls, method)
	switch method {
	case "ConfigureNetwork":
		f.configureCalls++
		if f.loseFirstConfigure && f.configureCalls == 1 {
			return io.ErrUnexpectedEOF
		}
		return nil
	case "DeleteProcess":
		f.deleteCalls++
		if f.loseFirstDelete && f.deleteCalls == 1 {
			return io.ErrUnexpectedEOF
		}
		return &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}
	case "CloseNetwork":
		f.closeCalls++
		if f.loseFirstClose && f.closeCalls == 1 {
			return io.ErrUnexpectedEOF
		}
		return nil
	case "Quiesce":
		f.quiesceCalls++
		if f.loseFirstQuiesce && f.quiesceCalls == 1 {
			return io.ErrUnexpectedEOF
		}
		status := f.quiesceStatus
		if status == "" {
			status = "quiesced"
		}
		err := setJSONResponse(response, guestQuiesceResponse{Status: status})
		if f.afterQuiesce != nil {
			f.afterQuiesce()
		}
		return err
	case "Shutdown":
		if f.shutdownRemoteError != nil {
			return f.shutdownRemoteError
		}
		if f.loseShutdown {
			return io.EOF
		}
		return setJSONResponse(response, guestQuiesceResponse{Status: "quiesced"})
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*shutdownBoundaryAgent) Close() error           { return nil }
func (*shutdownBoundaryAgent) Reconnect(string) error { return nil }
func (f *shutdownBoundaryAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

type stdinCaptureAgent struct{ writes chan []byte }

func (f *stdinCaptureAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *stdinCaptureAgent) CallContext(_ context.Context, method string, request, response any) error {
	if method != "WriteProcess" {
		return nil
	}
	values := request.(map[string]any)
	value, ok := values["data"].([]byte)
	if !ok {
		return errors.New("stdin write has invalid data")
	}
	f.writes <- append([]byte(nil), value...)
	offset := values["offset"].(uint64) + uint64(len(value))
	return setJSONResponse(response, map[string]uint64{"offset": offset})
}
func (f *stdinCaptureAgent) Close() error                                   { return nil }
func (f *stdinCaptureAgent) Reconnect(string) error                         { return nil }
func (f *stdinCaptureAgent) ReconnectContext(context.Context, string) error { return nil }

type controlledInput struct{ reads chan chan []byte }

func (f *controlledInput) Read(target []byte) (int, error) {
	value := make(chan []byte, 1)
	f.reads <- value
	return copy(target, <-value), nil
}
func (*controlledInput) Write(value []byte) (int, error) { return len(value), nil }
func (*controlledInput) Close() error                    { return nil }

type outputReconnectAgent struct {
	readFailures      int
	waitFailures      int
	closeFailures     int
	reconnectFailures int
	remoteReadFailure bool
	readCalls         int
	waitCalls         int
	closeCalls        int
	reconnects        int
}

type waitOutageAgent struct {
	observed  chan struct{}
	recovered chan struct{}
	once      sync.Once
}

type waitValidationAgent struct {
	reads  chan struct{}
	states chan agent.ProcessState
}

func (f *waitValidationAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *waitValidationAgent) CallContext(ctx context.Context, method string, _ any, response any) error {
	switch method {
	case "ReadProcessOutput":
		select {
		case f.reads <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return setJSONResponse(response, processOutput{Status: "STOPPED"})
	case "WaitProcess":
		select {
		case state := <-f.states:
			return setJSONResponse(response, state)
		case <-ctx.Done():
			return ctx.Err()
		}
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*waitValidationAgent) Close() error                                   { return nil }
func (*waitValidationAgent) Reconnect(string) error                         { return nil }
func (*waitValidationAgent) ReconnectContext(context.Context, string) error { return nil }

func (f *waitOutageAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *waitOutageAgent) CallContext(ctx context.Context, method string, request, response any) error {
	switch method {
	case "ReadProcessOutput":
		f.once.Do(func() { close(f.observed) })
		select {
		case <-f.recovered:
			return setJSONResponse(response, map[string]any{
				"stdout_offset": uint64(0), "stderr_offset": uint64(0), "status": "STOPPED",
			})
		case <-ctx.Done():
			return ctx.Err()
		}
	case "WaitProcess":
		return setStoppedResponse(request, response, 41, 37)
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*waitOutageAgent) Close() error                                   { return nil }
func (*waitOutageAgent) Reconnect(string) error                         { return nil }
func (*waitOutageAgent) ReconnectContext(context.Context, string) error { return nil }

type outputAckAgent struct{ offsets chan uint64 }

func (f *outputAckAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *outputAckAgent) CallContext(_ context.Context, method string, request, response any) error {
	switch method {
	case "ReadProcessOutput":
		offset := request.(map[string]any)["stdout_offset"].(uint64)
		f.offsets <- offset
		if offset == 0 {
			return setJSONResponse(response, processOutput{Stdout: []byte("once"), StdoutOffset: 4, Status: "RUNNING"})
		}
		if offset == 4 {
			return setJSONResponse(response, processOutput{StdoutOffset: 4, Status: "STOPPED"})
		}
		return errors.New("unexpected output offset")
	case "WaitProcess":
		return setStoppedResponse(request, response, 41, 11)
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*outputAckAgent) Close() error                                   { return nil }
func (*outputAckAgent) Reconnect(string) error                         { return nil }
func (*outputAckAgent) ReconnectContext(context.Context, string) error { return nil }

type controlledOutputWriter struct{ writes chan controlledOutputWrite }
type controlledOutputWrite struct {
	data    []byte
	release chan struct{}
}

func (f *controlledOutputWriter) Write(value []byte) (int, error) {
	write := controlledOutputWrite{data: append([]byte(nil), value...), release: make(chan struct{})}
	f.writes <- write
	<-write.release
	return len(value), nil
}
func (*controlledOutputWriter) Close() error { return nil }

type startCleanupAgent struct {
	finish    chan struct{}
	startErr  error
	stateErr  error
	signalErr error
	statePID  int
	stateID   string
	state     string
}

type createReconcileAgent struct {
	state       agent.ProcessState
	stateErr    error
	createErr   error
	stateCalls  int
	createCalls int
}

func (f *createReconcileAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *createReconcileAgent) CallContext(_ context.Context, method string, _ any, response any) error {
	switch method {
	case "StateProcess":
		f.stateCalls++
		if f.stateErr != nil {
			return f.stateErr
		}
		return setJSONResponse(response, f.state)
	case "CreateProcess":
		f.createCalls++
		return f.createErr
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*createReconcileAgent) Close() error                                   { return nil }
func (*createReconcileAgent) Reconnect(string) error                         { return nil }
func (*createReconcileAgent) ReconnectContext(context.Context, string) error { return nil }

func (f *startCleanupAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *startCleanupAgent) CallContext(ctx context.Context, method string, request, response any) error {
	switch method {
	case "StartProcess":
		return f.startErr
	case "StateProcess":
		if f.stateErr != nil {
			return f.stateErr
		}
		pid := f.statePID
		if pid == 0 && f.state != "CREATED" {
			pid = 41
		}
		id := f.stateID
		if id == "" {
			id = request.(map[string]string)["ID"]
		}
		status := f.state
		if status == "" {
			status = "RUNNING"
		}
		return setJSONResponse(response, agent.ProcessState{ID: id, PID: pid, Status: status})
	case "SignalProcess":
		return f.signalErr
	case "ReadProcessOutput":
		select {
		case <-f.finish:
			return setJSONResponse(response, processOutput{Status: "STOPPED"})
		case <-ctx.Done():
			return ctx.Err()
		}
	case "WaitProcess":
		return setStoppedResponse(request, response, 41, 9)
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (*startCleanupAgent) Close() error                                   { return nil }
func (*startCleanupAgent) Reconnect(string) error                         { return nil }
func (*startCleanupAgent) ReconnectContext(context.Context, string) error { return nil }

func (f *outputReconnectAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *outputReconnectAgent) CallContext(ctx context.Context, method string, request, response any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch method {
	case "ReadProcessOutput":
		f.readCalls++
		if f.remoteReadFailure {
			return &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}
		}
		if f.readCalls <= f.readFailures {
			return errors.New("injected output disconnect")
		}
		values := request.(map[string]any)
		if values["stdout_offset"] != uint64(7) || values["stderr_offset"] != uint64(9) {
			return errors.New("output retry changed acknowledged offsets")
		}
		return setJSONResponse(response, map[string]any{"stdout": []byte("after-reconnect"), "stdout_offset": uint64(22), "stderr_offset": uint64(9), "status": "RUNNING"})
	case "WaitProcess":
		f.waitCalls++
		if f.waitCalls <= f.waitFailures {
			return errors.New("injected wait disconnect")
		}
		return setStoppedResponse(request, response, 41, 19)
	case "CloseProcessStdin":
		f.closeCalls++
		if f.closeCalls <= f.closeFailures {
			return errors.New("injected close disconnect")
		}
		return nil
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}
func (f *outputReconnectAgent) Close() error           { return nil }
func (f *outputReconnectAgent) Reconnect(string) error { return nil }
func (f *outputReconnectAgent) ReconnectContext(ctx context.Context, _ string) error {
	f.reconnects++
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.reconnects <= f.reconnectFailures {
		return errors.New("injected reconnect failure")
	}
	return nil
}

type stdinReplayAgent struct {
	calls      int
	reconnects int
	accepted   []byte
}

func (f *stdinReplayAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *stdinReplayAgent) CallContext(_ context.Context, method string, request, response any) error {
	if method != "WriteProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls++
	values := request.(map[string]any)
	offset, data := values["offset"].(uint64), values["data"].([]byte)
	if offset != 0 || string(data) != "replay-safe" {
		return errors.New("stdin replay changed offset or bytes")
	}
	if f.calls == 1 {
		f.accepted = append(f.accepted, data...)
		return errors.New("injected lost stdin acknowledgement")
	}
	return setJSONResponse(response, map[string]uint64{"offset": uint64(len(f.accepted))})
}
func (f *stdinReplayAgent) Close() error           { return nil }
func (f *stdinReplayAgent) Reconnect(string) error { return nil }
func (f *stdinReplayAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

type networkPumpAgent struct {
	mu                sync.Mutex
	exchanges         int
	reconnects        int
	reconnectFailures int
	handler           func(int, []byte) ([]byte, error)
	packets           chan []byte
}

func (f *networkPumpAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *networkPumpAgent) CallContext(_ context.Context, method string, request, response any) error {
	if method != "ExchangeNetwork" {
		return fmt.Errorf("unexpected network pump method %s", method)
	}
	packet := append([]byte(nil), request.(map[string][]byte)["packet"]...)
	f.mu.Lock()
	f.exchanges++
	call := f.exchanges
	f.mu.Unlock()
	if len(packet) != 0 && f.packets != nil {
		f.packets <- packet
	}
	output, err := f.handler(call, packet)
	if err != nil {
		return err
	}
	return setJSONResponse(response, map[string][]byte{"packet": output})
}

func (f *networkPumpAgent) Close() error           { return nil }
func (f *networkPumpAgent) Reconnect(string) error { return nil }
func (f *networkPumpAgent) ReconnectContext(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconnects++
	if f.reconnects <= f.reconnectFailures {
		return errors.New("injected reconnect failure")
	}
	return nil
}

func (f *networkPumpAgent) reconnectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reconnects
}

type fakeNetworkClient struct {
	endpoint       mknetwork.Endpoint
	descriptorPath string
	descriptor     *os.File
	calls          []string
}

func validShimEndpoint(containerID, sandboxID, sandboxGeneration, generation, netns string) mknetwork.Endpoint {
	return mknetwork.Endpoint{
		ContainerID: containerID, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		Owner: "cni", SandboxID: sandboxID, SandboxGeneration: sandboxGeneration, Generation: generation,
		Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "READY",
	}
}

type retryNetworkReportClient struct {
	mu       sync.Mutex
	failures int
	attempts chan mknetwork.Endpoint
	success  chan mknetwork.Endpoint
}

type blockedNetworkCloseAgent struct{ calls chan struct{} }

func (f *blockedNetworkCloseAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *blockedNetworkCloseAgent) CallContext(ctx context.Context, method string, _ any, _ any) error {
	if method != "CloseNetwork" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}
func (*blockedNetworkCloseAgent) Close() error                                   { return nil }
func (*blockedNetworkCloseAgent) Reconnect(string) error                         { return nil }
func (*blockedNetworkCloseAgent) ReconnectContext(context.Context, string) error { return nil }

func (f *retryNetworkReportClient) Call(_ context.Context, request mknetwork.Request) (mknetwork.Response, error) {
	if request.Method != "REPORT" || request.Endpoint == nil {
		return mknetwork.Response{}, fmt.Errorf("unexpected network request %s", request.Method)
	}
	endpoint := *request.Endpoint
	f.attempts <- endpoint
	f.mu.Lock()
	if f.failures > 0 {
		f.failures--
		f.mu.Unlock()
		return mknetwork.Response{}, errors.New("injected network report failure")
	}
	f.mu.Unlock()
	f.success <- endpoint
	return mknetwork.Response{Version: mknetwork.ProtocolVersion, RequestID: request.RequestID}, nil
}
func (*retryNetworkReportClient) Attach(context.Context, mknetwork.Request) (mknetwork.Response, *os.File, error) {
	return mknetwork.Response{}, nil, errors.New("unexpected network attach")
}

type recoveryAgentClient struct {
	finish chan struct{}
	mu     sync.Mutex
	calls  []string
}

func (f *recoveryAgentClient) record(method string) {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	f.mu.Unlock()
}

func (f *recoveryAgentClient) snapshotCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func setJSONResponse(output, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}

func setStoppedResponse(request, response any, pid, exit int) error {
	id := request.(map[string]string)["ID"]
	return setJSONResponse(response, agent.ProcessState{ID: id, PID: pid, Status: "STOPPED", ExitCode: exit})
}

func (f *recoveryAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *recoveryAgentClient) CallContext(ctx context.Context, method string, request, response any) error {
	f.record(method)
	switch method {
	case "StateProcess":
		id := request.(map[string]string)["ID"]
		return setJSONResponse(response, agent.ProcessState{ID: id, PID: 41, Status: "RUNNING"})
	case "ReadProcessOutput":
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.finish:
		}
		return setJSONResponse(response, map[string]any{"status": "STOPPED", "stdout_offset": 0, "stderr_offset": 0})
	case "WaitProcess":
		return setStoppedResponse(request, response, 41, 17)
	default:
		return nil
	}
}

func (f *recoveryAgentClient) Close() error                                   { return nil }
func (f *recoveryAgentClient) Reconnect(string) error                         { return nil }
func (f *recoveryAgentClient) ReconnectContext(context.Context, string) error { return nil }

func (f *fakeNetworkClient) Call(_ context.Context, request mknetwork.Request) (mknetwork.Response, error) {
	f.calls = append(f.calls, request.Method)
	endpoint := f.endpoint
	return mknetwork.Response{Version: mknetwork.ProtocolVersion, RequestID: request.RequestID, Endpoint: &endpoint}, nil
}

func (f *fakeNetworkClient) Attach(_ context.Context, request mknetwork.Request) (mknetwork.Response, *os.File, error) {
	f.calls = append(f.calls, request.Method)
	descriptor, err := os.Open(f.descriptorPath)
	if err != nil {
		return mknetwork.Response{}, nil, err
	}
	endpoint := f.endpoint
	f.descriptor = descriptor
	return mknetwork.Response{Version: mknetwork.ProtocolVersion, RequestID: request.RequestID, Endpoint: &endpoint}, descriptor, nil
}

type daemonCallFunc func(context.Context, protocol.Request, any) *protocol.Error

func (f daemonCallFunc) Call(ctx context.Context, request protocol.Request, response any) *protocol.Error {
	return f(ctx, request, response)
}

type closeRetryAgent struct {
	attempts int
	called   chan int
}

func (f *closeRetryAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *closeRetryAgent) CallContext(_ context.Context, method string, _, _ any) error {
	if method != "CloseProcessStdin" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.attempts++
	f.called <- f.attempts
	if f.attempts == 1 {
		return errors.New("injected close failure")
	}
	return nil
}
func (f *closeRetryAgent) Close() error                                   { return nil }
func (f *closeRetryAgent) Reconnect(string) error                         { return nil }
func (f *closeRetryAgent) ReconnectContext(context.Context, string) error { return nil }

type signalFailureAgent struct {
	attempts int
	failAt   int
	calls    []string
	hook     func(string)
}

func (f *signalFailureAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *signalFailureAgent) CallContext(_ context.Context, method string, request, _ any) error {
	if method != "SignalProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	value := request.(map[string]any)
	f.attempts++
	call := fmt.Sprintf("%s:%s", value["ID"], value["Signal"])
	f.calls = append(f.calls, call)
	if f.hook != nil {
		f.hook(call)
	}
	if f.attempts == f.failAt {
		return errors.New("injected signal failure")
	}
	return nil
}
func (f *signalFailureAgent) Close() error                                   { return nil }
func (f *signalFailureAgent) Reconnect(string) error                         { return nil }
func (f *signalFailureAgent) ReconnectContext(context.Context, string) error { return nil }

type eofReadWriteCloser struct{}

func (eofReadWriteCloser) Read([]byte) (int, error)    { return 0, io.EOF }
func (eofReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (eofReadWriteCloser) Close() error                { return nil }

func (f *fakeAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *fakeAgentClient) CallContext(_ context.Context, method string, request any, response any) error {
	f.calls = append(f.calls, method)
	if err := f.fail[method]; err != nil {
		return err
	}
	if method == "StatsProcess" {
		value := f.stats
		if f.statsByID != nil {
			value = f.statsByID[request.(map[string]string)["ID"]]
		}
		*(response.(*agent.ProcessStats)) = value
	}
	return nil
}
func (f *fakeAgentClient) Close() error                                   { return nil }
func (f *fakeAgentClient) Reconnect(string) error                         { return nil }
func (f *fakeAgentClient) ReconnectContext(context.Context, string) error { return nil }

type fakePublisher struct {
	topics   []string
	failures int
	hook     func()
}

type retryPublisher struct {
	attempts  int
	published chan string
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(value []byte) (int, error) { return f(value) }

type relayOwnerFunc func() error

func (f relayOwnerFunc) Remove() error { return f() }

type inheritedFileSocket string

func (path inheritedFileSocket) File() (*os.File, error) { return os.Open(string(path)) }

func (f *retryPublisher) Publish(_ context.Context, topic string, _ events.Event) error {
	f.attempts++
	if f.attempts == 1 {
		return errors.New("injected transient disconnect")
	}
	f.published <- topic
	return nil
}
func (f *retryPublisher) Close() error { return nil }

func (f *fakePublisher) Publish(_ context.Context, topic string, _ events.Event) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("injected publication failure")
	}
	if f.hook != nil {
		f.hook()
	}
	f.topics = append(f.topics, topic)
	return nil
}
func (f *fakePublisher) Close() error { return nil }

func TestValidateExecProcessFailsClosed(t *testing.T) {
	base := func() *specs.Process {
		return &specs.Process{User: specs.User{UID: 1, GID: 2, AdditionalGids: []uint32{3}}, Args: []string{"/bin/true"}, Env: []string{"A=B"}, Cwd: "/"}
	}
	if err := validateExecProcess(base()); err != nil {
		t.Fatalf("supported process rejected: %v", err)
	}
	supported := base()
	supported.User.UID = 0
	supported.NoNewPrivileges = true
	supported.Rlimits = []specs.POSIXRlimit{{Type: "RLIMIT_NOFILE", Soft: 64, Hard: 64}}
	supported.Capabilities = &specs.LinuxCapabilities{Bounding: []string{"CAP_CHOWN"}, Permitted: []string{"CAP_CHOWN"}, Effective: []string{"CAP_CHOWN"}}
	if err := validateExecProcess(supported); err != nil {
		t.Fatalf("standard process controls rejected: %v", err)
	}
	projected := processSpec(supported)
	if projected.NoNewPrivileges == nil || !*projected.NoNewPrivileges || len(projected.Rlimits) != 1 || projected.Capabilities["effective"][0] != "CAP_CHOWN" {
		t.Fatalf("standard process controls were not projected: %+v", projected)
	}
	zero := 0
	umask := uint32(0o22)
	tests := []struct {
		name   string
		mutate func(*specs.Process)
	}{
		{"console-size", func(p *specs.Process) { p.ConsoleSize = &specs.Box{} }},
		{"command-line", func(p *specs.Process) { p.CommandLine = "true" }},
		{"apparmor", func(p *specs.Process) { p.ApparmorProfile = "profile" }},
		{"oom-score", func(p *specs.Process) { p.OOMScoreAdj = &zero }},
		{"scheduler", func(p *specs.Process) { p.Scheduler = &specs.Scheduler{} }},
		{"selinux", func(p *specs.Process) { p.SelinuxLabel = "label" }},
		{"io-priority", func(p *specs.Process) { p.IOPriority = &specs.LinuxIOPriority{} }},
		{"umask", func(p *specs.Process) { p.User.Umask = &umask }},
		{"username", func(p *specs.Process) { p.User.Username = "root" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process := base()
			test.mutate(process)
			if err := validateExecProcess(process); !errors.Is(err, errdefs.ErrNotImplemented) {
				t.Fatalf("error = %v, want not implemented", err)
			}
		})
	}
}

func TestValidateServiceIdentityRejectsUnsafeValues(t *testing.T) {
	bundle := t.TempDir()
	if err := validateServiceIdentity("task-1", "default", bundle); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	for _, test := range []struct{ id, namespace, bundle string }{
		{"../task", "default", bundle},
		{"task", "../namespace", bundle},
		{"task", "default", "relative"},
		{"task", "default", bundle + "/../" + filepath.Base(bundle)},
	} {
		if err := validateServiceIdentity(test.id, test.namespace, test.bundle); !errors.Is(err, errdefs.ErrInvalidArgument) {
			t.Fatalf("unsafe identity (%q, %q, %q) error = %v", test.id, test.namespace, test.bundle, err)
		}
	}
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(parent, "linked")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	if err := validateServiceIdentity("task", "default", linked); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("symlinked bundle error = %v", err)
	}
}

func TestSandboxIDBindsCompleteNamespaceAndTaskWithoutAliases(t *testing.T) {
	base := sandboxID("namespace-a", "task")
	if base != sandboxID("namespace-a", "task") || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(base) {
		t.Fatalf("sandbox ID is not stable and daemon-safe: %q", base)
	}
	aliases := []string{
		sandboxID("namespace-b", "task"),
		sandboxID("namespace-a", "task.with.punctuation"),
		sandboxID("namespace-a", "task_with_punctuation"),
		sandboxID("namespace-a", strings.Repeat("a", 127)+"b"),
		sandboxID("namespace-a", strings.Repeat("a", 127)+"c"),
	}
	seen := map[string]bool{base: true}
	for _, candidate := range aliases {
		if seen[candidate] {
			t.Fatalf("distinct namespace/task tuples alias as %q", candidate)
		}
		seen[candidate] = true
		if len(candidate) > 63 {
			t.Fatalf("sandbox ID exceeds daemon limit: %q", candidate)
		}
	}
}

func TestRetryWaitReturnsImmediatelyOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := waitContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitContext error = %v", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("canceled retry wait did not return promptly")
	}
}

func TestPreCancelledShimStartupAndRecoveryDoNotCreateOrInspectState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &service{bundle: filepath.Join(t.TempDir(), "missing-bundle")}
	if address, err := s.StartShim(ctx, shim.StartOpts{ID: "task"}); !errors.Is(err, context.Canceled) || address != "" {
		t.Fatalf("StartShim() = %q, %v", address, err)
	}
	if err := s.recoverExisting(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("recoverExisting() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.bundle, ".multikernel")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled startup created runtime state: %v", err)
	}
}

func TestLaunchShimWorkerCleansProcessGroupAndOwnedArtifactsOnPIDFailure(t *testing.T) {
	directory := t.TempDir()
	address := "unix://" + filepath.Join(directory, "shim.sock")
	inheritedPath := filepath.Join(directory, "inherited-descriptor")
	if err := os.WriteFile(inheritedPath, []byte("descriptor"), 0600); err != nil {
		t.Fatal(err)
	}
	addressPath := filepath.Join(directory, "address")
	pidPath := filepath.Join(directory, "shim.pid")
	if err := os.Mkdir(pidPath, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := launchShimWorker(context.Background(), cmd, inheritedFileSocket(inheritedPath), address, addressPath, pidPath); err == nil {
		t.Fatal("PID-file failure was accepted")
	}
	if _, statErr := os.Lstat(addressPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial address file survived: %v", statErr)
	}
	if info, statErr := os.Stat(pidPath); statErr != nil || !info.IsDir() {
		t.Fatalf("pre-existing PID path was removed or changed: %+v, %v", info, statErr)
	}
	if cmd.Process == nil {
		t.Fatal("worker did not reach the injected post-start failure")
	}
	deadline := time.Now().Add(time.Second)
	for {
		killErr := syscall.Kill(-cmd.Process.Pid, 0)
		if errors.Is(killErr, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker process group survived cleanup: %v", killErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCancelCreateUsesExactOriginalIdentity(t *testing.T) {
	wantConfig := protocol.SandboxConfig{SchemaVersion: 1, ID: "box-a", CPUs: []int{8}, MemoryBytes: 1 << 30,
		KernelManifest: "test", Bundle: "/bundle/box-a", AgentPort: 7001, ChildCID: 3}
	wantKey := "shim-create-box-a-0123456789ab"
	var callErr error
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, response any) *protocol.Error {
		var observed protocol.SandboxConfig
		if decodeErr := protocol.StrictDecode(request.Body, &observed); decodeErr != nil {
			callErr = decodeErr
			return nil
		}
		if request.Method != "CancelCreateSandbox" || request.IdempotencyKey != wantKey || !reflect.DeepEqual(observed, wantConfig) {
			callErr = fmt.Errorf("unexpected cancellation request: %+v %+v", request, observed)
			return nil
		}
		encoded, _ := json.Marshal(map[string]bool{"safe_to_cleanup": true})
		if decodeErr := json.Unmarshal(encoded, response); decodeErr != nil {
			callErr = decodeErr
		}
		return nil
	})
	service := &service{daemon: caller}
	if err := service.cancelCreate(context.Background(), wantConfig, wantKey); err != nil {
		t.Fatal(err)
	}
	if callErr != nil {
		t.Fatal(callErr)
	}
}

func TestLoadOrCreateTokenReusesExactSafeIdentity(t *testing.T) {
	directory := t.TempDir()
	first, encoded, err := loadOrCreateToken(directory)
	if err != nil || len(first) != 32 || len(encoded) != 64 {
		t.Fatalf("first token = %x %q, %v", first, encoded, err)
	}
	second, repeated, err := loadOrCreateToken(directory)
	if err != nil || !reflect.DeepEqual(second, first) || repeated != encoded {
		t.Fatalf("repeated token = %x %q, %v", second, repeated, err)
	}
	info, err := os.Lstat(filepath.Join(directory, "token"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 65 {
		t.Fatalf("token metadata = %+v, %v", info, err)
	}
}

func TestLoadOrCreateTokenRejectsMalformedAndSymlinkState(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(string) error
	}{
		{"malformed", func(path string) error { return os.WriteFile(path, []byte("short\n"), 0600) }},
		{"permissive", func(path string) error { return os.WriteFile(path, []byte(strings.Repeat("a", 64)+"\n"), 0644) }},
		{"symlink", func(path string) error { return os.Symlink("missing", path) }},
		{"hardlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
				return err
			}
			return os.Link(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := test.setup(filepath.Join(directory, "token")); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadOrCreateToken(directory); err == nil {
				t.Fatal("unsafe existing token accepted")
			}
		})
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		directory := t.TempDir()
		realRuntime := filepath.Join(directory, "real")
		if err := os.Mkdir(realRuntime, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realRuntime, "token"), []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		linkedRuntime := filepath.Join(directory, "linked")
		if err := os.Symlink(realRuntime, linkedRuntime); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrCreateToken(linkedRuntime); err == nil {
			t.Fatal("token beneath a symlinked ancestor was accepted")
		}
	})

	t.Run("symlinked empty ancestor does not redirect creation", func(t *testing.T) {
		directory := t.TempDir()
		realRuntime := filepath.Join(directory, "real-empty")
		if err := os.Mkdir(realRuntime, 0700); err != nil {
			t.Fatal(err)
		}
		linkedRuntime := filepath.Join(directory, "linked-empty")
		if err := os.Symlink(realRuntime, linkedRuntime); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrCreateToken(linkedRuntime); err == nil {
			t.Fatal("token creation beneath a symlinked ancestor was accepted")
		}
		if _, err := os.Stat(filepath.Join(realRuntime, "token")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("redirected token was created: %v", err)
		}
	})
}

func TestCreateAmbiguityCancellationRemovesPreparedArtifacts(t *testing.T) {
	bundle := t.TempDir()
	configJSON := `{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "shim.lock")
	t.Setenv("MK_SHIM_LOCK", lock)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	storageHash := strings.Repeat("a", 64)
	var calls []string
	respond := func(output any, value any) *protocol.Error {
		if output == nil {
			return nil
		}
		data, _ := json.Marshal(value)
		if err := json.Unmarshal(data, output); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
		calls = append(calls, request.Method)
		switch request.Method {
		case "ListSandboxes":
			return respond(output, []protocol.Sandbox{})
		case "PrepareRootfs":
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			return respond(output, rootfspkg.PrepareResult{Storage: protocol.StorageConfig{
				Path: "/srv/storage/root.ext4", ImageID: "image", FilesystemUUID: "12345678-1234-4234-8234-123456789abc",
				SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: storageHash,
			}})
		case "CreateSandbox":
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "ambiguous create", Retryable: true}
		case "CancelCreateSandbox":
			if !strings.HasPrefix(request.IdempotencyKey, "shim-create-task-a-") {
				t.Fatalf("unexpected create cancellation key %q", request.IdempotencyKey)
			}
			return respond(output, map[string]bool{"safe_to_cleanup": true})
		case "CleanupRootfs":
			var cleanup rootfspkg.CleanupRequest
			if err := protocol.StrictDecode(request.Body, &cleanup); err != nil || cleanup.Bundle != bundle || cleanup.StorageSHA256 != storageHash {
				t.Fatalf("cleanup request = %+v, %v", cleanup, err)
			}
			if err := os.RemoveAll(runtimeDir); err != nil {
				t.Fatal(err)
			}
			return respond(output, map[string]bool{"cleaned": true})
		default:
			t.Fatalf("unexpected daemon method %q", request.Method)
			return nil
		}
	})
	service := &service{id: "task-a", namespace: "default", bundle: bundle, daemon: caller, processes: map[string]*process{}}
	_, err := service.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task-a", Bundle: bundle})
	if err == nil || !strings.Contains(err.Error(), "ambiguous create") {
		t.Fatalf("Create() error = %v", err)
	}
	wantCalls := []string{"ListSandboxes", "PrepareRootfs", "CreateSandbox", "CancelCreateSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("daemon calls = %v, want %v", calls, wantCalls)
	}
	if _, statErr := os.Lstat(runtimeDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime artifacts survived cancellation: %v", statErr)
	}
	if len(service.token) != 0 || len(service.processes) != 0 || service.sandbox.ID != "" {
		t.Fatalf("shim state survived cancellation: token=%d processes=%d sandbox=%+v", len(service.token), len(service.processes), service.sandbox)
	}
}

func TestRootfsMountsAreSanitizedForDaemonAndValidated(t *testing.T) {
	snapshot := t.TempDir()
	mounts, err := rootfsMounts([]*types.Mount{{
		Type: "overlay", Source: "overlay", Options: []string{"rw", "lowerdir=" + snapshot, "nodev"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	options := map[string]bool{}
	for _, option := range mounts[0].Options {
		options[option] = true
	}
	if options["rw"] || !options["lowerdir="+snapshot] {
		t.Fatalf("sanitized mount options = %v", mounts[0].Options)
	}
	for _, input := range []*types.Mount{
		nil,
		{Type: "tmpfs", Source: "tmpfs"},
		{Type: "bind", Source: "relative"},
		{Type: "overlay", Source: "overlay", Options: []string{"ro\nmalicious"}},
		{Type: "overlay", Source: "overlay", Options: []string{"nodev", "nodev"}},
		{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=relative"}},
	} {
		if _, err = rootfsMounts([]*types.Mount{input}); err == nil {
			t.Fatalf("unsafe mount accepted: %+v", input)
		}
	}
	tooMany := make([]*types.Mount, 9)
	for index := range tooMany {
		tooMany[index] = &types.Mount{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=" + snapshot}}
	}
	if _, err = rootfsMounts(tooMany); err == nil {
		t.Fatal("too many rootfs mounts accepted")
	}
}

func TestResizePtyStoresInitialSizeBeforeStart(t *testing.T) {
	s := &service{processes: map[string]*process{
		"": {terminal: true, status: tasktypes.Status_CREATED},
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); err != nil {
		t.Fatal(err)
	}
	p := s.processes[""]
	if !p.sizeSet || p.width != 91 || p.height != 37 {
		t.Fatalf("pending size = set:%v %dx%d", p.sizeSet, p.width, p.height)
	}
}

func TestOutputFIFOCanBeReattached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout")
	initial, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("FIFO identity unavailable")
	}
	w, guard, err := openOutput(context.Background(), path, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer guard.Close()
	initial.Close()
	if _, err = w.Write([]byte("reattach-ok")); err != nil {
		t.Fatal(err)
	}
	attached, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer attached.Close()
	var got = make([]byte, len("reattach-ok"))
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, err = io.ReadFull(attached, got); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if string(got) != "reattach-ok" {
		t.Fatalf("attached output = %q, err = %v", got, err)
	}
}

func TestProcessIOPathsRejectUnsafeIdentityAndSymlinkAncestors(t *testing.T) {
	directory := t.TempDir()
	unsafeFIFO := filepath.Join(directory, "unsafe")
	if err := syscall.Mkfifo(unsafeFIFO, 0660); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(unsafeFIFO, false); err == nil {
		t.Fatal("group-accessible FIFO was accepted")
	}

	regular := filepath.Join(directory, "output")
	if err := os.WriteFile(regular, []byte("output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(regular, regular+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(regular, false); err == nil {
		t.Fatal("hard-linked output was accepted")
	}

	realParent := filepath.Join(directory, "real")
	if err := os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(realParent, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(directory, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(filepath.Join(linkedParent, "fifo"), false); err == nil {
		t.Fatal("stdio beneath a symlinked parent was accepted")
	}
}

func TestProcessIOOpenRejectsReplacementAndCancellation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "output")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok := processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("output identity unavailable")
	}
	if err = os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("same-inode stdio mode change was accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	info, err = inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok = processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("output identity unavailable")
	}
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("replacement stdio inode was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if file, openErr := openKnownProcessIOPath(ctx, path, unix.O_WRONLY, expected); !errors.Is(openErr, context.Canceled) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("cancelled stdio open error = %v", openErr)
	}
}

func TestInvalidProcessIOFailsBeforeCreateOrExecMutation(t *testing.T) {
	s := &service{id: "task", bundle: "/bundle", processes: make(map[string]*process)}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{
		ID: "task", Bundle: "/bundle", Stdin: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Create invalid stdio error = %v", err)
	}
	if len(s.processes) != 0 {
		t.Fatalf("Create mutated process state: %+v", s.processes)
	}
	if _, err := s.Exec(context.Background(), &taskapi.ExecProcessRequest{
		ExecID: "exec", Stdout: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Exec invalid stdio error = %v", err)
	}
}

func TestProcessStdinMustBeFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte("input"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(path, true); err == nil {
		t.Fatal("regular-file stdin was accepted")
	}
}

func TestStdinFIFOAcceptsLateAndRepeatedWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := inspectBoundProcessIOPath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	p := &process{stdin: path, stdinIdentity: identity, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	agentClient := &stdinCaptureAgent{writes: make(chan []byte, 3)}
	s := &service{agent: agentClient, processes: map[string]*process{"": p}}
	if err = s.openProcessIO(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	// Exercise the no-initial-peer state before attaching the first writer.
	time.Sleep(20 * time.Millisecond)
	assertWrite := func(want string) {
		t.Helper()
		select {
		case got := <-agentClient.writes:
			if string(got) != want {
				t.Fatalf("guest stdin = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("stdin %q was not forwarded", want)
		}
	}
	for attachment, writes := range [][]string{{"late-writer", "same-writer-after-idle"}, {"reattached-writer"}} {
		descriptor, openErr := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if openErr != nil {
			t.Fatal(openErr)
		}
		writer := os.NewFile(uintptr(descriptor), path)
		for index, want := range writes {
			if _, openErr = writer.WriteString(want); openErr != nil {
				_ = writer.Close()
				t.Fatal(openErr)
			}
			assertWrite(want)
			if attachment == 0 && index == 0 {
				// Leave the writer attached while the nonblocking reader observes
				// an empty FIFO. A transient EAGAIN must not terminate the pump.
				time.Sleep(20 * time.Millisecond)
			}
		}
		if openErr = writer.Close(); openErr != nil {
			t.Fatal(openErr)
		}
	}
	s.mu.Lock()
	closeProcessIO(p)
	s.mu.Unlock()
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("stdin pump survived descriptor teardown")
	}
}

func TestCloseIOStopsContinuouslyReadableStdinAfterInflightChunk(t *testing.T) {
	reader := &controlledInput{reads: make(chan chan []byte)}
	client := &stdinCaptureAgent{writes: make(chan []byte, 2)}
	p := &process{stdinReader: reader, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	first := <-reader.reads
	first <- []byte("before-close")
	if got := <-client.writes; string(got) != "before-close" {
		t.Fatalf("first guest stdin = %q", got)
	}
	// Prove the next read is already in flight when CloseIO records its intent.
	second := <-reader.reads
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	second <- []byte("in-flight")
	if got := <-client.writes; string(got) != "in-flight" {
		t.Fatalf("in-flight guest stdin = %q", got)
	}
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("CloseIO did not stop a continuously readable stdin pump")
	}
	if !p.stdinClosed || !p.stdinCloseAcked || p.stdinReader != nil {
		t.Fatalf("closed stdin state = requested:%v acknowledged:%v reader:%v", p.stdinClosed, p.stdinCloseAcked, p.stdinReader)
	}
	select {
	case <-reader.reads:
		t.Fatal("stdin pump accepted another chunk after CloseIO")
	default:
	}
}

func TestPendingStdinReplaysLostAcknowledgementExactlyOnce(t *testing.T) {
	client := &stdinReplayAgent{}
	p := &process{status: tasktypes.Status_RUNNING, stdinPending: []byte("replay-safe"), done: make(chan struct{})}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	if err := s.deliverPendingStdin("init", p); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || client.reconnects != 1 || string(client.accepted) != "replay-safe" {
		t.Fatalf("stdin replay calls=%d reconnects=%d accepted=%q", client.calls, client.reconnects, client.accepted)
	}
	if p.stdinOffset != uint64(len("replay-safe")) || len(p.stdinPending) != 0 {
		t.Fatalf("stdin durable acknowledgement offset=%d pending=%q", p.stdinOffset, p.stdinPending)
	}
}

func TestStdinIntentPersistenceFailurePrecedesGuestMutation(t *testing.T) {
	bundle := t.TempDir()
	path := filepath.Join(bundle, "stdin")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := inspectBoundProcessIOPath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	p := &process{stdin: path, stdinIdentity: identity, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	agentClient := &stdinCaptureAgent{writes: make(chan []byte, 1)}
	s := &service{bundle: bundle, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent: agentClient, processes: map[string]*process{"": p}}
	// Deliberately omit bundle/.multikernel so the pending-byte publication
	// fails before WriteProcess can mutate guest stdin.
	if err = s.openProcessIO(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	pumpDone := make(chan struct{})
	go func() {
		s.pumpStdin("init", p)
		close(pumpDone)
	}()
	descriptor, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	writer := os.NewFile(uintptr(descriptor), path)
	if _, err = writer.WriteString("durable-first"); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("stdin pump did not stop after durable intent failure")
	}
	select {
	case data := <-agentClient.writes:
		t.Fatalf("guest received stdin before durable intent: %q", data)
	default:
	}
	if p.stdinOffset != 0 || string(p.stdinPending) != "durable-first" || p.stdinReader != nil {
		t.Fatalf("failed stdin intent offset=%d pending=%q reader=%v", p.stdinOffset, p.stdinPending, p.stdinReader)
	}
}

func TestStdinAcknowledgementPersistenceFailureRetainsReplayIdentity(t *testing.T) {
	client := &stdinReplayAgent{}
	p := &process{status: tasktypes.Status_RUNNING, stdinPending: []byte("replay-safe"), done: make(chan struct{})}
	s := &service{bundle: t.TempDir(), sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	// Missing .multikernel makes the post-ack durable update fail. The guest may
	// have accepted the bytes, so the exact pending tuple must remain retryable.
	if err := s.deliverPendingStdin("init", p); err == nil || !strings.Contains(err.Error(), "persist acknowledged stdin offset") {
		t.Fatalf("stdin acknowledgement persistence error = %v", err)
	}
	if p.stdinOffset != 0 || string(p.stdinPending) != "replay-safe" || string(client.accepted) != "replay-safe" {
		t.Fatalf("failed acknowledgement offset=%d pending=%q accepted=%q", p.stdinOffset, p.stdinPending, client.accepted)
	}
}

func TestBoundProcessIORejectsReplacementBeforeStartOrRecovery(t *testing.T) {
	for _, output := range []bool{false, true} {
		name := "stdin"
		if output {
			name = "stdout"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if output {
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			identity, err := inspectBoundProcessIOPath(path, !output)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if output {
				if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err = syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			p := &process{done: make(chan struct{})}
			if output {
				p.stdout, p.stdoutIdentity = path, identity
			} else {
				p.stdin, p.stdinIdentity = path, identity
			}
			if err = (&service{}).openProcessIO(context.Background(), p); err == nil || !strings.Contains(err.Error(), "identity changed") {
				t.Fatalf("replacement I/O error = %v", err)
			}
			if _, err = os.Lstat(path); err != nil {
				t.Fatalf("replacement was changed: %v", err)
			}
		})
	}
}

func TestOutputOffsetsAdvanceOnlyAfterDeliveryOrBoundedDrop(t *testing.T) {
	now := time.Unix(100, 0)
	pressure := time.Time{}
	blocked := writerFunc(func([]byte) (int, error) { return 0, syscall.EAGAIN })
	advance, dropped := deliverOutput(blocked, []byte("output"), &pressure, now, time.Second)
	if advance || dropped || !pressure.Equal(now) {
		t.Fatalf("initial pressure advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(999*time.Millisecond), time.Second)
	if advance || dropped {
		t.Fatalf("pre-deadline pressure advanced=%v dropped=%v", advance, dropped)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(time.Second), time.Second)
	if !advance || !dropped || !pressure.IsZero() {
		t.Fatalf("bounded drop advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	pressure = now
	complete := writerFunc(func(value []byte) (int, error) { return len(value), nil })
	advance, dropped = deliverOutput(complete, []byte("output"), &pressure, now, time.Second)
	if !advance || dropped || !pressure.IsZero() {
		t.Fatalf("complete delivery advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(nil, []byte("discard-by-contract"), &pressure, now, time.Second)
	if !advance || dropped {
		t.Fatalf("unconfigured output advanced=%v dropped=%v", advance, dropped)
	}
}

func TestProcessOutputAndWaitRecoverTransportWithoutChangingOffsets(t *testing.T) {
	client := &outputReconnectAgent{readFailures: 2, waitFailures: 1, reconnectFailures: 1}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second}
	var output processOutput
	if err := s.readProcessOutput("init", 7, 9, &output); err != nil {
		t.Fatal(err)
	}
	if string(output.Stdout) != "after-reconnect" || output.StdoutOffset != 22 || output.StderrOffset != 9 || output.Status != "RUNNING" {
		t.Fatalf("output after reconnect = %+v", output)
	}
	var state agent.ProcessState
	if err := s.callAgentWithReconnect("WaitProcess", map[string]string{"ID": "init"}, &state); err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 19 || client.readCalls != 3 || client.waitCalls != 2 || client.reconnects != 3 {
		t.Fatalf("reconnect result state=%+v reads=%d waits=%d reconnects=%d", state, client.readCalls, client.waitCalls, client.reconnects)
	}
}

func TestWaitProcessRejectsMismatchedStoppedStateUntilExactCompletion(t *testing.T) {
	client := &waitValidationAgent{reads: make(chan struct{}), states: make(chan agent.ProcessState)}
	p := &process{id: "exec", pid: 41, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	publisher := &fakePublisher{}
	s := &service{id: "task", namespace: "tests", bundle: t.TempDir(), agent: client, publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}, processes: map[string]*process{"exec": p}}
	go s.waitProcess("exec", "exec", p)
	<-client.reads
	invalid := []agent.ProcessState{
		{ID: "other", PID: 41, Status: "STOPPED", ExitCode: 37},
		{ID: "exec", PID: 41, Status: "RUNNING", ExitCode: 37},
		{ID: "exec", PID: 42, Status: "STOPPED", ExitCode: 37},
		{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: -1},
		{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: 256},
	}
	for _, state := range invalid {
		client.states <- state
		// A subsequent output read proves the prior response was rejected and
		// the monitor returned to its observation loop.
		<-client.reads
		if p.status != tasktypes.Status_RUNNING || p.pid != 41 || p.exit != 0 {
			t.Fatalf("invalid stopped state mutated process: %+v", p)
		}
		select {
		case <-p.done:
			t.Fatal("invalid stopped state completed Task wait")
		default:
		}
		if len(publisher.topics) != 0 {
			t.Fatalf("invalid stopped state published events: %v", publisher.topics)
		}
	}
	client.states <- agent.ProcessState{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: 37}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("exact stopped state did not complete Task wait")
	}
	if p.status != tasktypes.Status_STOPPED || p.pid != 41 || p.exit != 37 {
		t.Fatalf("exact stopped state = status:%v pid:%d exit:%d", p.status, p.pid, p.exit)
	}
	if fmt.Sprint(publisher.topics) != "[/tasks/exit]" {
		t.Fatalf("completion events = %v", publisher.topics)
	}
}

func TestProcessOutputReconnectHasOneOverallDeadline(t *testing.T) {
	client := &outputReconnectAgent{readFailures: 1000, reconnectFailures: 1000}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: 80 * time.Millisecond}
	started := time.Now()
	var output processOutput
	err := s.readProcessOutput("init", 7, 9, &output)
	if err == nil || !strings.Contains(err.Error(), "injected output disconnect") {
		t.Fatalf("bounded reconnect error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded reconnect elapsed = %v", elapsed)
	}
	if client.readCalls < 2 || client.reconnects == 0 {
		t.Fatalf("bounded reconnect attempts reads=%d reconnects=%d", client.readCalls, client.reconnects)
	}
}

func TestProcessOutputDoesNotReplayAuthenticatedRemoteRejection(t *testing.T) {
	client := &outputReconnectAgent{remoteReadFailure: true}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second}
	var output processOutput
	err := s.readProcessOutput("init", 7, 9, &output)
	var remoteError *agent.RemoteError
	if !errors.As(err, &remoteError) || remoteError.Failure.Code != "NOT_FOUND" {
		t.Fatalf("remote output error = %#v", err)
	}
	if client.readCalls != 1 || client.reconnects != 0 {
		t.Fatalf("remote rejection was replayed: reads=%d reconnects=%d", client.readCalls, client.reconnects)
	}
}

func TestProcessOutputRequiresBoundedContiguousOffsetsAndKnownState(t *testing.T) {
	valid := processOutput{Stdout: []byte("x"), StdoutOffset: 8, StderrOffset: 9, Status: "RUNNING"}
	if err := validateProcessOutput(7, 9, valid); err != nil {
		t.Fatalf("valid process output = %v", err)
	}
	for name, test := range map[string]struct {
		stdoutOffset uint64
		stderrOffset uint64
		output       processOutput
	}{
		"oversized stdout":  {7, 9, processOutput{Stdout: make([]byte, processOutputChunk+1), StdoutOffset: 7 + processOutputChunk + 1, StderrOffset: 9, Status: "RUNNING"}},
		"oversized stderr":  {7, 9, processOutput{StdoutOffset: 7, Stderr: make([]byte, processOutputChunk+1), StderrOffset: 9 + processOutputChunk + 1, Status: "RUNNING"}},
		"stdout gap":        {7, 9, processOutput{Stdout: []byte("x"), StdoutOffset: 9, StderrOffset: 9, Status: "RUNNING"}},
		"stderr regression": {7, 9, processOutput{StdoutOffset: 7, StderrOffset: 8, Status: "RUNNING"}},
		"offset overflow":   {^uint64(0), 9, processOutput{Stdout: []byte("x"), StdoutOffset: 0, StderrOffset: 9, Status: "RUNNING"}},
		"unknown state":     {7, 9, processOutput{StdoutOffset: 7, StderrOffset: 9, Status: "PAUSED"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProcessOutput(test.stdoutOffset, test.stderrOffset, test.output); err == nil {
				t.Fatal("malformed process output was accepted")
			}
		})
	}
}

func TestOutputOffsetRetriesWhenDurableAcknowledgementFails(t *testing.T) {
	bundle := t.TempDir()
	client := &outputAckAgent{offsets: make(chan uint64, 4)}
	writer := &controlledOutputWriter{writes: make(chan controlledOutputWrite)}
	p := &process{status: tasktypes.Status_RUNNING, stdoutWriter: writer, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	first := <-writer.writes
	if string(first.data) != "once" {
		t.Fatalf("first output write = %q", first.data)
	}
	close(first.release)
	// The missing recovery directory makes the first offset publication fail.
	// Reaching the second write proves the request offset rolled back to zero.
	second := <-writer.writes
	if string(second.data) != "once" {
		t.Fatalf("retried output write = %q", second.data)
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	close(second.release)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("process did not finish after durable output acknowledgement recovered")
	}
	if firstOffset, secondOffset, finalOffset := <-client.offsets, <-client.offsets, <-client.offsets; firstOffset != 0 || secondOffset != 0 || finalOffset != 4 {
		t.Fatalf("output request offsets = %d, %d, %d", firstOffset, secondOffset, finalOffset)
	}
	s.mu.Lock()
	status, exit, stdoutOffset := p.status, p.exit, p.stdoutOffset
	s.mu.Unlock()
	if status != tasktypes.Status_STOPPED || exit != 11 || stdoutOffset != 4 {
		t.Fatalf("final process = status:%v exit:%d stdout-offset:%d", status, exit, stdoutOffset)
	}
}

func TestWaitProcessDoesNotFabricateExitAfterReconnectBudget(t *testing.T) {
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: make(chan struct{})}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: t.TempDir(), agent: client,
		ioCallTimeout: 20 * time.Millisecond, processes: map[string]*process{"": p},
		publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-client.observed:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not contact the guest")
	}
	// Let one complete reconnect budget expire. Transport uncertainty must not
	// become an observed process exit.
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	status, exit := p.status, p.exit
	s.mu.Unlock()
	if status != tasktypes.Status_RUNNING || exit != 0 {
		t.Fatalf("process after transport outage = status:%v exit:%d", status, exit)
	}
	select {
	case <-p.done:
		t.Fatal("transport outage fabricated process completion")
	default:
	}
	close(client.recovered)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not recover the exact guest exit")
	}
	s.mu.Lock()
	status, exit, queued := p.status, p.exit, p.exitEventQueued
	s.mu.Unlock()
	if status != tasktypes.Status_STOPPED || exit != 37 || !queued {
		t.Fatalf("recovered process exit = status:%v exit:%d event:%v", status, exit, queued)
	}
}

func TestWaitProcessDoesNotCompleteBeforeExitStateIsDurable(t *testing.T) {
	bundle := t.TempDir()
	recovered := make(chan struct{})
	close(recovered)
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: recovered}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-client.observed:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not observe the stopped guest")
	}
	// The missing recovery directory prevents the exact exit from becoming
	// durable. The previous process state and open wait channel must remain.
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	status, exit := p.status, p.exit
	s.mu.Unlock()
	if status != tasktypes.Status_RUNNING || exit != 0 {
		t.Fatalf("process before durable exit = status:%v exit:%d", status, exit)
	}
	select {
	case <-p.done:
		t.Fatal("task wait completed before exit state was durable")
	default:
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("task wait did not complete after recovery storage returned")
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Processes) != 1 || saved.Processes[0].Status != tasktypes.Status_STOPPED || saved.Processes[0].Exit != 37 {
		t.Fatalf("durable exit state = %+v", saved.Processes)
	}
}

func TestExitEventFlagRollsBackWhenRecoveryAcknowledgementFails(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	heldDir := filepath.Join(bundle, ".multikernel-held")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	recovered := make(chan struct{})
	close(recovered)
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: recovered}
	var hookErr error
	publisher := &fakePublisher{hook: func() {
		if hookErr == nil {
			hookErr = os.Rename(runtimeDir, heldDir)
		}
	}}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("task wait did not complete after the observed exit")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if p.exitEventQueued {
		t.Fatal("memory claimed the exit-event flag was durable")
	}
	data, err := os.ReadFile(filepath.Join(heldDir, "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Processes) != 1 || saved.Processes[0].Status != tasktypes.Status_STOPPED || saved.Processes[0].Exit != 37 || saved.Processes[0].ExitEventQueued {
		t.Fatalf("durable exit-event acknowledgement = %+v", saved.Processes)
	}
	if len(publisher.topics) != 1 || publisher.topics[0] != ctruntime.TaskExitEventTopic {
		t.Fatalf("published exit topics = %v", publisher.topics)
	}
}

func TestCloseIORetriesUntilGuestAcknowledges(t *testing.T) {
	client := &fakeAgentClient{fail: map[string]error{"CloseProcessStdin": errors.New("injected close failure")}}
	p := &process{id: "", status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err == nil {
		t.Fatal("failed guest close was accepted")
	}
	if !p.stdinClosed || p.stdinCloseAcked {
		t.Fatalf("failed close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	delete(client.fail, "CloseProcessStdin")
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !p.stdinClosed || !p.stdinCloseAcked {
		t.Fatalf("successful close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	calls := len(client.calls)
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != calls {
		t.Fatal("acknowledged repeated CloseIO contacted the guest")
	}
}

func TestCloseIOReconnectsTransportWithinCallerDeadline(t *testing.T) {
	client := &outputReconnectAgent{closeFailures: 1}
	p := &process{id: "", status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	if !p.stdinClosed || !p.stdinCloseAcked || client.closeCalls != 2 || client.reconnects != 1 {
		t.Fatalf("close reconnect requested=%v acked=%v calls=%d reconnects=%d", p.stdinClosed, p.stdinCloseAcked, client.closeCalls, client.reconnects)
	}
}

func TestCloseIOPersistsCreatedIntentAndDoesNotMutateStoppedProcess(t *testing.T) {
	created := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	stopped := &process{id: "stopped", status: tasktypes.Status_STOPPED, done: make(chan struct{})}
	s := &service{processes: map[string]*process{"": created, "stopped": stopped}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !created.stdinClosed || created.stdinCloseAcked {
		t.Fatalf("created close state = requested:%v acknowledged:%v", created.stdinClosed, created.stdinCloseAcked)
	}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatalf("repeated created CloseIO error = %v", err)
	}
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{ExecID: "stopped", Stdin: true}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped CloseIO error = %v", err)
	}
	if stopped.stdinClosed || stopped.stdinCloseAcked {
		t.Fatalf("stopped CloseIO mutated state: %+v", stopped)
	}
}

func TestStdinPumpRetriesRequestedCloseAndRecordsAcknowledgement(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinReader: eofReadWriteCloser{}, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged, reader := p.stdinCloseAcked, p.stdinReader
		s.mu.Unlock()
		if acknowledged && reader == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pump close state = acknowledged:%v reader:%v", acknowledged, reader)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRecoveredNoFIFOPumpRetriesPendingClose(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinClosed: true, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged := p.stdinCloseAcked
		s.mu.Unlock()
		if acknowledged {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovered no-FIFO close was not acknowledged")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestResizePtyRejectsInvalidRequests(t *testing.T) {
	stopped := &process{terminal: true, status: tasktypes.Status_STOPPED, width: 80, height: 24, sizeSet: true}
	s := &service{processes: map[string]*process{
		"":        {status: tasktypes.Status_CREATED},
		"stopped": stopped,
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("non-terminal resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 65536}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("oversized resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{ExecID: "stopped", Width: 91, Height: 37}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped resize error = %v", err)
	}
	if stopped.width != 80 || stopped.height != 24 || !stopped.sizeSet {
		t.Fatalf("rejected stopped resize mutated state: %+v", stopped)
	}
}

func TestResizePtyRollsBackIntentWhenGuestRejects(t *testing.T) {
	injected := errors.New("injected resize failure")
	fake := &fakeAgentClient{fail: map[string]error{"ResizeProcess": injected}}
	p := &process{terminal: true, status: tasktypes.Status_RUNNING, width: 80, height: 24, sizeSet: true}
	s := &service{agent: fake, processes: map[string]*process{"": p}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); !errors.Is(err, injected) {
		t.Fatalf("resize error = %v", err)
	}
	if p.width != 80 || p.height != 24 || !p.sizeSet {
		t.Fatalf("failed resize retained new intent: %+v", p)
	}
	delete(fake.fail, "ResizeProcess")
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 100, Height: 40}); err != nil {
		t.Fatal(err)
	}
	if p.width != 100 || p.height != 40 || !p.sizeSet {
		t.Fatalf("successful resize state: %+v", p)
	}
}

func TestExecRollsBackProcessOnAgentFailure(t *testing.T) {
	agentFailure := errors.New("injected agent failure")
	fake := &fakeAgentClient{fail: map[string]error{"ExecProcess": agentFailure}}
	s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	_, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "failed", Spec: encoded})
	if !errors.Is(err, agentFailure) {
		t.Fatalf("Exec() error = %v, want injected failure", err)
	}
	if _, exists := s.processes["failed"]; exists {
		t.Fatal("failed exec left a stale process")
	}
}

func TestExecCreationIntentReconcilesAmbiguousGuestMutation(t *testing.T) {
	newFixture := func(t *testing.T, fake *execCreationAgent) (*service, *fakePublisher, *taskapi.ExecProcessRequest) {
		t.Helper()
		bundle := t.TempDir()
		if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
			t.Fatal(err)
		}
		fake.bundle = bundle
		publisher := &fakePublisher{}
		s := &service{id: "task", namespace: "tests", bundle: bundle,
			sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: fake, publisher: publisher,
			processes: map[string]*process{"": {status: tasktypes.Status_RUNNING, done: make(chan struct{})}},
			events:    eventJournal{SchemaVersion: 1, NextSequence: 1}}
		spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
		encodedValue, err := typeurl.MarshalAny(spec)
		if err != nil {
			t.Fatal(err)
		}
		request := &taskapi.ExecProcessRequest{ID: "task", ExecID: "exec",
			Spec: &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}}
		return s, publisher, request
	}
	hasDurableExec := func(t *testing.T, bundle string) bool {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
		if err != nil {
			t.Fatal(err)
		}
		var saved persisted
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		for _, process := range saved.Processes {
			if process.ID == "exec" {
				return true
			}
		}
		return false
	}

	t.Run("lost reply confirms created owner", func(t *testing.T) {
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF,
			state: agent.ProcessState{ID: "exec", Status: "CREATED"}}
		s, publisher, request := newFixture(t, fake)
		if _, err := s.Exec(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if !fake.durableOwner || s.processes["exec"] == nil || !hasDurableExec(t, s.bundle) {
			t.Fatalf("confirmed creation ownership: before-call=%v memory=%v", fake.durableOwner, s.processes["exec"])
		}
		if fmt.Sprint(fake.calls) != "[ExecProcess StateProcess]" || fmt.Sprint(publisher.topics) != "[/tasks/exec-added]" {
			t.Fatalf("confirmed creation calls=%v events=%v", fake.calls, publisher.topics)
		}
	})

	t.Run("unavailable state and cleanup retain durable owner", func(t *testing.T) {
		stateFailure := errors.New("injected state outage")
		deleteFailure := errors.New("injected delete outage")
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF, stateErr: stateFailure, deleteErr: deleteFailure}
		s, publisher, request := newFixture(t, fake)
		_, err := s.Exec(context.Background(), request)
		if !errors.Is(err, stateFailure) || !errors.Is(err, deleteFailure) {
			t.Fatalf("ambiguous creation error = %v", err)
		}
		if !fake.durableOwner || s.processes["exec"] == nil || !hasDurableExec(t, s.bundle) || len(publisher.topics) != 0 {
			t.Fatalf("ambiguous creation ownership: before-call=%v memory=%v events=%v", fake.durableOwner, s.processes["exec"], publisher.topics)
		}
	})

	t.Run("confirmed absence removes durable intent", func(t *testing.T) {
		notFound := &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF, stateErr: notFound, deleteErr: notFound}
		s, publisher, request := newFixture(t, fake)
		if _, err := s.Exec(context.Background(), request); err == nil {
			t.Fatal("absent guest creation unexpectedly succeeded")
		}
		if !fake.durableOwner || s.processes["exec"] != nil || hasDurableExec(t, s.bundle) || len(publisher.topics) != 0 {
			t.Fatalf("absent creation ownership: before-call=%v memory=%v events=%v", fake.durableOwner, s.processes["exec"], publisher.topics)
		}
	})
}

func TestRecoveredCreatedExecRequiresExactGuestOwnership(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      agent.ProcessState
		stateErr   error
		wantExists bool
		wantErr    bool
		wantEvent  bool
	}{
		{name: "exact created", state: agent.ProcessState{ID: "exec", Status: "CREATED"}, wantExists: true, wantEvent: true},
		{name: "confirmed absent", stateErr: &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}},
		{name: "wrong identity", state: agent.ProcessState{ID: "other", Status: "CREATED"}, wantErr: true},
		{name: "running", state: agent.ProcessState{ID: "exec", PID: 41, Status: "RUNNING"}, wantErr: true},
		{name: "transport unavailable", stateErr: io.ErrUnexpectedEOF, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			fake := &execCreationAgent{state: test.state, stateErr: test.stateErr}
			publisher := &fakePublisher{}
			s := &service{id: "task", bundle: bundle, agent: fake, publisher: publisher,
				events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			exists, err := s.reconcileRecoveredCreatedExec(context.Background(), &process{id: "exec", status: tasktypes.Status_CREATED})
			if (err != nil) != test.wantErr || exists != test.wantExists {
				t.Fatalf("reconcile result exists=%v error=%v", exists, err)
			}
			if got := fmt.Sprint(publisher.topics); (got == "[/tasks/exec-added]") != test.wantEvent {
				t.Fatalf("recovered exec events = %v", publisher.topics)
			}
		})
	}
}

func TestExecRollbackRetainsOwnershipUntilGuestDeletion(t *testing.T) {
	deleteFailure := errors.New("injected exec cleanup failure")
	for name, failure := range map[string]error{
		"deleted":        nil,
		"already absent": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
		"delete failed":  deleteFailure,
	} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			fake := &fakeAgentClient{fail: map[string]error{}}
			if failure != nil {
				fake.fail["DeleteProcess"] = failure
			}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: fake,
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": {id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})},
				}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := s.rollbackExec(ctx, "exec")
			_, retained := s.processes["exec"]
			if errors.Is(failure, deleteFailure) {
				if !errors.Is(err, deleteFailure) || !retained {
					t.Fatalf("failed rollback error=%v retained=%v", err, retained)
				}
			} else if err != nil || retained {
				t.Fatalf("completed rollback error=%v retained=%v", err, retained)
			}
			data, readErr := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var saved persisted
			if readErr = json.Unmarshal(data, &saved); readErr != nil {
				t.Fatal(readErr)
			}
			found := false
			for _, process := range saved.Processes {
				found = found || process.ID == "exec"
			}
			if found != retained {
				t.Fatalf("durable exec ownership=%v, memory=%v", found, retained)
			}
		})
	}
}

func TestStartReturnsKillFailureAndRetainsMonitorAfterPersistenceFailure(t *testing.T) {
	bundle := t.TempDir()
	killFailure := errors.New("injected started-process kill failure")
	client := &startCleanupAgent{finish: make(chan struct{}), signalErr: killFailure}
	p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
		publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		processes: map[string]*process{
			"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
			"exec": p,
		}}
	_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
	if err == nil || !strings.Contains(err.Error(), "persist started process") || !errors.Is(err, killFailure) {
		t.Fatalf("Start cleanup error = %v", err)
	}
	if p.status != tasktypes.Status_RUNNING || p.pid != 41 {
		t.Fatalf("retained started process = status:%v pid:%d", p.status, p.pid)
	}
	select {
	case <-p.done:
		t.Fatal("failed Start abandoned its running-process monitor")
	default:
	}
	if err = os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	close(client.finish)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("retained monitor did not observe the exact later exit")
	}
	if p.status != tasktypes.Status_STOPPED || p.exit != 9 {
		t.Fatalf("observed retained-process exit = status:%v exit:%d", p.status, p.exit)
	}
}

func TestStartRejectsGuestPIDOutsideTaskRangeAndRetainsOwnership(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("host int cannot represent a PID above the Task v2 range")
	}
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	client := &startCleanupAgent{finish: make(chan struct{}), statePID: int(uint64(^uint32(0)) + 1)}
	p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	publisher := &fakePublisher{}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
		publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		processes: map[string]*process{
			"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
			"exec": p,
		}}
	_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
	if err == nil || !strings.Contains(err.Error(), "outside the Task v2 range") {
		t.Fatalf("Start invalid PID error = %v", err)
	}
	if p.status != tasktypes.Status_RUNNING || p.pid != 0 {
		t.Fatalf("unverified process ownership = status:%v pid:%d", p.status, p.pid)
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("invalid PID published lifecycle events: %v", publisher.topics)
	}
	data, readErr := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(data, []byte(`"status":2`)) || bytes.Contains(data, []byte(`"pid":`)) {
		t.Fatalf("unverified ownership was not persisted without a PID: %s", data)
	}
	close(client.finish)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("unverified process monitor did not observe cleanup completion")
	}
}

func TestStartRejectsMismatchedGuestProcessStateAndRetainsOwnership(t *testing.T) {
	tests := []struct {
		name, id, state string
	}{
		{name: "wrong identity", id: "other", state: "RUNNING"},
		{name: "created", state: "CREATED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			client := &startCleanupAgent{finish: make(chan struct{}), stateID: test.id, state: test.state}
			p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
			publisher := &fakePublisher{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
				publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": p,
				}}
			_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
			if err == nil || !strings.Contains(err.Error(), "mismatched identity or state") {
				t.Fatalf("Start mismatched state error = %v", err)
			}
			if p.status != tasktypes.Status_RUNNING || p.pid != 0 || len(publisher.topics) != 0 {
				t.Fatalf("mismatched state ownership status=%v pid=%d events=%v", p.status, p.pid, publisher.topics)
			}
			close(client.finish)
			select {
			case <-p.done:
			case <-time.After(time.Second):
				t.Fatal("mismatched-state process monitor did not observe cleanup")
			}
		})
	}
}

func TestStartReconcilesAmbiguousGuestResultWithoutLosingOwnership(t *testing.T) {
	startFailure := errors.New("injected start transport failure")
	stateFailure := errors.New("injected state transport failure")
	tests := []struct {
		name        string
		state       string
		stateErr    error
		wantSuccess bool
		wantCreated bool
		wantEvent   bool
	}{
		{name: "confirmed created remains retryable", state: "CREATED", wantCreated: true},
		{name: "running proves start applied", state: "RUNNING", wantSuccess: true, wantEvent: true},
		{name: "rapid stop proves start applied", state: "STOPPED", wantSuccess: true, wantEvent: true},
		{name: "unresolved transport failure retains ownership", stateErr: stateFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			client := &startCleanupAgent{finish: make(chan struct{}), startErr: startFailure, stateErr: test.stateErr, state: test.state}
			p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
			publisher := &fakePublisher{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
				publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": p,
				}}
			response, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
			if test.wantSuccess {
				if err != nil || response == nil || response.Pid != 41 {
					t.Fatalf("reconciled Start response=%+v error=%v", response, err)
				}
			} else if !errors.Is(err, startFailure) {
				t.Fatalf("ambiguous Start error = %v, want start failure", err)
			}
			startPublished := false
			for _, topic := range publisher.topics {
				startPublished = startPublished || topic == ctruntime.TaskExecStartedEventTopic
			}
			if startPublished != test.wantEvent {
				t.Fatalf("published events = %v, want start event=%v", publisher.topics, test.wantEvent)
			}
			if test.wantCreated {
				if p.status != tasktypes.Status_CREATED || p.pid != 0 {
					t.Fatalf("confirmed-created state=%v pid=%d", p.status, p.pid)
				}
				select {
				case <-p.done:
					t.Fatal("confirmed-created process completed")
				default:
				}
			} else {
				var expectedPID uint32
				if test.wantSuccess {
					expectedPID = 41
				}
				if p.status != tasktypes.Status_RUNNING || p.pid != expectedPID {
					t.Fatalf("retained ownership state=%v pid=%d", p.status, p.pid)
				}
				close(client.finish)
				select {
				case <-p.done:
				case <-time.After(time.Second):
					t.Fatal("reconciled process monitor did not observe completion")
				}
			}
		})
	}
}

func TestEnsureGuestProcessCreatedReconcilesBeforeMutation(t *testing.T) {
	notFound := &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}}
	transportFailure := errors.New("injected state transport failure")
	tests := []struct {
		name       string
		state      agent.ProcessState
		stateErr   error
		createErr  error
		wantCreate int
		wantErr    string
	}{
		{name: "absent is created", stateErr: notFound, wantCreate: 1},
		{name: "create failure is returned", stateErr: notFound, createErr: errors.New("injected create failure"), wantCreate: 1, wantErr: "create failure"},
		{name: "existing created is reused", state: agent.ProcessState{ID: "init", Status: "CREATED"}},
		{name: "wrong identity", state: agent.ProcessState{ID: "other", Status: "CREATED"}, wantErr: "mismatched identity or state"},
		{name: "running is not recreated", state: agent.ProcessState{ID: "init", PID: 41, Status: "RUNNING"}, wantErr: "mismatched identity or state"},
		{name: "created with pid is invalid", state: agent.ProcessState{ID: "init", PID: 41, Status: "CREATED"}, wantErr: "mismatched identity or state"},
		{name: "transport failure is not absence", stateErr: transportFailure, wantErr: "state transport failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &createReconcileAgent{state: test.state, stateErr: test.stateErr, createErr: test.createErr}
			err := ensureGuestProcessCreated(context.Background(), client, "init", "/bundle")
			if test.wantErr == "" && err != nil || test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("reconcile error = %v, want substring %q", err, test.wantErr)
			}
			if client.stateCalls != 1 || client.createCalls != test.wantCreate {
				t.Fatalf("calls state=%d create=%d, want 1/%d", client.stateCalls, client.createCalls, test.wantCreate)
			}
		})
	}
}

func TestStartedProcessCleanupTreatsAuthenticatedAbsenceAsSuccess(t *testing.T) {
	client := &fakeAgentClient{fail: map[string]error{
		"SignalProcess": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
	}}
	s := &service{agent: client}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.killStartedProcess(ctx, "exec"); err != nil {
		t.Fatal(err)
	}
}

func TestExecRejectsGuestUnrepresentableIDBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	for _, id := range []string{"", "../exec", "Exec", "exec_id", strings.Repeat("a", 65)} {
		if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: id, Spec: encoded}); !errors.Is(err, errdefs.ErrInvalidArgument) {
			t.Fatalf("ExecID %q error = %v, want invalid argument", id, err)
		}
	}
	if len(fake.calls) != 0 || len(s.processes) != 0 {
		t.Fatalf("invalid exec reached agent or state: calls=%v processes=%v", fake.calls, s.processes)
	}
}

func TestTaskStateGuardsRejectInvalidTransitionsBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	init := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	execProcess := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}

	if _, err := s.Start(context.Background(), &taskapi.StartRequest{ExecID: "exec"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec start before init error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "missing", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrNotFound) {
		t.Fatalf("missing kill error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("created kill error = %v", err)
	}
	init.status = tasktypes.Status_STOPPED
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "new-exec", Spec: encoded}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec after init stop error = %v", err)
	}
	if _, err = s.Delete(context.Background(), &taskapi.DeleteRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("init delete with retained exec error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("invalid transitions contacted agent: %v", fake.calls)
	}
}

func TestPreCancelledTaskMutationsPreserveStateAndAvoidGuestContact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assertCanceled := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	}

	t.Run("create", func(t *testing.T) {
		s := &service{id: "task", bundle: "/bundle", processes: map[string]*process{}}
		_, err := s.Create(ctx, &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"})
		assertCanceled(t, err)
		if len(s.processes) != 0 {
			t.Fatalf("processes = %+v", s.processes)
		}
	})

	t.Run("start", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		init := &process{status: tasktypes.Status_RUNNING}
		execProcess := &process{status: tasktypes.Status_CREATED}
		s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}
		_, err := s.Start(ctx, &taskapi.StartRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if execProcess.status != tasktypes.Status_CREATED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", execProcess.status, fake.calls)
		}
	})

	t.Run("kill", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Kill(ctx, &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM)})
		assertCanceled(t, err)
		if len(fake.calls) != 0 {
			t.Fatalf("calls=%v", fake.calls)
		}
	})

	t.Run("exec", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Exec(ctx, &taskapi.ExecProcessRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if len(s.processes) != 1 || len(fake.calls) != 0 {
			t.Fatalf("processes=%+v calls=%v", s.processes, fake.calls)
		}
	})

	t.Run("delete", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_STOPPED}
		s := &service{agent: fake, processes: map[string]*process{"exec": p}}
		_, err := s.Delete(ctx, &taskapi.DeleteRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if p.deleting || p.exitEventQueued || p.deleteEventQueued || len(fake.calls) != 0 {
			t.Fatalf("process=%+v calls=%v", p, fake.calls)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		s := &service{processes: map[string]*process{}}
		_, err := s.Shutdown(ctx, &taskapi.ShutdownRequest{})
		assertCanceled(t, err)
		if s.shuttingDown {
			t.Fatal("cancelled shutdown sealed service")
		}
	})

	t.Run("resize", func(t *testing.T) {
		p := &process{terminal: true, status: tasktypes.Status_CREATED, width: 80, height: 24, sizeSet: true}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.ResizePty(ctx, &taskapi.ResizePtyRequest{Width: 100, Height: 40})
		assertCanceled(t, err)
		if p.width != 80 || p.height != 24 || !p.sizeSet {
			t.Fatalf("size=%dx%d set=%v", p.width, p.height, p.sizeSet)
		}
	})

	t.Run("pause", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_RUNNING}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Pause(ctx, &taskapi.PauseRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_RUNNING || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("resume", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_PAUSED}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Resume(ctx, &taskapi.ResumeRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_PAUSED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("close-io", func(t *testing.T) {
		p := &process{status: tasktypes.Status_CREATED}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true})
		assertCanceled(t, err)
		if p.stdinClosed || p.stdinCloseAcked {
			t.Fatalf("close state=requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
		}
	})
}

func TestPreCancelledTaskReadsAndWaitAvoidGuestContact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeAgentClient{fail: map[string]error{}}
	done := make(chan struct{})
	s := &service{agent: fake, processes: map[string]*process{"": {
		pid: 41, status: tasktypes.Status_RUNNING, done: done,
	}}}
	for name, invoke := range map[string]func() error{
		"state": func() error {
			_, err := s.State(ctx, &taskapi.StateRequest{})
			return err
		},
		"wait": func() error {
			_, err := s.Wait(ctx, &taskapi.WaitRequest{})
			return err
		},
		"pids": func() error {
			_, err := s.Pids(ctx, &taskapi.PidsRequest{})
			return err
		},
		"connect": func() error {
			_, err := s.Connect(ctx, &taskapi.ConnectRequest{})
			return err
		},
		"stats": func() error {
			_, err := s.Stats(ctx, &taskapi.StatsRequest{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invoke(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
	if len(fake.calls) != 0 {
		t.Fatalf("cancelled reads contacted guest: %v", fake.calls)
	}
	select {
	case <-done:
		t.Fatal("cancelled Wait changed process completion")
	default:
	}
}

func TestTaskEntryLockWaitHonorsCancellation(t *testing.T) {
	for name, invoke := range map[string]func(context.Context, *service) error{
		"state": func(ctx context.Context, s *service) error {
			_, err := s.State(ctx, &taskapi.StateRequest{})
			return err
		},
		"close-io": func(ctx context.Context, s *service) error {
			_, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &service{processes: map[string]*process{"": {status: tasktypes.Status_CREATED}}}
			s.mu.Lock()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() { result <- invoke(ctx, s) }()
			time.Sleep(10 * time.Millisecond)
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					s.mu.Unlock()
					t.Fatalf("contended call error = %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				s.mu.Unlock()
				t.Fatal("contended Task call ignored cancellation")
			}
			if s.processes[""].stdinClosed {
				s.mu.Unlock()
				t.Fatal("cancelled contended call mutated process state")
			}
			s.mu.Unlock()
		})
	}
}

func TestWaitPostExitLockHonorsCancellation(t *testing.T) {
	done := make(chan struct{})
	s := &service{processes: map[string]*process{"": {status: tasktypes.Status_RUNNING, done: done}}}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := s.Wait(ctx, &taskapi.WaitRequest{})
		result <- err
	}()
	// Allow Wait to retain the process and block on its completion channel.
	time.Sleep(10 * time.Millisecond)
	s.mu.Lock()
	close(done)
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.mu.Unlock()
			t.Fatalf("post-exit lock error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.mu.Unlock()
		t.Fatal("Wait ignored cancellation while reacquiring its post-exit lock")
	}
	s.mu.Unlock()
}

func TestKillForwardsOnlyForLiveKnownProcess(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_PAUSED},
	}}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM), All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "exec", Signal: uint32(syscall.SIGKILL)}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[SignalProcess SignalProcess]" {
		t.Fatalf("kill calls = %v", fake.calls)
	}
}

func TestShutdownWaitsForEmptyOwnershipAndSealsService(t *testing.T) {
	called := make(chan struct{}, 1)
	s := &service{id: "task", bundle: "/bundle", shutdown: func() { called <- struct{}{} }, processes: map[string]*process{
		"": {status: tasktypes.Status_STOPPED},
	}}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task", Now: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("shutdown callback ran while process ownership remained")
	default:
	}
	s.mu.Lock()
	delete(s.processes, "")
	s.mu.Unlock()
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("empty service did not invoke shutdown callback")
	}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("create after shutdown error = %v", err)
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("repeated shutdown invoked callback again")
	default:
	}
}

func TestShutdownRetainsOwnershipUntilDurableEventsFlush(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{failures: 2}
	called := make(chan struct{}, 1)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher,
		shutdown: func() { called <- struct{}{} }, processes: map[string]*process{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: "task", ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if len(s.events.Pending) != 1 {
		t.Fatalf("pending events = %d", len(s.events.Pending))
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err == nil {
		t.Fatal("shutdown accepted while durable event delivery failed")
	}
	select {
	case <-called:
		t.Fatal("failed event flush invoked shutdown callback")
	default:
	}
	publisher.failures = 0
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not proceed after event delivery recovered")
	}
	if len(s.events.Pending) != 0 {
		t.Fatalf("events remain pending after shutdown: %+v", s.events.Pending)
	}
}

func TestStatePidsAndConnectReportGuestProcessIDs(t *testing.T) {
	s := &service{bundle: "/bundle", processes: map[string]*process{
		"":     {pid: 17, status: tasktypes.Status_RUNNING},
		"exec": {pid: 23, status: tasktypes.Status_RUNNING},
	}}
	state, err := s.State(context.Background(), &taskapi.StateRequest{ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Pid != 23 {
		t.Fatalf("State PID = %d, want guest PID 23", state.Pid)
	}
	pids, err := s.Pids(context.Background(), &taskapi.PidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint32]bool{}
	for _, process := range pids.Processes {
		got[process.Pid] = true
	}
	if !got[17] || !got[23] || len(got) != 2 {
		t.Fatalf("Pids = %v, want guest PIDs 17 and 23", got)
	}
	connected, err := s.Connect(context.Background(), &taskapi.ConnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if connected.TaskPid != 17 || connected.ShimPid != uint32(os.Getpid()) {
		t.Fatalf("Connect = task:%d shim:%d", connected.TaskPid, connected.ShimPid)
	}
}

func TestTaskRPCsRejectNilAndMismatchedTaskIdentityBeforeMutation(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{id: "task", bundle: "/bundle", agent: fake, processes: map[string]*process{
		"": {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
	}}
	tests := []struct {
		name  string
		nil   func() error
		wrong func() error
	}{
		{"Create", func() error { _, err := s.Create(context.Background(), nil); return err }, func() error {
			_, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "other"})
			return err
		}},
		{"Start", func() error { _, err := s.Start(context.Background(), nil); return err }, func() error { _, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "other"}); return err }},
		{"State", func() error { _, err := s.State(context.Background(), nil); return err }, func() error { _, err := s.State(context.Background(), &taskapi.StateRequest{ID: "other"}); return err }},
		{"Wait", func() error { _, err := s.Wait(context.Background(), nil); return err }, func() error { _, err := s.Wait(context.Background(), &taskapi.WaitRequest{ID: "other"}); return err }},
		{"Kill", func() error { _, err := s.Kill(context.Background(), nil); return err }, func() error { _, err := s.Kill(context.Background(), &taskapi.KillRequest{ID: "other"}); return err }},
		{"Exec", func() error { _, err := s.Exec(context.Background(), nil); return err }, func() error {
			_, err := s.Exec(context.Background(), &taskapi.ExecProcessRequest{ID: "other"})
			return err
		}},
		{"Delete", func() error { _, err := s.Delete(context.Background(), nil); return err }, func() error {
			_, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "other"})
			return err
		}},
		{"Pids", func() error { _, err := s.Pids(context.Background(), nil); return err }, func() error { _, err := s.Pids(context.Background(), &taskapi.PidsRequest{ID: "other"}); return err }},
		{"Connect", func() error { _, err := s.Connect(context.Background(), nil); return err }, func() error {
			_, err := s.Connect(context.Background(), &taskapi.ConnectRequest{ID: "other"})
			return err
		}},
		{"Shutdown", func() error { _, err := s.Shutdown(context.Background(), nil); return err }, func() error {
			_, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "other", Now: true})
			return err
		}},
		{"ResizePty", func() error { _, err := s.ResizePty(context.Background(), nil); return err }, func() error {
			_, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{ID: "other"})
			return err
		}},
		{"Pause", func() error { _, err := s.Pause(context.Background(), nil); return err }, func() error { _, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "other"}); return err }},
		{"Resume", func() error { _, err := s.Resume(context.Background(), nil); return err }, func() error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "other"})
			return err
		}},
		{"CloseIO", func() error { _, err := s.CloseIO(context.Background(), nil); return err }, func() error {
			_, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{ID: "other"})
			return err
		}},
		{"Stats", func() error { _, err := s.Stats(context.Background(), nil); return err }, func() error { _, err := s.Stats(context.Background(), &taskapi.StatsRequest{ID: "other"}); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for name, invoke := range map[string]func() error{"nil": test.nil, "mismatched": test.wrong} {
				t.Run(name, func(t *testing.T) {
					if err := invoke(); !errors.Is(err, errdefs.ErrInvalidArgument) {
						t.Fatalf("error = %v, want invalid argument", err)
					}
				})
			}
		})
	}
	if len(fake.calls) != 0 || len(s.processes) != 1 || s.processes[""].status != tasktypes.Status_RUNNING || s.shuttingDown {
		t.Fatalf("invalid requests mutated service: calls=%v processes=%+v shuttingDown=%v", fake.calls, s.processes, s.shuttingDown)
	}
}

func TestPauseResumeSignalsGuestAndPublishesTransitions(t *testing.T) {
	fakeAgent := &fakeAgentClient{fail: map[string]error{}}
	fakeEvents := &fakePublisher{}
	s := &service{
		id: "task", namespace: "default", bundle: t.TempDir(), agent: fakeAgent, publisher: fakeEvents,
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING}},
	}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_PAUSED || s.processes["exec"].status != tasktypes.Status_PAUSED {
		t.Fatalf("pause statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if _, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("resume statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if len(fakeAgent.calls) != 4 {
		t.Fatalf("agent calls = %v", fakeAgent.calls)
	}
	if len(fakeEvents.topics) != 2 || fakeEvents.topics[0] != ctruntime.TaskPausedEventTopic || fakeEvents.topics[1] != ctruntime.TaskResumedEventTopic {
		t.Fatalf("event topics = %v", fakeEvents.topics)
	}
}

func TestPauseRollsBackAlreadySignaledProcessOnPartialFailure(t *testing.T) {
	fake := &signalFailureAgent{failAt: 2}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{}); err == nil {
		t.Fatal("partial pause failure was accepted")
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("partial pause mutated states: init=%v exec=%v", s.processes[""].status, s.processes["exec"].status)
	}
	want := fmt.Sprintf("[init:%d exec:%d init:%d]", syscall.SIGSTOP, syscall.SIGSTOP, syscall.SIGCONT)
	if fmt.Sprint(fake.calls) != want {
		t.Fatalf("partial pause calls = %v, want %s", fake.calls, want)
	}
}

func TestPauseResumeReturnRecoveryRollbackFailure(t *testing.T) {
	for name, test := range map[string]struct {
		initial          tasktypes.Status
		rollbackSignal   syscall.Signal
		durableOnFailure tasktypes.Status
		invoke           func(*service) error
		errorLabel       string
	}{
		"pause": {tasktypes.Status_RUNNING, syscall.SIGCONT, tasktypes.Status_PAUSED, func(s *service) error {
			_, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"})
			return err
		}, "persist pause rollback"},
		"resume": {tasktypes.Status_PAUSED, syscall.SIGSTOP, tasktypes.Status_RUNNING, func(s *service) error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"})
			return err
		}, "persist resume rollback"},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			runtimeDir := filepath.Join(bundle, ".multikernel")
			heldDir := filepath.Join(bundle, ".multikernel-held")
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			var hookErr error
			fake := &signalFailureAgent{hook: func(call string) {
				if call == fmt.Sprintf("init:%d", test.rollbackSignal) {
					hookErr = os.Rename(runtimeDir, heldDir)
				}
			}}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
				agent:   fake, publisher: &fakePublisher{}, processes: map[string]*process{
					"": {status: test.initial, done: make(chan struct{})},
				}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			originalMarshal := jsonMarshal
			jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected transition event failure") }
			err := test.invoke(s)
			jsonMarshal = originalMarshal
			if hookErr != nil {
				t.Fatal(hookErr)
			}
			if err == nil || !strings.Contains(err.Error(), "injected transition event failure") || !strings.Contains(err.Error(), test.errorLabel) {
				t.Fatalf("transition rollback error = %v", err)
			}
			if observed := s.processes[""].status; observed != test.initial {
				t.Fatalf("memory state after rollback = %v, want %v", observed, test.initial)
			}
			data, readErr := os.ReadFile(filepath.Join(heldDir, "sandbox.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var saved persisted
			if readErr = json.Unmarshal(data, &saved); readErr != nil {
				t.Fatal(readErr)
			}
			if len(saved.Processes) != 1 || saved.Processes[0].Status != test.durableOnFailure {
				t.Fatalf("unrepaired durable transition = %+v", saved.Processes)
			}
		})
	}
}

func TestPauseResumeRepublishPriorStateAfterTransitionPersistenceFailure(t *testing.T) {
	for name, test := range map[string]struct {
		initial        tasktypes.Status
		forwardSignal  syscall.Signal
		rollbackSignal syscall.Signal
		invoke         func(*service) error
		errorLabel     string
	}{
		"pause": {tasktypes.Status_RUNNING, syscall.SIGSTOP, syscall.SIGCONT, func(s *service) error {
			_, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"})
			return err
		}, "persist paused state"},
		"resume": {tasktypes.Status_PAUSED, syscall.SIGCONT, syscall.SIGSTOP, func(s *service) error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"})
			return err
		}, "persist resumed state"},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			runtimeDir := filepath.Join(bundle, ".multikernel")
			heldDir := filepath.Join(bundle, ".multikernel-held")
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			var hookErr error
			fake := &signalFailureAgent{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
				agent:   fake, publisher: &fakePublisher{}, processes: map[string]*process{
					"": {status: test.initial, done: make(chan struct{})},
				}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := s.persistRecovery(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtimeDir, "sandbox.json")
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			fake.hook = func(call string) {
				switch call {
				case fmt.Sprintf("init:%d", test.forwardSignal):
					hookErr = os.Rename(runtimeDir, heldDir)
				case fmt.Sprintf("init:%d", test.rollbackSignal):
					if hookErr == nil {
						hookErr = os.Rename(heldDir, runtimeDir)
					}
				}
			}
			err = test.invoke(s)
			if hookErr != nil {
				t.Fatal(hookErr)
			}
			if err == nil || !strings.Contains(err.Error(), test.errorLabel) {
				t.Fatalf("transition persistence error = %v", err)
			}
			if observed := s.processes[""].status; observed != test.initial {
				t.Fatalf("memory state after persistence rollback = %v, want %v", observed, test.initial)
			}
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(before, after) {
				t.Fatal("prior recovery state was not republished after transition failure")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved persisted
			if err = json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Processes) != 1 || saved.Processes[0].Status != test.initial {
				t.Fatalf("republished durable state = %+v", saved.Processes)
			}
		})
	}
}

func TestStatsReturnsGuestProcessGroupMetrics(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: 11, CPUSystemNS: 7, RSSBytes: 4096, PIDs: 3},
		"exec": {CPUUserNS: 5, CPUSystemNS: 2, RSSBytes: 2048, PIDs: 1},
	}}
	s := &service{
		agent: fake, sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{MemoryBytes: 3 << 30}},
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_PAUSED}, "created": {status: tasktypes.Status_CREATED}},
	}
	response, err := s.Stats(context.Background(), &taskapi.StatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typeurl.UnmarshalAny(response.Stats)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := decoded.(*cgroupstats.Metrics)
	if !ok {
		t.Fatalf("stats type = %T", decoded)
	}
	if metrics.CPU.Usage.Total != 25 || metrics.CPU.Usage.User != 16 || metrics.CPU.Usage.Kernel != 9 || metrics.Memory.Usage.Usage != 6144 || metrics.Memory.Usage.Limit != 3<<30 || metrics.Pids.Current != 4 {
		t.Fatalf("stats = %+v", metrics)
	}
	if fmt.Sprint(fake.calls) != "[StatsProcess StatsProcess]" {
		t.Fatalf("stats calls = %v", fake.calls)
	}
}

func TestStatsRejectsAggregateOverflow(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: ^uint64(0)},
		"exec": {CPUUserNS: 1},
	}}
	s := &service{agent: fake, processes: map[string]*process{
		"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Stats(context.Background(), &taskapi.StatsRequest{}); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow stats error = %v", err)
	}
}

func TestUpdateAndCheckpointAreExcludedWithoutMutation(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	events := &fakePublisher{}
	s := &service{agent: fake, publisher: events, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	if _, err := s.Update(context.Background(), &taskapi.UpdateTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Update error = %v", err)
	}
	if _, err := s.Checkpoint(context.Background(), &taskapi.CheckpointTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Checkpoint error = %v", err)
	}
	if len(fake.calls) != 0 || len(events.topics) != 0 || s.processes[""].status != tasktypes.Status_RUNNING {
		t.Fatalf("excluded methods mutated state: calls=%v events=%v status=%v", fake.calls, events.topics, s.processes[""].status)
	}
}

func TestEventJournalPreservesOrderAndReplaysAfterFailure(t *testing.T) {
	bundle := t.TempDir()
	failed := &fakePublisher{failures: 2}
	s := &service{bundle: bundle, namespace: "default", publisher: failed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	if len(failed.topics) != 0 {
		t.Fatalf("later event bypassed failed predecessor: %v", failed.topics)
	}
	replayed := &fakePublisher{}
	recovered := &service{bundle: bundle, namespace: "default", publisher: replayed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := recovered.loadEventJournal(); err != nil {
		t.Fatal(err)
	}
	if err := recovered.flushEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskCreateEventTopic, ctruntime.TaskStartEventTopic}
	if fmt.Sprint(replayed.topics) != fmt.Sprint(want) {
		t.Fatalf("replayed topics = %v, want %v", replayed.topics, want)
	}
	if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged journal remains: %v", err)
	}
}

func TestEventJournalRefusesReplacedPathForWriteAndAcknowledgement(t *testing.T) {
	publisher := &fakePublisher{failures: 1}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(t.Context(), ctruntime.TaskCreateEventTopic,
		&eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	path := s.eventJournalPath()
	if err := os.Rename(path, path+".owned"); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("caller replacement")
	if err := os.WriteFile(path, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(t.Context(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err == nil {
		t.Fatal("event journal write replaced an unowned inode")
	}
	if len(s.events.Pending) != 1 || s.events.NextSequence != 2 {
		t.Fatalf("failed replacement write changed event ownership: %+v", s.events)
	}
	if err := s.flushEvents(t.Context()); err == nil {
		t.Fatal("event acknowledgement removed an unowned inode")
	}
	if len(s.events.Pending) != 1 || len(publisher.topics) != 1 {
		t.Fatalf("failed acknowledgement lost replay state: pending=%+v topics=%v", s.events.Pending, publisher.topics)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, replacement) {
		t.Fatalf("replacement journal changed: %q, %v", data, err)
	}
}

func TestEventJournalLockWaitHonorsCancellationWithoutMutation(t *testing.T) {
	s := &service{id: "task", namespace: "default", bundle: t.TempDir(), publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	s.eventMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- s.publish(ctx, ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 41})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.eventMu.Unlock()
			t.Fatalf("contended publication error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.eventMu.Unlock()
		t.Fatal("contended event publication ignored cancellation")
	}
	if len(s.events.Pending) != 0 || s.events.NextSequence != 1 {
		s.eventMu.Unlock()
		t.Fatalf("cancelled publication mutated journal: %+v", s.events)
	}
	s.eventMu.Unlock()

	s.eventMu.Lock()
	ctx, cancel = context.WithCancel(context.Background())
	result = make(chan error, 1)
	go func() { result <- s.flushEvents(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.eventMu.Unlock()
			t.Fatalf("contended flush error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.eventMu.Unlock()
		t.Fatal("contended event flush ignored cancellation")
	}
	s.eventMu.Unlock()
}

func TestEventJournalRetriesWithoutAnotherLifecycleRequest(t *testing.T) {
	publisher := &retryPublisher{published: make(chan string, 1)}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	s.startEventRetry(5 * time.Millisecond)
	defer s.stopEventRetry()
	select {
	case topic := <-publisher.published:
		if topic != ctruntime.TaskStartEventTopic {
			t.Fatalf("retried topic = %q", topic)
		}
	case <-time.After(time.Second):
		t.Fatal("durable event was not retried while the shim remained running")
	}
	s.stopEventRetry()
	if len(s.events.Pending) != 0 {
		t.Fatalf("acknowledged retry remains pending: %+v", s.events.Pending)
	}
	if _, err := os.Stat(s.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged retry journal remains: %v", err)
	}
}

func TestEventJournalPersistenceFailurePrecedesPublication(t *testing.T) {
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected journal failure") }
	defer func() { jsonMarshal = originalMarshal }()
	publisher := &fakePublisher{}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err == nil {
		t.Fatal("journal failure was ignored")
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("event published before durable queue: %v", publisher.topics)
	}
	if len(s.events.Pending) != 0 || s.events.NextSequence != 1 {
		t.Fatalf("failed queue mutated memory: %+v", s.events)
	}
}

func TestEventJournalAckFailureRetainsPublishedEventForReplay(t *testing.T) {
	publisher := &fakePublisher{failures: 2}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	for _, item := range []struct {
		topic string
		event any
	}{{ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}},
		{ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}}} {
		if err := s.publish(context.Background(), item.topic, item.event); err != nil {
			t.Fatal(err)
		}
	}
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected acknowledgement failure") }
	defer func() { jsonMarshal = originalMarshal }()
	if err := s.flushEvents(context.Background()); err == nil {
		t.Fatal("acknowledgement failure was ignored")
	}
	if len(publisher.topics) != 1 || len(s.events.Pending) != 2 || s.events.Pending[0].Sequence != 1 {
		t.Fatalf("at-least-once replay state was lost: topics=%v events=%+v", publisher.topics, s.events)
	}
}

func TestEventJournalRejectsSymlinkAndPermissiveState(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{"symlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
				return err
			}
			return os.Symlink(target, path)
		}},
		{"permissive", func(path string) error {
			return os.WriteFile(path, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0644)
		}},
		{"hardlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
				return err
			}
			return os.Link(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &service{bundle: t.TempDir(), events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := test.make(s.eventJournalPath()); err != nil {
				t.Fatal(err)
			}
			if err := s.loadEventJournal(); err == nil {
				t.Fatal("unsafe event journal accepted")
			}
		})
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		directory := t.TempDir()
		realBundle := filepath.Join(directory, "real")
		if err := os.Mkdir(realBundle, 0700); err != nil {
			t.Fatal(err)
		}
		realJournal := filepath.Join(realBundle, ".multikernel-events.json")
		if err := os.WriteFile(realJournal, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		linkedBundle := filepath.Join(directory, "linked")
		if err := os.Symlink(realBundle, linkedBundle); err != nil {
			t.Fatal(err)
		}
		s := &service{bundle: linkedBundle, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
		if err := s.loadEventJournal(); err == nil {
			t.Fatal("event journal beneath a symlinked ancestor was accepted")
		}
	})
}

func TestDeleteRepairsMissingExitEventBeforeDeleteEvent(t *testing.T) {
	publisher := &fakePublisher{}
	fake := &fakeAgentClient{fail: map[string]error{}}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exit: 17,
		exited: time.Unix(123, 0).UTC(), done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: t.TempDir(), publisher: publisher, agent: fake,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task", ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskExitEventTopic, ctruntime.TaskDeleteEventTopic}
	if response.ExitStatus != 17 || fmt.Sprint(publisher.topics) != fmt.Sprint(want) {
		t.Fatalf("response=%+v topics=%v, want %v", response, publisher.topics, want)
	}
	if _, exists := s.processes["exec"]; exists {
		t.Fatal("deleted exec process remains")
	}
}

func TestDeleteTreatsAuthenticatedGuestAbsenceAsIdempotentSuccess(t *testing.T) {
	publisher := &fakePublisher{}
	fake := &fakeAgentClient{fail: map[string]error{
		"DeleteProcess": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
	}}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exit: 17,
		exited: time.Unix(123, 0).UTC(), exitEventQueued: true, done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: t.TempDir(), publisher: publisher, agent: fake,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task", ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Pid != 23 || response.ExitStatus != 17 {
		t.Fatalf("idempotent delete response = %+v", response)
	}
	if _, exists := s.processes["exec"]; exists {
		t.Fatal("confirmed-absent guest retained ownership")
	}
	if fmt.Sprint(publisher.topics) != "[/tasks/delete]" {
		t.Fatalf("idempotent delete events = %v", publisher.topics)
	}
}

func TestGuestShutdownRetriesQuiesceAndAcceptsLostTerminalReply(t *testing.T) {
	fake := &shutdownBoundaryAgent{loseFirstQuiesce: true, loseShutdown: true}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
	if err := s.quiesceAndShutdownGuest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[Quiesce Quiesce Shutdown]" || fake.reconnects != 1 {
		t.Fatalf("shutdown calls=%v reconnects=%d", fake.calls, fake.reconnects)
	}
}

func TestGuestNetworkCloseRetriesLostReply(t *testing.T) {
	fake := &shutdownBoundaryAgent{loseFirstClose: true}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second, networkCloseTimeout: time.Second}
	if err := s.stopNetwork(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[CloseNetwork CloseNetwork]" || fake.reconnects != 1 {
		t.Fatalf("network close calls=%v reconnects=%d", fake.calls, fake.reconnects)
	}
}

func TestGuestNetworkConfigurationRetriesLostReply(t *testing.T) {
	fake := &shutdownBoundaryAgent{loseFirstConfigure: true}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
	config := agent.NetworkConfig{Name: "mkn0", Address: "192.0.2.2/30", Gateway: "192.0.2.1", MTU: 1500,
		Nameservers: []string{"192.0.2.53"}}
	if err := s.configureGuestNetwork(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[ConfigureNetwork ConfigureNetwork]" || fake.reconnects != 1 {
		t.Fatalf("network configure calls=%v reconnects=%d", fake.calls, fake.reconnects)
	}
}

func TestGuestProcessDeleteReconnectsAndConfirmsLostReply(t *testing.T) {
	fake := &shutdownBoundaryAgent{loseFirstDelete: true}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
	if err := s.deleteGuestProcess(context.Background(), "exec"); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[DeleteProcess DeleteProcess]" || fake.reconnects != 1 {
		t.Fatalf("process delete calls=%v reconnects=%d", fake.calls, fake.reconnects)
	}
}

func TestGuestShutdownRejectsUnprovenOrAuthenticatedFailure(t *testing.T) {
	t.Run("invalid quiesce acknowledgement", func(t *testing.T) {
		fake := &shutdownBoundaryAgent{quiesceStatus: "unknown"}
		s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
		if err := s.quiesceAndShutdownGuest(context.Background()); err == nil || !strings.Contains(err.Error(), "unexpected status") {
			t.Fatalf("quiesce error = %v", err)
		}
		if fmt.Sprint(fake.calls) != "[Quiesce]" {
			t.Fatalf("invalid quiesce calls = %v", fake.calls)
		}
	})

	t.Run("authenticated shutdown rejection", func(t *testing.T) {
		remote := &agent.RemoteError{Failure: protocol.Error{Code: "FAILED_PRECONDITION", Message: "injected rejection"}}
		fake := &shutdownBoundaryAgent{shutdownRemoteError: remote}
		s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
		if err := s.quiesceAndShutdownGuest(context.Background()); !errors.Is(err, remote) {
			t.Fatalf("shutdown error = %v", err)
		}
	})

	t.Run("cancellation after quiescence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		fake := &shutdownBoundaryAgent{afterQuiesce: cancel}
		s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
		if err := s.quiesceAndShutdownGuest(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown cancellation error = %v", err)
		}
		if fmt.Sprint(fake.calls) != "[Quiesce]" {
			t.Fatalf("cancelled shutdown calls = %v", fake.calls)
		}
	})
}

func TestDeleteRetainsRetryOwnershipAcrossGuestAndEventFailures(t *testing.T) {
	bundle := t.TempDir()
	agentFailure := errors.New("injected guest delete failure")
	fakeAgent := &fakeAgentClient{fail: map[string]error{"DeleteProcess": agentFailure}}
	publisher := &fakePublisher{}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exitEventQueued: true, done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher, agent: fakeAgent,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	request := &taskapi.DeleteRequest{ID: "task", ExecID: "exec"}
	if _, err := s.Delete(context.Background(), request); !errors.Is(err, agentFailure) {
		t.Fatalf("guest delete error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || p.deleteEventQueued {
		t.Fatalf("guest failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	delete(fakeAgent.fail, "DeleteProcess")
	publisher.failures = 2
	if _, err := s.Delete(context.Background(), request); err == nil || !strings.Contains(err.Error(), "flush task delete event") {
		t.Fatalf("event flush error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || !p.deleteEventQueued {
		t.Fatalf("event failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	publisher.failures = 0
	response, err := s.Delete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Pid != 23 || s.processes["exec"] != nil || len(publisher.topics) != 1 || publisher.topics[0] != ctruntime.TaskDeleteEventTopic {
		t.Fatalf("retry response=%+v processes=%v topics=%v", response, s.processes, publisher.topics)
	}
}

func TestRecoveryStatePersistsGenerationProcessesAndOffsetsAtomically(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	stdout := filepath.Join(bundle, "stdout")
	if err := os.WriteFile(stdout, nil, 0600); err != nil {
		t.Fatal(err)
	}
	stdoutIdentity, err := inspectBoundProcessIOPath(stdout, false)
	if err != nil {
		t.Fatal(err)
	}
	stdin := filepath.Join(bundle, "stdin")
	if err = syscall.Mkfifo(stdin, 0600); err != nil {
		t.Fatal(err)
	}
	stdinIdentity, err := inspectBoundProcessIOPath(stdin, true)
	if err != nil {
		t.Fatal(err)
	}
	s := &service{
		bundle:  bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: "0123456789abcdef0123456789abcdef"},
		processes: map[string]*process{
			"": {id: "", pid: 7, status: tasktypes.Status_RUNNING, stdin: stdin, stdinIdentity: stdinIdentity,
				stdout: stdout, stdoutIdentity: stdoutIdentity, stdinClosed: true,
				stdinOffset: 5, stdinPending: []byte("pending"), stdoutOffset: 123, stderrOffset: 45,
				exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{})},
		},
	}
	if err := s.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SchemaVersion != 2 || saved.Generation != s.sandbox.Generation || len(saved.Processes) != 1 || saved.Processes[0].PID != 7 || !saved.Processes[0].StdinClosed || saved.Processes[0].StdinCloseAcked || !saved.Processes[0].ExitEventQueued || !saved.Processes[0].DeleteEventQueued || saved.Processes[0].StdinOffset != 5 || string(saved.Processes[0].StdinPending) != "pending" || saved.Processes[0].StdinIdentity != stdinIdentity || saved.Processes[0].StdoutOffset != 123 || saved.Processes[0].StdoutIdentity != stdoutIdentity {
		t.Fatalf("persisted recovery = %+v", saved)
	}
	temporary, err := filepath.Glob(filepath.Join(bundle, ".multikernel", ".sandbox.json.*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary recovery files = %v, error = %v", temporary, err)
	}
}

func TestAtomicStatePublicationIsDescriptorAnchoredAndPrivate(t *testing.T) {
	t.Run("normal replacement", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "state.json")
		if err := atomicWriteFile(path, []byte("one"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(path, []byte("two"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		if err != nil || statErr != nil || string(data) != "two" || info.Mode().Perm() != 0600 {
			t.Fatalf("published state = %q mode=%v read=%v stat=%v", data, info.Mode(), err, statErr)
		}
		temporary, err := filepath.Glob(filepath.Join(directory, ".state.json.*"))
		if err != nil || len(temporary) != 0 {
			t.Fatalf("temporary files = %v, %v", temporary, err)
		}
	})

	t.Run("symlinked parent", func(t *testing.T) {
		directory := t.TempDir()
		realParent := filepath.Join(directory, "real")
		if err := os.Mkdir(realParent, 0700); err != nil {
			t.Fatal(err)
		}
		linkedParent := filepath.Join(directory, "linked")
		if err := os.Symlink(realParent, linkedParent); err != nil {
			t.Fatal(err)
		}
		if err := atomicWriteFile(filepath.Join(linkedParent, "state.json"), []byte("hostile"), 0600); err == nil {
			t.Fatal("atomic publication followed a symlinked parent")
		}
		if _, err := os.Stat(filepath.Join(realParent, "state.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("symlink target was modified: %v", err)
		}
	})

	t.Run("writable parent", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0777); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(directory, 0700)
		if err := atomicWriteFile(filepath.Join(directory, "state.json"), []byte("hostile"), 0600); err == nil {
			t.Fatal("atomic publication accepted a group/world-writable parent")
		}
	})
}

func TestRecoverExistingReconstructsExactSandboxProcessAndNetworkGeneration(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	sandboxGeneration := strings.Repeat("a", 32)
	networkGeneration := strings.Repeat("b", 32)
	sandbox := protocol.Sandbox{ID: sandboxID(namespace, task), Generation: sandboxGeneration, State: "RUNNING",
		Config: protocol.SandboxConfig{AgentPort: 7200}}
	recovery := validPersistedRecovery(namespace, task)
	recovery.Network = validShimEndpoint(task, sandbox.ID, sandbox.Generation, networkGeneration, "/run/netns/task-a")
	recovery.Processes[0].Status = tasktypes.Status_RUNNING
	recovery.Processes[0].PID = 41
	stdout := filepath.Join(bundle, "stdout")
	if err := os.WriteFile(stdout, nil, 0600); err != nil {
		t.Fatal(err)
	}
	stdoutIdentity, err := inspectBoundProcessIOPath(stdout, false)
	if err != nil {
		t.Fatal(err)
	}
	recovery.Processes[0].Stdout = stdout
	recovery.Processes[0].StdoutIdentity = stdoutIdentity
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "token"), []byte(strings.Repeat("c", 64)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptorPath := filepath.Join(bundle, "network-descriptor")
	if err = os.WriteFile(descriptorPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	network := &fakeNetworkClient{endpoint: recovery.Network, descriptorPath: descriptorPath}
	fakeAgent := &recoveryAgentClient{finish: make(chan struct{})}
	var daemonCalls []string
	var relayCaptured, relayRemoved bool
	service := &service{id: task, namespace: namespace, bundle: bundle, processes: map[string]*process{}, netClient: network,
		publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		relayPath: func(uint32, string) string { return filepath.Join(bundle, "agent-relay.sock") },
		daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
			daemonCalls = append(daemonCalls, request.Method)
			encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
			if err := json.Unmarshal(encoded, output); err != nil {
				t.Fatal(err)
			}
			return nil
		}),
		agentDial: func(_ context.Context, _ string, id, generation string, port uint32, token []byte) (agentClient, error) {
			if !relayCaptured {
				t.Fatal("agent dial preceded relay socket identity capture")
			}
			if id != sandbox.ID || generation != sandbox.Generation || port != 7200 || len(token) != 32 {
				t.Fatalf("agent identity = id:%q generation:%q port:%d token:%d", id, generation, port, len(token))
			}
			return fakeAgent, nil
		},
		newRelay: func(uint32, string) *exec.Cmd {
			command := exec.Command("/bin/sleep", "300")
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return command
		},
		newRelayOwner: func(string) (relayPathOwner, error) {
			relayCaptured = true
			return relayOwnerFunc(func() error { relayRemoved = true; return nil }), nil
		},
	}
	if err = service.recoverExisting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.sandbox.ID != sandbox.ID || service.sandbox.Generation != sandbox.Generation || service.netEndpoint.Generation != networkGeneration {
		t.Fatalf("recovered ownership = sandbox:%+v network:%+v", service.sandbox, service.netEndpoint)
	}
	if service.relaySocket != filepath.Join(bundle, "agent-relay.sock") || service.relay == nil {
		t.Fatalf("recovered relay ownership = socket:%q command:%v", service.relaySocket, service.relay)
	}
	process := service.processes[""]
	if process == nil || process.pid != 41 || process.status != tasktypes.Status_RUNNING || process.exitEventQueued || process.stdoutIdentity != stdoutIdentity {
		t.Fatalf("recovered process = %+v", process)
	}
	select {
	case <-process.done:
		t.Fatal("recovered running process completed before the guest exit")
	default:
	}
	close(fakeAgent.finish)
	select {
	case <-process.done:
	case <-time.After(time.Second):
		t.Fatal("recovered process did not observe the guest exit")
	}
	if process.status != tasktypes.Status_STOPPED || process.exit != 17 || !process.exitEventQueued {
		t.Fatalf("recovered exit = status:%v exit:%d event:%v", process.status, process.exit, process.exitEventQueued)
	}
	if err = service.stopNetwork(); err != nil {
		t.Fatal(err)
	}
	if err = service.stopRelay(); err != nil {
		t.Fatal(err)
	}
	if !relayRemoved {
		t.Fatal("captured relay socket identity was not released")
	}
	if err = service.agent.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(daemonCalls, []string{"ListSandboxes"}) || len(network.calls) < 2 || network.calls[0] != "ATTACH" {
		t.Fatalf("recovery calls = daemon:%v network:%v agent:%v", daemonCalls, network.calls, fakeAgent.snapshotCalls())
	}
}

func TestRecoverExistingClosesAcquiredNetworkDescriptorOnRelayStartFailure(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	sandbox := protocol.Sandbox{ID: sandboxID(namespace, task), Generation: strings.Repeat("a", 32), State: "RUNNING",
		Config: protocol.SandboxConfig{AgentPort: 7200}}
	recovery := validPersistedRecovery(namespace, task)
	recovery.Network = validShimEndpoint(task, sandbox.ID, sandbox.Generation, strings.Repeat("b", 32), "/run/netns/task-a")
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "token"), []byte(strings.Repeat("c", 64)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	descriptorPath := filepath.Join(bundle, "network-descriptor")
	if err = os.WriteFile(descriptorPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	network := &fakeNetworkClient{endpoint: recovery.Network, descriptorPath: descriptorPath}
	service := &service{id: task, namespace: namespace, bundle: bundle, processes: map[string]*process{}, netClient: network,
		relayPath: func(uint32, string) string { return filepath.Join(bundle, "agent-relay.sock") },
		daemon: daemonCallFunc(func(_ context.Context, _ protocol.Request, output any) *protocol.Error {
			encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
			if err := json.Unmarshal(encoded, output); err != nil {
				t.Fatal(err)
			}
			return nil
		}),
		newRelay: func(uint32, string) *exec.Cmd { return exec.Command(filepath.Join(bundle, "missing-relay")) },
	}
	if err = service.recoverExisting(context.Background()); err == nil || !strings.Contains(err.Error(), "restart recovered agent relay") {
		t.Fatalf("relay start failure = %v", err)
	}
	if service.netDevice != nil || service.relay != nil || service.relaySocket != "" {
		t.Fatalf("failed recovery retained ownership: device=%v relay=%v socket=%q", service.netDevice, service.relay, service.relaySocket)
	}
	if network.descriptor == nil {
		t.Fatal("network descriptor was not acquired before injected failure")
	}
	if _, statErr := network.descriptor.Stat(); !errors.Is(statErr, os.ErrClosed) {
		t.Fatalf("acquired network descriptor remains open: %v", statErr)
	}
}

func TestBundleNetworkNamespaceRequiresCanonicalOCIPath(t *testing.T) {
	config := func(path string) []byte {
		return []byte(fmt.Sprintf(`{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":%q}]}}`, path))
	}
	for name, test := range map[string]struct {
		path string
		err  bool
	}{
		"runtime creates namespace": {path: ""},
		"named namespace":           {path: "/run/netns/pod-one"},
		"relative rejected":         {path: "run/netns/pod-one", err: true},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), config(test.path), 0600); err != nil {
				t.Fatal(err)
			}
			actual, err := bundleNetworkNamespace(bundle)
			if (err != nil) != test.err || (!test.err && actual != test.path) {
				t.Fatalf("namespace=%q error=%v", actual, err)
			}
		})
	}
	t.Run("hardlink rejected", func(t *testing.T) {
		bundle := t.TempDir()
		path := filepath.Join(bundle, "config.json")
		if err := os.WriteFile(path, config(""), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(bundle, "config.link")); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(bundle); err == nil {
			t.Fatal("hard-linked OCI config accepted")
		}
	})
	t.Run("symlinked ancestor rejected", func(t *testing.T) {
		directory := t.TempDir()
		realBundle := filepath.Join(directory, "real")
		if err := os.Mkdir(realBundle, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realBundle, "config.json"), config(""), 0600); err != nil {
			t.Fatal(err)
		}
		linkedBundle := filepath.Join(directory, "linked")
		if err := os.Symlink(realBundle, linkedBundle); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(linkedBundle); err == nil {
			t.Fatal("OCI config beneath symlinked bundle accepted")
		}
	})
	t.Run("oversized valid prefix rejected", func(t *testing.T) {
		bundle := t.TempDir()
		data := append(config(""), bytes.Repeat([]byte(" "), (1<<20)+1)...)
		if err := os.WriteFile(filepath.Join(bundle, "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(bundle); err == nil {
			t.Fatal("oversized OCI config accepted")
		}
	})
}

func networkPumpSocket(t *testing.T) (*os.File, int) {
	t.Helper()
	descriptors, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	device := os.NewFile(uintptr(descriptors[0]), "network-pump-device")
	t.Cleanup(func() {
		_ = device.Close()
		_ = unix.Close(descriptors[1])
	})
	return device, descriptors[1]
}

func waitNetworkCondition(t *testing.T, condition func() bool, description string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func stopNetworkPumpForTest(s *service) {
	close(s.netDone)
	s.netWG.Wait()
	s.netDone = nil
}

func TestNetworkPumpMTUBoundsCountersAndReconnect(t *testing.T) {
	const mtu = 576
	t.Run("exact MTU round trip and oversized ingress", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		packets := make(chan []byte, 2)
		client := &networkPumpAgent{packets: packets, handler: func(_ int, packet []byte) ([]byte, error) {
			return packet, nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{1}, mtu+1)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return s.netRXDrops.Load() == 1 }, "oversized ingress drop")
		payload := bytes.Repeat([]byte{2}, mtu)
		if _, err := unix.Write(peer, payload); err != nil {
			t.Fatal(err)
		}
		select {
		case observed := <-packets:
			if !bytes.Equal(observed, payload) {
				t.Fatal("exact-MTU ingress changed")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("exact-MTU ingress was not forwarded")
		}
		buffer := make([]byte, mtu+1)
		waitNetworkCondition(t, func() bool {
			n, err := unix.Read(peer, buffer)
			return err == nil && n == len(payload) && bytes.Equal(buffer[:n], payload)
		}, "exact-MTU egress")
		if s.netRXPackets.Load() != 1 || s.netTXPackets.Load() != 1 || s.netRXDrops.Load() != 1 || s.netTXDrops.Load() != 0 || s.netErrors.Load() != 0 {
			t.Fatalf("network counters = rx:%d tx:%d rxdrop:%d txdrop:%d errors:%d", s.netRXPackets.Load(), s.netTXPackets.Load(), s.netRXDrops.Load(), s.netTXDrops.Load(), s.netErrors.Load())
		}
	})

	t.Run("oversized guest egress", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{handler: func(_ int, packet []byte) ([]byte, error) {
			if len(packet) == 0 {
				return nil, nil
			}
			return bytes.Repeat([]byte{3}, mtu+1), nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{2}, mtu)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return s.netTXDrops.Load() == 1 }, "oversized guest egress drop")
		if s.netRXPackets.Load() != 1 || s.netTXPackets.Load() != 0 {
			t.Fatalf("oversized egress counters = rx:%d tx:%d", s.netRXPackets.Load(), s.netTXPackets.Load())
		}
	})

	t.Run("disconnect drops packet and retries reconnect", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		packets := make(chan []byte, 2)
		client := &networkPumpAgent{packets: packets, reconnectFailures: 2}
		client.handler = func(_ int, packet []byte) ([]byte, error) {
			if len(packet) != 0 && client.reconnectCount() == 0 {
				return nil, errors.New("injected exchange disconnect")
			}
			return nil, nil
		}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{4}, mtu)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return client.reconnectCount() >= 3 }, "bounded reconnect retries")
		if _, err := unix.Write(peer, bytes.Repeat([]byte{5}, mtu)); err != nil {
			t.Fatal(err)
		}
		waitNetworkCondition(t, func() bool { return s.netRXPackets.Load() == 1 }, "post-reconnect packet")
		if s.netRXDrops.Load() != 1 || s.netErrors.Load() != 1 {
			t.Fatalf("disconnect counters = drops:%d errors:%d", s.netRXDrops.Load(), s.netErrors.Load())
		}
	})
}

func TestNetworkReporterRetriesAndCoalescesLatestState(t *testing.T) {
	client := &retryNetworkReportClient{failures: 2, attempts: make(chan mknetwork.Endpoint, 4), success: make(chan mknetwork.Endpoint, 2)}
	endpoint := validShimEndpoint("task", "box", strings.Repeat("b", 32), strings.Repeat("a", 32), "/run/netns/task")
	s := &service{netDone: make(chan struct{}), netReports: make(chan string, 1), netClient: client,
		netEndpoint: endpoint}
	s.netRXPackets.Store(3)
	s.startNetworkReporter()
	s.queueNetworkReport("DISCONNECTED")
	first := <-client.attempts
	if first.State != "DISCONNECTED" || first.RXPackets != 3 {
		t.Fatalf("first network report = %+v", first)
	}
	// Replace the failed state while the reporter is in its bounded retry wait.
	s.netRXPackets.Store(7)
	s.netErrors.Store(1)
	s.queueNetworkReport("READY")
	select {
	case reported := <-client.success:
		if reported.State != "READY" || reported.RXPackets != 7 || reported.Errors != 1 {
			t.Fatalf("successful coalesced report = %+v", reported)
		}
	case <-time.After(time.Second):
		t.Fatal("network reporter did not retry to success")
	}
	second, third := <-client.attempts, <-client.attempts
	if second.State != "READY" || third.State != "READY" {
		t.Fatalf("retried network states = %q, %q", second.State, third.State)
	}
	s.queueNetworkReport("DEGRADED")
	select {
	case reported := <-client.success:
		if reported.State != "DEGRADED" {
			t.Fatalf("later network report state = %q", reported.State)
		}
	case <-time.After(time.Second):
		t.Fatal("network reporter did not accept a later state")
	}
	close(s.netDone)
	s.netWG.Wait()
}

func TestShimRejectsInvalidNetworkEndpointsBeforeUse(t *testing.T) {
	sandboxGeneration := strings.Repeat("b", 32)
	generation := strings.Repeat("a", 32)
	sandbox := protocol.Sandbox{ID: "box", Generation: sandboxGeneration}
	valid := validShimEndpoint("task", sandbox.ID, sandbox.Generation, generation, "/run/netns/task")

	t.Run("provision identity", func(t *testing.T) {
		invalid := valid
		invalid.ContainerID = "other"
		client := &fakeNetworkClient{endpoint: invalid}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if err := s.provisionNetwork(context.Background(), valid.NetNS); err == nil {
			t.Fatal("PROVISION accepted a mismatched endpoint")
		}
		if !reflect.DeepEqual(s.netEndpoint, mknetwork.Endpoint{}) {
			t.Fatalf("invalid PROVISION endpoint was retained: %+v", s.netEndpoint)
		}
	})

	t.Run("provision managed ownership", func(t *testing.T) {
		client := &fakeNetworkClient{endpoint: valid}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if err := s.provisionNetwork(context.Background(), ""); err == nil {
			t.Fatal("managed PROVISION accepted a CNI-owned endpoint")
		}
		if !reflect.DeepEqual(s.netEndpoint, mknetwork.Endpoint{}) {
			t.Fatalf("wrong-ownership PROVISION endpoint was retained: %+v", s.netEndpoint)
		}
	})

	t.Run("attach generation and descriptor", func(t *testing.T) {
		directory := t.TempDir()
		descriptorPath := filepath.Join(directory, "tun")
		if err := os.WriteFile(descriptorPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		invalid := valid
		invalid.Generation = strings.Repeat("c", 32)
		client := &fakeNetworkClient{endpoint: invalid, descriptorPath: descriptorPath}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if _, descriptor, err := s.attachNetwork(context.Background(), valid.NetNS, generation); err == nil || descriptor != nil {
			t.Fatalf("ATTACH accepted mismatched generation: descriptor=%v error=%v", descriptor, err)
		}
		if client.descriptor == nil {
			t.Fatal("ATTACH fixture did not transfer a descriptor")
		}
		if _, err := client.descriptor.Stat(); err == nil {
			t.Fatal("invalid ATTACH descriptor remained open")
		}
	})

	t.Run("report and release generation", func(t *testing.T) {
		invalid := valid
		invalid.Generation = "short"
		client := &fakeNetworkClient{endpoint: valid}
		s := &service{netEndpoint: invalid, netClient: client}
		if err := s.reportNetwork("READY"); err == nil {
			t.Fatal("REPORT accepted a malformed generation")
		}
		if err := s.releaseNetwork(context.Background()); err == nil {
			t.Fatal("RELEASE accepted a malformed generation")
		}
		if len(client.calls) != 0 {
			t.Fatalf("malformed endpoint reached mknetd: %v", client.calls)
		}
	})
}

func TestStopNetworkBoundsGuestCloseAndContinuesLocalCleanup(t *testing.T) {
	device, peer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	client := &blockedNetworkCloseAgent{calls: make(chan struct{}, 1)}
	s := &service{agent: client, netDevice: device, networkCloseTimeout: 10 * time.Millisecond}
	started := time.Now()
	err = s.stopNetwork()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stopNetwork error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded network close took %s", elapsed)
	}
	select {
	case <-client.calls:
	default:
		t.Fatal("guest CloseNetwork was not attempted")
	}
	if s.netDevice != nil {
		t.Fatal("timed-out guest close retained the TUN owner")
	}
	if _, err := device.Stat(); err == nil {
		t.Fatal("timed-out guest close left the TUN descriptor open")
	}
}

func TestSupervisorRestartsSignaledWorker(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "worker-signaled")
	listener, err := os.CreateTemp(directory, "listener")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_TEST_MARKER="+marker)
	if status := superviseShimWorkerWith(listener, os.Args[0], []string{"-test.run=^$"}, directory, environment); status != 0 {
		t.Fatalf("supervisor status = %d", status)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "first-worker-signaled\n" {
		t.Fatalf("restart marker = %q, error = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker PID file remains after clean exit: %v", err)
	}
}

func TestRelayOwnershipKillsDescendantsAndRetainsSocketCleanup(t *testing.T) {
	t.Run("command has private process group", func(t *testing.T) {
		command := newRelayCommand("/bin/true", 7001, "/tmp/mk-relay-test.sock")
		if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
			t.Fatalf("relay process attributes = %+v", command.SysProcAttr)
		}
	})

	t.Run("stop kills group and removes socket", func(t *testing.T) {
		command := exec.Command("/bin/sh", "-c", "sleep 60 & wait")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		processGroup := command.Process.Pid
		socket := filepath.Join(t.TempDir(), "relay.sock")
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox forbids Unix pathname listeners")
		}
		if err != nil {
			t.Fatal(err)
		}
		listener.SetUnlinkOnClose(false)
		if err = os.Chmod(socket, 0755); err != nil {
			t.Fatal(err)
		}
		if err = listener.Close(); err != nil {
			t.Fatal(err)
		}
		s := &service{relay: command, relaySocket: socket}
		if err := s.stopRelay(); err != nil {
			t.Fatal(err)
		}
		if s.relay != nil || s.relaySocket != "" {
			t.Fatalf("relay ownership remains: command=%v socket=%q", s.relay, s.relaySocket)
		}
		if _, err := os.Stat(socket); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("relay socket remains: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			err := syscall.Kill(-processGroup, syscall.Signal(0))
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("relay process group %d remains: %v", processGroup, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("socket cleanup remains retryable", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "relay.sock")
		if err := os.Mkdir(socket, 0700); err != nil {
			t.Fatal(err)
		}
		occupied := filepath.Join(socket, "occupied")
		if err := os.WriteFile(occupied, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		s := &service{relaySocket: socket}
		if err := s.stopRelay(); err == nil || s.relaySocket != socket {
			t.Fatalf("failed socket cleanup: error=%v retained=%q", err, s.relaySocket)
		}
		if err := os.Remove(occupied); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(socket); err != nil {
			t.Fatal(err)
		}
		if err := s.stopRelay(); err != nil || s.relaySocket != "" {
			t.Fatalf("socket cleanup retry: error=%v retained=%q", err, s.relaySocket)
		}
	})
}

func TestStaleRelayCleanupRejectsNonSocketPathsWithoutRemoval(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sock")
	if err := removeStaleRelaySocket(missing); err != nil {
		t.Fatalf("missing stale socket error = %v", err)
	}
	for name, create := range map[string]func(string) error{
		"regular":   func(path string) error { return os.WriteFile(path, []byte("owned data"), 0600) },
		"directory": func(path string) error { return os.Mkdir(path, 0700) },
		"symlink":   func(path string) error { return os.Symlink("missing-target", path) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "relay.sock")
			if err := create(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = removeStaleRelaySocket(path); err == nil {
				t.Fatal("non-socket stale relay path was accepted")
			}
			after, statErr := os.Lstat(path)
			if statErr != nil || before.Mode() != after.Mode() || !os.SameFile(before, after) {
				t.Fatalf("rejected path changed: before=%+v after=%+v error=%v", before, after, statErr)
			}
		})
	}
}

func validPersistedRecovery(namespace, task string) persisted {
	return persisted{
		SchemaVersion: 2,
		ID:            sandboxID(namespace, task),
		Generation:    strings.Repeat("a", 32),
		TaskIdentity:  storageTaskIdentity(namespace, task),
		Processes:     []persistedProcess{{ID: "", Status: tasktypes.Status_STOPPED}},
	}
}

func TestRecoveryStateIsBoundedStrictAndIdentityBound(t *testing.T) {
	namespace, task := "default", "task-a"
	valid := validPersistedRecovery(namespace, task)
	encode := func(t *testing.T, value persisted) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, found, err := loadPersistedRecovery(path, namespace, task)
		if err != nil || !found || loaded.ID != valid.ID || loaded.Generation != valid.Generation {
			t.Fatalf("loaded recovery = %+v, found=%v error=%v", loaded, found, err)
		}
	})

	for name, mutate := range map[string]func(*persisted){
		"legacy schema":    func(value *persisted) { value.SchemaVersion = 1 },
		"wrong sandbox":    func(value *persisted) { value.ID = sandboxID(namespace, "other") },
		"wrong generation": func(value *persisted) { value.Generation = "short" },
		"wrong task owner": func(value *persisted) { value.TaskIdentity = storageTaskIdentity(namespace, "other") },
		"partial network":  func(value *persisted) { value.Network.SandboxID = value.ID },
		"duplicate process": func(value *persisted) {
			value.Processes = append(value.Processes, value.Processes[0])
		},
		"invalid state": func(value *persisted) { value.Processes[0].Status = tasktypes.Status_UNKNOWN },
		"stdio without ownership": func(value *persisted) {
			value.Processes[0].Stdout = "/run/containerd/fifo"
		},
		"stdin offset without stream": func(value *persisted) {
			value.Processes[0].StdinOffset = 1
		},
		"oversized pending stdin": func(value *persisted) {
			value.Processes[0].StdinPending = make([]byte, (32<<10)+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			value.Processes = append([]persistedProcess(nil), valid.Processes...)
			mutate(&value)
			path := filepath.Join(t.TempDir(), "sandbox.json")
			if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
				t.Fatalf("unsafe recovery accepted: %+v", value)
			}
		})
	}

	t.Run("unknown field", func(t *testing.T) {
		data := encode(t, valid)
		data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("recovery with unknown field was accepted")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		if err := os.WriteFile(target, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "sandbox.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("symlinked recovery was accepted")
		}
	})
}

func TestFallbackCleanupPropagatesStopFailureBeforeDelete(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	data, err := json.Marshal(validPersistedRecovery(namespace, task))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "StopSandbox" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected stop failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected stop failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(calls, []string{"StopSandbox"}) {
		t.Fatalf("cleanup calls after failed stop = %v", calls)
	}
}

func TestFallbackCleanupPropagatesRootfsFailureAfterSandboxDelete(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	recovery := validPersistedRecovery(namespace, task)
	recovery.StorageSHA256 = strings.Repeat("b", 64)
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "CleanupRootfs" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected rootfs cleanup failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected rootfs cleanup failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	want := []string{"StopSandbox", "DeleteSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("cleanup call order = %v, want %v", calls, want)
	}
}
