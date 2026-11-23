//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type recoveredOutputGate struct {
	agentClient
	started chan struct{}
	release chan struct{}
	once    sync.Once
	badWait bool
}

func (c *recoveredOutputGate) Call(method string, request, response any) error {
	return c.CallContext(context.Background(), method, request, response)
}

func (c *recoveredOutputGate) CallContext(ctx context.Context, method string, request, response any) error {
	if method == "ReadProcessOutput" {
		c.once.Do(func() { close(c.started) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	err := c.agentClient.CallContext(ctx, method, request, response)
	if err == nil && method == "WaitProcess" && c.badWait {
		response.(*agent.ProcessState).PID++
	}
	return err
}

func TestRecoveredOutputProducer(t *testing.T) {
	if os.Getenv("MK_RECOVERED_OUTPUT_PRODUCER") != "1" {
		return
	}
	if n, err := os.Stdout.Write(bytes.Repeat([]byte("r"), 8192)); err != nil || n != 8192 {
		os.Exit(96)
	}
	if n, err := os.Stderr.Write([]byte("accounted-stderr\n")); err != nil || n != 17 {
		os.Exit(95)
	}
	os.Exit(37)
}

// The test wrapper pauses inside Write after production has committed its
// intent. It adds no production fault switch. The parent kills this process,
// then calls the actual recoverExisting path against the still-running agent.
func TestRecoveredOutputCrashWorker(t *testing.T) {
	bundle := os.Getenv("MK_RECOVERED_OUTPUT_BUNDLE")
	if bundle == "" {
		return
	}
	s := &service{id: "task", namespace: "tests", bundle: bundle, processes: map[string]*process{}, publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	defer s.closeBundleIdentity()
	directory, err := s.ensureRuntimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	saved, found, identity, err := loadPersistedRecoveryFromDirectory(directory, "tests", "task")
	if err != nil || !found {
		t.Fatal("worker recovery input unavailable", err)
	}
	s.recoveryIdentity = identity
	s.sandbox = protocol.Sandbox{ID: saved.ID, Generation: saved.Generation}
	s.netEndpoint = saved.Network
	value := saved.Processes[0]
	p := &process{pid: value.PID, status: value.Status, stdin: value.Stdin, stdinIdentity: value.StdinIdentity, stdout: value.Stdout, stdoutIdentity: value.StdoutIdentity, stderr: value.Stderr, stderrIdentity: value.StderrIdentity, done: make(chan struct{})}
	s.processes[""] = p
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client, err := agent.DialPeerContext(ctx, os.Getenv("MK_RECOVERED_OUTPUT_SOCKET"), saved.ID, saved.Generation, 7001, bytes.Repeat([]byte("t"), 32), os.Getppid())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	s.agent = client
	if err = s.negotiateAgentCapabilities(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.openProcessIO(ctx, p); err != nil {
		t.Fatal(err)
	}
	defer closeProcessIO(p)
	writer := p.stdoutWriter
	p.stdoutWriter = dispositionWriter{write: func(data []byte) (int, error) {
		switch os.Getenv("MK_RECOVERED_OUTPUT_CRASH") {
		case "before-write":
		case "partial-write":
			if n, err := writer.Write(data[:17]); err != nil || n != 17 {
				t.Fatal("partial fixture write failed")
			}
		case "after-write":
			if n, err := writer.Write(data); err != nil || n != len(data) {
				t.Fatal("complete fixture write failed")
			}
		default:
			t.Fatal("unknown crash fixture")
		}
		ready := os.NewFile(3, "crash-boundary")
		if _, err := ready.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		<-ctx.Done()
		t.Fatal("parent did not kill worker at selected boundary")
		return 0, ctx.Err()
	}}
	s.waitProcess("init", "", p)
	t.Fatal("crash worker unexpectedly completed")
}

func TestRecoveredOutputNativeStoppedGuestAfterWorkerKill(t *testing.T) {
	for _, mode := range []string{"before-write", "partial-write", "after-write", "missing-stdin", "replaced-output", "mismatched-wait"} {
		t.Run(mode, func(t *testing.T) { testRecoveredStoppedOutput(t, mode) })
	}
}

func testRecoveredStoppedOutput(t *testing.T, mode string) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	guest := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(guest, "rootfs"), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(guest, "rootfs/probe"), binary, 0700); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"ociVersion": "1.1.0", "root": map[string]any{"path": "rootfs"}, "process": map[string]any{"args": []string{"/probe", "-test.run=^TestRecoveredOutputProducer$"}, "cwd": "/", "env": []string{"MK_RECOVERED_OUTPUT_PRODUCER=1"}, "user": map[string]any{"uid": os.Geteuid(), "gid": os.Getegid()}}}
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(guest, "config.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	manager := agent.NewManager(true)
	if err = manager.Create("init", guest); err != nil {
		t.Fatal(err)
	}
	defer manager.Delete("init")
	if err = manager.Start("init"); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Wait("init")
	if err != nil || state.ExitCode != 37 {
		t.Fatal("producer exit differs", err)
	}
	socketDir, err := os.MkdirTemp("/tmp", "mk-rec-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socket := filepath.Join(socketDir, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	bundle := privateTestDirectory(t)
	if err = os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	bundleIdentity := testBundleIdentity(t, bundle)
	sandbox := protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: strings.Repeat("a", 32), State: "RUNNING", Config: protocol.SandboxConfig{AgentPort: 7001, BundleIdentity: bundleIdentity}}
	server := &agent.Server{Manager: manager, SandboxID: sandbox.ID, Generation: sandbox.Generation, Endpoint: 7001, Token: bytes.Repeat([]byte("t"), 32)}
	go server.Serve(ctx, listener)
	p := &process{pid: uint32(state.PID), status: tasktypes.Status_RUNNING, stdout: filepath.Join(bundle, "stdout"), stderr: filepath.Join(bundle, "stderr"), done: make(chan struct{})}
	if mode == "missing-stdin" {
		p.stdin = filepath.Join(bundle, "stdin")
		if err = syscall.Mkfifo(p.stdin, 0600); err != nil {
			t.Fatal(err)
		}
		p.stdinIdentity, err = inspectBoundProcessIOPath(p.stdin, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, stream := range []struct {
		path     string
		identity *processIOIdentity
	}{{p.stdout, &p.stdoutIdentity}, {p.stderr, &p.stderrIdentity}} {
		if err = os.WriteFile(stream.path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		*stream.identity, err = inspectBoundProcessIOPath(stream.path, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	initial := &service{id: "task", namespace: "tests", bundle: bundle, bundleIdentity: bundleIdentity, sandbox: sandbox, netEndpoint: validShimEndpoint("task", sandbox.ID, sandbox.Generation, strings.Repeat("b", 32), "/run/netns/task"), processes: map[string]*process{"": p}}
	if err = initial.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	initial.closeBundleIdentity()
	if err = os.WriteFile(filepath.Join(bundle, ".multikernel/token"), []byte(strings.Repeat("74", 32)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ready, signal, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ready.Close()
	defer signal.Close()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestRecoveredOutputCrashWorker$")
	crashMode := mode
	if mode == "missing-stdin" || mode == "replaced-output" || mode == "mismatched-wait" {
		crashMode = "before-write"
	}
	command.Env = append(os.Environ(), "MK_RECOVERED_OUTPUT_BUNDLE="+bundle, "MK_RECOVERED_OUTPUT_SOCKET="+socket, "MK_RECOVERED_OUTPUT_CRASH="+crashMode)
	command.ExtraFiles = []*os.File{signal}
	var childLog bytes.Buffer
	command.Stdout, command.Stderr = &childLog, &childLog
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	signal.Close()
	_ = ready.SetReadDeadline(time.Now().Add(8 * time.Second))
	var marker [1]byte
	if _, err = io.ReadFull(ready, marker[:]); err != nil || marker[0] != 1 {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal("crash boundary not reached", err, childLog.String())
	}
	if err = command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = command.Wait()
	if err == nil || !command.ProcessState.Sys().(syscall.WaitStatus).Signaled() || command.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatal("worker was not killed")
	}
	saved, found, err := loadPersistedRecovery(filepath.Join(bundle, ".multikernel/sandbox.json"), "tests", "task")
	if err != nil || !found || saved.Processes[0].StdoutOffset != 0 || saved.Processes[0].OutputDisposition == nil || saved.Processes[0].OutputDisposition.Stdout.InFlightBytes != 4096 || saved.Processes[0].ExitEventQueued {
		t.Fatal("kill did not preserve unacknowledged intent", err)
	}
	beforeRecovery, err := os.ReadFile(filepath.Join(bundle, ".multikernel/sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	if mode == "missing-stdin" {
		if saved.Processes[0].StdinIdentity != p.stdinIdentity || saved.Processes[0].Stdin != p.stdin {
			t.Fatal("stdin fixture was not persisted")
		}
		if err = os.Remove(p.stdin); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "replaced-output" {
		if err = os.Rename(p.stdout, p.stdout+".retained"); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(p.stdout, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	descriptor := filepath.Join(bundle, "network-descriptor")
	if err = os.WriteFile(descriptor, nil, 0600); err != nil {
		t.Fatal(err)
	}
	gate := &recoveredOutputGate{started: make(chan struct{}), release: make(chan struct{}), badWait: mode == "mismatched-wait"}
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	s := &service{id: "task", namespace: "tests", bundle: bundle, processes: map[string]*process{}, publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		netClient: &fakeNetworkClient{endpoint: saved.Network, descriptorPath: descriptor},
		daemon: daemonCallFunc(func(_ context.Context, _ protocol.Request, output any) *protocol.Error {
			if err := setJSONResponse(output, []protocol.Sandbox{sandbox}); err != nil {
				t.Fatal(err)
			}
			return nil
		}),
		relayPath: func(uint32, string) string { return filepath.Join(bundle, "relay.sock") },
		newRelay: func(uint32, string) *exec.Cmd {
			c := exec.Command("/bin/sleep", "300")
			c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return c
		},
		newRelayOwner:      func(string) (relayPathOwner, error) { return relayOwnerFunc(func() error { return nil }), nil },
		newNamespaceHolder: func(string) (namespaceHolder, error) { return &fakeNamespaceHolder{pid: 73}, nil },
		agentDial: func(ctx context.Context, _ string, id, generation string, port uint32, token []byte) (agentClient, error) {
			client, err := agent.DialPeerContext(ctx, socket, id, generation, port, token, os.Getpid())
			if err != nil {
				return nil, err
			}
			gate.agentClient = client
			return gate, nil
		},
	}
	defer s.closeBundleIdentity()
	defer func() {
		release.Do(func() { close(gate.release) })
		if s.netDone != nil {
			stopNetworkPumpForTest(s)
		}
		if s.netDevice != nil {
			s.netDevice.Close()
		}
		if s.agent != nil {
			s.agent.Close()
		}
		_ = s.stopNamespaceHolder()
		_ = s.stopRelay()
	}()
	err = s.recoverExisting(ctx)
	if mode == "replaced-output" || mode == "mismatched-wait" {
		if err == nil {
			t.Fatal("invalid output/wait identity accepted")
		}
		select {
		case <-gate.started:
			t.Fatal("refused recovery started an output monitor")
		default:
		}
		if s.netDone != nil {
			t.Fatal("refused recovery started network workers")
		}
		afterRecovery, readErr := os.ReadFile(filepath.Join(bundle, ".multikernel/sandbox.json"))
		if readErr != nil || !bytes.Equal(beforeRecovery, afterRecovery) {
			t.Fatal("refusal changed durable process state", readErr)
		}
		if s.processes[""].exitEventQueued || s.processes[""].status == tasktypes.Status_STOPPED {
			t.Fatal("refusal published exit")
		}
		t.Logf("mode=%s refused=true monitor_started=false durable_state_unchanged=true exit_published=false", mode)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	recovered := s.processes[""]
	select {
	case <-gate.started:
	case <-ctx.Done():
		t.Fatal("recovery skipped the output monitor")
	}
	s.mu.Lock()
	prematureExit := recovered.status == tasktypes.Status_STOPPED || recovered.exitEventQueued || recovered.finalOutputAccounting != nil
	s.mu.Unlock()
	select {
	case <-recovered.done:
		t.Fatal("completion preceded output drain")
	default:
	}
	if prematureExit {
		t.Fatal("exit/accounting published before draining")
	}
	release.Do(func() { close(gate.release) })
	select {
	case <-recovered.done:
	case <-ctx.Done():
		t.Fatal("recovered monitor did not finish")
	}
	stdout, err := os.ReadFile(p.stdout)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.ReadFile(p.stderr)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := 0
	if mode == "partial-write" {
		duplicate = 17
	}
	if mode == "after-write" {
		duplicate = 4096
	}
	if !bytes.Equal(stdout, bytes.Repeat([]byte("r"), 8192+duplicate)) || string(stderr) != "accounted-stderr\n" {
		t.Fatalf("recovery lost output: stdout=%d expected=%d stderr=%d", len(stdout), 8192+duplicate, len(stderr))
	}
	saved, found, err = loadPersistedRecovery(filepath.Join(bundle, ".multikernel/sandbox.json"), "tests", "task")
	if err != nil || !found {
		t.Fatal(err)
	}
	final := saved.Processes[0]
	if final.Status != tasktypes.Status_STOPPED || final.Exit != 37 || !final.ExitEventQueued || final.StdoutOffset != 8192 || final.StderrOffset != 17 || final.FinalOutputAccounting == nil || final.FinalOutputAccounting.State != "validated" {
		t.Fatal("drained exit/accounting missing")
	}
	r := final.OutputDisposition
	if r == nil || r.Stdout.WrittenBytes != 8192 || r.Stderr.WrittenBytes != 17 || r.Stdout.ReplayExposureBytes != 4096 || r.Stdout.InFlightBytes != 0 || r.Stdout.UnknownPrefixBytes != 0 || r.Stdout.ExposureExhausted {
		t.Fatal("crash uncertainty disappeared")
	}
	if final.FinalOutputAccounting.Report.Stdout.DroppedBytes != 0 || final.FinalOutputAccounting.Report.Stdout.ObservedBytes != 8192 || final.FinalOutputAccounting.Report.Stderr.ObservedBytes != 17 {
		t.Fatal("guest accounting differs")
	}
	if final.FinalOutputAccounting.RecoveryDrain == nil || *final.FinalOutputAccounting.RecoveryDrain != (recoveredOutputDrain{StdoutInFlightBytes: 4096, StderrInFlightBytes: 17}) || r.Stderr.ReplayExposureBytes != 17 {
		t.Fatal("stopped-guest recovery provenance differs from the pre-kill record")
	}
	t.Logf("mode=%s guest_stdout=8192 guest_stderr=17 physical_stdout=%d replay_exposure=4096 unresolved_intent=0 exit=37 worker_sigkill=true", mode, len(stdout))
	t.Log("recovered_stopped_drain=true initial_stdout_offset=0 initial_stderr_offset=0 initial_stdout_intent=4096 initial_stderr_intent=17 stderr_replay_exposure=17")
}

func TestRecoveredOutputDrainStrictAndSnapshotSafe(t *testing.T) {
	for _, drain := range []recoveredOutputDrain{{StdoutOffset: 8}, {StderrOffset: 1}, {StdoutInFlightBytes: 4097}, {StderrInFlightBytes: 4097}, {StdoutOffset: ^uint64(0), StdoutInFlightBytes: 1}} {
		if drain.validate(7, 0) == nil {
			t.Fatal("forged recovery provenance accepted")
		}
	}
	if (recoveredOutputDrain{StdoutOffset: ^uint64(0), StdoutInFlightBytes: 1}).validate(^uint64(0), 0) == nil {
		t.Fatal("recovery provenance overflow accepted")
	}
	p := completedAccountingProcess("probe")
	p.finalOutputAccounting.RecoveryDrain = &recoveredOutputDrain{StdoutOffset: 3, StdoutInFlightBytes: 4}
	tail := (*completedOutputAccounting)(nil).append("probe", p)
	// Subsequent mutable process state cannot rewrite retained provenance.
	p.finalOutputAccounting.RecoveryDrain.StdoutOffset = 999
	if tail.Records[0].FinalOutputAccounting.RecoveryDrain.StdoutOffset != 3 {
		t.Fatal("retained recovery provenance aliases the removed process")
	}
	if err := tail.validate(); err != nil {
		t.Fatal(err)
	}
}
