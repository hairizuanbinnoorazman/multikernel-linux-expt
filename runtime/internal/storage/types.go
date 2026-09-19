package storage

import "time"

const StateVersion = 2

type ImageIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}

type PreparedImage struct {
	Path           string `json:"path"`
	ImageID        string `json:"image_id"`
	FilesystemUUID string `json:"filesystem_uuid"`
	SizeBytes      uint64 `json:"size_bytes"`
	QuotaBytes     uint64 `json:"quota_bytes"`
	InodeLimit     uint64 `json:"inode_limit"`
	Port           uint32 `json:"port"`
	SHA256         string `json:"sha256"`
}

type Counters struct {
	Reads        uint64 `json:"reads"`
	ReadBytes    uint64 `json:"read_bytes"`
	Writes       uint64 `json:"writes"`
	WrittenBytes uint64 `json:"written_bytes"`
	Flushes      uint64 `json:"flushes"`
}

type Export struct {
	SandboxID         string `json:"sandbox_id"`
	SandboxGeneration string `json:"sandbox_generation"`
	ExportGeneration  string `json:"export_generation"`
	PreparedImage
	ImageIdentity ImageIdentity `json:"image_identity"`
	State         string        `json:"state"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	ReleasedAt    time.Time     `json:"released_at,omitempty"`
	OfflineCheck  string        `json:"offline_check,omitempty"`
	Counters      Counters      `json:"counters"`
}

type Observation struct {
	Active     bool
	Closed     bool
	Generation string
	Counters   Counters
}
