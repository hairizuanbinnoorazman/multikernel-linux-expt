//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cgroupstats "github.com/containerd/cgroups/stats/v1"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestPauseResumeSignalsGuestAndPublishesTransitions(t *testing.T) {
	fakeAgent := &fakeAgentClient{fail: map[string]error{}}
	fakeEvents := &fakePublisher{}
	s := &service{
		id: "task", namespace: "default", bundle: privateTestDirectory(t), agent: fakeAgent, publisher: fakeEvents,
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING}},
	}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_PAUSED || s.processes["exec"].status != tasktypes.Status_PAUSED {
		t.Fatalf("pause statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if _, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("resume statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if len(fakeAgent.calls) != 8 {
		t.Fatalf("agent calls = %v", fakeAgent.calls)
	}
	if len(fakeEvents.topics) != 2 || fakeEvents.topics[0] != ctruntime.TaskPausedEventTopic || fakeEvents.topics[1] != ctruntime.TaskResumedEventTopic {
		t.Fatalf("event topics = %v", fakeEvents.topics)
	}
}

func TestPauseRollsBackAlreadySignaledProcessOnPartialFailure(t *testing.T) {
	fake := &signalFailureAgent{failAt: 2}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{}); err == nil {
		t.Fatal("partial pause failure was accepted")
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("partial pause mutated states: init=%v exec=%v", s.processes[""].status, s.processes["exec"].status)
	}
	want := fmt.Sprintf("[init:%d exec:%d init:%d]", syscall.SIGSTOP, syscall.SIGSTOP, syscall.SIGCONT)
	if fmt.Sprint(fake.calls) != want {
		t.Fatalf("partial pause calls = %v, want %s", fake.calls, want)
	}
}

func TestPauseResumeReturnRecoveryRollbackFailure(t *testing.T) {
	for name, test := range map[string]struct {
		initial          tasktypes.Status
		rollbackSignal   syscall.Signal
		durableOnFailure tasktypes.Status
		invoke           func(*service) error
		errorLabel       string
	}{
		"pause": {tasktypes.Status_RUNNING, syscall.SIGCONT, tasktypes.Status_PAUSED, func(s *service) error {
			_, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"})
			return err
		}, "persist pause rollback"},
		"resume": {tasktypes.Status_PAUSED, syscall.SIGSTOP, tasktypes.Status_RUNNING, func(s *service) error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"})
			return err
		}, "persist resume rollback"},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			runtimeDir := filepath.Join(bundle, ".multikernel")
			heldDir := filepath.Join(bundle, ".multikernel-held")
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			var hookErr error
			fake := &signalFailureAgent{hook: func(call string) {
				if call == fmt.Sprintf("init:%d", test.rollbackSignal) {
					hookErr = os.Rename(runtimeDir, heldDir)
				}
			}}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
				agent:   fake, publisher: &fakePublisher{}, processes: map[string]*process{
					"": {status: test.initial, done: make(chan struct{})},
				}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			originalMarshal := jsonMarshal
			jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected transition event failure") }
			err := test.invoke(s)
			jsonMarshal = originalMarshal
			if hookErr != nil {
				t.Fatal(hookErr)
			}
			if err == nil || !strings.Contains(err.Error(), "injected transition event failure") || !strings.Contains(err.Error(), test.errorLabel) {
				t.Fatalf("transition rollback error = %v", err)
			}
			if observed := s.processes[""].status; observed != test.initial {
				t.Fatalf("memory state after rollback = %v, want %v", observed, test.initial)
			}
			data, readErr := os.ReadFile(filepath.Join(heldDir, "sandbox.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var saved persisted
			if readErr = json.Unmarshal(data, &saved); readErr != nil {
				t.Fatal(readErr)
			}
			if len(saved.Processes) != 1 || saved.Processes[0].Status != test.durableOnFailure {
				t.Fatalf("unrepaired durable transition = %+v", saved.Processes)
			}
		})
	}
}

func TestPauseResumeRepublishPriorStateAfterTransitionPersistenceFailure(t *testing.T) {
	for name, test := range map[string]struct {
		initial        tasktypes.Status
		forwardSignal  syscall.Signal
		rollbackSignal syscall.Signal
		invoke         func(*service) error
		errorLabel     string
	}{
		"pause": {tasktypes.Status_RUNNING, syscall.SIGSTOP, syscall.SIGCONT, func(s *service) error {
			_, err := s.Pause(context.Background(), &taskapi.PauseRequest{ID: "task"})
			return err
		}, "persist paused state"},
		"resume": {tasktypes.Status_PAUSED, syscall.SIGCONT, syscall.SIGSTOP, func(s *service) error {
			_, err := s.Resume(context.Background(), &taskapi.ResumeRequest{ID: "task"})
			return err
		}, "persist resumed state"},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			runtimeDir := filepath.Join(bundle, ".multikernel")
			heldDir := filepath.Join(bundle, ".multikernel-held")
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			var hookErr error
			fake := &signalFailureAgent{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
				agent:   fake, publisher: &fakePublisher{}, processes: map[string]*process{
					"": {status: test.initial, done: make(chan struct{})},
				}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := s.persistRecovery(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtimeDir, "sandbox.json")
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			fake.hook = func(call string) {
				switch call {
				case fmt.Sprintf("init:%d", test.forwardSignal):
					hookErr = os.Rename(runtimeDir, heldDir)
				case fmt.Sprintf("init:%d", test.rollbackSignal):
					if hookErr == nil {
						hookErr = os.Rename(heldDir, runtimeDir)
					}
				}
			}
			err = test.invoke(s)
			if hookErr != nil {
				t.Fatal(hookErr)
			}
			if err == nil || !strings.Contains(err.Error(), test.errorLabel) {
				t.Fatalf("transition persistence error = %v", err)
			}
			if observed := s.processes[""].status; observed != test.initial {
				t.Fatalf("memory state after persistence rollback = %v, want %v", observed, test.initial)
			}
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(before, after) {
				t.Fatal("prior recovery state was not republished after transition failure")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var saved persisted
			if err = json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Processes) != 1 || saved.Processes[0].Status != test.initial {
				t.Fatalf("republished durable state = %+v", saved.Processes)
			}
		})
	}
}

func TestStatsReturnsGuestProcessGroupMetrics(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: 11, CPUSystemNS: 7, RSSBytes: 4096, PIDs: 3},
		"exec": {CPUUserNS: 5, CPUSystemNS: 2, RSSBytes: 2048, PIDs: 1},
	}}
	s := &service{
		agent: fake, sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{MemoryBytes: 3 << 30}},
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_PAUSED}, "created": {status: tasktypes.Status_CREATED}},
	}
	response, err := s.Stats(context.Background(), &taskapi.StatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typeurl.UnmarshalAny(response.Stats)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := decoded.(*cgroupstats.Metrics)
	if !ok {
		t.Fatalf("stats type = %T", decoded)
	}
	if metrics.CPU.Usage.Total != 25 || metrics.CPU.Usage.User != 16 || metrics.CPU.Usage.Kernel != 9 || metrics.Memory.Usage.Usage != 6144 || metrics.Memory.Usage.Limit != 3<<30 || metrics.Pids.Current != 4 {
		t.Fatalf("stats = %+v", metrics)
	}
	if fmt.Sprint(fake.calls) != "[StatsProcess StatsProcess]" {
		t.Fatalf("stats calls = %v", fake.calls)
	}
}

func TestStatsRejectsAggregateOverflow(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: ^uint64(0)},
		"exec": {CPUUserNS: 1},
	}}
	s := &service{agent: fake, processes: map[string]*process{
		"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Stats(context.Background(), &taskapi.StatsRequest{}); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow stats error = %v", err)
	}
}

func TestStatsRetriesLostGuestReply(t *testing.T) {
	fake := &statsReplyLossAgent{stats: agent.ProcessStats{CPUUserNS: 11, CPUSystemNS: 7, RSSBytes: 4096, PIDs: 3}}
	s := &service{agent: fake, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second,
		sandbox:   protocol.Sandbox{Config: protocol.SandboxConfig{MemoryBytes: 3 << 30}},
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	response, err := s.Stats(context.Background(), &taskapi.StatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typeurl.UnmarshalAny(response.Stats)
	if err != nil {
		t.Fatal(err)
	}
	metrics := decoded.(*cgroupstats.Metrics)
	if fake.calls != 2 || fake.reconnects != 1 || metrics.CPU.Usage.Total != 18 || metrics.Memory.Usage.Usage != 4096 || metrics.Pids.Current != 3 {
		t.Fatalf("calls=%d reconnects=%d stats=%+v", fake.calls, fake.reconnects, metrics)
	}
}

func TestUpdateAndCheckpointAreExcludedWithoutMutation(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	events := &fakePublisher{}
	s := &service{agent: fake, publisher: events, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	if _, err := s.Update(context.Background(), &taskapi.UpdateTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Update error = %v", err)
	}
	if _, err := s.Checkpoint(context.Background(), &taskapi.CheckpointTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Checkpoint error = %v", err)
	}
	if len(fake.calls) != 0 || len(events.topics) != 0 || s.processes[""].status != tasktypes.Status_RUNNING {
		t.Fatalf("excluded methods mutated state: calls=%v events=%v status=%v", fake.calls, events.topics, s.processes[""].status)
	}
}
