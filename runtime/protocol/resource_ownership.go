package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"regexp"
)

const ResourceOwnershipCapability = "resource-ownership-v1"

type ResourceOwnershipReport struct {
	Version            int                        `json:"version"`
	Source             string                     `json:"source"`
	Lifecycle          OwnershipReport            `json:"lifecycle"`
	StorageBindings    []LifecycleStorageBinding  `json:"storage_bindings"`
	Storage            StorageOwnershipReport     `json:"storage"`
	StorageDescriptors []StorageDescriptorBinding `json:"storage_descriptors"`
}
type LifecycleStorageBinding struct {
	SandboxID        string `json:"sandbox_id"`
	Generation       string `json:"generation"`
	Required         bool   `json:"required"`
	ExportGeneration string `json:"export_generation"`
	DescriptorSHA256 string `json:"descriptor_sha256"`
}
type StorageDescriptorBinding struct {
	SandboxID        string `json:"sandbox_id"`
	Generation       string `json:"generation"`
	ExportGeneration string `json:"export_generation"`
	DescriptorSHA256 string `json:"descriptor_sha256"`
}

var descriptorDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// StorageDescriptorSHA256 compares the complete typed descriptor without
// exposing its path, image name or source-image digest. It confers no authority.
func StorageDescriptorSHA256(v StorageConfig) string {
	h := sha256.New()
	h.Write([]byte("mkruntime-storage-descriptor-v1\x00"))
	var encoded [8]byte
	// Length-delimited raw bytes avoid JSON's replacement of invalid UTF-8 and
	// prevent ambiguous concatenation between private descriptor fields.
	for _, value := range []string{v.Path, v.ImageID, v.FilesystemUUID, v.SHA256} {
		binary.BigEndian.PutUint64(encoded[:], uint64(len(value)))
		h.Write(encoded[:])
		h.Write([]byte(value))
	}
	for _, value := range []uint64{v.SizeBytes, v.QuotaBytes, v.InodeLimit, uint64(v.Port)} {
		binary.BigEndian.PutUint64(encoded[:], value)
		h.Write(encoded[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (r ResourceOwnershipReport) ValidateLifecycle() error {
	bad := func() error { return errors.New("invalid resource lifecycle ownership report") }
	if r.Version != 1 || r.Source != "durable-lifecycle-storage" || r.Lifecycle.Validate() != nil || r.StorageBindings == nil || len(r.StorageBindings) != len(r.Lifecycle.Sandboxes) {
		return bad()
	}
	for i, b := range r.StorageBindings {
		owner := r.Lifecycle.Sandboxes[i]
		if b.SandboxID != owner.SandboxID || b.Generation != owner.Generation {
			return bad()
		}
		if b.Required {
			if !descriptorDigest.MatchString(b.DescriptorSHA256) || (b.ExportGeneration != "" && !auditGeneration.MatchString(b.ExportGeneration)) {
				return bad()
			}
		} else if b.DescriptorSHA256 != "" || b.ExportGeneration != "" {
			return bad()
		}
	}
	return nil
}
func (r ResourceOwnershipReport) Validate() error {
	if r.ValidateLifecycle() != nil || r.Storage.Validate() != nil || r.StorageDescriptors == nil || len(r.StorageDescriptors) != len(r.Storage.Exports) {
		return errors.New("invalid resource ownership report")
	}
	for i, b := range r.StorageDescriptors {
		v := r.Storage.Exports[i]
		if b.SandboxID != v.SandboxID || b.Generation != v.SandboxGeneration || b.ExportGeneration != v.ExportGeneration || !descriptorDigest.MatchString(b.DescriptorSHA256) {
			return errors.New("invalid resource storage descriptor")
		}
	}
	return nil
}
