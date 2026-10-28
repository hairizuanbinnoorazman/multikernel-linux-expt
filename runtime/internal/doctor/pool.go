package doctor

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/poolinspect"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
)

type PoolInputs struct {
	Ownership func(context.Context) (protocol.PoolOwnershipReport, error)
	Kernel    func(context.Context) (poolinspect.Snapshot, error)
	Boot      func() (string, error)
}
type PoolReport struct {
	Version                        int                           `json:"version"`
	Scope                          string                        `json:"scope"`
	Status                         string                        `json:"status"`
	Revision                       string                        `json:"revision"`
	BootID                         string                        `json:"boot_id,omitempty"`
	BootStable                     bool                          `json:"boot_stable"`
	Ownership                      *protocol.PoolOwnershipReport `json:"ownership,omitempty"`
	Kernel                         *poolinspect.Snapshot         `json:"kernel,omitempty"`
	Findings                       []Finding                     `json:"findings"`
	CPUCandidates                  []uint64                      `json:"cpu_candidates,omitempty"`
	RemainingConfiguredMemoryBytes uint64                        `json:"remaining_configured_memory_bytes"`
	MemoryCandidateUpperBoundBytes uint64                        `json:"memory_candidate_upper_bound_bytes"`
	AdmissionGuaranteed            bool                          `json:"admission_guaranteed"`
	Unobserved                     []string                      `json:"unobserved"`
}

// InspectPool compares repeated complete observations under one caller budget.
// Candidate quantities are conservative metadata bounds, never a reservation,
// successful allocation promise, health/readiness or incarnation attestation.
func InspectPool(ctx context.Context, in PoolInputs, revision string) PoolReport {
	r := PoolReport{Version: 1, Scope: "configured-durable-and-kernel-pool-capacity", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"kernel-incarnation", "cpu-transition-health", "allocator-internal-metadata", "native-storage-health", "native-network-health", "service-readiness", "docker-engine", "containment"}}
	fail := func(code string) PoolReport {
		if ctx.Err() != nil {
			code = "DEADLINE_OR_CANCELED"
		}
		r.Findings = []Finding{{Code: code}}
		return r
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	boot, err := in.Boot()
	if err != nil || !bootPattern.MatchString(boot) {
		return fail("BOOT_UNAVAILABLE")
	}
	r.BootID = boot
	owners := make([]protocol.PoolOwnershipReport, 2)
	kernels := make([]poolinspect.Snapshot, 2)
	for n := range 2 {
		owners[n], err = in.Ownership(ctx)
		if err != nil || owners[n].Validate() != nil {
			if errors.Is(err, daemon.ErrPoolOwnershipUnsupported) {
				return fail("POOL_OWNERSHIP_UNSUPPORTED")
			}
			return fail("POOL_OWNERSHIP_UNAVAILABLE")
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		kernels[n], err = in.Kernel(ctx)
		if err != nil || kernels[n].Validate() != nil {
			if errors.Is(err, poolinspect.ErrCPUFreeListUnavailable) {
				return fail("KERNEL_CPU_FREE_LIST_UNAVAILABLE")
			}
			return fail("KERNEL_POOL_UNAVAILABLE")
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
	}
	end, err := in.Boot()
	if err != nil || end != boot {
		return fail("BOOT_CHANGED")
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	r.BootStable = true
	if !reflect.DeepEqual(owners[0], owners[1]) || !reflect.DeepEqual(kernels[0], kernels[1]) {
		return fail("OBSERVATION_CHANGED")
	}
	owner, kernel := owners[1], kernels[1]
	r.Ownership = &owner
	r.Kernel = &kernel
	r.Status = "consistent"
	add := func(code string, sb *protocol.SandboxOwnership) {
		finding := Finding{Code: code}
		if sb != nil {
			finding.SandboxID = sb.SandboxID
			finding.Generation = sb.Generation
		}
		for _, old := range r.Findings {
			if old == finding {
				return
			}
		}
		r.Findings = append(r.Findings, finding)
		r.Status = "degraded"
	}
	policy, member, free, claimed := map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}
	for _, cpu := range owner.ConfiguredCPUs {
		policy[uint64(cpu)] = true
		if cpu == 0 {
			add("PRIMARY_CPU_ZERO_IN_CONFIGURED_POOL", nil)
		}
	}
	if len(policy) == 0 {
		add("CPU_POOL_POLICY_UNRESTRICTED", nil)
	}
	for _, cpu := range kernel.CPUs.Members {
		member[cpu] = true
		if cpu == 0 {
			add("PRIMARY_CPU_ZERO_IN_KERNEL_POOL", nil)
		}
		if len(policy) > 0 && !policy[cpu] {
			add("KERNEL_CPU_OUTSIDE_CONFIGURED_POLICY", nil)
		}
	}
	for _, cpu := range kernel.CPUs.Available {
		free[cpu] = true
	}
	usable := owner.ConfiguredMemoryBytes - owner.MemoryReserveBytes
	var claimedMemory uint64
	for _, sb := range owner.Lifecycle.Sandboxes {
		if sb.State == "ABSENT" {
			continue
		}
		for _, cpu := range sb.CPUs {
			id := uint64(cpu)
			if claimed[id] {
				add("OVERLAPPING_CPU_CLAIMS", &sb)
			}
			claimed[id] = true
			if len(policy) > 0 && !policy[id] {
				add("CPU_CLAIM_OUTSIDE_CONFIGURED_POLICY", &sb)
			}
			if !member[id] {
				add("CLAIMED_CPU_MISSING_FROM_KERNEL_POOL", &sb)
			} else if free[id] {
				add("CLAIMED_CPU_REPORTED_FREE", &sb)
			}
		}
		if sb.MemoryBytes > ^uint64(0)-claimedMemory {
			add("MEMORY_CLAIM_OVERFLOW", &sb)
			claimedMemory = ^uint64(0)
		} else {
			claimedMemory += sb.MemoryBytes
		}
	}
	if !owner.Initialized {
		add("DAEMON_POOL_NOT_INITIALIZED", nil)
	}
	for _, cpu := range kernel.CPUs.Members {
		if !free[cpu] && !claimed[cpu] {
			add("UNATTRIBUTED_KERNEL_CPU", nil)
		}
	}
	if claimedMemory > usable {
		add("CONFIGURED_MEMORY_OVERCOMMITTED", nil)
	} else {
		r.RemainingConfiguredMemoryBytes = usable - claimedMemory
	}
	if kernel.Memory.TotalBytes < owner.ConfiguredMemoryBytes {
		add("KERNEL_POOL_BELOW_CONFIGURED_MEMORY", nil)
	}
	if kernel.Memory.AllocatedBytes < claimedMemory {
		add("CLAIMED_MEMORY_MISSING_FROM_KERNEL_POOL", nil)
	} else if kernel.Memory.AllocatedBytes > claimedMemory {
		add("UNATTRIBUTED_KERNEL_MEMORY", nil)
	}
	for _, cpu := range kernel.CPUs.Available {
		if policy[cpu] && !claimed[cpu] {
			r.CPUCandidates = append(r.CPUCandidates, cpu)
		}
	}
	var actual uint64
	if kernel.Memory.AvailableBytes > owner.MemoryReserveBytes {
		actual = kernel.Memory.AvailableBytes - owner.MemoryReserveBytes
	}
	r.MemoryCandidateUpperBoundBytes = min(r.RemainingConfiguredMemoryBytes, actual, kernel.Memory.LargestFreeExtentBytes)
	if r.Status != "consistent" {
		r.CPUCandidates = nil
		r.MemoryCandidateUpperBoundBytes = 0
	}
	return r
}
