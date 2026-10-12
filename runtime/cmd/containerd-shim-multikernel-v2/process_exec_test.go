//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/typeurl/v2"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestExecRollsBackProcessOnAgentFailure(t *testing.T) {
	agentFailure := errors.New("injected agent failure")
	fake := &fakeAgentClient{fail: map[string]error{"ExecProcess": agentFailure}}
	s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	_, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "failed", Spec: encoded})
	if !errors.Is(err, agentFailure) {
		t.Fatalf("Exec() error = %v, want injected failure", err)
	}
	if _, exists := s.processes["failed"]; exists {
		t.Fatal("failed exec left a stale process")
	}
}

func TestExecCreationIntentReconcilesAmbiguousGuestMutation(t *testing.T) {
	newFixture := func(t *testing.T, fake *execCreationAgent) (*service, *fakePublisher, *taskapi.ExecProcessRequest) {
		t.Helper()
		bundle := privateTestDirectory(t)
		if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
			t.Fatal(err)
		}
		fake.bundle = bundle
		publisher := &fakePublisher{}
		s := &service{id: "task", namespace: "tests", bundle: bundle,
			sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: fake, publisher: publisher,
			processes: map[string]*process{"": {status: tasktypes.Status_RUNNING, done: make(chan struct{})}},
			events:    eventJournal{SchemaVersion: 1, NextSequence: 1}}
		spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
		encodedValue, err := typeurl.MarshalAny(spec)
		if err != nil {
			t.Fatal(err)
		}
		request := &taskapi.ExecProcessRequest{ID: "task", ExecID: "exec",
			Spec: &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}}
		return s, publisher, request
	}
	hasDurableExec := func(t *testing.T, bundle string) bool {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
		if err != nil {
			t.Fatal(err)
		}
		var saved persisted
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		for _, process := range saved.Processes {
			if process.ID == "exec" {
				return true
			}
		}
		return false
	}

	t.Run("lost reply confirms created owner", func(t *testing.T) {
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF,
			state: agent.ProcessState{ID: "exec", Status: "CREATED"}}
		s, publisher, request := newFixture(t, fake)
		if _, err := s.Exec(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if !fake.durableOwner || s.processes["exec"] == nil || !hasDurableExec(t, s.bundle) {
			t.Fatalf("confirmed creation ownership: before-call=%v memory=%v", fake.durableOwner, s.processes["exec"])
		}
		if fmt.Sprint(fake.calls) != "[ExecProcess StateProcess]" || fmt.Sprint(publisher.topics) != "[/tasks/exec-added]" {
			t.Fatalf("confirmed creation calls=%v events=%v", fake.calls, publisher.topics)
		}
	})

	t.Run("unavailable state and cleanup retain durable owner", func(t *testing.T) {
		stateFailure := errors.New("injected state outage")
		deleteFailure := errors.New("injected delete outage")
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF, stateErr: stateFailure, deleteErr: deleteFailure}
		s, publisher, request := newFixture(t, fake)
		_, err := s.Exec(context.Background(), request)
		if !errors.Is(err, stateFailure) || !errors.Is(err, deleteFailure) {
			t.Fatalf("ambiguous creation error = %v", err)
		}
		if !fake.durableOwner || s.processes["exec"] == nil || !hasDurableExec(t, s.bundle) || len(publisher.topics) != 0 {
			t.Fatalf("ambiguous creation ownership: before-call=%v memory=%v events=%v", fake.durableOwner, s.processes["exec"], publisher.topics)
		}
	})

	t.Run("confirmed absence removes durable intent", func(t *testing.T) {
		notFound := &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}
		fake := &execCreationAgent{createErr: io.ErrUnexpectedEOF, stateErr: notFound, deleteErr: notFound}
		s, publisher, request := newFixture(t, fake)
		if _, err := s.Exec(context.Background(), request); err == nil {
			t.Fatal("absent guest creation unexpectedly succeeded")
		}
		if !fake.durableOwner || s.processes["exec"] != nil || hasDurableExec(t, s.bundle) || len(publisher.topics) != 0 {
			t.Fatalf("absent creation ownership: before-call=%v memory=%v events=%v", fake.durableOwner, s.processes["exec"], publisher.topics)
		}
	})
}

func TestRecoveredCreatedExecRequiresExactGuestOwnership(t *testing.T) {
	for _, test := range []struct {
		name       string
		state      agent.ProcessState
		stateErr   error
		wantExists bool
		wantErr    bool
		wantEvent  bool
	}{
		{name: "exact created", state: agent.ProcessState{ID: "exec", Status: "CREATED"}, wantExists: true, wantEvent: true},
		{name: "confirmed absent", stateErr: &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "managed process was not found"}}},
		{name: "wrong identity", state: agent.ProcessState{ID: "other", Status: "CREATED"}, wantErr: true},
		{name: "running", state: agent.ProcessState{ID: "exec", PID: 41, Status: "RUNNING"}, wantErr: true},
		{name: "transport unavailable", stateErr: io.ErrUnexpectedEOF, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			fake := &execCreationAgent{state: test.state, stateErr: test.stateErr}
			publisher := &fakePublisher{}
			s := &service{id: "task", bundle: bundle, agent: fake, publisher: publisher,
				events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			exists, err := s.reconcileRecoveredCreatedExec(context.Background(), &process{id: "exec", status: tasktypes.Status_CREATED})
			if (err != nil) != test.wantErr || exists != test.wantExists {
				t.Fatalf("reconcile result exists=%v error=%v", exists, err)
			}
			if got := fmt.Sprint(publisher.topics); (got == "[/tasks/exec-added]") != test.wantEvent {
				t.Fatalf("recovered exec events = %v", publisher.topics)
			}
		})
	}
}

func TestExecRollbackRetainsOwnershipUntilGuestDeletion(t *testing.T) {
	deleteFailure := errors.New("injected exec cleanup failure")
	for name, failure := range map[string]error{
		"deleted":        nil,
		"already absent": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
		"delete failed":  deleteFailure,
	} {
		t.Run(name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			fake := &fakeAgentClient{fail: map[string]error{}}
			if failure != nil {
				fake.fail["DeleteProcess"] = failure
			}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: fake,
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": {id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})},
				}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := s.rollbackExec(ctx, "exec")
			_, retained := s.processes["exec"]
			if errors.Is(failure, deleteFailure) {
				if !errors.Is(err, deleteFailure) || !retained {
					t.Fatalf("failed rollback error=%v retained=%v", err, retained)
				}
			} else if err != nil || retained {
				t.Fatalf("completed rollback error=%v retained=%v", err, retained)
			}
			data, readErr := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var saved persisted
			if readErr = json.Unmarshal(data, &saved); readErr != nil {
				t.Fatal(readErr)
			}
			found := false
			for _, process := range saved.Processes {
				found = found || process.ID == "exec"
			}
			if found != retained {
				t.Fatalf("durable exec ownership=%v, memory=%v", found, retained)
			}
		})
	}
}
