package storage

import (
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"sort"
)

// Ownership takes a bounded metadata projection under the store lock. It does
// not acquire the service operation lock, call the backend, reopen state or
// mutate a record. Busy persistence returns an explicit retryable failure.
func (s *Service) Ownership() (protocol.StorageOwnershipReport, *protocol.Error) {
	report, _, failure := s.ownership(false)
	return report, failure
}
func (s *Service) OwnershipWithDescriptors() (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, *protocol.Error) {
	return s.ownership(true)
}
func (s *Service) ownership(withDescriptors bool) (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, *protocol.Error) {
	if !s.store.mu.TryLock() {
		return protocol.StorageOwnershipReport{}, nil, &protocol.Error{Code: "UNAVAILABLE", Message: "storage ownership snapshot is busy", Retryable: true}
	}
	defer s.store.mu.Unlock()
	if len(s.store.data.Exports) > protocol.MaxStorageOwnershipRecords {
		return protocol.StorageOwnershipReport{}, nil, &protocol.Error{Code: "RESOURCE_EXHAUSTED", Message: "storage ownership exceeds the record limit"}
	}
	report := protocol.StorageOwnershipReport{Version: 1, Source: "durable-storage", Exports: make([]protocol.StorageExportOwnership, 0, len(s.store.data.Exports))}
	invalid := func() (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, *protocol.Error) {
		return protocol.StorageOwnershipReport{}, nil, &protocol.Error{Code: "OPERATOR_ACTION", Message: "durable storage ownership metadata is invalid"}
	}
	descriptors := make([]protocol.StorageDescriptorBinding, 0, len(s.store.data.Exports))
	for key, v := range s.store.data.Exports {
		if validateExport(key, v) != nil {
			return invalid()
		}
		report.Exports = append(report.Exports, protocol.StorageExportOwnership{SandboxID: v.SandboxID, SandboxGeneration: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, PeerCID: v.PeerCID, State: v.State, FilesystemUUID: v.FilesystemUUID, SizeBytes: v.SizeBytes, QuotaBytes: v.QuotaBytes, InodeLimit: v.InodeLimit, Port: v.Port, ImageDevice: v.ImageIdentity.Device, ImageInode: v.ImageIdentity.Inode, OfflineCheck: v.OfflineCheck, Counters: protocol.StorageOwnershipCounters{Reads: v.Counters.Reads, ReadBytes: v.Counters.ReadBytes, Writes: v.Counters.Writes, WrittenBytes: v.Counters.WrittenBytes, Flushes: v.Counters.Flushes}})
		if withDescriptors {
			descriptors = append(descriptors, protocol.StorageDescriptorBinding{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, DescriptorSHA256: protocol.StorageDescriptorSHA256(protocol.StorageConfig{Path: v.Path, ImageID: v.ImageID, FilesystemUUID: v.FilesystemUUID, SizeBytes: v.SizeBytes, QuotaBytes: v.QuotaBytes, InodeLimit: v.InodeLimit, Port: v.Port, SHA256: v.SHA256})})
		}
	}
	sort.Slice(report.Exports, func(i, j int) bool {
		a, b := report.Exports[i], report.Exports[j]
		if a.SandboxID == b.SandboxID {
			return a.SandboxGeneration < b.SandboxGeneration
		}
		return a.SandboxID < b.SandboxID
	})
	if report.Validate() != nil {
		return invalid()
	}
	sort.Slice(descriptors, func(i, j int) bool {
		a, b := descriptors[i], descriptors[j]
		if a.SandboxID == b.SandboxID {
			return a.Generation < b.Generation
		}
		return a.SandboxID < b.SandboxID
	})
	return report, descriptors, nil
}
