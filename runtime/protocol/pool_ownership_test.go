package protocol

import "testing"

func TestPoolOwnershipValidation(t *testing.T) {
	r := PoolOwnershipReport{Version: 1, Source: "configured-pool-and-durable-lifecycle", Lifecycle: OwnershipReport{Version: 1, Source: "durable-lifecycle", Sandboxes: []SandboxOwnership{}}, ConfiguredCPUs: []int{8, 15}, ConfiguredMemoryBytes: 8 << 30, MemoryReserveBytes: 1 << 30}
	if r.Validate() != nil {
		t.Fatal("valid pool report refused")
	}
	for _, mutate := range []func(*PoolOwnershipReport){func(r *PoolOwnershipReport) { r.ConfiguredCPUs = nil }, func(r *PoolOwnershipReport) { r.ConfiguredCPUs = []int{8, 8} }, func(r *PoolOwnershipReport) { r.ConfiguredCPUs = []int{-1} }, func(r *PoolOwnershipReport) { r.MemoryReserveBytes = r.ConfiguredMemoryBytes }, func(r *PoolOwnershipReport) { r.Lifecycle.Sandboxes = nil }} {
		copy := r
		mutate(&copy)
		if copy.Validate() == nil {
			t.Fatal("invalid pool report accepted")
		}
	}
}
