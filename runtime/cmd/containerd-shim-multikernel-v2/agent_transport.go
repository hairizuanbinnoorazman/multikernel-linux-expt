//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"golang.org/x/sys/unix"
)

func (s *service) dialAgent(ctx context.Context, path string) (agentClient, error) {
	if s.agentDial != nil {
		return s.agentDial(ctx, path, s.sandbox.ID, s.sandbox.Generation, s.sandbox.Config.AgentPort, s.token)
	}
	return agent.DialContext(ctx, path, s.sandbox.ID, s.sandbox.Generation, s.sandbox.Config.AgentPort, s.token)
}

func (s *service) negotiateAgentCapabilities(ctx context.Context) error {
	var capabilities guestCapabilities
	if err := s.callAgentWithReconnectContext(ctx, "Capabilities", map[string]any{}, &capabilities); err != nil {
		return fmt.Errorf("query guest capabilities: %w", err)
	}
	if capabilities.Protocol != 1 || capabilities.Kernel == nil || capabilities.Agent == nil {
		return errors.New("guest capabilities omit the required protocol or runtime identity")
	}
	if err := requireCapabilitySet(capabilities.ProtocolFeatures, requiredAgentProtocolFeatures, "protocol"); err != nil {
		return err
	}
	if err := requireCapabilitySet(capabilities.OCIFeatures, requiredAgentOCIFeatures, "OCI"); err != nil {
		return err
	}
	return nil
}

func (s *service) relayCommand(port uint32, socket string) *exec.Cmd {
	if s.newRelay != nil {
		return s.newRelay(port, socket)
	}
	return newRelayCommand(getenv("MK_RELAY", "/opt/mkruntime/bin/mkvsock-relay"), s.sandbox.Config.ChildCID, port, socket)
}

func (s *service) agentRelaySocketPath(port uint32, generation string) string {
	if s.relayPath != nil {
		return s.relayPath(port, generation)
	}
	return relaySocketPath(port, generation)
}

func (s *service) captureRelaySocket(path string) (relayPathOwner, error) {
	if s.newRelayOwner != nil {
		return s.newRelayOwner(path)
	}
	return unixsocket.Capture(path)
}

func (s *service) acquireRelaySocketOwner(path string) error {
	owner, err := s.captureRelaySocket(path)
	if err != nil {
		return err
	}
	if owner == nil {
		return errors.New("relay socket capture returned no owner")
	}
	s.relayOwner = owner
	return nil
}

func taskGuestPID(pid int) (uint32, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return 0, errors.New("guest returned a process ID outside the Task v2 range")
	}
	return uint32(pid), nil
}

func runningGuestProcess(state agent.ProcessState, processID string) (uint32, error) {
	if state.ID != processID || state.Status != "RUNNING" {
		return 0, errors.New("guest returned a running process with mismatched identity or state")
	}
	return taskGuestPID(state.PID)
}

func startedGuestProcess(state agent.ProcessState, processID string) (uint32, error) {
	if state.Status == "RUNNING" {
		return runningGuestProcess(state, processID)
	}
	if state.Status == "STOPPED" {
		pid, _, err := stoppedGuestProcess(state, processID, 0)
		return pid, err
	}
	return 0, errors.New("guest returned a started process with mismatched identity or state")
}

func createdGuestProcess(state agent.ProcessState, processID string) error {
	if state.ID != processID || state.Status != "CREATED" || state.PID != 0 || state.ExitCode != 0 {
		return errors.New("guest returned a created process with mismatched identity or state")
	}
	return nil
}

func stoppedGuestProcess(state agent.ProcessState, processID string, expectedPID uint32) (uint32, uint32, error) {
	if state.ID != processID || state.Status != "STOPPED" {
		return 0, 0, errors.New("guest returned a stopped process with mismatched identity or state")
	}
	pid, err := taskGuestPID(state.PID)
	if err != nil {
		return 0, 0, err
	}
	if expectedPID != 0 && pid != expectedPID {
		return 0, 0, errors.New("guest returned a stopped process with a different PID")
	}
	if state.ExitCode < 0 || state.ExitCode > 255 {
		return 0, 0, errors.New("guest returned an exit status outside the agent contract")
	}
	return pid, uint32(state.ExitCode), nil
}

func relaySocketPath(port uint32, generation string) string {
	if len(generation) > 12 {
		generation = generation[:12]
	}
	return filepath.Join("/run", "mk-agent-"+strconv.Itoa(int(port))+"-"+generation+".sock")
}

func newRelayCommand(binary string, cid, port uint32, socket string) *exec.Cmd {
	// All lifecycle paths use the declared child peer, never a wildcard
	// fallback. Older helpers must refuse this mode rather than bridge an
	// arbitrary child into the authenticated agent stream.
	command := exec.Command(binary, "server-peer", strconv.FormatUint(uint64(cid), 10), strconv.FormatUint(uint64(port), 10), socket)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return command
}

func removeStaleRelaySocket(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		// A later publisher is not stale and must remain untouched.
		return nil
	} else if err != nil {
		return err
	}
	owner, err := unixsocket.Capture(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return owner.Remove()
}

func terminateRelay(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	if command.ProcessState != nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	waitErr := command.Wait()
	var exitErr *exec.ExitError
	if waitErr == nil || errors.As(waitErr, &exitErr) || errors.Is(waitErr, syscall.ECHILD) {
		return nil
	}
	return waitErr
}

func relayProcessLive(command *exec.Cmd) (bool, error) {
	if command == nil || command.Process == nil || command.ProcessState != nil {
		return false, nil
	}
	fd, err := unix.Open(filepath.Join("/proc", strconv.Itoa(command.Process.Pid)), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	directory := os.NewFile(uintptr(fd), "relay-process")
	defer directory.Close()
	stat, err := readProcEntry(directory, "stat", 4096)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	close := bytes.LastIndex(stat, []byte(") "))
	if close < 0 {
		return false, errors.New("relay process stat is malformed")
	}
	fields := strings.Fields(string(stat[close+2:]))
	if len(fields) == 0 {
		return false, errors.New("relay process stat omits state")
	}
	return fields[0] != "Z" && fields[0] != "X", nil
}

func (s *service) relaySnapshot() (string, uint64) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	return s.relaySocket, s.relayGeneration
}

func (s *service) replaceExitedRelay(ctx context.Context, observedGeneration uint64) (bool, error) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if observedGeneration != s.relayGeneration {
		return true, nil
	}
	if s.relay == nil {
		return false, nil
	}
	live, err := relayProcessLive(s.relay)
	if err != nil {
		return false, fmt.Errorf("inspect agent relay process: %w", err)
	}
	if live {
		return false, nil
	}
	if s.relaySocket == "" {
		return false, errors.New("agent relay socket is unavailable")
	}
	if err = terminateRelay(s.relay); err != nil {
		return false, fmt.Errorf("reap exited agent relay: %w", err)
	}
	s.relay = nil
	if s.relayOwner != nil {
		err = s.relayOwner.Remove()
	} else {
		err = removeStaleRelaySocket(s.relaySocket)
	}
	if err != nil {
		return false, fmt.Errorf("remove exited agent relay socket: %w", err)
	}
	s.relayOwner = nil
	command := s.relayCommand(s.sandbox.Config.AgentPort, s.relaySocket)
	if err = command.Start(); err != nil {
		return false, fmt.Errorf("restart exited agent relay: %w", err)
	}
	s.relay = command
	deadline := time.Now().Add(5 * time.Second)
	for {
		owner, captureErr := s.captureRelaySocket(s.relaySocket)
		if captureErr == nil {
			if owner == nil {
				captureErr = errors.New("relay socket capture returned no owner")
			} else {
				s.relayOwner = owner
				s.relayGeneration++
				return true, nil
			}
		}
		if !errors.Is(captureErr, os.ErrNotExist) || time.Now().After(deadline) || ctx.Err() != nil {
			cleanupErr := terminateRelay(command)
			s.relay = nil
			return false, errors.Join(fmt.Errorf("capture restarted agent relay socket: %w", captureErr), ctx.Err(), cleanupErr)
		}
		if err = waitContext(ctx, 50*time.Millisecond); err != nil {
			cleanupErr := terminateRelay(command)
			s.relay = nil
			return false, errors.Join(err, cleanupErr)
		}
	}
}

func daemonErrorCode(err error, code string) bool {
	observed, ok := daemon.ErrorCode(err)
	return ok && observed == code
}

func (s *service) stopRelay() error {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relay != nil {
		if err := terminateRelay(s.relay); err != nil {
			return err
		}
		s.relay = nil
	}
	if s.relaySocket != "" {
		var err error
		if s.relayOwner != nil {
			err = s.relayOwner.Remove()
		} else {
			err = removeStaleRelaySocket(s.relaySocket)
		}
		if err != nil {
			return err
		}
		s.relayOwner = nil
		s.relaySocket = ""
	}
	return nil
}

func (s *service) connectAgent(ctx context.Context) error {
	// Docker uses 64-byte container IDs and deeply nested runtime bundles;
	// placing the relay socket in the bundle can exceed sockaddr_un.sun_path.
	// Use a generation-qualified short path under /run instead.
	sock := s.agentRelaySocketPath(s.sandbox.Config.AgentPort, s.sandbox.Generation)
	s.relaySocket = sock
	if err := removeStaleRelaySocket(sock); err != nil {
		s.relaySocket = ""
		return fmt.Errorf("remove stale agent relay socket: %w", err)
	}
	if s.netEndpoint.Generation == "" {
		return errors.New("CNI endpoint is not bound")
	}
	endpoint, device, err := s.attachNetwork(ctx, s.netEndpoint.NetNS, s.netEndpoint.Generation)
	if err != nil {
		return err
	}
	s.netEndpoint, s.netDevice = endpoint, device
	if err := s.persistRecovery(); err != nil {
		return errors.Join(fmt.Errorf("persist network recovery state: %w", err), s.stopNetwork(), s.stopRelay())
	}
	s.relay = s.relayCommand(s.sandbox.Config.AgentPort, sock)
	if err := s.relay.Start(); err != nil {
		return errors.Join(err, s.stopNetwork(), s.stopRelay())
	}
	if _, err := daemon.Mutation(ctx, s.daemon, "StartSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-start-"+s.sandbox.Generation, nil); err != nil {
		return errors.Join(err, s.stopNetwork(), s.stopRelay())
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if s.relayOwner == nil {
			err = s.acquireRelaySocketOwner(sock)
			if errors.Is(err, os.ErrNotExist) {
				if err = waitContext(ctx, 100*time.Millisecond); err != nil {
					return errors.Join(err, s.stopNetwork(), s.stopRelay())
				}
				continue
			}
			if err != nil {
				return errors.Join(fmt.Errorf("capture agent relay socket: %w", err), s.stopNetwork(), s.stopRelay())
			}
		}
		client, err := s.dialAgent(ctx, sock)
		if err == nil {
			s.agent = client
			if err = s.negotiateAgentCapabilities(ctx); err != nil {
				_ = s.agent.Close()
				s.agent = nil
				return errors.Join(err, s.stopNetwork(), s.stopRelay())
			}
			if err = s.configureGuestNetwork(ctx, agent.NetworkConfig{
				Name: "mkn0", Address: s.netEndpoint.Address, Gateway: s.netEndpoint.Gateway,
				MTU: s.netEndpoint.MTU, Nameservers: s.netEndpoint.DNS.Nameservers,
			}); err != nil {
				_ = s.agent.Close()
				s.agent = nil
				return errors.Join(err, s.stopNetwork(), s.stopRelay())
			}
			s.startNetworkPump()
			return nil
		}
		if err := waitContext(ctx, 100*time.Millisecond); err != nil {
			return errors.Join(err, s.stopNetwork(), s.stopRelay())
		}
	}
	return errors.Join(errors.New("timed out connecting to child agent"), s.stopNetwork(), s.stopRelay())
}

// probeAgentTransport uses an authenticated, read-only RPC through the common
// reconnect path. The network pump must not dial independently: another agent
// caller may already own the sole connection accepted by a replacement relay.
func (s *service) probeAgentTransport(ctx context.Context) error {
	var capabilities map[string]any
	return s.callAgentWithReconnectContext(ctx, "Capabilities", map[string]any{}, &capabilities)
}

func (s *service) callAgentWithReconnect(method string, request, output any) error {
	return s.callAgentWithReconnectContext(context.Background(), method, request, output)
}

func (s *service) callAgentWithReconnectContext(parent context.Context, method string, request, output any) error {
	timeout := s.ioCallTimeout
	if timeout <= 0 {
		timeout = defaultAgentIOCallTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var lastErr error
	for {
		if err := s.agent.CallContext(ctx, method, request, output); err == nil {
			return nil
		} else {
			lastErr = err
		}
		var remoteError *agent.RemoteError
		relaySocket, relayGeneration := s.relaySnapshot()
		if errors.As(lastErr, &remoteError) || relaySocket == "" || ctx.Err() != nil {
			return lastErr
		}
		if err := s.agent.ReconnectContext(ctx, relaySocket); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("reconnect guest agent transport: %w", err))
			if _, replaceErr := s.replaceExitedRelay(ctx, relayGeneration); replaceErr != nil {
				lastErr = errors.Join(lastErr, replaceErr)
			}
		}
		if err := waitContext(ctx, 50*time.Millisecond); err != nil {
			return errors.Join(lastErr, err)
		}
	}
}

func (s *service) quiesceAndShutdownGuest(ctx context.Context) error {
	var quiesced guestQuiesceResponse
	if err := s.callAgentWithReconnectContext(ctx, "Quiesce", map[string]any{}, &quiesced); err != nil {
		return fmt.Errorf("quiesce guest agent: %w", err)
	}
	if quiesced.Status != "quiesced" {
		return fmt.Errorf("quiesce guest agent: unexpected status %q", quiesced.Status)
	}
	var shutdown guestQuiesceResponse
	err := s.agent.CallContext(ctx, "Shutdown", map[string]any{}, &shutdown)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		var remote *agent.RemoteError
		if errors.As(err, &remote) {
			return fmt.Errorf("shutdown guest agent: %w", err)
		}
		// Quiesce is the authenticated commit point. After it succeeds, a
		// transport loss during terminal Shutdown is safe to complete locally:
		// the guest is sealed against new mutations and storage is already quiet.
		return nil
	}
	if shutdown.Status != "quiesced" {
		return fmt.Errorf("shutdown guest agent: unexpected status %q", shutdown.Status)
	}
	return nil
}

func (s *service) deleteGuestProcess(ctx context.Context, id string) error {
	err := s.callAgentWithReconnectContext(ctx, "DeleteProcess", map[string]string{"ID": id}, nil)
	if err != nil && !agentNotFound(err) {
		return fmt.Errorf("delete guest process: %w", err)
	}
	return nil
}

func (s *service) resizeGuestProcess(ctx context.Context, id string, width, height uint32) error {
	return s.callAgentWithReconnectContext(ctx, "ResizeProcess", map[string]any{
		"id": id, "width": width, "height": height,
	}, nil)
}

func (s *service) signalGuestProcess(ctx context.Context, id string, signal uint32, operationID string) error {
	return s.callAgentWithReconnectContext(ctx, "SignalProcess", map[string]any{
		"ID": id, "Signal": strconv.FormatUint(uint64(signal), 10), "operation_id": operationID,
	}, nil)
}

func (s *service) acknowledgeGuestSignal(ctx context.Context, id, operationID string) error {
	return s.callAgentWithReconnectContext(ctx, "AcknowledgeSignal", map[string]any{
		"ID": id, "operation_id": operationID,
	}, nil)
}

func (s *service) applyGuestSignal(ctx context.Context, id string, signal uint32) error {
	operationID, err := newSignalOperationID()
	if err != nil {
		return fmt.Errorf("create process signal identity: %w", err)
	}
	if err = s.signalGuestProcess(ctx, id, signal, operationID); err != nil {
		var remoteError *agent.RemoteError
		if !errors.As(err, &remoteError) {
			return err
		}
		ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return errors.Join(err, s.acknowledgeGuestSignal(ackCtx, id, operationID))
	}
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.acknowledgeGuestSignal(ackCtx, id, operationID)
}
