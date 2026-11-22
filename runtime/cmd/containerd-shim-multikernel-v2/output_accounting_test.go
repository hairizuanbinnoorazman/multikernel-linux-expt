//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestFinalOutputAccountingProducer(t *testing.T) {
	if os.Getenv("MK_SHIM_ACCOUNTING_HELPER") != "1" {
		return
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte("p"), protocol.ProcessOutputBufferLimit+17)); err != nil {
		os.Exit(96)
	}
	if _, err := os.Stderr.Write([]byte("bounded-stderr")); err != nil {
		os.Exit(95)
	}
	os.Exit(37)
}

func TestFinalOutputAccountingNativeOwningClientDurableExit(t *testing.T) {
	testNativeFinalAccounting(t, false, false)
}
func TestOutputDispositionNativeFIFOConsumer(t *testing.T) { testNativeFinalAccounting(t, true, false) }
func TestCompletedOutputNativeRealExecDelete(t *testing.T) { testNativeFinalAccounting(t, true, true) }
func testNativeFinalAccounting(t *testing.T, fifo, completed bool) {
	processID, execID := "init", ""
	if completed {
		processID, execID = "native-exec", "native-exec"
	}
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
	config := map[string]any{"ociVersion": "1.1.0", "root": map[string]any{"path": "rootfs"}, "process": map[string]any{"args": []string{"/probe", "-test.run=^TestFinalOutputAccountingProducer$"}, "cwd": "/", "env": []string{"MK_SHIM_ACCOUNTING_HELPER=1"}, "user": map[string]any{"uid": os.Geteuid(), "gid": os.Getegid()}}}
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(guest, "config.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	manager := agent.NewManager(true)
	if err = manager.Create(processID, guest); err != nil {
		t.Fatal(err)
	}
	if err = manager.Start(processID); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Wait(processID)
	if err != nil || state.ExitCode != 37 {
		t.Fatal("managed producer exit differs", err)
	}
	defer manager.Delete(processID)
	// The unit scratch directory can exceed the Unix pathname limit. Use a
	// short private directory, including when TMPDIR is a long lab path.
	socketDir, err := os.MkdirTemp("/tmp", "mk-accounting-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	socket := filepath.Join(socketDir, "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	server := &agent.Server{Manager: manager, SandboxID: "accounting", Generation: strings.Repeat("a", 32), Endpoint: 7001, Token: bytes.Repeat([]byte("t"), 32)}
	go server.Serve(ctx, listener)
	client, err := agent.DialPeerContext(ctx, socket, server.SandboxID, server.Generation, server.Endpoint, server.Token, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	bundle := privateTestDirectory(t)
	if err = os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	p := &process{id: execID, pid: uint32(state.PID), status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	var received chan int64
	if fifo {
		p.stdout = filepath.Join(bundle, "stdout.fifo")
		if err = syscall.Mkfifo(p.stdout, 0600); err != nil {
			t.Fatal(err)
		}
		p.stdoutIdentity, err = inspectBoundProcessIOPath(p.stdout, false)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := openKnownProcessIOPath(ctx, p.stdout, unix.O_RDONLY|unix.O_NONBLOCK, p.stdoutIdentity)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		p.stdoutWriter, p.stdoutGuard, err = openOutput(ctx, p.stdout, p.stdoutIdentity)
		if err != nil {
			t.Fatal(err)
		}
		defer closeProcessIO(p)
		received = make(chan int64, 1)
		go func() {
			n, err := io.Copy(io.Discard, reader)
			if err != nil {
				received <- -1
				return
			}
			received <- n
		}()
	}

	s := &service{id: "task", namespace: "tests", bundle: bundle, agent: client, ioCallTimeout: time.Second, processes: map[string]*process{"": p}, sandbox: protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: server.Generation}, publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	defer s.closeBundleIdentity()
	if completed {
		s.processes = map[string]*process{"": {status: tasktypes.Status_RUNNING}, execID: p}
	}
	if err = s.negotiateAgentCapabilities(ctx); err != nil {
		t.Fatal(err)
	}
	if !s.agentOutputAccounting.Load() {
		t.Fatal("optional accounting was not negotiated")
	}
	go s.waitProcess(processID, execID, p)
	select {
	case <-p.done:
	case <-ctx.Done():
		t.Fatal("owning monitor did not complete")
	}
	saved, found, err := loadPersistedRecovery(filepath.Join(bundle, ".multikernel/sandbox.json"), "tests", "task")
	if err != nil || !found || len(saved.Processes) < 1 {
		t.Fatal("durable stopped report unavailable", err)
	}
	var value persistedProcess
	for _, entry := range saved.Processes {
		if entry.ID == execID {
			value = entry
		}
	}
	if value.Status != tasktypes.Status_STOPPED || value.Exit != 37 || !value.ExitEventQueued || value.FinalOutputAccounting == nil || value.FinalOutputAccounting.State != "validated" {
		t.Fatal("exit and accounting not committed together")
	}
	report := value.FinalOutputAccounting.Report
	if report.Stdout.ObservedBytes != 4194321 || report.Stdout.AcknowledgedBytes != 4194304 || report.Stdout.DroppedBytes != 17 || report.Stdout.BufferedPeakBytes != 4194304 || report.Stderr.AcknowledgedBytes != 14 || value.StdoutOffset != 4194304 || value.StderrOffset != 14 {
		t.Fatal("final conservation differs")
	}
	if value.OutputDisposition == nil || value.OutputDisposition.validate(4194304, 14) != nil {
		t.Fatal("disposition was not committed with exit")
	}
	if fifo {
		select {
		case n := <-received:
			if n != 4194304 {
				t.Fatal("FIFO consumer byte count differs", n)
			}
		case <-ctx.Done():
			t.Fatal("FIFO consumer not closed")
		}
		if value.OutputDisposition.Stdout.WrittenBytes != 4194304 || value.OutputDisposition.Stdout.ReplayExposureBytes != 0 || value.OutputDisposition.Stdout.InFlightBytes != 0 {
			t.Fatal("FIFO disposition differs")
		}
		t.Log("fifo_consumer_bytes=4194304 durable_written=4194304 replay_exposure=0 guest_dropped=17 stderr_no_destination=14")
	} else if value.OutputDisposition.Stdout.NoDestinationBytes != 4194304 || value.OutputDisposition.Stderr.NoDestinationBytes != 14 {
		t.Fatal("unconfigured streams not counted")
	}
	if completed {
		if _, err = s.Delete(ctx, &taskapi.DeleteRequest{ID: "task", ExecID: execID}); err != nil {
			t.Fatal("real guest exec deletion failed", err)
		}
		var inspection completedOutputInspection
		if binary := os.Getenv("MK_COMPLETED_OUTPUT_INSPECTOR"); binary != "" {
			callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			raw, err := exec.CommandContext(callCtx, binary, "--inspect-completed-output", "--bundle", bundle, "--namespace", "tests", "--task", "task").Output()
			cancel()
			if err != nil || len(raw) > completedOutputInspectionLimit || protocol.StrictDecode(raw, &inspection) != nil {
				t.Fatal("compiled inspector failed")
			}
		} else {
			inspection = observeCompletedOutput(bundle, "tests", "task")
		}
		if inspection.Status != "observed" || inspection.Tail == nil || inspection.Tail.DeletedCount != 1 || len(inspection.Tail.Records) != 1 || inspection.Tail.Records[0].ID != execID || inspection.Tail.Records[0].PID != uint32(state.PID) || inspection.Tail.Records[0].Exit != 37 || inspection.Tail.Records[0].OutputDisposition.Stdout.WrittenBytes != 4194304 || inspection.Tail.Records[0].FinalOutputAccounting.Report.Stdout.DroppedBytes != 17 {
			t.Fatal("deleted real exec lost final inspection")
		}
		if _, exists := s.processes[execID]; exists {
			t.Fatal("deleted exec still live in shim")
		}
		if _, err := manager.State(processID); err == nil {
			t.Fatal("deleted exec still managed by guest")
		}
		var caps guestCapabilities
		if err = client.CallContext(ctx, "Capabilities", map[string]any{}, &caps); err != nil {
			t.Fatal("inspection invalidated owning client")
		}
		t.Log("real_guest_exec_deleted=true tail_records=1 sequence=1 exit=37 written=4194304 guest_drop=17 owning_client_preserved=true")
	}
	t.Log("owning_checked_peer=true durable_exit=37 stdout_observed=4194321 acknowledged=4194304 dropped=17 retained_peak=4194304 stderr_acknowledged=14")
}

func TestFinalOutputAccountingRejectsFalseQualification(t *testing.T) {
	valid := func() *finalOutputAccounting {
		return &finalOutputAccounting{State: "validated", Report: &protocol.ProcessOutputAccounting{Version: 1, ID: "init", PID: 41, Status: "STOPPED", Stdout: protocol.OutputBufferAccounting{ObservedBytes: 10, AcknowledgedBytes: 7, DroppedBytes: 3, BufferedPeakBytes: 7, LimitBytes: protocol.ProcessOutputBufferLimit, Truncated: true}, Stderr: protocol.OutputBufferAccounting{LimitBytes: protocol.ProcessOutputBufferLimit}}}
	}
	if err := valid().validate("init", 41, tasktypes.Status_STOPPED, 7, 0); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*finalOutputAccounting){
		"other-id": func(r *finalOutputAccounting) { r.Report.ID = "other" }, "other-pid": func(r *finalOutputAccounting) { r.Report.PID = 42 }, "running": func(r *finalOutputAccounting) { r.Report.Status = "RUNNING" }, "pending": func(r *finalOutputAccounting) { r.Report.Stdout.ObservedBytes++; r.Report.Stdout.PendingBytes++ }, "unacknowledged": func(r *finalOutputAccounting) { r.Report.Stdout.AcknowledgedBytes--; r.Report.Stdout.BufferedBytes++ }, "missing": func(r *finalOutputAccounting) { r.Report = nil }, "unsupported-with-report": func(r *finalOutputAccounting) { r.State = "unsupported" }, "unknown": func(r *finalOutputAccounting) { r.State = "complete" }, "overflow": func(r *finalOutputAccounting) { r.Report.Stdout.AccountingExhausted = true },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid()
			mutate(r)
			if r.validate("init", 41, tasktypes.Status_STOPPED, 7, 0) == nil {
				t.Fatal("false qualification accepted")
			}
		})
	}
	if valid().validate("init", 41, tasktypes.Status_RUNNING, 7, 0) == nil || valid().validate("init", 41, tasktypes.Status_STOPPED, 8, 0) == nil {
		t.Fatal("wrong durable state/offset accepted")
	}
	for _, state := range []string{"unsupported", "unavailable"} {
		if (&finalOutputAccounting{State: state}).validate("init", 41, tasktypes.Status_STOPPED, 7, 0) != nil {
			t.Fatal("explicit gap rejected")
		}
	}
}

type accountingFailureClient struct {
	recoveryAgentClient
	calls int
}

func (f *accountingFailureClient) CallContext(_ context.Context, method string, _ any, _ any) error {
	if method == "ProcessOutputAccounting" {
		f.calls++
	}
	return io.EOF
}
func TestFinalOutputAccountingAbsentAndFailedReportsNeverBecomeZero(t *testing.T) {
	client := &accountingFailureClient{}
	s := &service{agent: client, ioCallTimeout: time.Millisecond}
	if r := s.collectFinalOutputAccounting("init", 41, 0, 0); r.State != "unsupported" || r.Report != nil || client.calls != 0 {
		t.Fatal("old agent queried or zero qualified")
	}
	s.agentOutputAccounting.Store(true)
	if r := s.collectFinalOutputAccounting("init", 41, 0, 0); r.State != "unavailable" || r.Report != nil || client.calls == 0 {
		t.Fatal("failed report fabricated zeros")
	}
}

func TestFinalOutputAccountingRecoveryLoaderRefusesForgedFinalSnapshot(t *testing.T) {
	value := validPersistedRecovery("tests", "task")
	value.Processes[0].PID = 41
	value.Processes[0].StdoutOffset = 7
	value.Processes[0].FinalOutputAccounting = &finalOutputAccounting{State: "validated", Report: &protocol.ProcessOutputAccounting{Version: 1, ID: "init", PID: 41, Status: "STOPPED", Stdout: protocol.OutputBufferAccounting{ObservedBytes: 10, AcknowledgedBytes: 7, DroppedBytes: 3, Truncated: true, BufferedPeakBytes: 7, LimitBytes: protocol.ProcessOutputBufferLimit}, Stderr: protocol.OutputBufferAccounting{LimitBytes: protocol.ProcessOutputBufferLimit}}}
	path := filepath.Join(privateTestDirectory(t), "sandbox.json")
	write := func() {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	saved, found, err := loadPersistedRecovery(path, "tests", "task")
	if err != nil || !found || saved.Processes[0].FinalOutputAccounting.Report.Stdout.DroppedBytes != 3 {
		t.Fatal("valid retained snapshot refused", err)
	}
	value.Processes[0].StdoutOffset++
	write()
	if _, _, err = loadPersistedRecovery(path, "tests", "task"); err == nil {
		t.Fatal("mismatched durable offset accepted")
	}
	value.Processes[0].FinalOutputAccounting = nil
	write()
	if _, found, err = loadPersistedRecovery(path, "tests", "task"); err != nil || !found {
		t.Fatal("legacy absence refused", err)
	}
}

func TestFinalOutputAccountingNegotiationClearsStaleSupport(t *testing.T) {
	caps := validGuestCapabilities()
	caps.ProtocolFeatures = append(caps.ProtocolFeatures, protocol.ProcessOutputAccountingCapability)
	client := &fakeAgentClient{fail: map[string]error{}, capabilities: &caps}
	s := &service{agent: client}
	if err := s.negotiateAgentCapabilities(t.Context()); err != nil || !s.agentOutputAccounting.Load() {
		t.Fatal("optional support unavailable", err)
	}
	caps = validGuestCapabilities()
	if err := s.negotiateAgentCapabilities(t.Context()); err != nil || s.agentOutputAccounting.Load() {
		t.Fatal("old agent inherited support", err)
	}
	s.agentOutputAccounting.Store(true)
	caps.Protocol = 2
	if s.negotiateAgentCapabilities(t.Context()) == nil || s.agentOutputAccounting.Load() {
		t.Fatal("invalid negotiation retained support")
	}
}
