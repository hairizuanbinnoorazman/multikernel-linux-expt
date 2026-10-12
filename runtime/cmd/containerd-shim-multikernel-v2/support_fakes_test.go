//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/containerd/containerd/events"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

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

type relayReplacementAgent struct {
	mu         sync.Mutex
	calls      int
	reconnects int
	recovered  bool
}

func (f *relayReplacementAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *relayReplacementAgent) CallContext(_ context.Context, method string, _ any, response any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if method != "StateProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.calls++
	if !f.recovered {
		return io.ErrUnexpectedEOF
	}
	return setJSONResponse(response, agent.ProcessState{ID: "init", PID: 41, Status: "RUNNING"})
}

func (*relayReplacementAgent) Close() error { return nil }

func (*relayReplacementAgent) Reconnect(string) error { return nil }

func (f *relayReplacementAgent) ReconnectContext(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconnects++
	if f.reconnects == 1 {
		return io.ErrUnexpectedEOF
	}
	f.recovered = true
	return nil
}

type inheritedFileSocket string

func (path inheritedFileSocket) File() (*os.File, error) { return os.Open(string(path)) }

type shimSocketFileFunc func() (*os.File, error)

func (f shimSocketFileFunc) File() (*os.File, error) { return f() }

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
