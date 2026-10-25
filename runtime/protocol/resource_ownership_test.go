package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func resourceFixture() ResourceOwnershipReport {
	s := storageReportFixture()
	v := s.Exports[0]
	hash := StorageDescriptorSHA256(StorageConfig{Path: "/PRIVATE_PATH/root.ext4", ImageID: "PRIVATE_IMAGE", FilesystemUUID: v.FilesystemUUID, SizeBytes: v.SizeBytes, QuotaBytes: v.QuotaBytes, InodeLimit: v.InodeLimit, Port: v.Port, SHA256: strings.Repeat("d", 64)})
	owner := SandboxOwnership{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, State: "RUNNING", CPUs: []int{8, 9}, MemoryBytes: 3 << 30, ChildCID: v.PeerCID, AgentPort: 7200}
	return ResourceOwnershipReport{Version: 1, Source: "durable-lifecycle-storage", Lifecycle: OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: 4, Sandboxes: []SandboxOwnership{owner}}, StorageBindings: []LifecycleStorageBinding{{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, Required: true, ExportGeneration: v.ExportGeneration, DescriptorSHA256: hash}}, Storage: s, StorageDescriptors: []StorageDescriptorBinding{{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, DescriptorSHA256: hash}}}
}
func TestResourceOwnershipValidationAndPrivateDescriptorFingerprint(t *testing.T) {
	if StorageDescriptorSHA256(StorageConfig{Path: "\xff"}) == StorageDescriptorSHA256(StorageConfig{Path: "\xfe"}) {
		t.Fatal("invalid UTF-8 path bytes normalized into the same fingerprint")
	}
	if StorageDescriptorSHA256(StorageConfig{Path: "ab", ImageID: "c"}) == StorageDescriptorSHA256(StorageConfig{Path: "a", ImageID: "bc"}) {
		t.Fatal("descriptor field boundaries are ambiguous")
	}
	r := resourceFixture()
	if r.Validate() != nil {
		t.Fatal("valid resource fixture refused")
	}
	raw, _ := json.Marshal(r)
	for _, secret := range []string{"PRIVATE_PATH", "PRIVATE_IMAGE", strings.Repeat("d", 64)} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private descriptor serialized")
		}
	}
	base := StorageConfig{Path: "/private/root.ext4", ImageID: "root", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("d", 64)}
	for _, field := range []string{"path", "image", "uuid", "size", "quota", "inodes", "port", "sha"} {
		v := base
		switch field {
		case "path":
			v.Path = "/alias/root.ext4"
		case "image":
			v.ImageID = "other"
		case "uuid":
			v.FilesystemUUID = "other"
		case "size":
			v.SizeBytes++
		case "quota":
			v.QuotaBytes++
		case "inodes":
			v.InodeLimit++
		case "port":
			v.Port++
		case "sha":
			v.SHA256 = strings.Repeat("c", 64)
		}
		if StorageDescriptorSHA256(v) == StorageDescriptorSHA256(base) {
			t.Fatal("descriptor omitted " + field)
		}
	}
	for _, mode := range []string{"version", "source", "nil-bindings", "owner", "generation", "digest", "ordinary", "descriptor-count", "descriptor-owner", "descriptor-export", "descriptor-digest"} {
		t.Run(mode, func(t *testing.T) {
			r := resourceFixture()
			switch mode {
			case "version":
				r.Version = 2
			case "source":
				r.Source = "SECRET"
			case "nil-bindings":
				r.StorageBindings = nil
			case "owner":
				r.StorageBindings[0].SandboxID = "other"
			case "generation":
				r.StorageBindings[0].Generation = strings.Repeat("f", 32)
			case "digest":
				r.StorageBindings[0].DescriptorSHA256 = "SECRET"
			case "ordinary":
				r.StorageBindings[0].Required = false
			case "descriptor-count":
				r.StorageDescriptors = nil
			case "descriptor-owner":
				r.StorageDescriptors[0].SandboxID = "other"
			case "descriptor-export":
				r.StorageDescriptors[0].ExportGeneration = strings.Repeat("f", 32)
			case "descriptor-digest":
				r.StorageDescriptors[0].DescriptorSHA256 = "SECRET"
			}
			if err := r.Validate(); err == nil || strings.Contains(err.Error(), "SECRET") {
				t.Fatal("malformed report accepted/reflected")
			}
		})
	}
}
