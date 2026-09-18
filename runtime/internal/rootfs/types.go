package rootfs

import (
	"encoding/json"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const Version = 1

type Mount struct {
	Type    string   `json:"type"`
	Source  string   `json:"source"`
	Options []string `json:"options,omitempty"`
}

type PrepareRequest struct {
	Version        int               `json:"version"`
	Bundle         string            `json:"bundle"`
	BundleIdentity DirectoryIdentity `json:"bundle_identity"`
	TaskIdentity   string            `json:"task_identity"`
	StoragePort    uint32            `json:"storage_port"`
	Mounts         []Mount           `json:"mounts"`
}

type PrepareResult struct {
	Storage         protocol.StorageConfig `json:"storage"`
	RuntimeIdentity DirectoryIdentity      `json:"runtime_identity"`
	BuildResult     json.RawMessage        `json:"build_result"`
}

type CleanupRequest struct {
	Version        int               `json:"version"`
	Bundle         string            `json:"bundle"`
	BundleIdentity DirectoryIdentity `json:"bundle_identity"`
	TaskIdentity   string            `json:"task_identity"`
	StorageSHA256  string            `json:"storage_sha256"`
}

type DirectoryIdentity = protocol.DirectoryIdentity

type Record struct {
	Version      int                     `json:"version"`
	Request      PrepareRequest          `json:"request"`
	Root         string                  `json:"root"`
	RuntimeDir   string                  `json:"runtime_dir"`
	StorageDir   string                  `json:"storage_dir"`
	BundleID     DirectoryIdentity       `json:"bundle_identity"`
	RootID       DirectoryIdentity       `json:"root_identity"`
	RuntimeID    DirectoryIdentity       `json:"runtime_identity"`
	StorageID    DirectoryIdentity       `json:"storage_root_identity"`
	StorageDirID DirectoryIdentity       `json:"storage_directory_identity"`
	Phase        string                  `json:"phase"`
	Storage      *protocol.StorageConfig `json:"storage,omitempty"`
	BuildResult  json.RawMessage         `json:"build_result,omitempty"`
}
