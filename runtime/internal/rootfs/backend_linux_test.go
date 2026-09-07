//go:build linux

package rootfs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStorageBuildRequiresExactDeterministicIdentity(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "storage.json")
	image := filepath.Join(directory, "root.ext4")
	value := storageBuildMetadata{SchemaVersion: 1, Path: image, ImageID: "root-abc",
		FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
		QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("a", 64),
		OfflineCheckSHA256: strings.Repeat("b", 64), Allocation: "posix_fallocate", Format: "ext4"}
	value.Determinism.FakeTime = 1
	value.Determinism.HashSeed = value.FilesystemUUID
	value.Determinism.SourceMetadataTime = 1
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadStorageBuild(path, image, 4061)
	if err != nil || config.SHA256 != value.SHA256 || config.FilesystemUUID != value.FilesystemUUID {
		t.Fatalf("storage config = %+v, %v", config, err)
	}
	value.Determinism.FakeTime = 0
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("wall-clock fake time accepted")
	}
	value.Determinism.FakeTime = 1
	value.Determinism.SourceMetadataTime = 0
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("unnormalized source metadata accepted")
	}
	value.Determinism.SourceMetadataTime = 1
	value.Determinism.LazyInitialization = true
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("lazy initialization accepted")
	}
	if err = os.WriteFile(path, []byte(`{"schema_version":1,"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
}
