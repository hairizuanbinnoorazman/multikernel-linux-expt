// Package protocol contains the versioned daemon and agent contracts.
package protocol

import (
	"encoding/json"
	"time"
)

const Version = 1

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	OperationID string `json:"operation_id,omitempty"`
	Retryable   bool   `json:"retryable"`
}
type Request struct {
	Version        int             `json:"version"`
	RequestID      string          `json:"request_id"`
	Method         string          `json:"method"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	SandboxID      string          `json:"sandbox_id,omitempty"`
	Generation     string          `json:"generation,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
}
type Response struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	Body      any    `json:"body,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

type SandboxConfig struct {
	SchemaVersion  int               `json:"schema_version"`
	ID             string            `json:"id"`
	CPUs           []int             `json:"cpus"`
	MemoryBytes    uint64            `json:"memory_bytes"`
	KernelManifest string            `json:"kernel_manifest"`
	Bundle         string            `json:"bundle"`
	BundleIdentity DirectoryIdentity `json:"bundle_identity"`
	AgentPort      uint32            `json:"agent_port"`
	ChildCID       uint32            `json:"child_cid"`
	Storage        *StorageConfig    `json:"storage,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

type DirectoryIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	UID    uint32 `json:"uid"`
}

type StorageConfig struct {
	Path           string `json:"path"`
	ImageID        string `json:"image_id"`
	FilesystemUUID string `json:"filesystem_uuid"`
	SizeBytes      uint64 `json:"size_bytes"`
	QuotaBytes     uint64 `json:"quota_bytes"`
	InodeLimit     uint64 `json:"inode_limit"`
	Port           uint32 `json:"port"`
	SHA256         string `json:"sha256"`
}

type StorageStatus struct {
	ExportGeneration string    `json:"export_generation"`
	State            string    `json:"state"`
	OfflineCheck     string    `json:"offline_check,omitempty"`
	Reads            uint64    `json:"reads"`
	ReadBytes        uint64    `json:"read_bytes"`
	Writes           uint64    `json:"writes"`
	WrittenBytes     uint64    `json:"written_bytes"`
	Flushes          uint64    `json:"flushes"`
	ReleasedAt       time.Time `json:"released_at,omitempty"`
}
type Sandbox struct {
	ID         string         `json:"id"`
	Generation string         `json:"generation"`
	State      string         `json:"state"`
	Error      *Error         `json:"error,omitempty"`
	Config     SandboxConfig  `json:"config"`
	Storage    *StorageStatus `json:"storage,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}
type MutationResult struct {
	Sandbox  Sandbox `json:"sandbox"`
	Replayed bool    `json:"replayed"`
}
type EventQuery struct {
	AfterSequence uint64 `json:"after_sequence"`
	Limit         uint32 `json:"limit,omitempty"`
}
type Event struct {
	Version    int       `json:"version"`
	Sequence   uint64    `json:"sequence"`
	At         time.Time `json:"at"`
	SandboxID  string    `json:"sandbox_id"`
	Generation string    `json:"generation,omitempty"`
	Method     string    `json:"method"`
	State      string    `json:"state,omitempty"`
	Error      *Error    `json:"error,omitempty"`
}
