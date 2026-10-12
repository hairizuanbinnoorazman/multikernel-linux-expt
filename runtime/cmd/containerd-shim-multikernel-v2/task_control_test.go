//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestKillForwardsOnlyForLiveKnownProcess(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_PAUSED},
	}}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM), All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "exec", Signal: uint32(syscall.SIGKILL)}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[SignalProcess AcknowledgeSignal SignalProcess AcknowledgeSignal]" {
		t.Fatalf("kill calls = %v", fake.calls)
	}
}

func TestKillPersistsAndDeduplicatesLostSignalReply(t *testing.T) {
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	fake := &signalReplyLossAgent{bundle: bundle}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle, agent: fake,
		relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second,
		sandbox:   protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: strings.Repeat("a", 32)},
		processes: map[string]*process{"": p}}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ID: "task", Signal: uint32(syscall.SIGUSR1)}); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 || fake.applied != 1 || fake.reconnects != 1 || fake.ackCalls != 1 || !fake.durable {
		t.Fatalf("signal calls=%d applied=%d reconnects=%d acknowledgements=%d durable=%v", fake.calls, fake.applied, fake.reconnects, fake.ackCalls, fake.durable)
	}
	if p.pendingSignalOperation != "" || p.pendingSignal != 0 {
		t.Fatalf("acknowledged signal intent = %q/%d", p.pendingSignalOperation, p.pendingSignal)
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Processes) != 1 || saved.Processes[0].PendingSignalOp != "" || saved.Processes[0].PendingSignal != 0 {
		t.Fatalf("durable acknowledged signal = %+v", saved.Processes)
	}
}

func TestShutdownWaitsForEmptyOwnershipAndSealsService(t *testing.T) {
	called := make(chan struct{}, 1)
	s := &service{id: "task", bundle: "/bundle", shutdown: func() { called <- struct{}{} }, processes: map[string]*process{
		"": {status: tasktypes.Status_STOPPED},
	}}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task", Now: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("shutdown callback ran while process ownership remained")
	default:
	}
	s.mu.Lock()
	delete(s.processes, "")
	s.mu.Unlock()
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("empty service did not invoke shutdown callback")
	}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("create after shutdown error = %v", err)
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("repeated shutdown invoked callback again")
	default:
	}
}

func TestShutdownClosesHeldBundleIdentity(t *testing.T) {
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	called := make(chan struct{})
	identity := directory.Identity()
	s := &service{id: "task", bundle: bundle, bundleDirectory: directory,
		bundleIdentity: rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID},
		shutdown:       func() { close(called) }, processes: map[string]*process{}}
	runtimeDirectory, err := s.ensureRuntimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
	if err = directory.Sync(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("held bundle descriptor after shutdown = %v", err)
	}
	if err = runtimeDirectory.Sync(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("held runtime descriptor after shutdown = %v", err)
	}
}

func TestShutdownRetainsOwnershipUntilDurableEventsFlush(t *testing.T) {
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{failures: 2}
	called := make(chan struct{}, 1)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher,
		shutdown: func() { called <- struct{}{} }, processes: map[string]*process{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: "task", ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if len(s.events.Pending) != 1 {
		t.Fatalf("pending events = %d", len(s.events.Pending))
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err == nil {
		t.Fatal("shutdown accepted while durable event delivery failed")
	}
	select {
	case <-called:
		t.Fatal("failed event flush invoked shutdown callback")
	default:
	}
	publisher.failures = 0
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not proceed after event delivery recovered")
	}
	if len(s.events.Pending) != 0 {
		t.Fatalf("events remain pending after shutdown: %+v", s.events.Pending)
	}
}

func TestInitTaskAPIsExposeHostNamespaceHolderAndRetainGuestIdentity(t *testing.T) {
	holder := &fakeNamespaceHolder{pid: 73}
	s := &service{bundle: "/bundle", processes: map[string]*process{
		"":     {pid: 17, status: tasktypes.Status_RUNNING},
		"exec": {pid: 23, status: tasktypes.Status_RUNNING},
	}, namespaceHolder: holder}
	initState, err := s.State(context.Background(), &taskapi.StateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if initState.Pid != 73 || s.processes[""].pid != 17 {
		t.Fatalf("init State PID=%d guest PID=%d, want holder 73 and guest 17", initState.Pid, s.processes[""].pid)
	}
	state, err := s.State(context.Background(), &taskapi.StateRequest{ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Pid != 23 {
		t.Fatalf("State PID = %d, want guest PID 23", state.Pid)
	}
	pids, err := s.Pids(context.Background(), &taskapi.PidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint32]bool{}
	for _, process := range pids.Processes {
		got[process.Pid] = true
	}
	if !got[73] || !got[23] || len(got) != 2 {
		t.Fatalf("Pids = %v, want holder PID 73 and exec guest PID 23", got)
	}
	connected, err := s.Connect(context.Background(), &taskapi.ConnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if connected.TaskPid != 73 || connected.ShimPid != uint32(os.Getpid()) || connected.Version != "multikernel-v2-host-netns-pid" {
		t.Fatalf("Connect = task:%d shim:%d", connected.TaskPid, connected.ShimPid)
	}
}

func TestTaskRPCsRejectNilAndMismatchedTaskIdentityBeforeMutation(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{id: "task", bundle: "/bundle", agent: fake, processes: map[string]*process{
		"": {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
	}}
	tests := []struct {
		name  string
		nil   func() error
		wrong func() error
	}{
		{"Create", func() error { _, err := s.Create(context.Background(), nil); return err }, func() error {
			_, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "other"})
			return err
		}},
		{"Start", func() error { _, err := s.Start(context.Background(), nil); return err }, func() error { _, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "other"}); return err }},
		{"State", func() error { _, err := s.State(context.Background(), nil); return err }, func() error { _, err := s.State(context.Background(), &taskapi.StateRequest{ID: "other"}); return err }},
		{"Wait", func() error { _, err := s.Wait(context.Background(), nil); return err }, func() error { _, err := s.Wait(context.Background(), &taskapi.WaitRequest{ID: "other"}); return err }},
		{"Kill", func() error { _, err := s.Kill(context.Background(), nil); return err }, func() error { _, err := s.Kill(context.Background(), &taskapi.KillRequest{ID: "other"}); return err }},
		{"Exec", func() error { _, err := s.Exec(context.Background(), nil); return err }, func() error {
			_, err := s.Exec(context.Background(), &taskapi.ExecProcessRequest{ID: "other"})
			return err
		}},
		{"Delete", func() error { _, err := s.Delete(context.Background(), nil); return err }, func() error {
			_, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "other"})
			return err
		}},
		{"Pids", func() error { _, err := s.Pids(context.Background(), nil); return err }, func() error { _, err := s.Pids(context.Background(), &taskapi.PidsRequest{ID: "other"}); return err }},
		{"Connect", func() error { _, err := s.Connect(context.Background(), nil); return err }, func() error {
			_, err := s.Connect(context.Background(), &taskapi.ConnectRequest{ID: "other"})
			return err
		}},
		{"Shutdown", func() error { _, err := s.Shutdown(context.Background(), nil); return err }, func() error {
			_, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "other", Now: true})
			return err
		}},
		{"ResizePty", func() error { _, err := s.ResizePty(context.Background(), nil); return err }, func() error {
			_, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{ID: "other"})
			return err
		}},
		{"Pause", func() error { _, err := s.Pause(context.Background(), nil); return err }, func() error { _, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "other"}); return err }},
		{"Resume", func() error { _, err := s.Resume(context.Background(), nil); return err }, func() error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "other"})
			return err
		}},
		{"CloseIO", func() error { _, err := s.CloseIO(context.Background(), nil); return err }, func() error {
			_, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{ID: "other"})
			return err
		}},
		{"Stats", func() error { _, err := s.Stats(context.Background(), nil); return err }, func() error { _, err := s.Stats(context.Background(), &taskapi.StatsRequest{ID: "other"}); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for name, invoke := range map[string]func() error{"nil": test.nil, "mismatched": test.wrong} {
				t.Run(name, func(t *testing.T) {
					if err := invoke(); !errors.Is(err, errdefs.ErrInvalidArgument) {
						t.Fatalf("error = %v, want invalid argument", err)
					}
				})
			}
		})
	}
	if len(fake.calls) != 0 || len(s.processes) != 1 || s.processes[""].status != tasktypes.Status_RUNNING || s.shuttingDown {
		t.Fatalf("invalid requests mutated service: calls=%v processes=%+v shuttingDown=%v", fake.calls, s.processes, s.shuttingDown)
	}
}
