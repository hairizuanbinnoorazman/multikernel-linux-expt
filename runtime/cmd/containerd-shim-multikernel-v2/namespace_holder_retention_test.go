//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type retentionNetworkCaller func(context.Context, mknetwork.Request) (mknetwork.Response, error)

func (f retentionNetworkCaller) Call(ctx context.Context, request mknetwork.Request) (mknetwork.Response, error) {
	return f(ctx, request)
}
func (retentionNetworkCaller) Attach(context.Context, mknetwork.Request) (mknetwork.Response, *os.File, error) {
	return mknetwork.Response{}, nil, errors.New("unexpected attach")
}

func TestNamespaceHolderRejectedOwnershipSurvivesCleanupFailure(t *testing.T) {
	for _, source := range []string{"factory", "validation"} {
		t.Run(source, func(t *testing.T) {
			rejected := errors.New("private startup failure")
			stopFailed := errors.New("private stop failure")
			holder := &namespaceDiagnosticHolderFixture{validation: rejected, stop: stopFailed}
			replacement := &namespaceDiagnosticHolderFixture{}
			sink := newProcessDiagnosticSink(nil)
			defer sink.close()
			calls := 0
			s := &service{netEndpoint: validShimEndpoint("container", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/private"), processDiagnosticSink: sink,
				newNamespaceHolder: func(string) (namespaceHolder, error) {
					calls++
					if calls > 1 {
						return replacement, nil
					}
					if source == "factory" {
						return holder, errors.Join(rejected, stopFailed)
					}
					return holder, nil
				}}
			err := s.ensureNamespaceHolder()
			if !errors.Is(err, rejected) || !errors.Is(err, stopFailed) || s.namespaceHolder == nil {
				t.Fatal("rejected helper or cleanup cause was dropped")
			}
			if s.namespaceHolder.PID() != 0 || s.taskPID("", &process{pid: 17}) != 0 {
				t.Fatal("rejected helper became a task representative")
			}
			holder.validation = nil // Namespace-only validation cannot rehabilitate startup.
			if s.ensureNamespaceHolder() == nil || calls != 1 || s.namespaceHolder == nil {
				t.Fatal("rejected helper was reused or replacement launched before stop")
			}
			holder.stop = nil
			if err = s.stopNamespaceHolder(); err != nil || s.namespaceHolder != nil {
				t.Fatal("explicit retry did not release retained helper")
			}
			if err = s.stopNamespaceHolder(); err != nil {
				t.Fatal("completed cleanup was not idempotent")
			}
			if err = s.ensureNamespaceHolder(); err != nil || s.namespaceHolder != replacement || calls != 2 {
				t.Fatal("replacement did not start after cleanup")
			}
		})
	}
}

func TestNamespaceHolderRollbackPreservesOwnershipUntilStop(t *testing.T) {
	for _, lifecycleAttempted := range []bool{false, true} {
		t.Run(map[bool]string{false: "allocated", true: "pending-create"}[lifecycleAttempted], func(t *testing.T) {
			stopFailed := errors.New("stop unavailable")
			holder := &namespaceDiagnosticHolderFixture{stop: stopFailed}
			endpoint := validShimEndpoint("container", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/private")
			p := &process{pid: 17}
			calls := []string{}
			s := &service{namespaceHolder: holder, netEndpoint: endpoint, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, token: []byte("private"), processes: map[string]*process{"": p},
				daemon: daemonCallFunc(func(_ context.Context, r protocol.Request, response any) *protocol.Error {
					calls = append(calls, r.Method)
					if r.Method == "CancelCreateSandbox" {
						if err := setJSONResponse(response, map[string]bool{"safe_to_cleanup": true}); err != nil {
							t.Fatal(err)
						}
					}
					return nil
				}),
				netClient: retentionNetworkCaller(func(_ context.Context, r mknetwork.Request) (mknetwork.Response, error) {
					calls = append(calls, r.Method)
					return mknetwork.Response{}, nil
				})}
			request := &rootfspkg.CleanupRequest{}
			err := s.rollbackCreate(t.Context(), request, lifecycleAttempted, protocol.SandboxConfig{}, "key")
			if !errors.Is(err, stopFailed) || len(calls) != 0 || s.namespaceHolder != holder || s.processes[""] != p || s.sandbox.ID != "box" || !reflect.DeepEqual(s.netEndpoint, endpoint) || string(s.token) != "private" {
				t.Fatal("failed helper stop discarded ownership or crossed a cleanup barrier")
			}
			holder.stop = nil
			if err = s.rollbackCreate(t.Context(), request, lifecycleAttempted, protocol.SandboxConfig{}, "key"); err != nil {
				t.Fatalf("cleanup retry failed: %v", err)
			}
			method := "DeleteSandbox"
			if lifecycleAttempted {
				method = "CancelCreateSandbox"
			}
			if !reflect.DeepEqual(calls, []string{"RELEASE", method, "CleanupRootfs"}) || s.namespaceHolder != nil || s.sandbox.ID != "" || s.processes[""] != nil || s.token != nil {
				t.Fatal("cleanup retry did not finish in ownership order")
			}
		})
	}
}

type retainedCommandHolderFixture struct {
	delegate         *commandNamespaceHolder
	validation, stop error
}

func (h *retainedCommandHolderFixture) PID() uint32           { return h.delegate.PID() }
func (h *retainedCommandHolderFixture) Validate(string) error { return h.validation }
func (h *retainedCommandHolderFixture) Stop() error {
	if h.stop != nil {
		return h.stop
	}
	return h.delegate.Stop()
}

func TestNamespaceHolderRejectedRealProcessRetainedUntilRetry(t *testing.T) {
	command := exec.Command("/bin/sleep", "30")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		if command.ProcessState == nil {
			_ = command.Wait()
		}
	})
	fd, err := unix.PidfdOpen(command.Process.Pid, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	holder := &retainedCommandHolderFixture{delegate: &commandNamespaceHolder{command: command}, validation: errors.New("private validation failure"), stop: errors.New("stop refused")}
	sink := newProcessDiagnosticSink(nil)
	defer sink.close()
	calls := 0
	s := &service{netEndpoint: validShimEndpoint("container", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/private"), processDiagnosticSink: sink, newNamespaceHolder: func(string) (namespaceHolder, error) { calls++; return holder, nil }}
	if s.ensureNamespaceHolder() == nil || s.namespaceHolder == nil || s.namespaceHolder.PID() != 0 {
		t.Fatal("real rejected process lost cleanup ownership")
	}
	holder.validation = nil
	if s.ensureNamespaceHolder() == nil || calls != 1 {
		t.Fatal("live rejected process was reused")
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if n, err := unix.Poll(poll, 0); err != nil || n != 0 {
		t.Fatal("refused stop killed its retained process")
	}
	holder.stop = nil
	if err = s.stopNamespaceHolder(); err != nil || s.namespaceHolder != nil || command.ProcessState == nil {
		t.Fatal("retry did not stop and reap the exact owned process")
	}
	if n, err := unix.Poll(poll, 0); err != nil || n != 1 {
		t.Fatal("successful stop left the owned process live")
	}
}
