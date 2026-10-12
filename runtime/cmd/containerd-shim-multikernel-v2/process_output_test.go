//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestOutputOffsetsAdvanceOnlyAfterDeliveryOrBoundedDrop(t *testing.T) {
	now := time.Unix(100, 0)
	pressure := time.Time{}
	blocked := writerFunc(func([]byte) (int, error) { return 0, syscall.EAGAIN })
	advance, dropped := deliverOutput(blocked, []byte("output"), &pressure, now, time.Second)
	if advance || dropped || !pressure.Equal(now) {
		t.Fatalf("initial pressure advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(999*time.Millisecond), time.Second)
	if advance || dropped {
		t.Fatalf("pre-deadline pressure advanced=%v dropped=%v", advance, dropped)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(time.Second), time.Second)
	if !advance || !dropped || !pressure.IsZero() {
		t.Fatalf("bounded drop advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	pressure = now
	complete := writerFunc(func(value []byte) (int, error) { return len(value), nil })
	advance, dropped = deliverOutput(complete, []byte("output"), &pressure, now, time.Second)
	if !advance || dropped || !pressure.IsZero() {
		t.Fatalf("complete delivery advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(nil, []byte("discard-by-contract"), &pressure, now, time.Second)
	if !advance || dropped {
		t.Fatalf("unconfigured output advanced=%v dropped=%v", advance, dropped)
	}
}

func TestProcessOutputAndWaitRecoverTransportWithoutChangingOffsets(t *testing.T) {
	client := &outputReconnectAgent{readFailures: 2, waitFailures: 1, reconnectFailures: 1}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second}
	var output processOutput
	if err := s.readProcessOutput("init", 7, 9, &output); err != nil {
		t.Fatal(err)
	}
	if string(output.Stdout) != "after-reconnect" || output.StdoutOffset != 22 || output.StderrOffset != 9 || output.Status != "RUNNING" {
		t.Fatalf("output after reconnect = %+v", output)
	}
	var state agent.ProcessState
	if err := s.callAgentWithReconnect("WaitProcess", map[string]string{"ID": "init"}, &state); err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 19 || client.readCalls != 3 || client.waitCalls != 2 || client.reconnects != 3 {
		t.Fatalf("reconnect result state=%+v reads=%d waits=%d reconnects=%d", state, client.readCalls, client.waitCalls, client.reconnects)
	}
}

func TestWaitProcessRejectsMismatchedStoppedStateUntilExactCompletion(t *testing.T) {
	client := &waitValidationAgent{reads: make(chan struct{}), states: make(chan agent.ProcessState)}
	p := &process{id: "exec", pid: 41, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	publisher := &fakePublisher{}
	s := &service{id: "task", namespace: "tests", bundle: privateTestDirectory(t), agent: client, publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}, processes: map[string]*process{"exec": p}}
	go s.waitProcess("exec", "exec", p)
	<-client.reads
	invalid := []agent.ProcessState{
		{ID: "other", PID: 41, Status: "STOPPED", ExitCode: 37},
		{ID: "exec", PID: 41, Status: "RUNNING", ExitCode: 37},
		{ID: "exec", PID: 42, Status: "STOPPED", ExitCode: 37},
		{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: -1},
		{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: 256},
	}
	for _, state := range invalid {
		client.states <- state
		// A subsequent output read proves the prior response was rejected and
		// the monitor returned to its observation loop.
		<-client.reads
		if p.status != tasktypes.Status_RUNNING || p.pid != 41 || p.exit != 0 {
			t.Fatalf("invalid stopped state mutated process: %+v", p)
		}
		select {
		case <-p.done:
			t.Fatal("invalid stopped state completed Task wait")
		default:
		}
		if len(publisher.topics) != 0 {
			t.Fatalf("invalid stopped state published events: %v", publisher.topics)
		}
	}
	client.states <- agent.ProcessState{ID: "exec", PID: 41, Status: "STOPPED", ExitCode: 37}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("exact stopped state did not complete Task wait")
	}
	if p.status != tasktypes.Status_STOPPED || p.pid != 41 || p.exit != 37 {
		t.Fatalf("exact stopped state = status:%v pid:%d exit:%d", p.status, p.pid, p.exit)
	}
	if fmt.Sprint(publisher.topics) != "[/tasks/exit]" {
		t.Fatalf("completion events = %v", publisher.topics)
	}
}

func TestProcessOutputReconnectHasOneOverallDeadline(t *testing.T) {
	client := &outputReconnectAgent{readFailures: 1000, reconnectFailures: 1000}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: 80 * time.Millisecond}
	started := time.Now()
	var output processOutput
	err := s.readProcessOutput("init", 7, 9, &output)
	if err == nil || !strings.Contains(err.Error(), "injected output disconnect") {
		t.Fatalf("bounded reconnect error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded reconnect elapsed = %v", elapsed)
	}
	if client.readCalls < 2 || client.reconnects == 0 {
		t.Fatalf("bounded reconnect attempts reads=%d reconnects=%d", client.readCalls, client.reconnects)
	}
}

func TestProcessOutputDoesNotReplayAuthenticatedRemoteRejection(t *testing.T) {
	client := &outputReconnectAgent{remoteReadFailure: true}
	s := &service{agent: client, relaySocket: "/run/multikernel-agent/test.sock", ioCallTimeout: time.Second}
	var output processOutput
	err := s.readProcessOutput("init", 7, 9, &output)
	var remoteError *agent.RemoteError
	if !errors.As(err, &remoteError) || remoteError.Failure.Code != "NOT_FOUND" {
		t.Fatalf("remote output error = %#v", err)
	}
	if client.readCalls != 1 || client.reconnects != 0 {
		t.Fatalf("remote rejection was replayed: reads=%d reconnects=%d", client.readCalls, client.reconnects)
	}
}

func TestProcessOutputRequiresBoundedContiguousOffsetsAndKnownState(t *testing.T) {
	valid := processOutput{Stdout: []byte("x"), StdoutOffset: 8, StderrOffset: 9, Status: "RUNNING"}
	if err := validateProcessOutput(7, 9, valid); err != nil {
		t.Fatalf("valid process output = %v", err)
	}
	for name, test := range map[string]struct {
		stdoutOffset uint64
		stderrOffset uint64
		output       processOutput
	}{
		"oversized stdout":  {7, 9, processOutput{Stdout: make([]byte, processOutputChunk+1), StdoutOffset: 7 + processOutputChunk + 1, StderrOffset: 9, Status: "RUNNING"}},
		"oversized stderr":  {7, 9, processOutput{StdoutOffset: 7, Stderr: make([]byte, processOutputChunk+1), StderrOffset: 9 + processOutputChunk + 1, Status: "RUNNING"}},
		"stdout gap":        {7, 9, processOutput{Stdout: []byte("x"), StdoutOffset: 9, StderrOffset: 9, Status: "RUNNING"}},
		"stderr regression": {7, 9, processOutput{StdoutOffset: 7, StderrOffset: 8, Status: "RUNNING"}},
		"offset overflow":   {^uint64(0), 9, processOutput{Stdout: []byte("x"), StdoutOffset: 0, StderrOffset: 9, Status: "RUNNING"}},
		"unknown state":     {7, 9, processOutput{StdoutOffset: 7, StderrOffset: 9, Status: "PAUSED"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateProcessOutput(test.stdoutOffset, test.stderrOffset, test.output); err == nil {
				t.Fatal("malformed process output was accepted")
			}
		})
	}
}

func TestOutputOffsetRetriesWhenDurableAcknowledgementFails(t *testing.T) {
	bundle := privateTestDirectory(t)
	client := &outputAckAgent{offsets: make(chan uint64, 4)}
	writer := &controlledOutputWriter{writes: make(chan controlledOutputWrite)}
	p := &process{status: tasktypes.Status_RUNNING, stdoutWriter: writer, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	first := <-writer.writes
	if string(first.data) != "once" {
		t.Fatalf("first output write = %q", first.data)
	}
	close(first.release)
	// The missing recovery directory makes the first offset publication fail.
	// Reaching the second write proves the request offset rolled back to zero.
	second := <-writer.writes
	if string(second.data) != "once" {
		t.Fatalf("retried output write = %q", second.data)
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	close(second.release)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("process did not finish after durable output acknowledgement recovered")
	}
	if firstOffset, secondOffset, finalOffset := <-client.offsets, <-client.offsets, <-client.offsets; firstOffset != 0 || secondOffset != 0 || finalOffset != 4 {
		t.Fatalf("output request offsets = %d, %d, %d", firstOffset, secondOffset, finalOffset)
	}
	s.mu.Lock()
	status, exit, stdoutOffset := p.status, p.exit, p.stdoutOffset
	s.mu.Unlock()
	if status != tasktypes.Status_STOPPED || exit != 11 || stdoutOffset != 4 {
		t.Fatalf("final process = status:%v exit:%d stdout-offset:%d", status, exit, stdoutOffset)
	}
}

func TestWaitProcessDoesNotFabricateExitAfterReconnectBudget(t *testing.T) {
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: make(chan struct{})}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: privateTestDirectory(t), agent: client,
		ioCallTimeout: 20 * time.Millisecond, processes: map[string]*process{"": p},
		publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-client.observed:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not contact the guest")
	}
	// Let one complete reconnect budget expire. Transport uncertainty must not
	// become an observed process exit.
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	status, exit := p.status, p.exit
	s.mu.Unlock()
	if status != tasktypes.Status_RUNNING || exit != 0 {
		t.Fatalf("process after transport outage = status:%v exit:%d", status, exit)
	}
	select {
	case <-p.done:
		t.Fatal("transport outage fabricated process completion")
	default:
	}
	close(client.recovered)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not recover the exact guest exit")
	}
	s.mu.Lock()
	status, exit, queued := p.status, p.exit, p.exitEventQueued
	s.mu.Unlock()
	if status != tasktypes.Status_STOPPED || exit != 37 || !queued {
		t.Fatalf("recovered process exit = status:%v exit:%d event:%v", status, exit, queued)
	}
}

func TestWaitProcessDoesNotCompleteBeforeExitStateIsDurable(t *testing.T) {
	bundle := privateTestDirectory(t)
	recovered := make(chan struct{})
	close(recovered)
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: recovered}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-client.observed:
	case <-time.After(time.Second):
		t.Fatal("output monitor did not observe the stopped guest")
	}
	// The missing recovery directory prevents the exact exit from becoming
	// durable. The previous process state and open wait channel must remain.
	time.Sleep(60 * time.Millisecond)
	s.mu.Lock()
	status, exit := p.status, p.exit
	s.mu.Unlock()
	if status != tasktypes.Status_RUNNING || exit != 0 {
		t.Fatalf("process before durable exit = status:%v exit:%d", status, exit)
	}
	select {
	case <-p.done:
		t.Fatal("task wait completed before exit state was durable")
	default:
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("task wait did not complete after recovery storage returned")
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Processes) != 1 || saved.Processes[0].Status != tasktypes.Status_STOPPED || saved.Processes[0].Exit != 37 {
		t.Fatalf("durable exit state = %+v", saved.Processes)
	}
}

func TestExitEventFlagRollsBackWhenRecoveryAcknowledgementFails(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	heldDir := filepath.Join(bundle, ".multikernel-held")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	recovered := make(chan struct{})
	close(recovered)
	client := &waitOutageAgent{observed: make(chan struct{}), recovered: recovered}
	var hookErr error
	publisher := &fakePublisher{hook: func() {
		if hookErr == nil {
			hookErr = os.Rename(runtimeDir, heldDir)
		}
	}}
	p := &process{status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		agent:   client, processes: map[string]*process{"": p}, publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	go s.waitProcess("init", "", p)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("task wait did not complete after the observed exit")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if p.exitEventQueued {
		t.Fatal("memory claimed the exit-event flag was durable")
	}
	data, err := os.ReadFile(filepath.Join(heldDir, "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Processes) != 1 || saved.Processes[0].Status != tasktypes.Status_STOPPED || saved.Processes[0].Exit != 37 || saved.Processes[0].ExitEventQueued {
		t.Fatalf("durable exit-event acknowledgement = %+v", saved.Processes)
	}
	if len(publisher.topics) != 1 || publisher.topics[0] != ctruntime.TaskExitEventTopic {
		t.Fatalf("published exit topics = %v", publisher.topics)
	}
}
