//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestRecoverExistingReconstructsExactSandboxProcessAndNetworkGeneration(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	sandboxGeneration := strings.Repeat("a", 32)
	networkGeneration := strings.Repeat("b", 32)
	bundleIdentity := testBundleIdentity(t, bundle)
	sandbox := protocol.Sandbox{ID: sandboxID(namespace, task), Generation: sandboxGeneration, State: "RUNNING",
		Config: protocol.SandboxConfig{AgentPort: 7200, BundleIdentity: bundleIdentity}}
	recovery := validPersistedRecovery(namespace, task)
	recovery.BundleIdentity = bundleIdentity
	recovery.Network = validShimEndpoint(task, sandbox.ID, sandbox.Generation, networkGeneration, "/run/netns/task-a")
	recovery.Processes[0].Status = tasktypes.Status_RUNNING
	recovery.Processes[0].PID = 41
	recovery.Processes[0].PendingSignal = uint32(syscall.SIGUSR1)
	recovery.Processes[0].PendingSignalOp = strings.Repeat("d", 32)
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
	fakeAgent := &recoveryAgentClient{finish: make(chan struct{}), failFirstState: true}
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
		newNamespaceHolder: func(string) (namespaceHolder, error) { return &fakeNamespaceHolder{pid: 73}, nil },
	}
	if err = service.recoverExisting(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner := service.eventDiagnosticOwner.Load()
	if owner == nil || owner.sandbox != sandbox.ID || owner.generation != sandbox.Generation {
		t.Fatal("recovery did not install validated diagnostic ownership")
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
	fakeAgent.mu.Lock()
	reconnects := fakeAgent.reconnects
	fakeAgent.mu.Unlock()
	if reconnects != 1 {
		t.Fatalf("recovery reconnects = %d, want 1", reconnects)
	}
	fakeAgent.mu.Lock()
	recoveredSignal := fakeAgent.recoveredSignal
	fakeAgent.mu.Unlock()
	if !recoveredSignal || process.pendingSignalOperation != "" || process.pendingSignal != 0 {
		t.Fatalf("recovered signal replay=%v pending=%q/%d", recoveredSignal, process.pendingSignalOperation, process.pendingSignal)
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
	if err = service.stopNamespaceHolder(); err != nil {
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
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	bundleIdentity := testBundleIdentity(t, bundle)
	sandbox := protocol.Sandbox{ID: sandboxID(namespace, task), Generation: strings.Repeat("a", 32), State: "RUNNING",
		Config: protocol.SandboxConfig{AgentPort: 7200, BundleIdentity: bundleIdentity}}
	recovery := validPersistedRecovery(namespace, task)
	recovery.BundleIdentity = bundleIdentity
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
		newRelay:           func(uint32, string) *exec.Cmd { return exec.Command(filepath.Join(bundle, "missing-relay")) },
		newNamespaceHolder: func(string) (namespaceHolder, error) { return &fakeNamespaceHolder{pid: 73}, nil },
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

func TestRecoverExistingRejectsBundleIdentityBeforeDaemonCall(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	recovery := validPersistedRecovery("default", "task-a")
	recovery.BundleIdentity = testBundleIdentity(t, bundle)
	recovery.BundleIdentity.Inode++
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	daemonCalls := 0
	service := &service{id: "task-a", namespace: "default", bundle: bundle, processes: map[string]*process{},
		daemon: daemonCallFunc(func(context.Context, protocol.Request, any) *protocol.Error {
			daemonCalls++
			return nil
		})}
	err = service.recoverExisting(context.Background())
	if err == nil || !strings.Contains(err.Error(), "bundle identity differs") {
		t.Fatalf("recovery bundle mismatch = %v", err)
	}
	if daemonCalls != 0 {
		t.Fatalf("bundle mismatch reached daemon %d times", daemonCalls)
	}
}

func TestRecoverExistingRejectsDaemonBundleIdentityBeforeReconnect(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	recovery := validPersistedRecovery("default", "task-a")
	recovery.BundleIdentity = testBundleIdentity(t, bundle)
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	daemonIdentity := recovery.BundleIdentity
	daemonIdentity.Inode++
	sandbox := protocol.Sandbox{ID: recovery.ID, Generation: recovery.Generation,
		Config: protocol.SandboxConfig{BundleIdentity: daemonIdentity}}
	daemonCalls := 0
	service := &service{id: "task-a", namespace: "default", bundle: bundle, processes: map[string]*process{},
		daemon: daemonCallFunc(func(_ context.Context, _ protocol.Request, output any) *protocol.Error {
			daemonCalls++
			encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
			if decodeErr := json.Unmarshal(encoded, output); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			return nil
		})}
	err = service.recoverExisting(context.Background())
	if err == nil || !strings.Contains(err.Error(), "daemon sandbox bundle identity differs") {
		t.Fatalf("daemon bundle mismatch = %v", err)
	}
	if daemonCalls != 1 || service.agent != nil || service.netDevice != nil || service.relay != nil {
		t.Fatalf("daemon mismatch calls=%d agent=%v network=%v relay=%v", daemonCalls, service.agent, service.netDevice, service.relay)
	}
}
