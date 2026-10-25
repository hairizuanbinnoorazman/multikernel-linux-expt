package protocol

import (
	"errors"
	"regexp"
)

const StorageOwnershipCapability = "storage-ownership-v1"
const MaxStorageOwnershipRecords = 256

// StorageOwnershipReport is recorded lease metadata, not native process health.
// Paths, image names, credentials and free-form failures must never be added.
type StorageOwnershipReport struct {
	Version int                      `json:"version"`
	Source  string                   `json:"source"`
	Exports []StorageExportOwnership `json:"exports"`
}
type StorageExportOwnership struct {
	SandboxID         string                   `json:"sandbox_id"`
	SandboxGeneration string                   `json:"sandbox_generation"`
	ExportGeneration  string                   `json:"export_generation"`
	PeerCID           uint32                   `json:"peer_cid"`
	State             string                   `json:"state"`
	FilesystemUUID    string                   `json:"filesystem_uuid"`
	SizeBytes         uint64                   `json:"size_bytes"`
	QuotaBytes        uint64                   `json:"quota_bytes"`
	InodeLimit        uint64                   `json:"inode_limit"`
	Port              uint32                   `json:"port"`
	ImageDevice       uint64                   `json:"image_device"`
	ImageInode        uint64                   `json:"image_inode"`
	OfflineCheck      string                   `json:"offline_check"`
	Counters          StorageOwnershipCounters `json:"counters"`
}
type StorageOwnershipCounters struct {
	Reads        uint64 `json:"reads"`
	ReadBytes    uint64 `json:"read_bytes"`
	Writes       uint64 `json:"writes"`
	WrittenBytes uint64 `json:"written_bytes"`
	Flushes      uint64 `json:"flushes"`
}

var storageOwnerID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var storageUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var storageOfflineCheck = regexp.MustCompile(`^e2fsck-clean-sha256:[a-f0-9]{64}$`)

func (r StorageOwnershipReport) Validate() error {
	bad := func() error { return errors.New("invalid storage ownership report") }
	if r.Version != 1 || r.Source != "durable-storage" || r.Exports == nil || len(r.Exports) > MaxStorageOwnershipRecords {
		return bad()
	}
	previous := ""
	generations := map[string]bool{}
	liveOwners := map[string]bool{}
	peers := map[uint32]bool{}
	ports := map[uint32]bool{}
	uuids := map[string]bool{}
	for _, v := range r.Exports {
		key := v.SandboxID + "\x00" + v.SandboxGeneration
		if !storageOwnerID.MatchString(v.SandboxID) || !auditGeneration.MatchString(v.SandboxGeneration) || !auditGeneration.MatchString(v.ExportGeneration) || key <= previous || generations[v.ExportGeneration] || !storageUUID.MatchString(v.FilesystemUUID) || v.ImageDevice == 0 || v.ImageInode == 0 || v.SizeBytes < 64<<20 || v.SizeBytes > 16<<30 || v.SizeBytes%4096 != 0 || v.QuotaBytes != v.SizeBytes || v.InodeLimit < 128 || v.InodeLimit > 2097152 || v.Port < 1024 || v.Port > 65535 || v.PeerCID == ^uint32(0) {
			return bad()
		}
		previous = key
		generations[v.ExportGeneration] = true
		switch v.State {
		case "RELEASED":
			if !storageOfflineCheck.MatchString(v.OfflineCheck) {
				return bad()
			}
			// Legacy completed records may have no peer binding. Never invent one.
		case "PREPARING", "ACTIVE", "QUIESCING":
			if v.PeerCID == 0 || v.OfflineCheck != "" || v.Counters != (StorageOwnershipCounters{}) || liveOwners[v.SandboxID] || peers[v.PeerCID] || ports[v.Port] || uuids[v.FilesystemUUID] {
				return bad()
			}
			liveOwners[v.SandboxID] = true
			peers[v.PeerCID] = true
			ports[v.Port] = true
			uuids[v.FilesystemUUID] = true
		default:
			return bad()
		}
	}
	return nil
}
