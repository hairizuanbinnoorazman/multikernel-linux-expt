package protocol

import (
	"strings"
	"testing"
)

func storageReportFixture() StorageOwnershipReport {
	return StorageOwnershipReport{Version: 1, Source: "durable-storage", Exports: []StorageExportOwnership{{SandboxID: "box", SandboxGeneration: strings.Repeat("a", 32), ExportGeneration: strings.Repeat("b", 32), PeerCID: 40, State: "ACTIVE", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, ImageDevice: 1, ImageInode: 2}}}
}
func TestStorageOwnershipStatesAndLegacyCompletedPeer(t *testing.T) {
	for _, state := range []string{"PREPARING", "ACTIVE", "QUIESCING", "RELEASED"} {
		r := storageReportFixture()
		r.Exports[0].State = state
		if state == "RELEASED" {
			r.Exports[0].PeerCID = 0
			r.Exports[0].OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("c", 64)
			r.Exports[0].Counters.Writes = 9
		}
		if err := r.Validate(); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
	}
}
func TestStorageOwnershipRejectsAmbiguousOrUnboundedMetadata(t *testing.T) {
	for _, mode := range []string{"version", "source", "nil", "limit", "id", "generation", "export-generation", "uuid", "peer", "reserved-peer", "state", "size", "quota", "inode", "port", "identity", "live-check", "live-counters", "released-check", "duplicate", "duplicate-export-generation", "live-owner", "live-port", "live-peer", "live-uuid", "order"} {
		t.Run(mode, func(t *testing.T) {
			r := storageReportFixture()
			v := &r.Exports[0]
			switch mode {
			case "version":
				r.Version = 2
			case "source":
				r.Source = "private-secret"
			case "nil":
				r.Exports = nil
			case "limit":
				r.Exports = make([]StorageExportOwnership, 257)
			case "id":
				v.SandboxID = "../private-secret"
			case "generation":
				v.SandboxGeneration = "private-secret"
			case "export-generation":
				v.ExportGeneration = "private-secret"
			case "uuid":
				v.FilesystemUUID = "private-secret"
			case "peer":
				v.PeerCID = 0
			case "reserved-peer":
				v.PeerCID = ^uint32(0)
			case "state":
				v.State = "private-secret"
			case "size":
				v.SizeBytes = 16<<30 + 4096
			case "quota":
				v.QuotaBytes--
			case "inode":
				v.InodeLimit = 1
			case "port":
				v.Port = 1
			case "identity":
				v.ImageInode = 0
			case "live-check":
				v.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("c", 64)
			case "live-counters":
				v.Counters.Writes = 1
			case "released-check":
				v.State = "RELEASED"
			default:
				other := *v
				other.SandboxID = "box-z"
				other.ExportGeneration = strings.Repeat("d", 32)
				other.PeerCID = 41
				other.Port = 4062
				other.FilesystemUUID = "22222222-2222-4333-8444-555555555555"
				switch mode {
				case "duplicate":
					other = *v
				case "duplicate-export-generation":
					other.ExportGeneration = v.ExportGeneration
				case "live-owner":
					other.SandboxID = v.SandboxID
					other.SandboxGeneration = strings.Repeat("c", 32)
				case "live-port":
					other.Port = v.Port
				case "live-peer":
					other.PeerCID = v.PeerCID
				case "live-uuid":
					other.FilesystemUUID = v.FilesystemUUID
				case "order":
					other.SandboxID = "aaa"
				}
				r.Exports = append(r.Exports, other)
			}
			if err := r.Validate(); err == nil || strings.Contains(err.Error(), "private-secret") {
				t.Fatalf("invalid report accepted or reflected: %v", err)
			}
		})
	}
}
