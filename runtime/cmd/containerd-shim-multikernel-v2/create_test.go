//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCancelCreateUsesExactOriginalIdentity(t *testing.T) {
	wantConfig := protocol.SandboxConfig{SchemaVersion: 1, ID: "box-a", CPUs: []int{8}, MemoryBytes: 1 << 30,
		KernelManifest: "test", Bundle: "/bundle/box-a", AgentPort: 7001, ChildCID: 3}
	wantKey := "shim-create-box-a-0123456789ab"
	var callErr error
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, response any) *protocol.Error {
		var observed protocol.SandboxConfig
		if decodeErr := protocol.StrictDecode(request.Body, &observed); decodeErr != nil {
			callErr = decodeErr
			return nil
		}
		if request.Method != "CancelCreateSandbox" || request.IdempotencyKey != wantKey || !reflect.DeepEqual(observed, wantConfig) {
			callErr = fmt.Errorf("unexpected cancellation request: %+v %+v", request, observed)
			return nil
		}
		encoded, _ := json.Marshal(map[string]bool{"safe_to_cleanup": true})
		if decodeErr := json.Unmarshal(encoded, response); decodeErr != nil {
			callErr = decodeErr
		}
		return nil
	})
	service := &service{daemon: caller}
	if err := service.cancelCreate(context.Background(), wantConfig, wantKey); err != nil {
		t.Fatal(err)
	}
	if callErr != nil {
		t.Fatal(callErr)
	}
}

func TestCreateAmbiguityCancellationRemovesPreparedArtifacts(t *testing.T) {
	bundle := privateTestDirectory(t)
	configJSON := `{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	lockDirectory := t.TempDir()
	if err := os.Chmod(lockDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(lockDirectory, "shim.lock")
	t.Setenv("MK_SHIM_LOCK", lock)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	storageHash := strings.Repeat("a", 64)
	var calls []string
	respond := func(output any, value any) *protocol.Error {
		if output == nil {
			return nil
		}
		data, _ := json.Marshal(value)
		if err := json.Unmarshal(data, output); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
		calls = append(calls, request.Method)
		switch request.Method {
		case "ValidateRootfs":
			return respond(output, map[string]bool{"validated": true})
		case "ListSandboxes":
			return respond(output, []protocol.Sandbox{})
		case "PrepareRootfs":
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			return respond(output, rootfspkg.PrepareResult{RuntimeIdentity: testBundleIdentity(t, runtimeDir), Storage: protocol.StorageConfig{
				Path: "/srv/storage/root.ext4", ImageID: "image", FilesystemUUID: "12345678-1234-4234-8234-123456789abc",
				SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: storageHash,
			}})
		case "CreateSandbox":
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "ambiguous create", Retryable: true}
		case "CancelCreateSandbox":
			if !strings.HasPrefix(request.IdempotencyKey, "shim-create-task-a-") {
				t.Fatalf("unexpected create cancellation key %q", request.IdempotencyKey)
			}
			return respond(output, map[string]bool{"safe_to_cleanup": true})
		case "CleanupRootfs":
			var cleanup rootfspkg.CleanupRequest
			if err := protocol.StrictDecode(request.Body, &cleanup); err != nil || cleanup.Bundle != bundle || cleanup.StorageSHA256 != storageHash {
				t.Fatalf("cleanup request = %+v, %v", cleanup, err)
			}
			if err := os.RemoveAll(runtimeDir); err != nil {
				t.Fatal(err)
			}
			return respond(output, map[string]bool{"cleaned": true})
		default:
			t.Fatalf("unexpected daemon method %q", request.Method)
			return nil
		}
	})
	service := &service{id: "task-a", namespace: "default", bundle: bundle, daemon: caller, processes: map[string]*process{}}
	_, err := service.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task-a", Bundle: bundle})
	if err == nil || !strings.Contains(err.Error(), "ambiguous create") {
		t.Fatalf("Create() error = %v", err)
	}
	wantCalls := []string{"ValidateRootfs", "ListSandboxes", "PrepareRootfs", "CreateSandbox", "CancelCreateSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("daemon calls = %v, want %v", calls, wantCalls)
	}
	if _, statErr := os.Lstat(runtimeDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime artifacts survived cancellation: %v", statErr)
	}
	if len(service.token) != 0 || len(service.processes) != 0 || service.sandbox.ID != "" {
		t.Fatalf("shim state survived cancellation: token=%d processes=%d sandbox=%+v", len(service.token), len(service.processes), service.sandbox)
	}
}

func TestCreateRejectsOCIValidationBeforeAllocationOrArtifacts(t *testing.T) {
	bundle := privateTestDirectory(t)
	config := `{"ociVersion":"1.1.0","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var calls []string
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method != "ValidateRootfs" {
			t.Fatalf("validation failure reached daemon method %q", request.Method)
		}
		var validate rootfspkg.ValidateRequest
		if err := protocol.StrictDecode(request.Body, &validate); err != nil || validate.Bundle != bundle || validate.Version != rootfspkg.Version {
			t.Fatalf("validation request=%+v error=%v", validate, err)
		}
		return &protocol.Error{Code: "FAILED_PRECONDITION", Message: "unsupported OCI configuration"}
	})
	s := &service{id: "task", namespace: "default", bundle: bundle, daemon: caller, processes: map[string]*process{}}
	if _, err := s.Create(t.Context(), &taskapi.CreateTaskRequest{ID: "task", Bundle: bundle}); err == nil ||
		!strings.Contains(err.Error(), "unsupported OCI configuration") {
		t.Fatalf("Create error = %v", err)
	}
	if fmt.Sprint(calls) != "[ValidateRootfs]" || len(s.processes) != 0 || s.sandbox.ID != "" || len(s.token) != 0 {
		t.Fatalf("rejected validation calls=%v processes=%v sandbox=%+v token=%d", calls, s.processes, s.sandbox, len(s.token))
	}
	for _, name := range []string{".multikernel", "rootfs"} {
		if _, err := os.Lstat(filepath.Join(bundle, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected validation created %s: %v", name, err)
		}
	}
}

func TestCreatePostValidationFailureRollbackMatrix(t *testing.T) {
	stages := []string{
		"prepare-rootfs", "runtime-handoff", "token", "create-sandbox",
		"load-sandbox", "provision-network", "namespace-holder",
		"persist-recovery", "persist-create-event",
	}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			configJSON := `{"ociVersion":"1.1.0","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(configJSON), 0600); err != nil {
				t.Fatal(err)
			}
			lockDirectory := t.TempDir()
			if err := os.Chmod(lockDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MK_SHIM_LOCK", filepath.Join(lockDirectory, "shim.lock"))
			runtimeDir := filepath.Join(bundle, ".multikernel")
			storageHash := strings.Repeat("a", 64)
			sandboxGeneration := strings.Repeat("b", 32)
			networkGeneration := strings.Repeat("c", 32)
			task, namespace := "task-a", "default"
			sandboxIdentifier := sandboxID(namespace, task)
			var calls []string
			cleanupCalls, deleteCalls, cancelCalls := 0, 0, 0
			respond := func(output any, value any) *protocol.Error {
				if output == nil {
					return nil
				}
				data, _ := json.Marshal(value)
				if err := json.Unmarshal(data, output); err != nil {
					t.Fatal(err)
				}
				return nil
			}
			caller := daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
				calls = append(calls, request.Method)
				switch request.Method {
				case "ValidateRootfs":
					return nil
				case "ListSandboxes":
					return respond(output, []protocol.Sandbox{})
				case "PrepareRootfs":
					if stage == "prepare-rootfs" {
						return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected prepare-rootfs failure"}
					}
					if err := os.Mkdir(runtimeDir, 0700); err != nil {
						t.Fatal(err)
					}
					if stage == "token" {
						if err := os.WriteFile(filepath.Join(runtimeDir, "token"), []byte("unsafe"), 0644); err != nil {
							t.Fatal(err)
						}
					}
					identity := testBundleIdentity(t, runtimeDir)
					if stage == "runtime-handoff" {
						identity.Inode++
					}
					return respond(output, rootfspkg.PrepareResult{RuntimeIdentity: identity, Storage: protocol.StorageConfig{
						Path: "/srv/storage/root.ext4", ImageID: "image", FilesystemUUID: "12345678-1234-4234-8234-123456789abc",
						SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: storageHash,
					}})
				case "CreateSandbox":
					if stage == "create-sandbox" {
						return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected create-sandbox failure", Retryable: true}
					}
					sandbox := protocol.Sandbox{ID: sandboxIdentifier, Generation: sandboxGeneration, State: "CREATED",
						Config: protocol.SandboxConfig{BundleIdentity: testBundleIdentity(t, bundle)}}
					return respond(output, protocol.MutationResult{Sandbox: sandbox})
				case "CancelCreateSandbox":
					cancelCalls++
					return respond(output, map[string]bool{"safe_to_cleanup": true})
				case "LoadSandbox":
					if stage == "load-sandbox" {
						return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected load-sandbox failure"}
					}
					return respond(output, protocol.MutationResult{})
				case "DeleteSandbox":
					deleteCalls++
					return respond(output, protocol.MutationResult{})
				case "CleanupRootfs":
					cleanupCalls++
					var cleanup rootfspkg.CleanupRequest
					if err := protocol.StrictDecode(request.Body, &cleanup); err != nil || cleanup.StorageSHA256 != storageHash {
						t.Fatalf("cleanup request = %+v, %v", cleanup, err)
					}
					if err := os.RemoveAll(runtimeDir); err != nil {
						t.Fatal(err)
					}
					return nil
				default:
					t.Fatalf("unexpected daemon method %q", request.Method)
					return nil
				}
			})
			network := &fakeNetworkClient{endpoint: validShimEndpoint(task, sandboxIdentifier, sandboxGeneration,
				networkGeneration, "/run/netns/test"), fail: map[string]error{}}
			if stage == "provision-network" {
				network.fail["PROVISION"] = errors.New("injected provision-network failure")
			}
			holder := &fakeNamespaceHolder{pid: 73}
			oldJSONMarshal := jsonMarshal
			if stage == "persist-create-event" {
				jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected persist-create-event failure") }
			}
			defer func() { jsonMarshal = oldJSONMarshal }()
			service := &service{id: task, namespace: namespace, bundle: bundle, daemon: caller, netClient: network,
				publisher: &fakePublisher{}, processes: map[string]*process{},
				events: eventJournal{SchemaVersion: 1, NextSequence: 1},
				newNamespaceHolder: func(string) (namespaceHolder, error) {
					if stage == "namespace-holder" {
						return nil, errors.New("injected namespace-holder failure")
					}
					if stage == "persist-recovery" {
						if err := os.Chmod(runtimeDir, 0750); err != nil {
							t.Fatal(err)
						}
					}
					return holder, nil
				},
			}
			_, err := service.Create(t.Context(), &taskapi.CreateTaskRequest{ID: task, Bundle: bundle})
			if err == nil {
				t.Fatal("injected post-validation failure was accepted")
			}
			if len(service.processes) != 0 || service.sandbox.ID != "" || len(service.token) != 0 || service.netEndpoint.SandboxID != "" || service.namespaceHolder != nil {
				t.Fatalf("rollback retained state: processes=%v sandbox=%+v token=%d endpoint=%+v holder=%v",
					service.processes, service.sandbox, len(service.token), service.netEndpoint, service.namespaceHolder)
			}
			if _, statErr := os.Lstat(runtimeDir); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("runtime artifacts survived %s: %v; calls=%v", stage, statErr, calls)
			}
			prepared := stage != "prepare-rootfs"
			expectedCleanup := 0
			if prepared {
				expectedCleanup = 1
			}
			if cleanupCalls != expectedCleanup {
				t.Fatalf("cleanup calls after %s = %d, want %d; daemon calls=%v", stage, cleanupCalls, expectedCleanup, calls)
			}
			if deleteCalls != 0 {
				t.Fatalf("delete calls after %s = %d, want 0; daemon calls=%v", stage, deleteCalls, calls)
			}
			expectedCancel := 0
			if slices.Contains(stages[3:], stage) {
				expectedCancel = 1
			}
			if cancelCalls != expectedCancel {
				t.Fatalf("cancel calls after %s = %d", stage, cancelCalls)
			}
			endpointOwned := slices.Contains(stages[6:], stage)
			if slices.Contains(network.calls, "RELEASE") != endpointOwned {
				t.Fatalf("network calls after %s = %v, expected release=%v", stage, network.calls, endpointOwned)
			}
			if holder.stopped != slices.Contains(stages[7:], stage) {
				t.Fatalf("holder stopped after %s = %v", stage, holder.stopped)
			}
		})
	}
}

func TestAllocationLockUsesParentIdentityAndHonorsCancellation(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "shim.lock")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	first, err := openAllocationLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if second, lockErr := openAllocationLock(ctx, path); !errors.Is(lockErr, context.Canceled) || second != nil {
		t.Fatalf("contended cancelled lock = %v, %v", second, lockErr)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancelled allocation lock was not bounded")
	}
	if value, readErr := os.ReadFile(target); readErr != nil || string(value) != "untouched" {
		t.Fatalf("configured lock symlink target = %q, %v", value, readErr)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := openAllocationLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err = third.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(directory, 0770); err != nil {
		t.Fatal(err)
	}
	if lock, lockErr := openAllocationLock(context.Background(), path); lockErr == nil || lock != nil {
		t.Fatalf("group-writable lock parent accepted: %v, %v", lock, lockErr)
	}
}

func TestRootfsMountsAreSanitizedForDaemonAndValidated(t *testing.T) {
	snapshot := t.TempDir()
	mounts, err := rootfsMounts([]*types.Mount{{
		Type: "overlay", Source: "overlay", Options: []string{"rw", "lowerdir=" + snapshot, "nodev"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	options := map[string]bool{}
	for _, option := range mounts[0].Options {
		options[option] = true
	}
	if options["rw"] || !options["lowerdir="+snapshot] {
		t.Fatalf("sanitized mount options = %v", mounts[0].Options)
	}
	for _, input := range []*types.Mount{
		nil,
		{Type: "tmpfs", Source: "tmpfs"},
		{Type: "bind", Source: "relative"},
		{Type: "overlay", Source: "overlay", Options: []string{"ro\nmalicious"}},
		{Type: "overlay", Source: "overlay", Options: []string{"nodev", "nodev"}},
		{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=relative"}},
	} {
		if _, err = rootfsMounts([]*types.Mount{input}); err == nil {
			t.Fatalf("unsafe mount accepted: %+v", input)
		}
	}
	tooMany := make([]*types.Mount, 9)
	for index := range tooMany {
		tooMany[index] = &types.Mount{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=" + snapshot}}
	}
	if _, err = rootfsMounts(tooMany); err == nil {
		t.Fatal("too many rootfs mounts accepted")
	}
}
