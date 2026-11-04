package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var privateRootfsFailure = errors.New(strings.Repeat("SYNTHETIC_ROOTFS_RPC_TOKEN /private/rootfs/credential\n", 256))

type rpcRootfsBackend struct {
	validateErr, mountErr, buildErr, unmountErr error
	mounts, unmounts                            int
}

func (b *rpcRootfsBackend) Validate(context.Context, rootfs.ValidateRequest, *os.File) error {
	return b.validateErr
}
func (b *rpcRootfsBackend) Mount(context.Context, []rootfs.Mount, string, rootfs.DirectoryIdentity) error {
	b.mounts++
	return b.mountErr
}
func (b *rpcRootfsBackend) Unmount(context.Context, string) error { b.unmounts++; return b.unmountErr }
func (b *rpcRootfsBackend) VerifyPrepared(context.Context, rootfs.Record, rootfs.PreparedRoots, bool) error {
	return nil
}
func (b *rpcRootfsBackend) OpenVerifiedInitramfs(context.Context, rootfs.Record, rootfs.PreparedRoots) (*os.File, error) {
	return nil, errors.New("unexpected boot request")
}
func (b *rpcRootfsBackend) Build(_ context.Context, r rootfs.PrepareRequest, roots rootfs.BuildRoots) (rootfs.PrepareResult, error) {
	if b.buildErr != nil {
		return rootfs.PrepareResult{}, b.buildErr
	}
	return rootfs.PrepareResult{Storage: protocol.StorageConfig{Path: filepath.Join(roots.StorageDirPath, "root.ext4"), ImageID: "image", FilesystemUUID: "12345678-1234-1234-1234-123456789abc", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 128, Port: r.StoragePort, SHA256: strings.Repeat("a", 64)}, BuildResult: json.RawMessage(`{"schema_version":1}`)}, nil
}
func rpcRootfsFixture(t *testing.T) (*Server, *rootfs.Store, *rpcRootfsBackend, rootfs.PrepareRequest) {
	t.Helper()
	base := t.TempDir()
	bundle := filepath.Join(base, "bundle")
	snapshot := filepath.Join(base, "snapshot")
	for _, p := range []string{bundle, snapshot} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(bundle)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	store, err := rootfs.OpenStore(filepath.Join(base, "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &rpcRootfsBackend{}
	service, err := rootfs.NewService(store, backend, filepath.Join(base, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	request := rootfs.PrepareRequest{Version: 1, Bundle: bundle, BundleIdentity: rootfs.DirectoryIdentity{Device: uint64(st.Dev), Inode: st.Ino, UID: st.Uid}, TaskIdentity: "task-0123456789abcdef0123456789abcdef", StoragePort: 4061, Mounts: []rootfs.Mount{{Type: "bind", Source: snapshot, Options: []string{"ro"}}}}
	return &Server{Rootfs: service}, store, backend, request
}
func rootfsWire(t *testing.T, s *Server, method string, body any) (json.RawMessage, *protocol.Error) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	c, connection := net.Pipe()
	defer c.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handle(context.Background(), connection) }()
	if err = c.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = json.NewEncoder(c).Encode(protocol.Request{Version: 1, RequestID: "rootfs-private-error", Method: method, Body: data}); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rootfs RPC retained handler")
	}
	if len(raw) > 1024 || strings.Contains(string(raw), "SYNTHETIC_ROOTFS_RPC_TOKEN") || strings.Contains(string(raw), "/private/rootfs/credential") {
		t.Fatal("rootfs RPC reflects private backend data")
	}
	var out struct {
		Body      json.RawMessage `json:"body"`
		Error     *protocol.Error `json:"error"`
		RequestID string          `json:"request_id"`
	}
	if json.Unmarshal(raw, &out) != nil || out.RequestID != "rootfs-private-error" {
		t.Fatal("rootfs RPC binding or framing changed")
	}
	return out.Body, out.Error
}
func requireRootfsFailure(t *testing.T, body json.RawMessage, issue *protocol.Error) {
	t.Helper()
	if issue == nil || issue.Code != "FAILED_PRECONDITION" || issue.Retryable || len(issue.Message) > 128 || len(body) != 0 {
		t.Fatal("rootfs failure changed code/retryability or retained a result body")
	}
}
func TestRootfsRPCFailureRedactionAndRecovery(t *testing.T) {
	for _, stage := range []string{"validate", "mount-rollback", "build-rollback", "build-retained", "cleanup-retained"} {
		t.Run(stage, func(t *testing.T) {
			server, store, backend, prepare := rpcRootfsFixture(t)
			if stage == "validate" {
				backend.validateErr = privateRootfsFailure
				body, issue := rootfsWire(t, server, "ValidateRootfs", rootfs.ValidateRequest{Version: 1, Bundle: prepare.Bundle, BundleIdentity: prepare.BundleIdentity})
				requireRootfsFailure(t, body, issue)
				if len(store.List()) != 0 || backend.mounts != 0 {
					t.Fatal("validation mutated rootfs ownership")
				}
				return
			}
			if stage == "mount-rollback" {
				backend.mountErr = privateRootfsFailure
			}
			if stage == "build-rollback" || stage == "build-retained" {
				backend.buildErr = privateRootfsFailure
			}
			if stage == "build-retained" {
				backend.unmountErr = privateRootfsFailure
			}
			body, issue := rootfsWire(t, server, "PrepareRootfs", prepare)
			if stage != "cleanup-retained" {
				requireRootfsFailure(t, body, issue)
				records := store.List()
				if stage == "build-retained" {
					if len(records) != 1 || records[0].Phase != "MOUNTED" || records[0].Request.TaskIdentity != prepare.TaskIdentity {
						t.Fatal("uncertain unmount lost durable owner")
					}
					backend.unmountErr = nil
					if err := server.Rootfs.Reconcile(context.Background(), nil); err != nil || len(store.List()) != 0 {
						t.Fatal("retained preparation did not reconcile")
					}
				} else if len(records) != 0 || backend.unmounts != 1 {
					t.Fatal("failed preparation rollback changed")
				}
				return
			}
			if issue != nil || len(body) == 0 {
				t.Fatal("successful rootfs preparation failed")
			}
			before := store.List()
			if len(before) != 1 || before[0].Phase != "PREPARED" {
				t.Fatal("preparation did not retain exact ownership")
			}
			cleanup := rootfs.CleanupRequest{Version: 1, Bundle: prepare.Bundle, BundleIdentity: prepare.BundleIdentity, TaskIdentity: prepare.TaskIdentity, StorageSHA256: strings.Repeat("a", 64)}
			backend.unmountErr = privateRootfsFailure
			body, issue = rootfsWire(t, server, "CleanupRootfs", cleanup)
			requireRootfsFailure(t, body, issue)
			if !reflect.DeepEqual(before, store.List()) {
				t.Fatal("failed cleanup changed durable ownership")
			}
			backend.unmountErr = nil
			body, issue = rootfsWire(t, server, "CleanupRootfs", cleanup)
			if issue != nil || string(body) != `{"cleaned":true}` || len(store.List()) != 0 {
				t.Fatal("exact-identity cleanup retry failed")
			}
		})
	}
}

func TestRootfsRPCMissingBundleOmitsFilesystemPath(t *testing.T) {
	server, store, _, prepare := rpcRootfsFixture(t)
	request := rootfs.ValidateRequest{Version: 1, Bundle: filepath.Join(prepare.Bundle, "SYNTHETIC_ROOTFS_RPC_TOKEN"), BundleIdentity: prepare.BundleIdentity}
	body, issue := rootfsWire(t, server, "ValidateRootfs", request)
	requireRootfsFailure(t, body, issue)
	if len(store.List()) != 0 {
		t.Fatal("filesystem failure changed ownership")
	}
}

func TestRootfsRPCStaleCleanupPreservesOwnerAndBackend(t *testing.T) {
	server, store, backend, prepare := rpcRootfsFixture(t)
	if _, issue := rootfsWire(t, server, "PrepareRootfs", prepare); issue != nil {
		t.Fatal("initial preparation failed")
	}
	before := store.List()
	unmounts := backend.unmounts
	request := rootfs.CleanupRequest{Version: 1, Bundle: prepare.Bundle, BundleIdentity: prepare.BundleIdentity, TaskIdentity: prepare.TaskIdentity, StorageSHA256: strings.Repeat("b", 64)}
	body, issue := rootfsWire(t, server, "CleanupRootfs", request)
	requireRootfsFailure(t, body, issue)
	if backend.unmounts != unmounts || !reflect.DeepEqual(before, store.List()) {
		t.Fatal("stale cleanup changed backend or durable owner")
	}
}

func TestRootfsRPCUnsupportedAndMalformedKeepClassification(t *testing.T) {
	for _, method := range []string{"PrepareRootfs", "ValidateRootfs", "CleanupRootfs"} {
		t.Run(method, func(t *testing.T) {
			body, issue := rootfsWire(t, &Server{}, method, map[string]any{})
			if issue == nil || issue.Code != "UNSUPPORTED" || issue.Retryable || len(body) != 0 {
				t.Fatal("missing rootfs service classification changed")
			}
			server, store, backend, _ := rpcRootfsFixture(t)
			body, issue = rootfsWire(t, server, method, map[string]any{"SYNTHETIC_ROOTFS_RPC_TOKEN": "private"})
			if issue == nil || issue.Code != "INVALID_ARGUMENT" || issue.Retryable || len(body) != 0 || backend.mounts != 0 || len(store.List()) != 0 {
				t.Fatal("malformed rootfs request mutated state or changed classification")
			}
		})
	}
}
