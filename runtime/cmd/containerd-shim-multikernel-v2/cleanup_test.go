//go:build linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestFallbackCleanupPropagatesStopFailureBeforeDelete(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	recovery := validPersistedRecovery(namespace, task)
	recovery.BundleIdentity = testBundleIdentity(t, bundle)
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	sandbox := protocol.Sandbox{ID: recovery.ID, Generation: recovery.Generation, Config: protocol.SandboxConfig{BundleIdentity: recovery.BundleIdentity}}
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "ListSandboxes" {
			encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
			if decodeErr := json.Unmarshal(encoded, output); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			return nil
		}
		if request.Method == "StopSandbox" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected stop failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected stop failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(calls, []string{"ListSandboxes", "StopSandbox"}) {
		t.Fatalf("cleanup calls after failed stop = %v", calls)
	}
}

func TestFallbackCleanupPropagatesRootfsFailureAfterSandboxDelete(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	recovery := validPersistedRecovery(namespace, task)
	recovery.BundleIdentity = testBundleIdentity(t, bundle)
	recovery.StorageSHA256 = strings.Repeat("b", 64)
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	sandbox := protocol.Sandbox{ID: recovery.ID, Generation: recovery.Generation, Config: protocol.SandboxConfig{BundleIdentity: recovery.BundleIdentity}}
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "ListSandboxes" {
			encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
			if decodeErr := json.Unmarshal(encoded, output); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			return nil
		}
		if request.Method == "CleanupRootfs" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected rootfs cleanup failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected rootfs cleanup failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	want := []string{"ListSandboxes", "StopSandbox", "DeleteSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("cleanup call order = %v, want %v", calls, want)
	}
}

func TestFallbackCleanupRequiresHeldAndDaemonBundleIdentity(t *testing.T) {
	t.Run("daemon mismatch", func(t *testing.T) {
		bundle := privateTestDirectory(t)
		runtimeDir := filepath.Join(bundle, ".multikernel")
		if err := os.Mkdir(runtimeDir, 0700); err != nil {
			t.Fatal(err)
		}
		recovery := validPersistedRecovery("default", "task-a")
		recovery.BundleIdentity = testBundleIdentity(t, bundle)
		data, err := json.Marshal(recovery)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		daemonIdentity := recovery.BundleIdentity
		daemonIdentity.Inode++
		sandbox := protocol.Sandbox{ID: recovery.ID, Generation: recovery.Generation,
			Config: protocol.SandboxConfig{BundleIdentity: daemonIdentity}}
		var calls []string
		s := &service{id: "task-a", namespace: "default", bundle: bundle,
			daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
				calls = append(calls, request.Method)
				encoded, _ := json.Marshal([]protocol.Sandbox{sandbox})
				if err := json.Unmarshal(encoded, output); err != nil {
					t.Fatal(err)
				}
				return nil
			})}
		if response, err := s.Cleanup(t.Context()); err == nil || response != nil || !strings.Contains(err.Error(), "conflicting cleanup ownership") {
			t.Fatalf("daemon mismatch cleanup response=%+v error=%v", response, err)
		}
		if !reflect.DeepEqual(calls, []string{"ListSandboxes"}) {
			t.Fatalf("daemon mismatch cleanup calls = %v", calls)
		}
	})

	t.Run("absent sandbox permits identity-bound rootfs cleanup", func(t *testing.T) {
		bundle := privateTestDirectory(t)
		runtimeDir := filepath.Join(bundle, ".multikernel")
		if err := os.Mkdir(runtimeDir, 0700); err != nil {
			t.Fatal(err)
		}
		recovery := validPersistedRecovery("default", "task-a")
		recovery.BundleIdentity = testBundleIdentity(t, bundle)
		recovery.StorageSHA256 = strings.Repeat("b", 64)
		data, err := json.Marshal(recovery)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		previous, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Chdir(bundle); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(previous)
		var calls []string
		s := &service{id: "task-a", namespace: "default", bundle: bundle,
			daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
				calls = append(calls, request.Method)
				if request.Method == "ListSandboxes" {
					encoded, _ := json.Marshal([]protocol.Sandbox{})
					if err := json.Unmarshal(encoded, output); err != nil {
						t.Fatal(err)
					}
					return nil
				}
				if request.Method == "StopSandbox" || request.Method == "DeleteSandbox" {
					return &protocol.Error{Code: "NOT_FOUND", Message: "sandbox not found"}
				}
				return nil
			})}
		if response, err := s.Cleanup(t.Context()); err != nil || response == nil {
			t.Fatalf("absent-sandbox cleanup response=%+v error=%v", response, err)
		}
		want := []string{"ListSandboxes", "StopSandbox", "DeleteSandbox", "CleanupRootfs"}
		if !reflect.DeepEqual(calls, want) {
			t.Fatalf("absent-sandbox cleanup calls=%v, want %v", calls, want)
		}
	})

	t.Run("public bundle replacement", func(t *testing.T) {
		base := privateTestDirectory(t)
		bundle := filepath.Join(base, "bundle")
		if err := os.Mkdir(bundle, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
			t.Fatal(err)
		}
		calls := 0
		s := &service{id: "task-a", namespace: "default", bundle: bundle,
			daemon: daemonCallFunc(func(context.Context, protocol.Request, any) *protocol.Error { calls++; return nil })}
		if _, err := s.ensureRuntimeDirectory(); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(bundle, bundle+"-held"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(bundle, 0700); err != nil {
			t.Fatal(err)
		}
		if response, err := s.Cleanup(t.Context()); err == nil || response != nil || !strings.Contains(err.Error(), "bundle directory identity changed") {
			t.Fatalf("bundle replacement cleanup response=%+v error=%v", response, err)
		}
		if calls != 0 {
			t.Fatalf("bundle replacement reached daemon %d times", calls)
		}
	})
}

func TestCleanupShimSocketRequiresDeterministicAddress(t *testing.T) {
	bundle := privateTestDirectory(t)
	s := &service{id: "task-a", namespace: "default", bundle: bundle}
	if err := s.ensureBundleIdentity(); err != nil {
		t.Fatal(err)
	}
	containerdAddress := "/run/containerd/containerd.sock"
	ctx := namespaces.WithNamespace(t.Context(), s.namespace)
	expected, err := shim.SocketAddress(ctx, containerdAddress, s.id)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "address"), []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}
	var captured string
	removed := false
	s.newShimSocketOwner = func(path string) (relayPathOwner, error) {
		captured = path
		return relayOwnerFunc(func() error { removed = true; return nil }), nil
	}
	if err = s.cleanupShimSocket(ctx, []string{"-address", containerdAddress}); err != nil {
		t.Fatal(err)
	}
	if captured != strings.TrimPrefix(expected, "unix://") || !removed {
		t.Fatalf("authenticated socket capture=%q removed=%v", captured, removed)
	}
	captured, removed = "", false
	if err = os.WriteFile(filepath.Join(bundle, "address"), []byte("unix:///run/containerd/s/"+strings.Repeat("f", 64)), 0644); err != nil {
		t.Fatal(err)
	}
	if err = s.cleanupShimSocket(ctx, []string{"--address=" + containerdAddress}); err == nil || !strings.Contains(err.Error(), "differs from the deterministic") {
		t.Fatalf("redirected shim address = %v", err)
	}
	if captured != "" || removed {
		t.Fatalf("redirected address reached socket owner: capture=%q removed=%v", captured, removed)
	}
}

func TestInvocationFlagValueRejectsMissingEmptyAndDuplicateAddress(t *testing.T) {
	if value, err := invocationFlagValue([]string{"-address=/run/containerd.sock"}, "address"); err != nil || value != "/run/containerd.sock" {
		t.Fatalf("valid address flag = %q, %v", value, err)
	}
	for _, arguments := range [][]string{{}, {"-address"}, {"-address="}, {"-address", "one", "--address=two"}} {
		if _, err := invocationFlagValue(arguments, "address"); err == nil {
			t.Fatalf("invalid address arguments accepted: %v", arguments)
		}
	}
}

func TestFallbackCleanupRemovesAuthenticatedSocketWithoutRecovery(t *testing.T) {
	bundle := privateTestDirectory(t)
	containerdAddress := "/run/containerd/containerd.sock"
	expected, err := shim.SocketAddress(namespaces.WithNamespace(t.Context(), "default"), containerdAddress, "task-a")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "address"), []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}
	removed := false
	s := &service{id: "task-a", namespace: "default", bundle: bundle, shimArguments: []string{"-address", containerdAddress},
		newShimSocketOwner: func(path string) (relayPathOwner, error) {
			if path != strings.TrimPrefix(expected, "unix://") {
				t.Fatalf("captured socket path = %q", path)
			}
			return relayOwnerFunc(func() error { removed = true; return nil }), nil
		}}
	response, err := s.Cleanup(t.Context())
	if err != nil || response == nil || !removed {
		t.Fatalf("cleanup without recovery response=%+v removed=%v error=%v", response, removed, err)
	}
}
