//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

type networkPumpAgent struct {
	mu                               sync.Mutex
	exchanges                        int
	reconnects                       int
	reconnectFailures                int
	transportBroken                  bool
	preserveTransportOnExchangeError bool
	handler                          func(int, []byte) ([]byte, error)
	packets                          chan []byte
}

func (f *networkPumpAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}

func (f *networkPumpAgent) CallContext(_ context.Context, method string, request, response any) error {
	if method == "Capabilities" {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.transportBroken {
			return errors.New("injected disconnected transport")
		}
		return setJSONResponse(response, map[string]any{})
	}
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
		f.mu.Lock()
		if !f.preserveTransportOnExchangeError {
			f.transportBroken = true
		}
		f.mu.Unlock()
		return err
	}
	return setJSONResponse(response, map[string][]byte{"packet": output})
}

func (f *networkPumpAgent) Close() error { return nil }

func (f *networkPumpAgent) Reconnect(string) error { return nil }

func (f *networkPumpAgent) ReconnectContext(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconnects++
	if f.reconnects <= f.reconnectFailures {
		return errors.New("injected reconnect failure")
	}
	f.transportBroken = false
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
	fail           map[string]error
}

type fakeNamespaceHolder struct {
	pid     uint32
	stopped bool
}

func (h *fakeNamespaceHolder) PID() uint32 { return h.pid }

func (h *fakeNamespaceHolder) Validate(string) error {
	if h.pid == 0 || h.stopped {
		return errors.New("fake holder is not running")
	}
	return nil
}

func (h *fakeNamespaceHolder) Stop() error {
	h.stopped = true
	return nil
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

func (*blockedNetworkCloseAgent) Close() error { return nil }

func (*blockedNetworkCloseAgent) Reconnect(string) error { return nil }

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

func (f *fakeNetworkClient) Call(_ context.Context, request mknetwork.Request) (mknetwork.Response, error) {
	f.calls = append(f.calls, request.Method)
	if err := f.fail[request.Method]; err != nil {
		return mknetwork.Response{}, err
	}
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
