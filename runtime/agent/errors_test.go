package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestWireErrorCategoriesSurviveWrappingAndWordingChanges(t *testing.T) {
	for _, test := range []struct {
		kind errorKind
		code string
	}{
		{invalidArgument, "INVALID_ARGUMENT"}, {unsupported, "UNSUPPORTED"},
		{notFound, "NOT_FOUND"}, {alreadyExists, "ALREADY_EXISTS"},
		{failedPrecondition, "FAILED_PRECONDITION"}, {unauthenticated, "UNAUTHENTICATED"},
		{frameTooLarge, "INVALID_ARGUMENT"},
	} {
		t.Run(test.code+fmt.Sprint(test.kind), func(t *testing.T) {
			for _, diagnostic := range []string{"wording without classification keywords", "unsupported identity not found: /private/path token=secret"} {
				cause := errors.New(diagnostic)
				err := fmt.Errorf("context: %w", errors.Join(&operationError{kind: test.kind, cause: cause}, errors.New("cleanup failed")))
				if !errors.Is(err, cause) {
					t.Fatal("lost original cause")
				}
				reply := Reply{Version: 1, Sequence: 42, Body: "discard on failure"}
				reply.setError(err)
				data, marshalErr := json.Marshal(reply)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				var wire Reply
				if err := json.Unmarshal(data, &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Failure == nil || wire.Failure.Code != test.code || wire.Failure.OperationID != "agent-42" || wire.Failure.Retryable || wire.Body != nil {
					t.Fatalf("wire failure: %+v", wire)
				}
				if strings.Contains(string(data), diagnostic) || strings.Contains(string(data), "secret") {
					t.Fatalf("diagnostic leaked: %s", data)
				}
			}
		})
	}
}

func TestUntypedErrorsCannotSelectWireClassification(t *testing.T) {
	for _, raw := range []string{"unsupported method", "authentication not found", "guest process start stage exec failed: token=secret"} {
		failure := safeAgentError(1, errors.New(raw))
		if failure.Code != "INTERNAL" || failure.Message != "agent operation failed" {
			t.Fatalf("untyped text selected wire failure: %+v", failure)
		}
	}
}

func TestStageErrorsPreserveCauseAndRestrictPublicStages(t *testing.T) {
	for _, test := range []struct {
		operation string
		stages    []string
	}{
		{"process start", []string{"executable", "constraints", "terminal", "stdio", "exec"}},
		{"process create", []string{"bundle-load", "root-policy"}},
		{"network setup", []string{"tun-open", "tun-request", "tun-create", "address", "link", "route", "dns"}},
	} {
		for _, stage := range test.stages {
			cause := errorf(invalidArgument, "private validation details")
			err := &stageError{operation: test.operation, stage: stage, cause: cause}
			failure := safeAgentError(1, errors.Join(err, errors.New("cleanup secret")))
			if !errors.Is(err, cause) || failure.Code != "INTERNAL" || !strings.HasSuffix(failure.Message, "failed at "+stage) {
				t.Fatalf("stage %s: %+v", stage, failure)
			}
		}
	}
	for _, err := range []error{
		&stageError{operation: "process start", stage: "token=secret", cause: errors.New("secret")},
		&stageError{operation: "token=secret", stage: "exec", cause: errors.New("secret")},
	} {
		if failure := safeAgentError(1, err); failure.Message != "agent operation failed" || failure.Code != "INTERNAL" {
			t.Fatalf("unknown stage leaked: %+v", failure)
		}
	}
}

func TestDispatchProducesTypedWireErrors(t *testing.T) {
	for _, test := range []struct {
		name, method, body, code string
	}{
		{"unknown method", "Missing", `{}`, "UNSUPPORTED"},
		{"malformed JSON", "StateProcess", `{`, "INVALID_ARGUMENT"},
		{"unknown field", "StateProcess", `{"id":"absent","extra":true}`, "INVALID_ARGUMENT"},
		{"missing process", "StateProcess", `{"id":"absent"}`, "NOT_FOUND"},
		{"invalid process", "ExecProcess", `{"id":"!"}`, "INVALID_ARGUMENT"},
		{"missing process on start", "StartProcess", `{"ID":"absent"}`, "NOT_FOUND"},
		{"wrong state", "ResizeProcess", `{"id":"existing"}`, "FAILED_PRECONDITION"},
		{"duplicate process", "ExecProcess", `{"id":"existing","parent_id":"existing","spec":{"args":["/bin/true"],"cwd":"/"}}`, "ALREADY_EXISTS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{Manager: NewManager(false), SandboxID: "box", Generation: "generation", Token: []byte("private-token")}
			server.Manager.processes["existing"] = &process{root: t.TempDir(), state: ProcessState{ID: "existing", Status: "CREATED"}}
			envelope := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Sequence: 1, Method: test.method, Body: json.RawMessage(test.body)}
			Sign(&envelope, server.Token)
			reply := server.Dispatch(envelope)
			data, err := json.Marshal(reply)
			if err != nil {
				t.Fatal(err)
			}
			var wire Reply
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Failure == nil || wire.Failure.Code != test.code {
				t.Fatalf("failure=%+v local=%s", wire.Failure, reply.Error)
			}
		})
	}
}

func TestSignalReplayPreservesTypedFailureAndCause(t *testing.T) {
	manager := NewManager(true)
	manager.processes["signal"] = &process{cmd: &exec.Cmd{Process: &os.Process{Pid: 41}}, state: ProcessState{ID: "signal", Status: "RUNNING"}}
	cause := errors.New("private signal failure")
	failure := &operationError{kind: failedPrecondition, cause: cause}
	calls := 0
	manager.signalProcess = func(int, syscall.Signal) error { calls++; return failure }
	for range 2 {
		err := manager.SignalOnce("signal", syscall.SIGTERM, strings.Repeat("a", 32))
		if !errors.Is(err, cause) || safeAgentError(1, err).Code != "FAILED_PRECONDITION" {
			t.Fatalf("signal replay lost classification or cause: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("signal calls=%d, want 1", calls)
	}
}
