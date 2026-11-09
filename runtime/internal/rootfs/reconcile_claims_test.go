//go:build linux

package rootfs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func rootfsClaimFiles(t *testing.T, service *Service) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, record := range service.store.List() {
		for _, path := range []string{filepath.Join(record.RuntimeDir, "private-control"), filepath.Join(record.StorageDir, "root.ext4")} {
			value := "SYNTHETIC_PRIVATE_ROOT_DATA"
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			files[path] = value
		}
	}
	return files
}

func assertRootfsClaimRefusalPreserved(t *testing.T, service *Service, backend *fakeBackend, owners map[string]string, files map[string]string) {
	t.Helper()
	backend.calls = nil
	records := service.store.List()
	before, err := os.ReadFile(filepath.Join(service.store.dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]os.FileInfo{}
	for path := range files {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		identities[path] = info
	}
	if err = service.Reconcile(t.Context(), owners); err == nil {
		t.Fatal("conflicting lifecycle claim was treated as orphan authority")
	}
	if len(backend.calls) != 0 || !reflect.DeepEqual(records, service.store.List()) {
		t.Fatal("ownership refusal reached backend or mutated records")
	}
	after, err := os.ReadFile(filepath.Join(service.store.dir, "state.json"))
	if err != nil || string(after) != string(before) {
		t.Fatal("ownership refusal changed durable state")
	}
	for path, value := range files {
		raw, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		if err != nil || statErr != nil || string(raw) != value || !os.SameFile(identities[path], info) || identities[path].Mode() != info.Mode() {
			t.Fatal("ownership refusal changed synthetic backing/control data")
		}
	}
	reopened, err := OpenStore(service.store.dir)
	if err != nil {
		t.Fatalf("refused ownership store reopen failed: %v", err)
	}
	// RawMessage formatting changes under the store's indented disk encoding.
	// Compare canonical serialization, while the unchanged raw file above proves
	// that refusal itself performed no durable write.
	expected, _ := json.Marshal(records)
	observed, _ := json.Marshal(reopened.List())
	if string(expected) != string(observed) {
		t.Fatal("refused ownership did not survive store reopen")
	}
}

func TestReconcileConflictingDigestRetainsPresentAndAbsentBundle(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "bundle-present", true: "bundle-absent"}[missing], func(t *testing.T) {
			service, backend, request, _ := rootfsFixture(t)
			result, err := service.Prepare(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			files := rootfsClaimFiles(t, service)
			if missing {
				if err = os.RemoveAll(request.Bundle); err != nil {
					t.Fatal(err)
				}
				for path := range files {
					if filepath.Dir(path) != filepath.Dir(result.Storage.Path) {
						delete(files, path)
					}
				}
			}
			assertRootfsClaimRefusalPreserved(t, service, backend, map[string]string{result.Storage.Path: strings.Repeat("b", 64)}, files)
			if !missing {
				backend.calls = nil
				if err = service.Reconcile(t.Context(), map[string]string{result.Storage.Path: result.Storage.SHA256}); err != nil || !reflect.DeepEqual(backend.calls, []string{"verify-owned:" + request.TaskIdentity}) {
					t.Fatal("exact writable owner did not retain metadata-only verification")
				}
			}
		})
	}
}

func TestReconcileClaimedPartialPreparationIsNotOrphaned(t *testing.T) {
	for _, phase := range []string{"MOUNTING", "MOUNTED"} {
		t.Run(phase, func(t *testing.T) {
			service, backend, request, base := rootfsFixture(t)
			backend.unmountErr = errors.New("synthetic interrupted unmount")
			if phase == "MOUNTING" {
				backend.mountErr = errors.New("synthetic interrupted mount")
			} else {
				backend.buildErr = errors.New("synthetic interrupted build")
			}
			if _, err := service.Prepare(t.Context(), request); err == nil {
				t.Fatal("partial preparation fixture completed")
			}
			record, _ := service.store.Get(request.TaskIdentity)
			if record.Phase != phase || record.Storage != nil {
				t.Fatal("interrupted preparation did not retain its actual durable phase")
			}
			backend.mountErr, backend.buildErr, backend.unmountErr = nil, nil, nil
			files := rootfsClaimFiles(t, service)
			path := filepath.Join(base, "storage", request.TaskIdentity, "root.ext4")
			assertRootfsClaimRefusalPreserved(t, service, backend, map[string]string{path: strings.Repeat("a", 64)}, files)
			// Only an authoritative no-owner input exercises the pre-existing orphan
			// cleanup path. A digest alone never upgrades this partial record.
			if err := service.Reconcile(t.Context(), nil); err != nil {
				t.Fatal("known-unowned partial cleanup regressed")
			}
			if _, ok := service.store.Get(request.TaskIdentity); ok {
				t.Fatal("known orphan record retained")
			}
		})
	}
}

func TestReconcileMalformedOwnerSnapshotRefusesBeforeCleanup(t *testing.T) {
	for _, kind := range []string{"relative-path", "noncanonical-path", "nul-path", "empty-digest", "invalid-digest", "unrelated-invalid-claim"} {
		t.Run(kind, func(t *testing.T) {
			service, backend, request, base := rootfsFixture(t)
			result, err := service.Prepare(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			path, digest := result.Storage.Path, result.Storage.SHA256
			switch kind {
			case "relative-path":
				path = "private/root.ext4"
			case "noncanonical-path":
				path = filepath.Dir(path) + "/../" + filepath.Base(filepath.Dir(path)) + "/root.ext4"
			case "nul-path":
				path = "/private/\x00/root.ext4"
			case "empty-digest":
				digest = ""
			case "invalid-digest":
				digest = "SYNTHETIC_PRIVATE_DIGEST"
			case "unrelated-invalid-claim":
				path = filepath.Join(base, "unrelated")
				digest = "invalid"
			}
			assertRootfsClaimRefusalPreserved(t, service, backend, map[string]string{path: digest}, rootfsClaimFiles(t, service))
		})
	}
}

func TestReconcilePreflightsAllClaimsBeforeFirstOrphanCleanup(t *testing.T) {
	service, backend, first, base := rootfsFixture(t)
	if _, err := service.Prepare(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Bundle = filepath.Join(base, "second-bundle")
	second.TaskIdentity = "task-" + strings.Repeat("f", 32)
	second.StoragePort++
	if err := os.Mkdir(second.Bundle, 0700); err != nil {
		t.Fatal(err)
	}
	held, identity, err := inspectStableRoot(second.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	held.Close()
	second.BundleIdentity = identity
	result, err := service.Prepare(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	files := rootfsClaimFiles(t, service)
	assertRootfsClaimRefusalPreserved(t, service, backend, map[string]string{result.Storage.Path: strings.Repeat("b", 64)}, files)
}

func TestReconcileClaimsInsideCleanupTreesRefuseBeforeMutation(t *testing.T) {
	for _, tree := range []string{"root", "runtime", "storage"} {
		t.Run(tree, func(t *testing.T) {
			service, backend, request, _ := rootfsFixture(t)
			if _, err := service.Prepare(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			record, _ := service.store.Get(request.TaskIdentity)
			base := record.Root
			if tree == "runtime" {
				base = record.RuntimeDir
			}
			if tree == "storage" {
				base = record.StorageDir
			}
			path := filepath.Join(base, "nested", "live.ext4")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			value := "SYNTHETIC_PRIVATE_LIVE_BACKING"
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			files := rootfsClaimFiles(t, service)
			files[path] = value
			assertRootfsClaimRefusalPreserved(t, service, backend, map[string]string{path: strings.Repeat("b", 64)}, files)
		})
	}
}

func TestReconcileUnrelatedSiblingClaimDoesNotAdoptOrBlockOrphan(t *testing.T) {
	service, _, request, base := rootfsFixture(t)
	if _, err := service.Prepare(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "storage", request.TaskIdentity+"-other", "root.ext4")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("SYNTHETIC_PRIVATE_SIBLING"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(t.Context(), map[string]string{path: strings.Repeat("b", 64)}); err != nil {
		t.Fatal("unrelated sibling prevented known-orphan cleanup")
	}
	if _, ok := service.store.Get(request.TaskIdentity); ok {
		t.Fatal("sibling claim adopted the orphan")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "SYNTHETIC_PRIVATE_SIBLING" {
		t.Fatal("sibling claim backing was modified")
	}
}
