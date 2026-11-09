//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/errdefs"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Exercise the public Create entry after actual rollback refusals, rather than
// setting an internal admission flag. The backend sentinel stops a newly
// admitted request before allocation, so the old behavior is safe to reproduce.
func TestCreateRefusesAdmissionUntilRollbackCompletes(t *testing.T) {
	for _, stage := range []string{"holder-stop", "network-release", "sandbox-delete", "cancel-create", "rootfs-cleanup", "pre-sandbox-rootfs"} {
		t.Run(stage, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			config := `{"ociVersion":"1.1.0","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("synthetic cleanup refusal")
			calls := []string{}
			failing := true
			s := &service{id: "task", namespace: "default", bundle: bundle, processes: map[string]*process{}, token: []byte("private"),
				sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, netEndpoint: validShimEndpoint("task", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/test")}
			t.Cleanup(func() {
				if s.bundleDirectory != nil {
					_ = s.bundleDirectory.Close()
				}
			})
			holder := &namespaceDiagnosticHolderFixture{}
			if stage == "holder-stop" {
				holder.stop = injected
				s.namespaceHolder = holder
			}
			s.daemon = daemonCallFunc(func(_ context.Context, r protocol.Request, out any) *protocol.Error {
				calls = append(calls, r.Method)
				if r.Method == "ValidateRootfs" {
					return &protocol.Error{Code: "VALIDATION_SENTINEL", Message: "new Create reached validation"}
				}
				if failing && (stage == "sandbox-delete" && r.Method == "DeleteSandbox" || stage == "cancel-create" && r.Method == "CancelCreateSandbox" || (stage == "rootfs-cleanup" || stage == "pre-sandbox-rootfs") && r.Method == "CleanupRootfs") {
					return &protocol.Error{Code: "SYNTHETIC_CLEANUP_REFUSAL", Message: "cleanup refused"}
				}
				if r.Method == "CancelCreateSandbox" {
					if err := setJSONResponse(out, map[string]bool{"safe_to_cleanup": true}); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			})
			s.netClient = retentionNetworkCaller(func(_ context.Context, r mknetwork.Request) (mknetwork.Response, error) {
				calls = append(calls, r.Method)
				if failing && stage == "network-release" {
					return mknetwork.Response{}, injected
				}
				return mknetwork.Response{}, nil
			})
			if stage == "pre-sandbox-rootfs" {
				s.sandbox = protocol.Sandbox{}
				s.netEndpoint = mknetwork.Endpoint{}
				s.token = nil
			}
			pending := stage == "cancel-create"
			cleanup := &rootfspkg.CleanupRequest{}
			if err := s.rollbackCreate(t.Context(), cleanup, pending, protocol.SandboxConfig{}, "key"); err == nil {
				t.Fatal("rollback refusal accepted")
			}
			// No public task record remains in any case: it cannot serve as the guard.
			if len(s.processes) != 0 {
				t.Fatal("fixture unexpectedly has a task record")
			}
			before := append([]string(nil), calls...)
			endpoint, sandbox, token, held := s.netEndpoint, s.sandbox, append([]byte(nil), s.token...), s.namespaceHolder
			for range 2 {
				result, err := s.Create(t.Context(), &taskapi.CreateTaskRequest{ID: "task", Bundle: bundle})
				if result != nil || !errors.Is(err, errdefs.ErrFailedPrecondition) || !reflect.DeepEqual(calls, before) {
					t.Fatal("incomplete rollback allowed new Create setup")
				}
				if !reflect.DeepEqual(s.netEndpoint, endpoint) || !reflect.DeepEqual(s.sandbox, sandbox) || !reflect.DeepEqual(s.token, token) || s.namespaceHolder != held {
					t.Fatal("refused Create changed retained ownership")
				}
			}
			failing = false
			holder.stop = nil
			if err := s.rollbackCreate(t.Context(), cleanup, pending, protocol.SandboxConfig{}, "key"); err != nil {
				t.Fatal("explicit rollback retry failed")
			}
			before = append([]string(nil), calls...)
			result, err := s.Create(t.Context(), &taskapi.CreateTaskRequest{ID: "task", Bundle: bundle})
			if result != nil || err == nil || !strings.Contains(err.Error(), "VALIDATION_SENTINEL") || !reflect.DeepEqual(calls, append(before, "ValidateRootfs")) {
				t.Fatal("completed rollback did not reopen ordinary validation")
			}
		})
	}
}
