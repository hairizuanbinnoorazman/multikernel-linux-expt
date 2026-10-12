//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type shutdownBoundaryAgent struct {
	calls               []string
	closeCalls          int
	configureCalls      int
	deleteCalls         int
	quiesceCalls        int
	resizeCalls         int
	reconnects          int
	loseFirstClose      bool
	loseFirstConfigure  bool
	loseFirstDelete     bool
	loseFirstQuiesce    bool
	loseFirstResize     bool
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

func (*execCreationAgent) Close() error { return nil }

func (*execCreationAgent) Reconnect(string) error { return nil }

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
	case "ResizeProcess":
		f.resizeCalls++
		if f.loseFirstResize && f.resizeCalls == 1 {
			return io.ErrUnexpectedEOF
		}
		return nil
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

func (*shutdownBoundaryAgent) Close() error { return nil }

func (*shutdownBoundaryAgent) Reconnect(string) error { return nil }

func (f *shutdownBoundaryAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

type startCleanupAgent struct {
	finish            chan struct{}
	startErr          error
	stateErr          error
	transientStateErr error
	stateFailures     int
	signalErr         error
	statePID          int
	stateID           string
	state             string
	reconnects        int
}

type createReconcileAgent struct {
	state        agent.ProcessState
	createdState agent.ProcessState
	stateErr     error
	createErr    error
	created      bool
	disconnected bool
	stateCalls   int
	createCalls  int
	reconnects   int
}

func (f *createReconcileAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *createReconcileAgent) CallContext(_ context.Context, method string, _ any, response any) error {
	switch method {
	case "StateProcess":
		f.stateCalls++
		if f.created {
			if f.disconnected {
				return io.ErrUnexpectedEOF
			}
			return setJSONResponse(response, f.createdState)
		}
		if f.stateErr != nil {
			return f.stateErr
		}
		return setJSONResponse(response, f.state)
	case "CreateProcess":
		f.createCalls++
		if f.createdState.ID != "" {
			f.created = true
			f.disconnected = f.createErr != nil
		}
		return f.createErr
	default:
		return fmt.Errorf("unexpected method %s", method)
	}
}

func (*createReconcileAgent) Close() error { return nil }

func (*createReconcileAgent) Reconnect(string) error { return nil }

func (f *createReconcileAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	f.disconnected = false
	return nil
}

func (f *startCleanupAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *startCleanupAgent) CallContext(ctx context.Context, method string, request, response any) error {
	switch method {
	case "StartProcess":
		return f.startErr
	case "StateProcess":
		if f.stateFailures > 0 {
			f.stateFailures--
			return f.transientStateErr
		}
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

func (*startCleanupAgent) Close() error { return nil }

func (*startCleanupAgent) Reconnect(string) error { return nil }

func (f *startCleanupAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

type daemonCallFunc func(context.Context, protocol.Request, any) *protocol.Error

func (f daemonCallFunc) Call(ctx context.Context, request protocol.Request, response any) *protocol.Error {
	return f(ctx, request, response)
}

type eofReadWriteCloser struct{}

func (eofReadWriteCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (eofReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }

func (eofReadWriteCloser) Close() error { return nil }

func (f *fakeAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *fakeAgentClient) CallContext(_ context.Context, method string, request any, response any) error {
	f.calls = append(f.calls, method)
	if err := f.fail[method]; err != nil {
		return err
	}
	if method == "Capabilities" {
		capabilities := validGuestCapabilities()
		if f.capabilities != nil {
			capabilities = *f.capabilities
		}
		return setJSONResponse(response, capabilities)
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

func (f *fakeAgentClient) Close() error { return nil }

func (f *fakeAgentClient) Reconnect(string) error { return nil }

func (f *fakeAgentClient) ReconnectContext(context.Context, string) error { return nil }
