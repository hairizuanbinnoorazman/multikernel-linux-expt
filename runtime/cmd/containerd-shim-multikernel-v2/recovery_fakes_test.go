//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

type recoveryAgentClient struct {
	finish          chan struct{}
	mu              sync.Mutex
	calls           []string
	failFirstState  bool
	reconnects      int
	recoveredSignal bool
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

func validGuestCapabilities() guestCapabilities {
	return guestCapabilities{Protocol: 1,
		ProtocolFeatures: append([]string(nil), requiredAgentProtocolFeatures...),
		OCIFeatures:      append([]string(nil), requiredAgentOCIFeatures...),
		Kernel:           map[string]any{"architecture": "amd64"},
		Agent:            map[string]any{"uid": 0.0},
	}
}

func (f *recoveryAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *recoveryAgentClient) CallContext(ctx context.Context, method string, request, response any) error {
	f.record(method)
	switch method {
	case "Capabilities":
		return setJSONResponse(response, validGuestCapabilities())
	case "StateProcess":
		f.mu.Lock()
		fail := f.failFirstState
		f.failFirstState = false
		f.mu.Unlock()
		if fail {
			return io.ErrUnexpectedEOF
		}
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
	case "SignalProcess":
		values := request.(map[string]any)
		if values["ID"] != "init" || values["Signal"] != strconv.Itoa(int(syscall.SIGUSR1)) ||
			values["operation_id"] != strings.Repeat("d", 32) {
			return errors.New("unexpected recovered signal identity")
		}
		f.mu.Lock()
		f.recoveredSignal = true
		f.mu.Unlock()
		return nil
	case "AcknowledgeSignal":
		values := request.(map[string]any)
		if values["ID"] != "init" || values["operation_id"] != strings.Repeat("d", 32) {
			return errors.New("unexpected recovered signal acknowledgement")
		}
		return nil
	default:
		return nil
	}
}

func (f *recoveryAgentClient) Close() error { return nil }

func (f *recoveryAgentClient) Reconnect(string) error { return nil }

func (f *recoveryAgentClient) ReconnectContext(context.Context, string) error {
	f.mu.Lock()
	f.reconnects++
	f.mu.Unlock()
	return nil
}
