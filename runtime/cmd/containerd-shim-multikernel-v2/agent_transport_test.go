//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestAgentCapabilityNegotiationRequiresCompleteUniqueContract(t *testing.T) {
	valid := validGuestCapabilities()
	tests := map[string]struct {
		capabilities guestCapabilities
		wantError    bool
	}{
		"complete": {capabilities: valid},
		"wrong protocol": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.Protocol = 2
			return value
		}(), wantError: true},
		"missing replay feature": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.ProtocolFeatures = value.ProtocolFeatures[1:]
			return value
		}(), wantError: true},
		"missing OCI feature": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.OCIFeatures = value.OCIFeatures[1:]
			return value
		}(), wantError: true},
		"duplicate": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.OCIFeatures = append(value.OCIFeatures, value.OCIFeatures[0])
			return value
		}(), wantError: true},
		"malformed feature": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.ProtocolFeatures[0] = "INVALID FEATURE"
			return value
		}(), wantError: true},
		"missing identity": {capabilities: func() guestCapabilities {
			value := validGuestCapabilities()
			value.Kernel = nil
			return value
		}(), wantError: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := &fakeAgentClient{fail: map[string]error{}, capabilities: &test.capabilities}
			service := &service{agent: client, ioCallTimeout: time.Second}
			err := service.negotiateAgentCapabilities(context.Background())
			if (err != nil) != test.wantError {
				t.Fatalf("negotiateAgentCapabilities() error = %v, wantError=%v", err, test.wantError)
			}
			if !reflect.DeepEqual(client.calls, []string{"Capabilities"}) {
				t.Fatalf("agent calls = %v", client.calls)
			}
		})
	}
}

func TestAgentCapabilityNegotiationReconnectsLostReply(t *testing.T) {
	client := &capabilityReplyLossAgent{}
	service := &service{agent: client, relaySocket: "/run/test-agent.sock", ioCallTimeout: time.Second}
	if err := service.negotiateAgentCapabilities(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || client.reconnects != 1 {
		t.Fatalf("capability calls/reconnects = %d/%d, want 2/1", client.calls, client.reconnects)
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

func TestResizePtyRetriesLostReply(t *testing.T) {
	fake := &shutdownBoundaryAgent{loseFirstResize: true}
	p := &process{terminal: true, status: tasktypes.Status_RUNNING, width: 80, height: 24, sizeSet: true}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second,
		processes: map[string]*process{"": p}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[ResizeProcess ResizeProcess]" || fake.reconnects != 1 {
		t.Fatalf("resize calls=%v reconnects=%d", fake.calls, fake.reconnects)
	}
	if !p.sizeSet || p.width != 91 || p.height != 37 {
		t.Fatalf("reconciled resize state: %+v", p)
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
