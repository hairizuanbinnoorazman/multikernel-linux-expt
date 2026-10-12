//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

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

func (f *stdinCaptureAgent) Close() error { return nil }

func (f *stdinCaptureAgent) Reconnect(string) error { return nil }

func (f *stdinCaptureAgent) ReconnectContext(context.Context, string) error { return nil }

type controlledInput struct{ reads chan chan []byte }

func (f *controlledInput) Read(target []byte) (int, error) {
	value := make(chan []byte, 1)
	f.reads <- value
	return copy(target, <-value), nil
}

func (*controlledInput) Write(value []byte) (int, error) { return len(value), nil }

func (*controlledInput) Close() error { return nil }

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

func (*waitValidationAgent) Close() error { return nil }

func (*waitValidationAgent) Reconnect(string) error { return nil }

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

func (*waitOutageAgent) Close() error { return nil }

func (*waitOutageAgent) Reconnect(string) error { return nil }

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

func (*outputAckAgent) Close() error { return nil }

func (*outputAckAgent) Reconnect(string) error { return nil }

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

func (f *outputReconnectAgent) Close() error { return nil }

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

func (f *stdinReplayAgent) Close() error { return nil }

func (f *stdinReplayAgent) Reconnect(string) error { return nil }

func (f *stdinReplayAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
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

func (f *closeRetryAgent) Close() error { return nil }

func (f *closeRetryAgent) Reconnect(string) error { return nil }

func (f *closeRetryAgent) ReconnectContext(context.Context, string) error { return nil }
