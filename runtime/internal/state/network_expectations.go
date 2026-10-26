package state

import (
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"sort"
)

// NetworkExpectationsSnapshot reads only allowlisted fields under one try-lock.
// It neither clones private labels/results nor takes lifecycle mutation locks.
func (s *Store) NetworkExpectationsSnapshot() (protocol.NetworkExpectationsReport, *protocol.Error) {
	fail := func(code, message string) (protocol.NetworkExpectationsReport, *protocol.Error) {
		return protocol.NetworkExpectationsReport{}, &protocol.Error{Code: code, Message: message, Retryable: code == "UNAVAILABLE"}
	}
	if !s.mu.TryLock() {
		return fail("UNAVAILABLE", "network expectations snapshot is busy")
	}
	defer s.mu.Unlock()
	if len(s.data.Sandboxes) > protocol.MaxOwnershipRecords {
		return fail("RESOURCE_EXHAUSTED", "network expectations exceed the record limit")
	}
	r := protocol.NetworkExpectationsReport{Version: 1, Source: "durable-lifecycle-network", Lifecycle: protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: s.data.Sequence, Sandboxes: make([]protocol.SandboxOwnership, 0, len(s.data.Sandboxes))}, Expectations: make([]protocol.LifecycleNetworkExpectation, 0, len(s.data.Sandboxes))}
	cpuCount := 0
	for key, sb := range s.data.Sandboxes {
		if key != sb.ID || key != sb.Config.ID {
			return fail("OPERATOR_ACTION", "network expectation identity is invalid")
		}
		cpuCount += len(sb.Config.CPUs)
		if cpuCount > protocol.MaxOwnershipCPUs {
			return fail("RESOURCE_EXHAUSTED", "network expectations exceed the CPU limit")
		}
		expectation, err := protocol.NetworkExpectation(sb.Config)
		if err != nil {
			return fail("OPERATOR_ACTION", "network expectation metadata is invalid")
		}
		cpus := append([]int{}, sb.Config.CPUs...)
		sort.Ints(cpus)
		r.Lifecycle.Sandboxes = append(r.Lifecycle.Sandboxes, protocol.SandboxOwnership{SandboxID: sb.ID, Generation: sb.Generation, State: sb.State, CPUs: cpus, MemoryBytes: sb.Config.MemoryBytes, ChildCID: sb.Config.ChildCID, AgentPort: sb.Config.AgentPort})
		r.Expectations = append(r.Expectations, protocol.LifecycleNetworkExpectation{SandboxID: sb.ID, Generation: sb.Generation, Expectation: expectation})
	}
	sort.Slice(r.Lifecycle.Sandboxes, func(i, j int) bool { return r.Lifecycle.Sandboxes[i].SandboxID < r.Lifecycle.Sandboxes[j].SandboxID })
	sort.Slice(r.Expectations, func(i, j int) bool { return r.Expectations[i].SandboxID < r.Expectations[j].SandboxID })
	if r.Validate() != nil {
		return fail("OPERATOR_ACTION", "network expectation metadata is invalid")
	}
	return r, nil
}
