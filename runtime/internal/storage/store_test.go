package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validStoredExport(path string) Export {
	now := time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC)
	return Export{
		SandboxID: "box", SandboxGeneration: sandboxGeneration,
		ExportGeneration: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", State: "PREPARING",
		PreparedImage: PreparedImage{
			Path: path, ImageID: "image", FilesystemUUID: "11111111-2222-4333-8444-555555555555",
			SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061,
			SHA256: strings.Repeat("a", 64),
		},
		ImageIdentity: ImageIdentity{Device: 1, Inode: 2},
		CreatedAt:     now, UpdatedAt: now,
	}
}

func TestStorePersistsStrictPrivateState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "storage")
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	value := validStoredExport("/var/lib/multikernel/root.ext4")
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.Get("box", sandboxGeneration); !ok || got.ExportGeneration != value.ExportGeneration {
		t.Fatalf("reopened export = %+v, %v", got, ok)
	}
	info, err := os.Stat(filepath.Join(directory, "state.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("state mode = %v, %v", info.Mode(), err)
	}
}

func TestStoreUpgradesOnlyEmptyLegacyState(t *testing.T) {
	directory := t.TempDir()
	legacy := diskState{Version: 1, Exports: map[string]Export{}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "state.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(directory); err != nil {
		t.Fatalf("empty legacy state did not upgrade: %v", err)
	}
	var upgraded diskState
	if err = json.Unmarshal(mustRead(t, path), &upgraded); err != nil || upgraded.Version != StateVersion {
		t.Fatalf("upgraded state = %+v, %v", upgraded, err)
	}

	legacy.Exports[exportKey("box", sandboxGeneration)] = validStoredExport("/var/lib/multikernel/root.ext4")
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(directory); err == nil {
		t.Fatal("active legacy state without a version-2 identity contract was guessed")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStoreRejectsForgedSemanticStateBeforeReconciliation(t *testing.T) {
	base := validStoredExport("/var/lib/multikernel/root.ext4")
	for name, mutate := range map[string]func(*diskState){
		"forged key": func(state *diskState) {
			state.Exports["forged"] = base
		},
		"invalid generation": func(state *diskState) {
			value := base
			value.ExportGeneration = "short"
			state.Exports[exportKey(value.SandboxID, value.SandboxGeneration)] = value
		},
		"invalid state": func(state *diskState) {
			value := base
			value.State = "UNKNOWN"
			state.Exports[exportKey(value.SandboxID, value.SandboxGeneration)] = value
		},
		"premature release evidence": func(state *diskState) {
			value := base
			value.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("b", 64)
			state.Exports[exportKey(value.SandboxID, value.SandboxGeneration)] = value
		},
		"missing image identity": func(state *diskState) {
			value := base
			value.ImageIdentity = ImageIdentity{}
			state.Exports[exportKey(value.SandboxID, value.SandboxGeneration)] = value
		},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			state := diskState{Version: StateVersion, Exports: map[string]Export{}}
			mutate(&state)
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(directory, "state.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenStore(directory); err == nil {
				t.Fatal("forged storage state was accepted")
			}
		})
	}
	for _, collision := range []string{"path", "port", "filesystem UUID", "sandbox owner"} {
		t.Run("duplicate live "+collision, func(t *testing.T) {
			directory := t.TempDir()
			other := base
			other.Path = "/var/lib/multikernel/other.ext4"
			other.Port = 4062
			other.FilesystemUUID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
			other.SandboxID = "other"
			other.SandboxGeneration = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			other.ExportGeneration = "cccccccccccccccccccccccccccccccc"
			switch collision {
			case "path":
				other.Path = base.Path
			case "port":
				other.Port = base.Port
			case "filesystem UUID":
				other.FilesystemUUID = base.FilesystemUUID
			case "sandbox owner":
				other.SandboxID = base.SandboxID
			}
			state := diskState{Version: StateVersion, Exports: map[string]Export{
				exportKey(base.SandboxID, base.SandboxGeneration):   base,
				exportKey(other.SandboxID, other.SandboxGeneration): other,
			}}
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(directory, "state.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenStore(directory); err == nil || !strings.Contains(err.Error(), collision) {
				t.Fatalf("duplicate %s error = %v", collision, err)
			}
		})
	}
}

func TestStoreEnforcesStorageStateTransitions(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := validStoredExport("/var/lib/multikernel/root.ext4")
	invalid := value
	invalid.State = "ACTIVE"
	if err = store.Put(invalid); err == nil {
		t.Fatal("new ACTIVE record bypassed PREPARING")
	}
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	invalid = value
	invalid.ExportGeneration = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err = store.Put(invalid); err == nil {
		t.Fatal("storage generation changed in place")
	}
	active := value
	active.State = "ACTIVE"
	active.UpdatedAt = active.UpdatedAt.Add(time.Second)
	if err = store.Put(active); err != nil {
		t.Fatal(err)
	}
	regressed := active
	regressed.State = "PREPARING"
	if err = store.Put(regressed); err == nil {
		t.Fatal("storage state regressed")
	}
}

func TestStoreRejectsDirectoryReplacementBeforePersistence(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(directory, directory+".moved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = store.Put(validStoredExport("/var/lib/multikernel/root.ext4")); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("directory replacement error = %v", err)
	}
	if entries, readErr := os.ReadDir(directory); readErr != nil || len(entries) != 0 {
		t.Fatalf("replacement directory was mutated: %v, %v", entries, readErr)
	}
}

func TestStoreRejectsSymlinkAndUnknownOrDuplicateState(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(parent, "linked")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(linked); err == nil {
		t.Fatal("symlinked state directory accepted")
	}
	stateTarget := filepath.Join(parent, "state-target")
	if err := os.WriteFile(stateTarget, []byte(`{"version":1,"exports":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	hardlinkedDirectory := filepath.Join(parent, "hardlinked")
	if err := os.Mkdir(hardlinkedDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(stateTarget, filepath.Join(hardlinkedDirectory, "state.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(hardlinkedDirectory); err == nil || !strings.Contains(err.Error(), "single-link") {
		t.Fatalf("hard-linked state error = %v", err)
	}
	for name, body := range map[string]string{
		"unknown":   `{"version":1,"exports":{},"extra":true}`,
		"duplicate": `{"version":1,"version":1,"exports":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "state.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(directory); err == nil {
				t.Fatal("malformed state accepted")
			}
		})
	}
}
