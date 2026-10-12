//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestDeleteRepairsMissingExitEventBeforeDeleteEvent(t *testing.T) {
	publisher := &fakePublisher{}
	fake := &fakeAgentClient{fail: map[string]error{}}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exit: 17,
		exited: time.Unix(123, 0).UTC(), done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), publisher: publisher, agent: fake,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task", ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskExitEventTopic, ctruntime.TaskDeleteEventTopic}
	if response.ExitStatus != 17 || fmt.Sprint(publisher.topics) != fmt.Sprint(want) {
		t.Fatalf("response=%+v topics=%v, want %v", response, publisher.topics, want)
	}
	if _, exists := s.processes["exec"]; exists {
		t.Fatal("deleted exec process remains")
	}
}

func TestDeleteTreatsAuthenticatedGuestAbsenceAsIdempotentSuccess(t *testing.T) {
	publisher := &fakePublisher{}
	fake := &fakeAgentClient{fail: map[string]error{
		"DeleteProcess": &agent.RemoteError{Failure: protocol.Error{Code: "NOT_FOUND", Message: "process not found"}},
	}}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exit: 17,
		exited: time.Unix(123, 0).UTC(), exitEventQueued: true, done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), publisher: publisher, agent: fake,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task", ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Pid != 23 || response.ExitStatus != 17 {
		t.Fatalf("idempotent delete response = %+v", response)
	}
	if _, exists := s.processes["exec"]; exists {
		t.Fatal("confirmed-absent guest retained ownership")
	}
	if fmt.Sprint(publisher.topics) != "[/tasks/delete]" {
		t.Fatalf("idempotent delete events = %v", publisher.topics)
	}
}

func TestInitDeleteContinuesHostCleanupAfterGuestNetworkFailure(t *testing.T) {
	guestFailure := errors.New("injected guest network close failure")
	fake := &fakeAgentClient{fail: map[string]error{"CloseNetwork": guestFailure}}
	holder := &fakeNamespaceHolder{pid: 73}
	p := &process{pid: 17, status: tasktypes.Status_STOPPED, exit: 9, exited: time.Unix(123, 0).UTC(),
		exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{})}
	close(p.done)
	sandbox := protocol.Sandbox{ID: "mk-task", Generation: strings.Repeat("a", 32)}
	endpoint := validShimEndpoint("task", sandbox.ID, sandbox.Generation, strings.Repeat("b", 32), "/run/netns/mk-task")
	network := &fakeNetworkClient{endpoint: endpoint}
	var daemonCalls []string
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), agent: fake,
		namespaceHolder: holder, netEndpoint: endpoint, netClient: network, sandbox: sandbox,
		processes: map[string]*process{"": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
			daemonCalls = append(daemonCalls, request.Method)
			return nil
		})}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task"})
	if err != nil {
		t.Fatalf("guest network failure prevented host cleanup: %v", err)
	}
	if response.Pid != 73 || response.ExitStatus != 9 {
		t.Fatalf("delete response = %+v, want holder PID 73 and exit 9", response)
	}
	if !holder.stopped || s.namespaceHolder != nil || s.netEndpoint.Generation != "" || s.processes[""] != nil {
		t.Fatalf("host ownership remains: holderStopped=%v holder=%v endpoint=%+v process=%v",
			holder.stopped, s.namespaceHolder, s.netEndpoint, s.processes[""])
	}
	if !reflect.DeepEqual(daemonCalls, []string{"StopSandbox", "DeleteSandbox"}) {
		t.Fatalf("sandbox cleanup calls = %v", daemonCalls)
	}
	if fmt.Sprint(network.calls) != "[REPORT RELEASE]" {
		t.Fatalf("network cleanup calls = %v", network.calls)
	}
}

func TestInitDeleteAcceptsAlreadyReapedRelayAndAbsentSandboxOnRetry(t *testing.T) {
	command := exec.Command("/bin/true")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	holderCommand := exec.Command("/bin/true")
	if err := holderCommand.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := holderCommand.Process.Wait(); err != nil {
		t.Fatal(err)
	}
	p := &process{pid: 17, status: tasktypes.Status_STOPPED, exitEventQueued: true,
		deleteEventQueued: true, exited: time.Unix(123, 0).UTC(), done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), relay: command,
		namespaceHolder: &commandNamespaceHolder{command: holderCommand},
		sandbox:         protocol.Sandbox{ID: "mk-task", Generation: strings.Repeat("a", 32)},
		processes:       map[string]*process{"": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1},
		daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
			if request.Method != "StopSandbox" && request.Method != "DeleteSandbox" {
				t.Fatalf("unexpected daemon method %s", request.Method)
			}
			return &protocol.Error{Code: "NOT_FOUND", Message: "sandbox not found"}
		})}
	if _, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ID: "task"}); err != nil {
		t.Fatalf("idempotent delete retry failed: %v", err)
	}
	if s.relay != nil || s.namespaceHolder != nil || s.processes[""] != nil {
		t.Fatalf("retry retained relay/holder/process: relay=%v holder=%v process=%v", s.relay, s.namespaceHolder, s.processes[""])
	}
}

func TestDeleteRetainsRetryOwnershipAcrossGuestAndEventFailures(t *testing.T) {
	bundle := privateTestDirectory(t)
	agentFailure := errors.New("injected guest delete failure")
	fakeAgent := &fakeAgentClient{fail: map[string]error{"DeleteProcess": agentFailure}}
	publisher := &fakePublisher{}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exitEventQueued: true, done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher, agent: fakeAgent,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	request := &taskapi.DeleteRequest{ID: "task", ExecID: "exec"}
	if _, err := s.Delete(context.Background(), request); !errors.Is(err, agentFailure) {
		t.Fatalf("guest delete error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || p.deleteEventQueued {
		t.Fatalf("guest failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	delete(fakeAgent.fail, "DeleteProcess")
	publisher.failures = 2
	if _, err := s.Delete(context.Background(), request); err == nil || !strings.Contains(err.Error(), "flush task delete event") {
		t.Fatalf("event flush error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || !p.deleteEventQueued {
		t.Fatalf("event failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	publisher.failures = 0
	response, err := s.Delete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Pid != 23 || s.processes["exec"] != nil || len(publisher.topics) != 1 || publisher.topics[0] != ctruntime.TaskDeleteEventTopic {
		t.Fatalf("retry response=%+v processes=%v topics=%v", response, s.processes, publisher.topics)
	}
}
