//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCreateIntentTracksActualCreateCompletion(t *testing.T) {
	for _, stage := range []string{"success", "publication-failed", "rollback-refused", "commit-marker-changed"} {
		t.Run(stage, func(t *testing.T) {
			s := intentTestOwner(t)
			bundle := s.bundle
			config := `{"ociVersion":"1.1.0","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("MK_SHIM_LOCK", filepath.Join(privateTestDirectory(t), "lock"))
			runtimeDir := filepath.Join(bundle, ".multikernel")
			hash := strings.Repeat("c", 64)
			generation := strings.Repeat("a", 32)
			held := &fakeNamespaceHolder{pid: 73}
			s.newNamespaceHolder = func(string) (namespaceHolder, error) { return held, nil }
			identifier := sandboxID(s.namespace, s.id)
			s.netClient = &fakeNetworkClient{endpoint: validShimEndpoint(s.id, identifier, generation, strings.Repeat("b", 32), "/run/netns/test"), fail: map[string]error{}}
			publisher := &fakePublisher{}
			s.publisher = publisher
			if stage == "publication-failed" {
				old := jsonMarshal
				jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("synthetic event persistence failure") }
				defer func() { jsonMarshal = old }()
			}
			if stage == "commit-marker-changed" {
				publisher.hook = func() {
					path := filepath.Join(bundle, createIntentName)
					if err := os.Rename(path, path+".held"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("foreign"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			destructive := 0
			refuse := true
			var original protocol.SandboxConfig
			var key string
			s.daemon = daemonCallFunc(func(_ context.Context, r protocol.Request, out any) *protocol.Error {
				if r.Method == "PrepareRootfs" || r.Method == "CreateSandbox" {
					if _, err := os.Stat(filepath.Join(bundle, createIntentName)); err != nil {
						t.Fatal("allocation preceded durable intent")
					}
				}
				switch r.Method {
				case "ValidateRootfs":
					return nil
				case "ListSandboxes":
					if err := setJSONResponse(out, []protocol.Sandbox{}); err != nil {
						t.Fatal(err)
					}
				case "PrepareRootfs":
					if err := os.Mkdir(runtimeDir, 0700); err != nil {
						t.Fatal(err)
					}
					result := rootfspkg.PrepareResult{RuntimeIdentity: testBundleIdentity(t, runtimeDir), Storage: protocol.StorageConfig{Path: "/private/root.ext4", ImageID: "image", FilesystemUUID: "12345678-1234-4234-8234-123456789abc", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: hash}}
					if err := setJSONResponse(out, result); err != nil {
						t.Fatal(err)
					}
				case "CreateSandbox":
					if protocol.StrictDecode(r.Body, &original) != nil {
						t.Fatal("invalid create config")
					}
					key = r.IdempotencyKey
					if err := setJSONResponse(out, protocol.MutationResult{Sandbox: protocol.Sandbox{ID: identifier, Generation: generation, State: "CREATED", Config: original}}); err != nil {
						t.Fatal(err)
					}
				case "LoadSandbox":
					if stage == "rollback-refused" {
						return &protocol.Error{Code: "SYNTHETIC_LOAD_REFUSAL"}
					}
				case "CancelCreateSandbox":
					destructive++
					if stage == "rollback-refused" && refuse {
						return &protocol.Error{Code: "SYNTHETIC_CANCEL_REFUSAL"}
					}
					if err := setJSONResponse(out, map[string]bool{"safe_to_cleanup": true}); err != nil {
						t.Fatal(err)
					}
				case "CleanupRootfs":
					destructive++
					if err := os.RemoveAll(runtimeDir); err != nil {
						t.Fatal(err)
					}
				default:
					t.Fatal("unexpected backend operation")
				}
				return nil
			})
			result, err := s.Create(t.Context(), &taskapi.CreateTaskRequest{ID: s.id, Bundle: bundle})
			marker := filepath.Join(bundle, createIntentName)
			switch stage {
			case "success":
				if err != nil || result == nil || result.Pid != 73 || destructive != 0 {
					t.Fatal("successful Create changed")
				}
				if _, err = os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("successful Create retained marker")
				}
			case "publication-failed":
				if err == nil || result != nil || destructive != 2 || s.sandbox.ID != "" || len(s.processes) != 0 {
					t.Fatal("publication rollback did not complete")
				}
				if _, err = os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("completed rollback retained marker")
				}
			case "rollback-refused":
				if err == nil || result != nil || s.refusePendingCreate() == nil || s.sandbox.ID != identifier {
					t.Fatal("failed rollback lost intent")
				}
				refuse = false
				cleanup := &rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: bundle, BundleIdentity: s.bundleIdentity, TaskIdentity: storageTaskIdentity(s.namespace, s.id), StorageSHA256: hash}
				if err = s.rollbackCreate(t.Context(), cleanup, true, original, key); err != nil || s.refusePendingCreate() != nil {
					t.Fatal("explicit cleanup did not clear owned intent")
				}
			case "commit-marker-changed":
				if err == nil || result != nil || destructive != 0 || s.processes[""] == nil || s.namespaceHolder != held || held.stopped || len(s.token) != 32 || !s.createCleanupPending {
					t.Fatal("post-publication marker error destroyed committed task")
				}
				raw, readErr := os.ReadFile(marker)
				if readErr != nil || string(raw) != "foreign" {
					t.Fatal("foreign marker was altered")
				}
			}
			if s.runtimeDirectory != nil {
				_ = s.runtimeDirectory.Close()
			}
		})
	}
}
