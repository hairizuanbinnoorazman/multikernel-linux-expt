package protocol

import "errors"

const PoolOwnershipCapability = "pool-ownership-v1"

// PoolOwnershipReport exposes configured admission limits and durable claims,
// never a promise that the kernel allocated a pool or can satisfy a new request.
type PoolOwnershipReport struct {
	Version               int             `json:"version"`
	Source                string          `json:"source"`
	Lifecycle             OwnershipReport `json:"lifecycle"`
	ConfiguredCPUs        []int           `json:"configured_cpus"`
	ConfiguredMemoryBytes uint64          `json:"configured_memory_bytes"`
	MemoryReserveBytes    uint64          `json:"memory_reserve_bytes"`
	Initialized           bool            `json:"initialized"`
}

func (r PoolOwnershipReport) Validate() error {
	if r.Version != 1 || r.Source != "configured-pool-and-durable-lifecycle" || r.Lifecycle.Validate() != nil || r.ConfiguredCPUs == nil || len(r.ConfiguredCPUs) > MaxOwnershipCPUs || r.ConfiguredMemoryBytes == 0 || r.MemoryReserveBytes >= r.ConfiguredMemoryBytes {
		return errors.New("invalid pool ownership report")
	}
	last := -1
	for _, cpu := range r.ConfiguredCPUs {
		if cpu < 0 || cpu <= last {
			return errors.New("invalid configured pool CPU inventory")
		}
		last = cpu
	}
	return nil
}
