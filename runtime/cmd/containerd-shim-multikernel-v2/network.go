//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

func (s *service) networkReport(state string) mknetwork.Endpoint {
	endpoint := s.netEndpoint
	endpoint.State = state
	endpoint.RXPackets = s.netRXPackets.Load()
	endpoint.TXPackets = s.netTXPackets.Load()
	endpoint.RXDrops = s.netRXDrops.Load()
	endpoint.TXDrops = s.netTXDrops.Load()
	endpoint.Errors = s.netErrors.Load()
	return endpoint
}

func (s *service) reportNetwork(state string) error {
	endpoint := s.networkReport(state)
	if endpoint.Generation == "" {
		return nil
	}
	if err := mknetwork.ValidateEndpoint(endpoint); err != nil {
		return fmt.Errorf("validate network report endpoint: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-report-" + endpoint.Generation[:12], Method: "REPORT", Endpoint: &endpoint}
	_, err := s.netClient.Call(ctx, request)
	return err
}

func (s *service) configureGuestNetwork(ctx context.Context, config agent.NetworkConfig) error {
	if err := s.callAgentWithReconnectContext(ctx, "ConfigureNetwork", config, nil); err != nil {
		return fmt.Errorf("configure child network: %w", err)
	}
	return nil
}

func (s *service) startNetworkPump() {
	s.netDone = make(chan struct{})
	s.netReports = make(chan string, 1)
	s.startNetworkReporter()
	s.queueNetworkReport("READY")
	s.netWG.Add(1)
	go func() {
		defer s.netWG.Done()
		buffer := make([]byte, s.netEndpoint.MTU+1)
		lastReported := s.netRXPackets.Load() + s.netTXPackets.Load()
		for {
			select {
			case <-s.netDone:
				return
			default:
			}
			var packet []byte
			n, err := unix.Read(int(s.netDevice.Fd()), buffer)
			if err == nil && n > 0 {
				if n > s.netEndpoint.MTU {
					s.netRXDrops.Add(1)
					continue
				}
				packet = append([]byte(nil), buffer[:n]...)
			} else if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				s.netErrors.Add(1)
				s.queueNetworkReport("DEGRADED")
				fmt.Fprintf(os.Stderr, "multikernel network: host TUN read: %v\n", err)
				return
			}
			var response struct {
				Packet []byte `json:"packet"`
			}
			exchangeCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			err = s.agent.CallContext(exchangeCtx, "ExchangeNetwork", map[string][]byte{"packet": packet}, &response)
			cancel()
			if err != nil {
				s.netErrors.Add(1)
				if len(packet) > 0 {
					s.netRXDrops.Add(1)
				}
				s.queueNetworkReport("DISCONNECTED")
				for {
					select {
					case <-s.netDone:
						return
					case <-time.After(100 * time.Millisecond):
					}
					reconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					reconnectErr := s.probeAgentTransport(reconnectCtx)
					cancel()
					if reconnectErr == nil {
						s.queueNetworkReport("READY")
						break
					}
				}
				continue
			}
			if len(packet) > 0 {
				s.netRXPackets.Add(1)
			}
			if len(response.Packet) > 0 {
				if len(response.Packet) > s.netEndpoint.MTU {
					s.netTXDrops.Add(1)
					continue
				}
				if _, err = unix.Write(int(s.netDevice.Fd()), response.Packet); err == nil {
					s.netTXPackets.Add(1)
				} else if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
					s.netTXDrops.Add(1)
				} else {
					s.netErrors.Add(1)
					s.queueNetworkReport("DEGRADED")
					fmt.Fprintf(os.Stderr, "multikernel network: host TUN write: %v\n", err)
					return
				}
			}
			total := s.netRXPackets.Load() + s.netTXPackets.Load()
			if total-lastReported >= 256 {
				s.queueNetworkReport("READY")
				lastReported = total
			}
			if len(packet) == 0 && len(response.Packet) == 0 {
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
}

func (s *service) stopNetwork() error {
	var failures []error
	// Cleanup-only recovery reconstructs the persisted endpoint so it can be
	// released, but it does not own the guest, pump, or TUN counter state.  In
	// that case reporting zero-valued counters would regress mknetd's durable
	// counters and fail closed with STALE_COUNTER.
	reportCounters := s.netDone != nil || s.agent != nil || s.netDevice != nil
	if s.netDone != nil {
		close(s.netDone)
		s.netWG.Wait()
		s.netDone = nil
		s.netReports = nil
	}
	if s.agent != nil {
		timeout := s.networkCloseTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), timeout)
		err := s.callAgentWithReconnectContext(closeCtx, "CloseNetwork", map[string]any{}, nil)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Errorf("close guest network: %w", err))
		}
	}
	if s.netDevice != nil {
		if err := s.netDevice.Close(); err != nil {
			failures = append(failures, fmt.Errorf("close TUN: %w", err))
		}
		s.netDevice = nil
	}
	if reportCounters {
		if err := s.reportNetwork("READY"); err != nil {
			failures = append(failures, fmt.Errorf("persist network counters: %w", err))
		}
	}
	return errors.Join(failures...)
}

func (s *service) queueNetworkReport(state string) {
	if s.netReports == nil {
		return
	}
	select {
	case s.netReports <- state:
		return
	default:
	}
	select {
	case <-s.netReports:
	default:
	}
	select {
	case s.netReports <- state:
	default:
	}
}

func (s *service) startNetworkReporter() {
	s.netWG.Add(1)
	go func() {
		defer s.netWG.Done()
		for {
			var state string
			select {
			case <-s.netDone:
				return
			case state = <-s.netReports:
			}
			for {
				if err := s.reportNetwork(state); err == nil {
					break
				} else {
					fmt.Fprintf(os.Stderr, "multikernel network report %s: %v\n", state, err)
				}
				timer := time.NewTimer(100 * time.Millisecond)
				select {
				case <-s.netDone:
					timer.Stop()
					return
				case state = <-s.netReports:
					timer.Stop()
				case <-timer.C:
				}
				select {
				case state = <-s.netReports:
				default:
				}
			}
		}
	}()
}

func (s *service) provisionNetwork(ctx context.Context, netns string) error {
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-provision-" + s.sandbox.Generation[:16], Method: "PROVISION", Endpoint: &mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}}
	response, err := s.netClient.Call(ctx, request)
	if err != nil {
		return err
	}
	if response.Endpoint == nil {
		return errors.New("mknetd PROVISION returned no endpoint")
	}
	if err = validateShimNetworkEndpoint(*response.Endpoint, mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}); err != nil {
		return fmt.Errorf("validate mknetd PROVISION endpoint: %w", err)
	}
	s.netEndpoint = *response.Endpoint
	return nil
}

func (s *service) ensureNamespaceHolder() error {
	if s.namespaceHolder != nil {
		if err := s.namespaceHolder.Validate(s.netEndpoint.NetNS); err == nil {
			return nil
		}
		if err := s.stopNamespaceHolder(); err != nil {
			return fmt.Errorf("replace invalid network namespace holder: %w", err)
		}
	}
	if s.netEndpoint.NetNS == "" {
		return errors.New("network endpoint has no namespace for its holder")
	}
	factory := s.newNamespaceHolder
	if factory == nil {
		factory = startNamespaceHolder
	}
	holder, err := factory(s.netEndpoint.NetNS)
	if err != nil {
		return fmt.Errorf("start network namespace holder: %w", err)
	}
	if holder == nil {
		return errors.New("network namespace holder factory returned no holder")
	}
	if err = holder.Validate(s.netEndpoint.NetNS); err != nil {
		if holder != nil {
			_ = holder.Stop()
		}
		return fmt.Errorf("validate network namespace holder: %w", err)
	}
	s.namespaceHolder = holder
	return nil
}

func (s *service) stopNamespaceHolder() error {
	if s.namespaceHolder == nil {
		return nil
	}
	holder := s.namespaceHolder
	if err := holder.Stop(); err != nil {
		return err
	}
	s.namespaceHolder = nil
	return nil
}

func validateShimNetworkEndpoint(endpoint, expected mknetwork.Endpoint) error {
	if err := mknetwork.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	if endpoint.ContainerID != expected.ContainerID || endpoint.NetworkName != expected.NetworkName ||
		endpoint.IfName != expected.IfName || endpoint.SandboxID != expected.SandboxID ||
		endpoint.SandboxGeneration != expected.SandboxGeneration ||
		expected.NetNS != "" && endpoint.NetNS != expected.NetNS ||
		expected.Generation != "" && endpoint.Generation != expected.Generation {
		return errors.New("network endpoint differs from the requested ownership identity")
	}
	if expected.NetNS == "" && (endpoint.Owner != "runtime" || !endpoint.ManagedNamespace) {
		return errors.New("network endpoint differs from the requested managed-namespace ownership")
	}
	return nil
}

func (s *service) attachNetwork(ctx context.Context, netns, endpointGeneration string) (mknetwork.Endpoint, *os.File, error) {
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-attach-" + s.sandbox.Generation[:16], Method: "ATTACH", Endpoint: &mknetwork.Endpoint{
		NetNS: netns, Generation: endpointGeneration, SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}}
	response, descriptor, err := s.netClient.Attach(ctx, request)
	if err != nil {
		return mknetwork.Endpoint{}, nil, err
	}
	if response.Endpoint == nil || descriptor == nil {
		if descriptor != nil {
			_ = descriptor.Close()
		}
		return mknetwork.Endpoint{}, nil, errors.New("mknetd ATTACH returned no endpoint descriptor")
	}
	if err = validateShimNetworkEndpoint(*response.Endpoint, mknetwork.Endpoint{
		ContainerID: s.id, NetworkName: "multikernel", IfName: "mktun0", NetNS: netns,
		Generation: endpointGeneration, SandboxID: s.sandbox.ID, SandboxGeneration: s.sandbox.Generation,
	}); err != nil {
		_ = descriptor.Close()
		return mknetwork.Endpoint{}, nil, fmt.Errorf("validate mknetd ATTACH endpoint: %w", err)
	}
	return *response.Endpoint, descriptor, nil
}

func (s *service) releaseNetwork(ctx context.Context) error {
	if s.netEndpoint.SandboxID == "" {
		return nil
	}
	if err := mknetwork.ValidateEndpoint(s.netEndpoint); err != nil {
		return fmt.Errorf("validate network release endpoint: %w", err)
	}
	request := mknetwork.Request{Version: mknetwork.ProtocolVersion, RequestID: "shim-release-" + s.netEndpoint.SandboxGeneration[:16], Method: "RELEASE", Endpoint: &mknetwork.Endpoint{
		SandboxID: s.netEndpoint.SandboxID, SandboxGeneration: s.netEndpoint.SandboxGeneration,
	}}
	_, err := s.netClient.Call(ctx, request)
	var apiErr *mknetwork.APIError
	if err == nil || errors.As(err, &apiErr) && apiErr.Code == "NOT_FOUND" {
		s.netEndpoint = mknetwork.Endpoint{}
		return nil
	}
	return err
}
