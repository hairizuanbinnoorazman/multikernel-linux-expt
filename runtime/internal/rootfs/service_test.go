//go:build linux

package rootfs

import (
	"context"
	"errors"
	"fmt"
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
	mountID     DirectoryIdentity
	buildRoots  BuildRoots
	buildHook   func()
	verifyRoots PreparedRoots
	verifyHook  func()
}

func (f *fakeBackend) Mount(_ context.Context, _ []Mount, target string, identity DirectoryIdentity) error {
	f.calls = append(f.calls, "mount:"+target)
	f.mountID = identity
	return f.mountErr
}
func (f *fakeBackend) Unmount(_ context.Context, target string) error {
	f.calls = append(f.calls, "unmount:"+target)
	return f.unmountErr
}
func (f *fakeBackend) Build(_ context.Context, request PrepareRequest, roots BuildRoots) (PrepareResult, error) {
	f.calls = append(f.calls, "build:"+roots.RuntimeDirPath)
	f.buildRoots = roots
	if f.buildErr != nil {
		return PrepareResult{}, f.buildErr
	}
	f.storagePath = filepath.Join(roots.StorageDirPath, "root.ext4")
	if f.buildHook != nil {
		f.buildHook()
	}
	return PrepareResult{Storage: protocol.StorageConfig{Path: f.storagePath, ImageID: "image", FilesystemUUID: "12345678-1234-1234-1234-123456789abc", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 128, Port: request.StoragePort, SHA256: strings.Repeat("a", 64)}, BuildResult: []byte(`{"schema_version":1}`)}, nil
}

func TestBuildUsesPinnedArtifactDirectoriesAndRejectsNameReplacement(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	runtimePath := filepath.Join(request.Bundle, ".multikernel")
	storagePath := filepath.Join(base, "storage", request.TaskIdentity)
	backend.buildHook = func() {
		for _, item := range []struct {
			logical string
			file    *os.File
		}{
			{runtimePath, backend.buildRoots.RuntimeDir},
			{storagePath, backend.buildRoots.StorageDir},
		} {
			if err := os.Rename(item.logical, item.logical+".original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(item.logical, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(item.logical, "replacement"), []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			anchored := fmt.Sprintf("/proc/self/fd/%d/original", item.file.Fd())
			if err := os.WriteFile(anchored, []byte("pinned"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := service.Prepare(t.Context(), request); err == nil || !strings.Contains(err.Error(), "artifact directory") {
		t.Fatalf("artifact replacement result = %v", err)
	}
	record, ok := service.store.Get(request.TaskIdentity)
	if !ok || record.Phase != "MOUNTED" {
		t.Fatalf("replacement did not retain recoverable ownership: %+v, %v", record, ok)
	}
	for _, logical := range []string{runtimePath, storagePath} {
		if data, err := os.ReadFile(filepath.Join(logical, "replacement")); err != nil || string(data) != "preserve" {
			t.Fatalf("replacement was modified at %s: %q, %v", logical, data, err)
		}
		if data, err := os.ReadFile(filepath.Join(logical+".original", "original")); err != nil || string(data) != "pinned" {
			t.Fatalf("builder descriptor did not retain original at %s: %q, %v", logical, data, err)
		}
	}
}

func TestPreparedVerificationPinsArtifactsAndRejectsNameReplacement(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	result, err := service.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(request.Bundle, ".multikernel")
	storagePath := filepath.Join(base, "storage", request.TaskIdentity)
	for _, path := range []string{runtimePath, storagePath} {
		if err = os.WriteFile(filepath.Join(path, "original-marker"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	backend.verifyHook = func() {
		for _, item := range []struct {
			logical string
			file    *os.File
		}{
			{runtimePath, backend.verifyRoots.RuntimeDir},
			{storagePath, backend.verifyRoots.StorageDir},
		} {
			if err = os.Rename(item.logical, item.logical+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.Mkdir(item.logical, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(item.logical, "replacement"), []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			anchored := fmt.Sprintf("/proc/self/fd/%d/original-marker", item.file.Fd())
			if data, readErr := os.ReadFile(anchored); readErr != nil || string(data) != "original" {
				t.Fatalf("verification descriptor was redirected: %q, %v", data, readErr)
			}
		}
	}
	if replayed, replayErr := service.Prepare(t.Context(), request); replayErr == nil ||
		replayed.Storage.Path != "" || !strings.Contains(replayErr.Error(), "artifact directory") {
		t.Fatalf("verification replacement result = %+v, %v", replayed, replayErr)
	}
	for _, path := range []string{runtimePath, storagePath} {
		if data, readErr := os.ReadFile(filepath.Join(path, "replacement")); readErr != nil || string(data) != "preserve" {
			t.Fatalf("verification replacement was modified at %s: %q, %v", path, data, readErr)
		}
	}
	record, ok := service.store.Get(request.TaskIdentity)
	if !ok || record.Phase != "PREPARED" || record.Storage == nil || record.Storage.SHA256 != result.Storage.SHA256 {
		t.Fatalf("verification replacement lost durable ownership: %+v, %v", record, ok)
	}
}

func TestOpenPreparedBootReturnsJournalBoundDescriptors(t *testing.T) {
	service, _, request, _ := rootfsFixture(t)
	result, err := service.Prepare(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	runtimePath := filepath.Join(request.Bundle, ".multikernel")
	initrdPath := filepath.Join(runtimePath, "initramfs.cpio.gz")
	if err = os.WriteFile(initrdPath, []byte("original-initramfs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtimeDir, initramfs, err := service.OpenPreparedBoot(t.Context(), request.Bundle, &result.Storage)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimeDir.Close()
	defer initramfs.Close()
	if err = os.Rename(runtimePath, runtimePath+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(runtimePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(initrdPath, []byte("substitute-initramfs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(fmt.Sprintf("/proc/self/fd/%d", initramfs.Fd()))
	if err != nil || string(value) != "original-initramfs\n" {
		t.Fatalf("prepared boot descriptor = %q, %v", value, err)
	}
	value, err = os.ReadFile(fmt.Sprintf("/proc/self/fd/%d/initramfs.cpio.gz", runtimeDir.Fd()))
	if err != nil || string(value) != "original-initramfs\n" {
		t.Fatalf("prepared runtime descriptor = %q, %v", value, err)
	}
	if value, err = os.ReadFile(initrdPath); err != nil || string(value) != "substitute-initramfs\n" {
		t.Fatalf("substitute initramfs = %q, %v", value, err)
	}
}

func (f *fakeBackend) VerifyPrepared(_ context.Context, record Record, roots PreparedRoots) error {
	f.calls = append(f.calls, "verify:"+record.Request.TaskIdentity)
	f.verifyRoots = roots
	if f.verifyHook != nil {
		f.verifyHook()
	}
	return f.verifyErr
}

func (f *fakeBackend) OpenVerifiedInitramfs(ctx context.Context, record Record, roots PreparedRoots) (*os.File, error) {
	if err := f.VerifyPrepared(ctx, record, roots); err != nil {
		return nil, err
	}
	return os.Open(fmt.Sprintf("/proc/self/fd/%d/initramfs.cpio.gz", roots.RuntimeDir.Fd()))
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
	snapshot := filepath.Join(base, "snapshot")
	if err := os.Mkdir(snapshot, 0700); err != nil {
		t.Fatal(err)
	}
	request := PrepareRequest{Version: Version, Bundle: bundle, TaskIdentity: "task-0123456789abcdef0123456789abcdef", StoragePort: 4061,
		Mounts: []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=" + snapshot}}}}
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
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	rootID, ok := openedDirectoryIdentity(rootInfo)
	if !ok || backend.mountID != rootID {
		t.Fatalf("mount target identity = %+v, want %+v", backend.mountID, rootID)
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

func TestCleanupRejectsWholeRootReplacementBeforeBackendMutation(t *testing.T) {
	for _, replace := range []string{"bundle", "storage"} {
		t.Run(replace, func(t *testing.T) {
			service, backend, request, base := rootfsFixture(t)
			result, err := service.Prepare(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			backend.calls = nil
			target := request.Bundle
			if replace == "storage" {
				target = filepath.Join(base, "storage")
			}
			moved := target + ".original"
			if err = os.Rename(target, moved); err != nil {
				t.Fatal(err)
			}
			if err = os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(target, "replacement-marker")
			if err = os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			err = service.Cleanup(t.Context(), CleanupRequest{Version: Version, Bundle: request.Bundle,
				TaskIdentity: request.TaskIdentity, StorageSHA256: result.Storage.SHA256})
			if err == nil || !strings.Contains(err.Error(), "identity") {
				t.Fatalf("replacement cleanup error = %v", err)
			}
			if len(backend.calls) != 0 {
				t.Fatalf("backend mutated after root replacement: %v", backend.calls)
			}
			if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "preserve" {
				t.Fatalf("replacement was modified: %q, %v", data, readErr)
			}
			if _, ok := service.store.Get(request.TaskIdentity); !ok {
				t.Fatal("replacement failure discarded durable cleanup ownership")
			}
		})
	}
}

func TestCleanupRejectsArtifactDirectoryReplacementBeforeBackendMutation(t *testing.T) {
	for _, replace := range []string{"runtime", "storage-directory"} {
		t.Run(replace, func(t *testing.T) {
			service, backend, request, base := rootfsFixture(t)
			result, err := service.Prepare(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			backend.calls = nil
			target := filepath.Join(request.Bundle, ".multikernel")
			if replace == "storage-directory" {
				target = filepath.Join(base, "storage", request.TaskIdentity)
			}
			if err = os.Rename(target, target+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(target, "replacement-marker")
			if err = os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			err = service.Cleanup(t.Context(), CleanupRequest{Version: Version, Bundle: request.Bundle,
				TaskIdentity: request.TaskIdentity, StorageSHA256: result.Storage.SHA256})
			if err == nil || !strings.Contains(err.Error(), "artifact directory") {
				t.Fatalf("replacement cleanup error = %v", err)
			}
			if len(backend.calls) != 0 {
				t.Fatalf("backend mutated after artifact replacement: %v", backend.calls)
			}
			if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "preserve" {
				t.Fatalf("replacement was modified: %q, %v", data, readErr)
			}
			if _, ok := service.store.Get(request.TaskIdentity); !ok {
				t.Fatal("replacement failure discarded durable cleanup ownership")
			}
		})
	}
}

func TestValidateMountsRejectsHostileInputBeforeBackendUse(t *testing.T) {
	snapshot := t.TempDir()
	valid := []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=" + snapshot, "nodev"}}}
	if err := ValidateMounts(valid); err != nil {
		t.Fatalf("valid mounts rejected: %v", err)
	}
	for name, mounts := range map[string][]Mount{
		"empty source":     {{Type: "overlay", Source: ""}},
		"unsafe source":    {{Type: "overlay", Source: "overlay\nmalicious"}},
		"duplicate option": {{Type: "overlay", Source: "overlay", Options: []string{"nodev", "nodev"}}},
		"relative overlay": {{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=relative"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMounts(mounts); err == nil {
				t.Fatal("hostile mounts accepted")
			}
		})
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

func TestMountFailureDefensivelyUnmountsAndRemovesArtifacts(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	backend.mountErr = errors.New("injected partial mount failure")
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("mount failure accepted")
	}
	if !reflect.DeepEqual(backend.calls, []string{"mount:" + filepath.Join(request.Bundle, "rootfs"), "unmount:" + filepath.Join(request.Bundle, "rootfs")}) {
		t.Fatalf("rollback calls = %v", backend.calls)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("failed mount remained journaled")
	}
	for _, path := range []string{filepath.Join(request.Bundle, ".multikernel"), filepath.Join(base, "storage", request.TaskIdentity)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact survived at %s: %v", path, err)
		}
	}
}

func TestRejectedMountTargetDoesNotUnmountReplacement(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	backend.mountErr = errors.Join(errMountNotAttempted, errors.New("injected target replacement"))
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("mount-target rejection accepted")
	}
	if !reflect.DeepEqual(backend.calls, []string{"mount:" + filepath.Join(request.Bundle, "rootfs")}) {
		t.Fatalf("pre-mount rejection touched the target: %v", backend.calls)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("rejected mount remained journaled")
	}
	for _, path := range []string{filepath.Join(request.Bundle, ".multikernel"), filepath.Join(base, "storage", request.TaskIdentity)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact survived at %s: %v", path, err)
		}
	}
}

func TestUncertainPartialMountRemainsRecoverable(t *testing.T) {
	service, backend, request, _ := rootfsFixture(t)
	backend.mountErr = errors.New("injected partial mount failure")
	backend.unmountErr = errors.New("injected defensive unmount failure")
	_, err := service.Prepare(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "rollback uncertain rootfs mount") {
		t.Fatalf("uncertain mount error = %v", err)
	}
	record, ok := service.store.Get(request.TaskIdentity)
	if !ok || record.Phase != "MOUNTING" {
		t.Fatalf("recoverable mount record = %+v, %v", record, ok)
	}
	backend.unmountErr = nil
	if err = service.Reconcile(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("reconciled partial mount remained journaled")
	}
}

func TestFinalStateFailureRemovesBuiltArtifactsAndRecoveryRecord(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	backend.buildHook = func() {
		service.store.persistFault = func() error {
			service.store.persistFault = nil
			return errors.New("injected final state persistence failure")
		}
	}
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("final state failure accepted")
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("failed final state remained journaled")
	}
	for _, path := range []string{filepath.Join(request.Bundle, ".multikernel"), filepath.Join(base, "storage", request.TaskIdentity)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("artifact survived at %s: %v", path, err)
		}
	}
}

func TestCleanupPersistenceFailurePreservesDiagnosableRecord(t *testing.T) {
	service, backend, request, base := rootfsFixture(t)
	remainingFailures := 2
	backend.buildHook = func() {
		service.store.persistFault = func() error {
			if remainingFailures > 0 {
				remainingFailures--
				return errors.New("injected unavailable state storage")
			}
			return nil
		}
	}
	_, err := service.Prepare(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "remove rootfs recovery record") {
		t.Fatalf("cleanup persistence error = %v", err)
	}
	record, ok := service.store.Get(request.TaskIdentity)
	if !ok || record.Phase != "MOUNTED" {
		t.Fatalf("diagnosable recovery record = %+v, %v", record, ok)
	}
	for _, path := range []string{filepath.Join(request.Bundle, ".multikernel"), filepath.Join(base, "storage", request.TaskIdentity)} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("artifact survived at %s: %v", path, statErr)
		}
	}
	if err = service.Reconcile(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("diagnosable record survived reconciliation after storage recovered")
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
	if err := service.Reconcile(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("partial state survived reconcile")
	}
}

func TestReconcileRetainsOnlyExactLifecycleOwner(t *testing.T) {
	service, _, request, _ := rootfsFixture(t)
	result, err := service.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Reconcile(context.Background(), map[string]string{result.Storage.Path: result.Storage.SHA256}); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); !ok {
		t.Fatal("owned preparation was removed")
	}
	if err = service.Reconcile(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("orphaned preparation survived")
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
	request.Mounts[0].Options = []string{"lowerdir=relative"}
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("relative overlay path accepted")
	}
	request.Mounts[0].Options = []string{"context=untrusted"}
	if _, err := service.Prepare(context.Background(), request); err == nil {
		t.Fatal("unknown overlay option accepted")
	}
}

func TestRootfsOperationsRejectPreCancelledContextWithoutMutation(t *testing.T) {
	t.Run("prepare", func(t *testing.T) {
		service, backend, request, _ := rootfsFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := service.Prepare(ctx, request); !errors.Is(err, context.Canceled) {
			t.Fatalf("Prepare cancellation error = %v", err)
		}
		if len(backend.calls) != 0 || len(service.store.List()) != 0 {
			t.Fatalf("cancelled Prepare mutated state: calls=%v records=%v", backend.calls, service.store.List())
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		service, backend, request, _ := rootfsFixture(t)
		result, err := service.Prepare(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		calls := len(backend.calls)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = service.Cleanup(ctx, CleanupRequest{Version: Version, Bundle: request.Bundle,
			TaskIdentity: request.TaskIdentity, StorageSHA256: result.Storage.SHA256})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Cleanup cancellation error = %v", err)
		}
		if len(backend.calls) != calls {
			t.Fatalf("cancelled Cleanup called backend: %v", backend.calls[calls:])
		}
		if _, ok := service.store.Get(request.TaskIdentity); !ok {
			t.Fatal("cancelled Cleanup removed ownership record")
		}
	})

	t.Run("reconcile", func(t *testing.T) {
		service, backend, request, _ := rootfsFixture(t)
		if _, err := service.Prepare(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		calls := len(backend.calls)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := service.Reconcile(ctx, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("Reconcile cancellation error = %v", err)
		}
		if len(backend.calls) != calls {
			t.Fatalf("cancelled Reconcile called backend: %v", backend.calls[calls:])
		}
		if _, ok := service.store.Get(request.TaskIdentity); !ok {
			t.Fatal("cancelled Reconcile removed ownership record")
		}
	})
}
