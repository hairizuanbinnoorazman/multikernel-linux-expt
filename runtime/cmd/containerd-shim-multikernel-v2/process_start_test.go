//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestStartReturnsKillFailureAndRetainsMonitorAfterPersistenceFailure(t *testing.T) {
	bundle := privateTestDirectory(t)
	killFailure := errors.New("injected started-process kill failure")
	client := &startCleanupAgent{finish: make(chan struct{}), signalErr: killFailure}
	p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
		publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		processes: map[string]*process{
			"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
			"exec": p,
		}}
	_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
	if err == nil || !strings.Contains(err.Error(), "persist started process") || !errors.Is(err, killFailure) {
		t.Fatalf("Start cleanup error = %v", err)
	}
	if p.status != tasktypes.Status_RUNNING || p.pid != 41 {
		t.Fatalf("retained started process = status:%v pid:%d", p.status, p.pid)
	}
	select {
	case <-p.done:
		t.Fatal("failed Start abandoned its running-process monitor")
	default:
	}
	if err = os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	close(client.finish)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("retained monitor did not observe the exact later exit")
	}
	if p.status != tasktypes.Status_STOPPED || p.exit != 9 {
		t.Fatalf("observed retained-process exit = status:%v exit:%d", p.status, p.exit)
	}
}

func TestStartRejectsGuestPIDOutsideTaskRangeAndRetainsOwnership(t *testing.T) {
	if ^uint(0)>>32 == 0 {
		t.Skip("host int cannot represent a PID above the Task v2 range")
	}
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	client := &startCleanupAgent{finish: make(chan struct{}), statePID: int(uint64(^uint32(0)) + 1)}
	p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	publisher := &fakePublisher{}
	s := &service{id: "task", namespace: "tests", bundle: bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
		publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		processes: map[string]*process{
			"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
			"exec": p,
		}}
	_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
	if err == nil || !strings.Contains(err.Error(), "outside the Task v2 range") {
		t.Fatalf("Start invalid PID error = %v", err)
	}
	if p.status != tasktypes.Status_RUNNING || p.pid != 0 {
		t.Fatalf("unverified process ownership = status:%v pid:%d", p.status, p.pid)
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("invalid PID published lifecycle events: %v", publisher.topics)
	}
	data, readErr := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(data, []byte(`"status":2`)) || bytes.Contains(data, []byte(`"pid":`)) {
		t.Fatalf("unverified ownership was not persisted without a PID: %s", data)
	}
	close(client.finish)
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("unverified process monitor did not observe cleanup completion")
	}
}

func TestStartRejectsMismatchedGuestProcessStateAndRetainsOwnership(t *testing.T) {
	tests := []struct {
		name, id, state string
	}{
		{name: "wrong identity", id: "other", state: "RUNNING"},
		{name: "created", state: "CREATED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			client := &startCleanupAgent{finish: make(chan struct{}), stateID: test.id, state: test.state}
			p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
			publisher := &fakePublisher{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
				publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": p,
				}}
			_, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
			if err == nil || !strings.Contains(err.Error(), "mismatched identity or state") {
				t.Fatalf("Start mismatched state error = %v", err)
			}
			if p.status != tasktypes.Status_RUNNING || p.pid != 0 || len(publisher.topics) != 0 {
				t.Fatalf("mismatched state ownership status=%v pid=%d events=%v", p.status, p.pid, publisher.topics)
			}
			close(client.finish)
			select {
			case <-p.done:
			case <-time.After(time.Second):
				t.Fatal("mismatched-state process monitor did not observe cleanup")
			}
		})
	}
}

func TestStartReconcilesAmbiguousGuestResultWithoutLosingOwnership(t *testing.T) {
	startFailure := errors.New("injected start transport failure")
	stateFailure := errors.New("injected state transport failure")
	tests := []struct {
		name              string
		state             string
		stateErr          error
		transientStateErr error
		stateFailures     int
		wantReconnect     int
		wantSuccess       bool
		wantCreated       bool
		wantEvent         bool
	}{
		{name: "confirmed created remains retryable", state: "CREATED", wantCreated: true},
		{name: "running proves start applied", state: "RUNNING", wantSuccess: true, wantEvent: true},
		{name: "rapid stop proves start applied", state: "STOPPED", wantSuccess: true, wantEvent: true},
		{name: "lost state reply reconnects", state: "RUNNING", transientStateErr: io.ErrUnexpectedEOF, stateFailures: 1, wantReconnect: 1, wantSuccess: true, wantEvent: true},
		{name: "unresolved transport failure retains ownership", stateErr: stateFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			client := &startCleanupAgent{finish: make(chan struct{}), startErr: startFailure, stateErr: test.stateErr,
				transientStateErr: test.transientStateErr, stateFailures: test.stateFailures, state: test.state}
			p := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
			publisher := &fakePublisher{}
			s := &service{id: "task", namespace: "tests", bundle: bundle,
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: client,
				publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				processes: map[string]*process{
					"":     {status: tasktypes.Status_RUNNING, done: make(chan struct{})},
					"exec": p,
				}}
			if test.wantReconnect != 0 {
				s.relaySocket = "/run/multikernel/relay.sock"
				s.ioCallTimeout = time.Second
			}
			response, err := s.Start(context.Background(), &taskapi.StartRequest{ID: "task", ExecID: "exec"})
			if test.wantSuccess {
				if err != nil || response == nil || response.Pid != 41 {
					t.Fatalf("reconciled Start response=%+v error=%v", response, err)
				}
			} else if !errors.Is(err, startFailure) {
				t.Fatalf("ambiguous Start error = %v, want start failure", err)
			}
			if client.reconnects != test.wantReconnect {
				t.Fatalf("state reconnects = %d, want %d", client.reconnects, test.wantReconnect)
			}
			startPublished := false
			for _, topic := range publisher.topics {
				startPublished = startPublished || topic == ctruntime.TaskExecStartedEventTopic
			}
			if startPublished != test.wantEvent {
				t.Fatalf("published events = %v, want start event=%v", publisher.topics, test.wantEvent)
			}
			if test.wantCreated {
				if p.status != tasktypes.Status_CREATED || p.pid != 0 {
					t.Fatalf("confirmed-created state=%v pid=%d", p.status, p.pid)
				}
				select {
				case <-p.done:
					t.Fatal("confirmed-created process completed")
				default:
				}
			} else {
				var expectedPID uint32
				if test.wantSuccess {
					expectedPID = 41
				}
				if p.status != tasktypes.Status_RUNNING || p.pid != expectedPID {
					t.Fatalf("retained ownership state=%v pid=%d", p.status, p.pid)
				}
				close(client.finish)
				select {
				case <-p.done:
				case <-time.After(time.Second):
					t.Fatal("reconciled process monitor did not observe completion")
				}
			}
		})
	}
}

func TestEnsureGuestProcessCreatedReconcilesBeforeMutation(t *testing.T) {
	notFound := &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}}
	transportFailure := errors.New("injected state transport failure")
	tests := []struct {
		name       string
		state      agent.ProcessState
		stateErr   error
		createErr  error
		wantCreate int
		wantErr    string
	}{
		{name: "absent is created", stateErr: notFound, wantCreate: 1},
		{name: "create rejection is returned", stateErr: notFound, createErr: &agent.RemoteError{Failure: protocol.Error{Code: "FAILED_PRECONDITION", Message: "injected create failure"}}, wantCreate: 1, wantErr: "create failure"},
		{name: "existing created is reused", state: agent.ProcessState{ID: "init", Status: "CREATED"}},
		{name: "wrong identity", state: agent.ProcessState{ID: "other", Status: "CREATED"}, wantErr: "mismatched identity or state"},
		{name: "running is not recreated", state: agent.ProcessState{ID: "init", PID: 41, Status: "RUNNING"}, wantErr: "mismatched identity or state"},
		{name: "created with pid is invalid", state: agent.ProcessState{ID: "init", PID: 41, Status: "CREATED"}, wantErr: "mismatched identity or state"},
		{name: "transport failure is not absence", stateErr: transportFailure, wantErr: "state transport failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &createReconcileAgent{state: test.state, stateErr: test.stateErr, createErr: test.createErr}
			s := &service{agent: client}
			err := s.ensureGuestProcessCreated(context.Background(), "init", "/bundle")
			if test.wantErr == "" && err != nil || test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("reconcile error = %v, want substring %q", err, test.wantErr)
			}
			if client.stateCalls != 1 || client.createCalls != test.wantCreate {
				t.Fatalf("calls state=%d create=%d, want 1/%d", client.stateCalls, client.createCalls, test.wantCreate)
			}
		})
	}
}

func TestEnsureGuestProcessCreatedReconcilesLostReply(t *testing.T) {
	notFound := &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}}
	client := &createReconcileAgent{
		stateErr: notFound, createErr: io.ErrUnexpectedEOF,
		createdState: agent.ProcessState{ID: "init", Status: "CREATED"},
	}
	s := &service{agent: client, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second}
	if err := s.ensureGuestProcessCreated(context.Background(), "init", "/bundle"); err != nil {
		t.Fatal(err)
	}
	if client.stateCalls != 3 || client.createCalls != 1 || client.reconnects != 1 {
		t.Fatalf("calls state=%d create=%d reconnect=%d", client.stateCalls, client.createCalls, client.reconnects)
	}
}

func TestStartedProcessCleanupTreatsAuthenticatedAbsenceAsSuccess(t *testing.T) {
	client := &fakeAgentClient{fail: map[string]error{
		"SignalProcess": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
	}}
	s := &service{agent: client}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.killStartedProcess(ctx, "exec"); err != nil {
		t.Fatal(err)
	}
}

func TestExecRejectsGuestUnrepresentableIDBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	for _, id := range []string{"", "../exec", "Exec", "exec_id", strings.Repeat("a", 65)} {
		if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: id, Spec: encoded}); !errors.Is(err, errdefs.ErrInvalidArgument) {
			t.Fatalf("ExecID %q error = %v, want invalid argument", id, err)
		}
	}
	if len(fake.calls) != 0 || len(s.processes) != 0 {
		t.Fatalf("invalid exec reached agent or state: calls=%v processes=%v", fake.calls, s.processes)
	}
}
