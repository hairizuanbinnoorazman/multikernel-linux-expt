//go:build linux

package rootfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type fakeBackend struct {
	calls       []string
	mountErr    error
	buildErr    error
	unmountErr  error
	verifyErr   error
	storagePath string
}

func (f *fakeBackend) Mount(_ context.Context, _ []Mount, target string) error {
	f.calls = append(f.calls, "mount:"+target)
	return f.mountErr
}
func (f *fakeBackend) Unmount(_ context.Context, target string) error {
	f.calls = append(f.calls, "unmount:"+target)
	return f.unmountErr
}
func (f *fakeBackend) Build(_ context.Context, request PrepareRequest, runtimeDir, storageDir string) (PrepareResult, error) {
	f.calls = append(f.calls, "build:"+runtimeDir)
	if f.buildErr != nil {
		return PrepareResult{}, f.buildErr
	}
	f.storagePath = filepath.Join(storageDir, "root.ext4")
	return PrepareResult{Storage: protocol.StorageConfig{Path: f.storagePath, ImageID: "image", FilesystemUUID: "12345678-1234-1234-1234-123456789abc", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 128, Port: request.StoragePort, SHA256: strings.Repeat("a", 64)}, BuildResult: []byte(`{"schema_version":1}`)}, nil
}
func (f *fakeBackend) VerifyPrepared(_ context.Context, record Record) error {
	f.calls = append(f.calls, "verify:"+record.Request.TaskIdentity)
	return f.verifyErr
}

func rootfsFixture(t *testing.T) (*Service, *fakeBackend, PrepareRequest, string) {
	t.Helper()
	base := t.TempDir()
	bundle := filepath.Join(base, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(base, "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	service, err := NewService(store, backend, filepath.Join(base, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	request := PrepareRequest{Version: Version, Bundle: bundle, TaskIdentity: "task-0123456789abcdef0123456789abcdef", StoragePort: 4061,
		Mounts: []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/snapshot"}}}}
	return service, backend, request, base
}

func TestPrepareJournalsBuildUnmountAndReplays(t *testing.T) {
	service, backend, request, _ := rootfsFixture(t)
	result, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(request.Bundle, "rootfs")
	want := []string{"mount:" + root, "build:" + filepath.Join(request.Bundle, ".multikernel"), "unmount:" + root}
	if !reflect.DeepEqual(backend.calls, want) || result.Storage.Path != backend.storagePath {
		t.Fatalf("calls/result = %v %+v", backend.calls, result)
	}
	backend.calls = nil
	replayed, err := service.Prepare(context.Background(), request)
	if err != nil || replayed.Storage != result.Storage || !reflect.DeepEqual(backend.calls, []string{"verify:" + request.TaskIdentity}) {
		t.Fatalf("replay = %+v %v calls=%v", replayed, err, backend.calls)
	}
	conflict := request
	conflict.StoragePort++
	if _, err = service.Prepare(context.Background(), conflict); err == nil {
		t.Fatal("conflicting replay accepted")
	}
}

func TestPrepareBuildFailureUnmountsAndRemovesArtifacts(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	backend.buildErr = errors.New("injected build failure")
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("build failure accepted")
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("failed preparation remained journaled")
	}
	for _, path := range []string{filepath.Join(request.Bundle, ".multikernel"), filepath.Join(base, "storage", request.TaskIdentity)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact survived at %s: %v", path, err)
		}
	}
	if len(backend.calls) != 3 || !strings.HasPrefix(backend.calls[2], "unmount:") {
		t.Fatalf("calls = %v", backend.calls)
	}
}

func TestUnmountFailurePreservesRecoverableState(t *testing.T) {
	service, backend, request, _ := rootfsFixture(t)
	backend.unmountErr = errors.New("injected unmount failure")
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("unmount failure accepted")
	}
	record, ok := service.store.Get(request.TaskIdentity)
	if !ok || record.Phase != "MOUNTED" {
		t.Fatalf("recoverable record = %+v, %v", record, ok)
	}
	backend.unmountErr = nil
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("partial state survived reconcile")
	}
}

func TestCleanupRequiresExactImageIdentity(t *testing.T) {
	service, _, request, _ := rootfsFixture(t)
	result, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := CleanupRequest{Version: Version, Bundle: request.Bundle, TaskIdentity: request.TaskIdentity, StorageSHA256: strings.Repeat("b", 64)}
	if err = service.Cleanup(context.Background(), cleanup); err == nil {
		t.Fatal("stale digest accepted")
	}
	cleanup.StorageSHA256 = result.Storage.SHA256
	if err = service.Cleanup(context.Background(), cleanup); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("cleanup record survived")
	}
}

func TestValidationRejectsSymlinkBundleAndPropagation(t *testing.T) {
	service, _, request, base := rootfsFixture(t)
	link := filepath.Join(base, "bundle-link")
	if err := os.Symlink(request.Bundle, link); err != nil {
		t.Fatal(err)
	}
	request.Bundle = link
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("symlink bundle accepted")
	}
	request.Bundle = filepath.Join(base, "bundle")
	request.Mounts[0].Options = []string{"rshared"}
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("propagation option accepted")
	}
}
