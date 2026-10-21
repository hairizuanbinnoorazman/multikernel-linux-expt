package protocol

import "errors"

const OwnershipCapability = "ownership-v1"
const MaxOwnershipRecords = 256
const MaxOwnershipCPUs = 4096

// OwnershipReport is a metadata-only view of durable lifecycle ownership.
// It is not a probe of child liveness or proof of kernel containment.
type OwnershipReport struct {
	Version   int                `json:"version"`
	Source    string             `json:"source"`
	Sequence  uint64             `json:"sequence"`
	Sandboxes []SandboxOwnership `json:"sandboxes"`
}

// Never add bundle paths, labels, command lines, credentials or raw errors.
type SandboxOwnership struct {
	SandboxID   string `json:"sandbox_id"`
	Generation  string `json:"generation"`
	State       string `json:"state"`
	CPUs        []int  `json:"cpus"`
	MemoryBytes uint64 `json:"memory_bytes"`
	ChildCID    uint32 `json:"child_cid"`
	AgentPort   uint32 `json:"agent_port"`
}

func (r OwnershipReport) Validate() error {
	if r.Version != 1 || r.Source != "durable-lifecycle" || r.Sandboxes == nil || len(r.Sandboxes) > MaxOwnershipRecords {
		return errors.New("invalid ownership report")
	}
	previous, cpuCount := "", 0
	for _, sb := range r.Sandboxes {
		if !auditSandboxID.MatchString(sb.SandboxID) || sb.SandboxID <= previous || !auditGeneration.MatchString(sb.Generation) || len(sb.CPUs) == 0 || sb.MemoryBytes == 0 || sb.ChildCID < 3 || sb.AgentPort == 0 {
			return errors.New("invalid ownership identity")
		}
		previous = sb.SandboxID
		switch sb.State {
		case "ALLOCATING", "CREATED", "LOADED", "RUNNING", "STOPPING", "STOPPED", "RELEASING", "ABSENT":
		default:
			return errors.New("invalid ownership state")
		}
		last := -1
		for _, cpu := range sb.CPUs {
			cpuCount++
			if cpu < 0 || cpu <= last || cpuCount > MaxOwnershipCPUs {
				return errors.New("invalid ownership CPU inventory")
			}
			last = cpu
		}
	}
	return nil
}
