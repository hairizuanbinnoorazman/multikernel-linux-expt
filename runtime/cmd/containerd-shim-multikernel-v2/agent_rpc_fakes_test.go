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
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

type fakeAgentClient struct {
	calls        []string
	fail         map[string]error
	stats        agent.ProcessStats
	statsByID    map[string]agent.ProcessStats
	capabilities *guestCapabilities
}

type statsReplyLossAgent struct {
	calls      int
	reconnects int
	stats      agent.ProcessStats
}

type capabilityReplyLossAgent struct {
	calls, reconnects int
}

func (f *capabilityReplyLossAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *capabilityReplyLossAgent) CallContext(_ context.Context, method string, _ any, response any) error {
	if method != "Capabilities" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls++
	if f.calls == 1 {
		return io.ErrUnexpectedEOF
	}
	return setJSONResponse(response, validGuestCapabilities())
}

func (*capabilityReplyLossAgent) Close() error { return nil }

func (*capabilityReplyLossAgent) Reconnect(string) error { return nil }

func (f *capabilityReplyLossAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

type signalReplyLossAgent struct {
	bundle     string
	calls      int
	applied    int
	reconnects int
	ackCalls   int
	operation  string
	durable    bool
}

func (f *signalReplyLossAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *signalReplyLossAgent) CallContext(_ context.Context, method string, request, _ any) error {
	if method == "AcknowledgeSignal" {
		values := request.(map[string]any)
		if values["operation_id"] != f.operation {
			return errors.New("acknowledged signal operation identity changed")
		}
		f.ackCalls++
		return nil
	}
	if method != "SignalProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls++
	values := request.(map[string]any)
	operation := values["operation_id"].(string)
	data, err := os.ReadFile(filepath.Join(f.bundle, ".multikernel", "sandbox.json"))
	if err == nil {
		var saved persisted
		if json.Unmarshal(data, &saved) == nil && len(saved.Processes) == 1 {
			f.durable = saved.Processes[0].PendingSignalOp == operation && saved.Processes[0].PendingSignal == uint32(syscall.SIGUSR1)
		}
	}
	if f.operation == "" {
		f.operation = operation
		f.applied++
		return io.ErrUnexpectedEOF
	}
	if operation != f.operation {
		return errors.New("signal operation identity changed")
	}
	return nil
}

func (*signalReplyLossAgent) Close() error { return nil }

func (*signalReplyLossAgent) Reconnect(string) error { return nil }

func (f *signalReplyLossAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

func (f *statsReplyLossAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *statsReplyLossAgent) CallContext(_ context.Context, method string, _ any, response any) error {
	if method != "StatsProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls++
	if f.calls == 1 {
		return io.ErrUnexpectedEOF
	}
	*(response.(*agent.ProcessStats)) = f.stats
	return nil
}

func (*statsReplyLossAgent) Close() error { return nil }

func (*statsReplyLossAgent) Reconnect(string) error { return nil }

func (f *statsReplyLossAgent) ReconnectContext(context.Context, string) error {
	f.reconnects++
	return nil
}

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
	if method == "AcknowledgeSignal" {
		return nil
	}
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

func (f *signalFailureAgent) Close() error { return nil }

func (f *signalFailureAgent) Reconnect(string) error { return nil }

func (f *signalFailureAgent) ReconnectContext(context.Context, string) error { return nil }
