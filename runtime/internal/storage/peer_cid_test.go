package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPeerCIDRequestsRefuseBeforeAnyBackendOrStateEffect(t *testing.T) {
	for _, cid := range []uint32{0, ^uint32(0)} {
		service, store, backend, image := fixture(t)
		if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, cid, image); err == nil {
			t.Fatal("reserved CID accepted")
		}
		if len(backend.calls) != 0 || len(store.List()) != 0 {
			t.Fatal("reserved CID reached backend or state")
		}
	}
	service, store, backend, image := fixture(t)
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, 40, image)
	if err != nil {
		t.Fatal(err)
	}
	before := store.List()
	calls := len(backend.calls)
	if _, err = service.Provision(context.Background(), "box-a", sandboxGeneration, 41, image); err == nil {
		t.Fatal("peer changed under same owner")
	}
	if _, err = service.Release(context.Background(), "box-a", sandboxGeneration, value.ExportGeneration, 41); err == nil {
		t.Fatal("wrong peer released export")
	}
	if len(backend.calls) != calls || !reflect.DeepEqual(before, store.List()) {
		t.Fatal("peer mismatch had effects")
	}
	other := image
	other.Path += ".other"
	other.Port++
	other.FilesystemUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	if _, err = service.Provision(context.Background(), "box-b", strings.Repeat("b", 32), 40, other); err == nil {
		t.Fatal("one peer claimed by two owners")
	}
	if len(backend.calls) != calls || !reflect.DeepEqual(before, store.List()) {
		t.Fatal("duplicate peer had effects")
	}
}

func TestPeerCIDReconciliationPreflightsAllLifecycleOwners(t *testing.T) {
	for _, kind := range []string{"missing", "generation", "cid", "image", "export-generation"} {
		t.Run(kind, func(t *testing.T) {
			service, store, backend, image := fixture(t)
			value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, 40, image)
			if err != nil {
				t.Fatal(err)
			}
			owner := Owner{Generation: sandboxGeneration, PeerCID: 40, Image: image, ExportGeneration: value.ExportGeneration}
			owners := map[string]Owner{"box-a": owner}
			switch kind {
			case "missing":
				delete(owners, "box-a")
			case "generation":
				owner.Generation = strings.Repeat("b", 32)
			case "cid":
				owner.PeerCID = 41
			case "image":
				owner.Image.Path += ".wrong"
			case "export-generation":
				owner.ExportGeneration = strings.Repeat("b", 32)
			}
			if kind != "missing" {
				owners["box-a"] = owner
			}
			before := store.List()
			calls := len(backend.calls)
			backend.fail["observe"] = os.ErrPermission
			if err = service.Reconcile(context.Background(), owners); err == nil || !strings.HasPrefix(err.Error(), "OPERATOR_ACTION:") {
				t.Fatalf("ownership refusal: %v", err)
			}
			if len(backend.calls) != calls || !reflect.DeepEqual(before, store.List()) {
				t.Fatal("invalid owner reached backend or state")
			}
		})
	}
}

func TestPeerCIDLegacyStatePreservesAmbiguousExportsAndUpgradesOnlyCompleted(t *testing.T) {
	for _, status := range []string{"PREPARING", "ACTIVE", "QUIESCING", "RELEASED", "invalid-released"} {
		t.Run(status, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "state.json")
			value := validStoredExport("/var/lib/multikernel/root.ext4")
			value.PeerCID = 0
			value.State = status
			if strings.Contains(status, "released") || status == "RELEASED" {
				value.State = "RELEASED"
				value.ReleasedAt = value.UpdatedAt
				value.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("b", 64)
				if status == "invalid-released" {
					value.OfflineCheck = "unproved"
				}
			}
			state := diskState{Version: 2, Exports: map[string]Export{exportKey(value.SandboxID, value.SandboxGeneration): value}}
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := OpenStore(directory)
			if status != "RELEASED" {
				if err == nil {
					t.Fatal("ambiguous or invalid legacy state accepted")
				}
				if string(mustRead(t, path)) != string(raw) {
					t.Fatal("refusal rewrote original evidence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after := store.List()
			if len(after) != 1 || !reflect.DeepEqual(after[0], value) {
				t.Fatal("completed legacy evidence changed")
			}
			var saved diskState
			if err = json.Unmarshal(mustRead(t, path), &saved); err != nil || saved.Version != StateVersion {
				t.Fatalf("completed migration: %v", err)
			}
			if _, err = OpenStore(directory); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPeerCIDCannotChangeThroughStorePublication(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := validStoredExport("/var/lib/multikernel/root.ext4")
	value.PeerCID = 40
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	next := value
	next.PeerCID = 41
	if err = store.Put(next); err == nil {
		t.Fatal("peer changed in place")
	}
	if actual, ok := store.Get(value.SandboxID, value.SandboxGeneration); !ok || !reflect.DeepEqual(value, actual) {
		t.Fatal("refusal changed published owner")
	}
}

func TestPeerCIDReconciliationChecksWholeSetBeforeObservingValidLease(t *testing.T) {
	service, store, backend, image := fixture(t)
	first, err := service.Provision(t.Context(), "box-a", sandboxGeneration, 40, image)
	if err != nil {
		t.Fatal(err)
	}
	other := image
	other.Path += ".other"
	other.Port++
	other.FilesystemUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	second, err := service.Provision(t.Context(), "box-z", strings.Repeat("b", 32), 41, other)
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]Owner{
		first.SandboxID:  {Generation: first.SandboxGeneration, PeerCID: 40, Image: image, ExportGeneration: first.ExportGeneration},
		second.SandboxID: {Generation: second.SandboxGeneration, PeerCID: 42, Image: other, ExportGeneration: second.ExportGeneration},
	}
	before, calls := store.List(), len(backend.calls)
	if err = service.Reconcile(t.Context(), owners); err == nil {
		t.Fatal("second ownership conflict accepted")
	}
	if len(backend.calls) != calls || !reflect.DeepEqual(before, store.List()) {
		t.Fatal("valid first lease touched before whole-set refusal")
	}
}
