package state

import (
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"sort"
)

// ResourceOwnershipSnapshot projects only allowlisted metadata, with no JSON
// clone of results, labels, paths or credentials. Busy persistence fails closed.
func (s *Store) ResourceOwnershipSnapshot() (protocol.ResourceOwnershipReport, *protocol.Error) {
	fail := func(code, message string) (protocol.ResourceOwnershipReport, *protocol.Error) {
		return protocol.ResourceOwnershipReport{}, &protocol.Error{Code: code, Message: message, Retryable: code == "UNAVAILABLE"}
	}
	if !s.mu.TryLock() {
		return fail("UNAVAILABLE", "resource ownership snapshot is busy")
	}
	defer s.mu.Unlock()
	if len(s.data.Sandboxes) > protocol.MaxOwnershipRecords {
		return fail("RESOURCE_EXHAUSTED", "resource ownership exceeds the record limit")
	}
	r := protocol.ResourceOwnershipReport{Version: 1, Source: "durable-lifecycle-storage", Lifecycle: protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: s.data.Sequence, Sandboxes: make([]protocol.SandboxOwnership, 0, len(s.data.Sandboxes))}, StorageBindings: make([]protocol.LifecycleStorageBinding, 0, len(s.data.Sandboxes))}
	cpus := 0
	for key, sb := range s.data.Sandboxes {
		if key != sb.ID || key != sb.Config.ID {
			return fail("OPERATOR_ACTION", "resource ownership identity is invalid")
		}
		cpus += len(sb.Config.CPUs)
		if cpus > protocol.MaxOwnershipCPUs {
			return fail("RESOURCE_EXHAUSTED", "resource ownership exceeds the CPU limit")
		}
		ids := append([]int{}, sb.Config.CPUs...)
		sort.Ints(ids)
		r.Lifecycle.Sandboxes = append(r.Lifecycle.Sandboxes, protocol.SandboxOwnership{SandboxID: sb.ID, Generation: sb.Generation, State: sb.State, CPUs: ids, MemoryBytes: sb.Config.MemoryBytes, ChildCID: sb.Config.ChildCID, AgentPort: sb.Config.AgentPort})
		binding := protocol.LifecycleStorageBinding{SandboxID: sb.ID, Generation: sb.Generation, Required: sb.Config.Storage != nil}
		if sb.Config.Storage != nil {
			binding.DescriptorSHA256 = protocol.StorageDescriptorSHA256(*sb.Config.Storage)
		}
		if sb.Storage != nil {
			binding.ExportGeneration = sb.Storage.ExportGeneration
		}
		r.StorageBindings = append(r.StorageBindings, binding)
	}
	sort.Slice(r.Lifecycle.Sandboxes, func(i, j int) bool { return r.Lifecycle.Sandboxes[i].SandboxID < r.Lifecycle.Sandboxes[j].SandboxID })
	sort.Slice(r.StorageBindings, func(i, j int) bool { return r.StorageBindings[i].SandboxID < r.StorageBindings[j].SandboxID })
	if r.ValidateLifecycle() != nil {
		return fail("OPERATOR_ACTION", "resource ownership metadata is invalid")
	}
	return r, nil
}
