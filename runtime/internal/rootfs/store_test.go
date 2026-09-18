package rootfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func validRootfsRecord(bundle, storageRoot, identity string, port uint32) Record {
	bundleIdentity := DirectoryIdentity{Device: 1, Inode: 2, UID: uint32(os.Geteuid())}
	return Record{
		Version: Version,
		Request: PrepareRequest{Version: Version, Bundle: bundle, BundleIdentity: bundleIdentity, TaskIdentity: identity, StoragePort: port,
			Mounts: []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/snapshots/root"}}}},
		Root: filepath.Join(bundle, "rootfs"), RuntimeDir: filepath.Join(bundle, ".multikernel"),
		StorageDir: filepath.Join(storageRoot, identity), BundleID: bundleIdentity,
		RootID:       DirectoryIdentity{Device: 1, Inode: 3, UID: uint32(os.Geteuid())},
		RuntimeID:    DirectoryIdentity{Device: 1, Inode: 4, UID: uint32(os.Geteuid())},
		StorageID:    DirectoryIdentity{Device: 1, Inode: 5, UID: uint32(os.Geteuid())},
		StorageDirID: DirectoryIdentity{Device: 1, Inode: 6, UID: uint32(os.Geteuid())}, Phase: "MOUNTING",
	}
}

func TestRootfsStorePersistsOnlyForwardValidState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	identity := "task-0123456789abcdef0123456789abcdef"
	value := validRootfsRecord("/srv/bundles/box", "/var/lib/multikernel/rootfs", identity, 4061)
	invalid := value
	invalid.Phase = "PREPARED"
	if err = store.Put(invalid); err == nil {
		t.Fatal("new PREPARED record bypassed MOUNTING")
	}
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	mounted := value
	mounted.Phase = "MOUNTED"
	if err = store.Put(mounted); err != nil {
		t.Fatal(err)
	}
	prepared := mounted
	prepared.Phase = "PREPARED"
	prepared.Storage = &protocol.StorageConfig{
		Path: filepath.Join(prepared.StorageDir, "root.ext4"), ImageID: "image",
		FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
		QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("a", 64),
	}
	prepared.BuildResult = json.RawMessage(`{"schema_version":1}`)
	if err = store.Put(prepared); err != nil {
		t.Fatal(err)
	}
	canonicalPrepared := cloneRootfsRecord(prepared)
	prepared.Request.Mounts[0].Options[0] = "lowerdir=/attacker"
	prepared.Storage.Path = "/tmp/attacker.ext4"
	prepared.BuildResult[0] = '['
	returned, ok := store.Get(identity)
	if !ok || returned.Storage.Path != canonicalPrepared.Storage.Path ||
		string(returned.BuildResult) != string(canonicalPrepared.BuildResult) ||
		returned.Request.Mounts[0].Options[0] != canonicalPrepared.Request.Mounts[0].Options[0] {
		t.Fatalf("caller mutation changed durable rootfs state: %+v", returned)
	}
	returned.Storage.Path = "/tmp/returned.ext4"
	if again, _ := store.Get(identity); again.Storage.Path != canonicalPrepared.Storage.Path {
		t.Fatal("Get returned aliased rootfs state")
	}
	prepared = canonicalPrepared
	regressed := prepared
	regressed.Phase = "MOUNTED"
	regressed.Storage = nil
	regressed.BuildResult = nil
	if err = store.Put(regressed); err == nil {
		t.Fatal("rootfs phase regressed")
	}
	changed := prepared
	changed.StorageDir = filepath.Join("/tmp", identity)
	changed.Storage.Path = filepath.Join(changed.StorageDir, "root.ext4")
	if err = store.Put(changed); err == nil {
		t.Fatal("rootfs artifact identity changed in place")
	}
	reopened, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.Get(identity); !ok || got.Phase != "PREPARED" || got.Storage == nil {
		t.Fatalf("reopened rootfs record = %+v, %v", got, ok)
	}
}

func TestRootfsStoreRejectsForgedSemanticState(t *testing.T) {
	identity := "task-0123456789abcdef0123456789abcdef"
	base := validRootfsRecord("/srv/bundles/box", "/var/lib/multikernel/rootfs", identity, 4061)
	for name, mutate := range map[string]func(*diskState){
		"forged key": func(state *diskState) {
			state.Records["forged"] = base
		},
		"forged runtime path": func(state *diskState) {
			value := base
			value.RuntimeDir = "/tmp/attacker"
			state.Records[identity] = value
		},
		"prepared without result": func(state *diskState) {
			value := base
			value.Phase = "PREPARED"
			state.Records[identity] = value
		},
		"missing directory identity": func(state *diskState) {
			value := base
			value.BundleID = DirectoryIdentity{}
			state.Records[identity] = value
		},
		"missing mount target identity": func(state *diskState) {
			value := base
			value.RootID = DirectoryIdentity{}
			state.Records[identity] = value
		},
		"missing runtime directory identity": func(state *diskState) {
			value := base
			value.RuntimeID = DirectoryIdentity{}
			state.Records[identity] = value
		},
		"missing storage directory identity": func(state *diskState) {
			value := base
			value.StorageDirID = DirectoryIdentity{}
			state.Records[identity] = value
		},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			state := diskState{Version: rootfsDiskVersion, Records: map[string]Record{}}
			mutate(&state)
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(directory, "state.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenStore(directory); err == nil {
				t.Fatal("forged rootfs state was accepted")
			}
		})
	}
}

func TestRootfsStoreRejectsDuplicateLiveClaims(t *testing.T) {
	firstID := "task-0123456789abcdef0123456789abcdef"
	secondID := "task-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	base := validRootfsRecord("/srv/bundles/box", "/var/lib/multikernel/rootfs", firstID, 4061)
	for _, collision := range []string{"bundle", "storage port"} {
		t.Run(collision, func(t *testing.T) {
			other := validRootfsRecord("/srv/bundles/other", "/var/lib/multikernel/rootfs", secondID, 4062)
			switch collision {
			case "bundle":
				other.Request.Bundle = base.Request.Bundle
				other.Root = base.Root
				other.RuntimeDir = base.RuntimeDir
			case "storage port":
				other.Request.StoragePort = base.Request.StoragePort
			}
			state := diskState{Version: rootfsDiskVersion, Records: map[string]Record{firstID: base, secondID: other}}
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			if err = os.WriteFile(filepath.Join(directory, "state.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenStore(directory); err == nil || !strings.Contains(err.Error(), collision) {
				t.Fatalf("duplicate %s error = %v", collision, err)
			}
		})
	}
}

func TestRootfsStoreRejectsDirectoryReplacementBeforePersistence(t *testing.T) {
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
	identity := "task-0123456789abcdef0123456789abcdef"
	record := validRootfsRecord("/srv/bundles/box", "/var/lib/multikernel/rootfs", identity, 4061)
	if err = store.Put(record); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("directory replacement error = %v", err)
	}
	if entries, readErr := os.ReadDir(directory); readErr != nil || len(entries) != 0 {
		t.Fatalf("replacement directory was mutated: %v, %v", entries, readErr)
	}
}

func TestRootfsStoreRejectsSymlinkHardlinkAndPermissiveState(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.WriteFile(target, []byte(`{"version":1,"records":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for name, install := range map[string]func(string) error{
		"symlink":  func(path string) error { return os.Symlink(target, path) },
		"hardlink": func(path string) error { return os.Link(target, path) },
		"permissive": func(path string) error {
			return os.WriteFile(path, []byte(`{"version":1,"records":{}}`), 0644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(parent, name)
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			if err := install(filepath.Join(directory, "state.json")); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(directory); err == nil {
				t.Fatalf("%s state was accepted", name)
			}
		})
	}
}

func TestRootfsStoreUpgradesOnlyEmptyLegacyState(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("version-%d", version), func(t *testing.T) {
			directory := t.TempDir()
			statePath := filepath.Join(directory, "state.json")
			data, err := json.Marshal(diskState{Version: version, Records: map[string]Record{}})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(statePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = OpenStore(directory); err != nil {
				t.Fatal(err)
			}
			var upgraded diskState
			data, err = os.ReadFile(statePath)
			if err != nil || json.Unmarshal(data, &upgraded) != nil || upgraded.Version != rootfsDiskVersion {
				t.Fatalf("upgraded state = %+v, read error = %v", upgraded, err)
			}
		})
	}
	directory := t.TempDir()
	statePath := filepath.Join(directory, "state.json")
	legacy := diskState{Version: 1, Records: map[string]Record{
		"task-0123456789abcdef0123456789abcdef": validRootfsRecord("/srv/bundles/box", "/var/lib/multikernel/rootfs", "task-0123456789abcdef0123456789abcdef", 4061),
	}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(directory); err == nil {
		t.Fatal("active legacy state without directory identities was upgraded")
	}
}

func TestRootfsReconcileRefusesPostInitializationCleanupPathCorruption(t *testing.T) {
	service, _, request, base := rootfsFixture(t)
	victim := filepath.Join(base, "victim")
	if err := os.Mkdir(victim, 0700); err != nil {
		t.Fatal(err)
	}
	record := validRootfsRecord(request.Bundle, filepath.Join(base, "storage"), request.TaskIdentity, request.StoragePort)
	record.RuntimeDir = victim
	service.store.mu.Lock()
	service.store.data.Records[request.TaskIdentity] = record
	service.store.mu.Unlock()
	if err := service.Reconcile(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "configured roots") {
		t.Fatalf("forged cleanup path error = %v", err)
	}
	if info, err := os.Stat(victim); err != nil || !info.IsDir() {
		t.Fatalf("victim directory was removed: %v", err)
	}
}

func TestNewRootfsServiceRejectsMismatchedDurableStorageRoot(t *testing.T) {
	base := t.TempDir()
	bundle := filepath.Join(base, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "rootfs"), 0700); err != nil {
		t.Fatal(err)
	}
	identity := "task-0123456789abcdef0123456789abcdef"
	store, err := OpenStore(filepath.Join(base, "state"))
	if err != nil {
		t.Fatal(err)
	}
	record := validRootfsRecord(bundle, filepath.Join(base, "attacker-storage"), identity, 4061)
	record.Request.Mounts = nil
	if err = store.Put(record); err != nil {
		t.Fatal(err)
	}
	if _, err = NewService(store, &fakeBackend{}, filepath.Join(base, "storage")); err == nil ||
		!strings.Contains(err.Error(), "configured roots") {
		t.Fatalf("mismatched storage root error = %v", err)
	}
}

func TestDescriptorAnchoredCleanupDoesNotFollowReplacementRoot(t *testing.T) {
	base := t.TempDir()
	victim := filepath.Join(base, "victim")
	if err := os.MkdirAll(filepath.Join(victim, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "bundle")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	if err := removeRelativeTree(link, ".multikernel"); err == nil {
		t.Fatal("symlink cleanup root was accepted")
	}
	if info, err := os.Stat(filepath.Join(victim, ".multikernel")); err != nil || !info.IsDir() {
		t.Fatalf("cleanup escaped through root symlink: %v", err)
	}
	original := filepath.Join(base, "original")
	replacement := filepath.Join(base, "replacement")
	if err := os.MkdirAll(filepath.Join(original, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(replacement, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	anchored, err := openStableRoot(original)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "moved-original")
	if err = os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, original); err != nil {
		t.Fatal(err)
	}
	if err = anchored.RemoveAll(".multikernel"); err != nil {
		t.Fatal(err)
	}
	if err = anchored.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(moved, ".multikernel")); !os.IsNotExist(err) {
		t.Fatalf("anchored original was not cleaned: %v", err)
	}
	if info, err := os.Stat(filepath.Join(original, ".multikernel")); err != nil || !info.IsDir() {
		t.Fatalf("replacement root was modified: %v", err)
	}
}

func TestDescriptorAnchoredCleanupRejectsReplacedArtifact(t *testing.T) {
	parent := t.TempDir()
	artifact := filepath.Join(parent, ".multikernel")
	if err := os.Mkdir(artifact, 0700); err != nil {
		t.Fatal(err)
	}
	root, rootID, err := inspectStableRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	info, err := root.Lstat(".multikernel")
	if err != nil {
		t.Fatal(err)
	}
	artifactID, ok := openedDirectoryIdentity(info)
	if !ok {
		t.Fatal("artifact identity unavailable")
	}
	if err = root.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(artifact, artifact+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(artifact, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(artifact, "preserve")
	if err = os.WriteFile(marker, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = removeRelativeTree(parent, ".multikernel", rootID, artifactID); err == nil {
		t.Fatal("replaced artifact was accepted for cleanup")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement artifact was modified: %q, %v", data, err)
	}
}

func TestCleanupQuarantineRejectsRemovalTimeReplacement(t *testing.T) {
	parent := t.TempDir()
	artifact := filepath.Join(parent, ".multikernel")
	if err := os.Mkdir(artifact, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "original"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, rootID, err := inspectStableRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	info, err := root.Lstat(".multikernel")
	if err != nil {
		t.Fatal(err)
	}
	artifactID, ok := openedDirectoryIdentity(info)
	if !ok {
		t.Fatal("artifact identity unavailable")
	}
	if err = root.Close(); err != nil {
		t.Fatal(err)
	}
	replaced := false
	err = removeRelativeTreeWithHook(parent, ".multikernel", []DirectoryIdentity{rootID, artifactID}, func() {
		replaced = true
		if renameErr := os.Rename(artifact, artifact+".original"); renameErr != nil {
			t.Fatal(renameErr)
		}
		if mkdirErr := os.Mkdir(artifact, 0700); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		if writeErr := os.WriteFile(filepath.Join(artifact, "preserve"), []byte("replacement"), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if !replaced || err == nil || !strings.Contains(err.Error(), "changed before quarantine") {
		t.Fatalf("removal-time replacement result = replaced:%v err:%v", replaced, err)
	}
	if value, readErr := os.ReadFile(filepath.Join(artifact, "preserve")); readErr != nil || string(value) != "replacement" {
		t.Fatalf("replacement artifact was not restored intact: %q, %v", value, readErr)
	}
	if value, readErr := os.ReadFile(filepath.Join(artifact+".original", "original")); readErr != nil || string(value) != "original" {
		t.Fatalf("original artifact changed: %q, %v", value, readErr)
	}
}

func TestCleanupQuarantineIsCrashRecoverable(t *testing.T) {
	parent := t.TempDir()
	artifact := filepath.Join(parent, ".multikernel")
	if err := os.MkdirAll(filepath.Join(artifact, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifact, "nested", "value"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, rootID, err := inspectStableRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	info, err := root.Lstat(".multikernel")
	if err != nil {
		t.Fatal(err)
	}
	artifactID, ok := openedDirectoryIdentity(info)
	if !ok {
		t.Fatal("artifact identity unavailable")
	}
	if err = root.Close(); err != nil {
		t.Fatal(err)
	}
	quarantine := filepath.Join(parent, fmt.Sprintf(".mklinux-cleanup-%016x-%016x", artifactID.Device, artifactID.Inode))
	if err = os.Rename(artifact, quarantine); err != nil {
		t.Fatal(err)
	}
	if err = removeRelativeTree(parent, ".multikernel", rootID, artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(quarantine); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovered quarantine remains: %v", err)
	}
}
