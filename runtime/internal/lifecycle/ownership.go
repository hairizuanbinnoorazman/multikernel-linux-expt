package lifecycle

import (
	"sort"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Ownership takes one snapshot without consulting or mutating the backend.
// Fail closed instead of returning a truncated inventory as a complete view.
func (s *Service) Ownership() (protocol.OwnershipReport, *protocol.Error) {
	snapshot := s.store.Snapshot()
	if len(snapshot.Sandboxes) > protocol.MaxOwnershipRecords {
		return protocol.OwnershipReport{}, apierr("RESOURCE_EXHAUSTED", "ownership inventory exceeds the record limit", false)
	}
	report := protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: snapshot.Sequence, Sandboxes: make([]protocol.SandboxOwnership, 0, len(snapshot.Sandboxes))}
	cpuCount := 0
	for key, sb := range snapshot.Sandboxes {
		if key != sb.ID || sb.Config.ID != key {
			return protocol.OwnershipReport{}, apierr("OPERATOR_ACTION", "durable ownership identity is inconsistent", false)
		}
		cpuCount += len(sb.Config.CPUs)
		if cpuCount > protocol.MaxOwnershipCPUs {
			return protocol.OwnershipReport{}, apierr("RESOURCE_EXHAUSTED", "ownership inventory exceeds the CPU limit", false)
		}
		cpus := append([]int{}, sb.Config.CPUs...)
		sort.Ints(cpus)
		report.Sandboxes = append(report.Sandboxes, protocol.SandboxOwnership{SandboxID: sb.ID, Generation: sb.Generation, State: sb.State, CPUs: cpus, MemoryBytes: sb.Config.MemoryBytes, ChildCID: sb.Config.ChildCID, AgentPort: sb.Config.AgentPort})
	}
	sort.Slice(report.Sandboxes, func(i, j int) bool { return report.Sandboxes[i].SandboxID < report.Sandboxes[j].SandboxID })
	if report.Validate() != nil {
		return protocol.OwnershipReport{}, apierr("OPERATOR_ACTION", "durable ownership metadata is invalid", false)
	}
	return report, nil
}
