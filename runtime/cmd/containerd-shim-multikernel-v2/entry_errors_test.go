//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/runtime/v2/shim"
	"google.golang.org/protobuf/proto"
)

const entryPrivateCanary = "SYNTHETIC_ENTRY_PRIVATE /private/credential"

type entryUnprintableError struct{}

func (entryUnprintableError) Error() string { panic("entry cause formatted") }

func TestProtectedShimEntryFailureDoesNotRetainOrFormatCause(t *testing.T) {
	for _, operation := range []string{"initialize", "start", "cleanup", "nil-initializer-result"} {
		t.Run(operation, func(t *testing.T) {
			backend := &taskRPCBackend{failure: entryUnprintableError{}, calls: map[string]int{}}
			calls := 0
			initialize := protectShimEntry(func(context.Context, string, shim.Publisher, func()) (shim.Shim, error) {
				calls++
				if operation == "initialize" {
					return backend, entryUnprintableError{}
				}
				if operation == "nil-initializer-result" {
					return nil, nil
				}
				return backend, nil
			})
			entry, issue := initialize(t.Context(), "task", nil, func() {})
			want := "shim initialization failed"
			if operation == "start" {
				address, err := entry.StartShim(t.Context(), shim.StartOpts{})
				issue = err
				want = "shim manager startup failed"
				if address != "" {
					t.Fatal("failed entry returned an address")
				}
			}
			if operation == "cleanup" {
				result, err := entry.Cleanup(t.Context())
				issue = err
				want = "shim manager cleanup failed"
				if result != nil {
					t.Fatal("failed entry returned cleanup data")
				}
			}
			if calls != 1 || issue == nil || issue.Error() != want || errors.Unwrap(issue) != nil {
				t.Fatal("entry retained or formatted private cause, retried, or changed classification")
			}
		})
	}
}

func TestProtectedShimEntrySuccessAndTaskDelegationRemainExact(t *testing.T) {
	response := &taskapi.StateResponse{ID: "task", Pid: 42}
	backend := &taskRPCBackend{calls: map[string]int{}, results: map[string]proto.Message{"State": response}}
	ctx := context.WithValue(t.Context(), struct{}{}, "synthetic context")
	var observedID string
	entry, err := protectShimEntry(func(observed context.Context, id string, p shim.Publisher, shutdown func()) (shim.Shim, error) {
		if observed != ctx || p != nil || shutdown == nil {
			t.Fatal("initializer arguments changed")
		}
		observedID = id
		return newTaskRPCShim(backend), nil
	})(ctx, "task", nil, func() {})
	if err != nil || observedID != "task" {
		t.Fatal("entry initialization changed")
	}
	opts := shim.StartOpts{ID: "task", Address: "synthetic-address", Debug: true}
	address, err := entry.StartShim(ctx, opts)
	if err != nil || address != "manager-address" || backend.requests["ManagerStartShim"] != opts || backend.contexts["ManagerStartShim"] != ctx {
		t.Fatal("successful manager start changed")
	}
	deleted, err := entry.Cleanup(ctx)
	if err != nil || deleted == nil || deleted.Pid != 42 || backend.contexts["ManagerCleanup"] != ctx {
		t.Fatal("successful manager cleanup changed")
	}
	request := &taskapi.StateRequest{ID: "task"}
	state, err := entry.State(ctx, request)
	if err != nil || state != response || backend.requests["State"] != request || backend.contexts["State"] != ctx {
		t.Fatal("Task delegation changed")
	}
	for _, name := range []string{"State", "ManagerStartShim", "ManagerCleanup"} {
		if backend.calls[name] != 1 {
			t.Fatal("entry retried delegate")
		}
	}
}

// Run the pinned upstream terminal formatter in an isolated child process.
// No real service is initialized, listener opened, or backend invoked.
func TestProtectedShimEntryRunnerHelper(t *testing.T) {
	mode := os.Getenv("MK_ENTRY_RUNNER_CASE")
	if mode == "" {
		return
	}
	operation := strings.TrimPrefix(mode, "protected-")
	backend := &taskRPCBackend{calls: map[string]int{}}
	if operation == "start" || operation == "cleanup" {
		backend.failure = errors.New(entryPrivateCanary)
	}
	initialize := shim.Init(func(context.Context, string, shim.Publisher, func()) (shim.Shim, error) {
		if operation == "initialize" {
			return nil, errors.New(entryPrivateCanary)
		}
		return newTaskRPCShim(backend), nil
	})
	if strings.HasPrefix(mode, "protected-") {
		initialize = protectShimEntry(initialize)
	}
	action := "start"
	if operation == "cleanup" || operation == "delete-success" {
		action = "delete"
	}
	os.Args = []string{"shim-entry-fixture", "-namespace=moby", "-id=synthetic-task", action}
	shim.Run(runtimeName, initialize, func(config *shim.Config) {
		config.NoSubreaper = true
		config.NoReaper = true
		config.NoSetupLogger = true
	})
	os.Exit(0)
}

func TestProtectedShimEntryUpstreamTerminalOracle(t *testing.T) {
	for _, operation := range []string{"initialize", "start", "cleanup", "start-success", "delete-success"} {
		for _, protected := range []bool{false, true} {
			mode := operation
			if protected {
				mode = "protected-" + mode
			}
			t.Run(mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProtectedShimEntryRunnerHelper$")
				command.Dir = t.TempDir()
				command.Env = []string{"PATH=/usr/bin:/bin", "MK_ENTRY_RUNNER_CASE=" + mode}
				var stdout, stderr bytes.Buffer
				command.Stdout = &stdout
				command.Stderr = &stderr
				issue := command.Run()
				if ctx.Err() != nil {
					t.Fatal("upstream runner exceeded fixture budget")
				}
				if strings.HasSuffix(operation, "success") {
					if issue != nil || stderr.Len() != 0 {
						t.Fatal("successful runner changed")
					}
					if operation == "start-success" && stdout.String() != "manager-address" {
						t.Fatal("successful start address changed")
					}
					if operation == "delete-success" {
						var result taskapi.DeleteResponse
						if proto.Unmarshal(stdout.Bytes(), &result) != nil || result.Pid != 42 {
							t.Fatal("successful delete protobuf changed")
						}
					}
					return
				}
				var exit *exec.ExitError
				if !errors.As(issue, &exit) || exit.ExitCode() != 1 || stdout.Len() != 0 {
					t.Fatal("terminal failure status/output changed")
				}
				want := runtimeName + ": " + entryPrivateCanary
				if protected {
					messages := map[string]string{"initialize": "shim initialization failed", "start": "shim manager startup failed", "cleanup": "shim manager cleanup failed"}
					want = runtimeName + ": " + messages[operation]
				}
				if stderr.String() != want {
					t.Fatal("upstream terminal boundary differs")
				}
			})
		}
	}
}
