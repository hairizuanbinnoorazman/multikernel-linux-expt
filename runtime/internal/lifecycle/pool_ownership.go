package lifecycle

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"sort"
	"time"
)

func (s *Service) PoolOwnershipAvailable() bool {
	return s.poolMemoryBytes > 0 && s.poolMemoryReserve < s.poolMemoryBytes
}

// PoolOwnership serializes configuration, pool-init status and durable claims
// with mutation. Inspection neither initializes a pool nor calls the backend.
func (s *Service) PoolOwnership(ctx context.Context) (protocol.PoolOwnershipReport, *protocol.Error) {
	fail := func(code string) (protocol.PoolOwnershipReport, *protocol.Error) {
		return protocol.PoolOwnershipReport{}, apierr(code, "pool ownership inspection is unavailable", false)
	}
	if ctx.Err() != nil {
		return fail("UNAVAILABLE")
	}
	for !s.global.TryLock() {
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail("UNAVAILABLE")
		case <-timer.C:
		}
	}
	defer s.global.Unlock()
	if ctx.Err() != nil {
		return fail("UNAVAILABLE")
	}
	if !s.PoolOwnershipAvailable() {
		return fail("UNSUPPORTED")
	}
	ownership, failure := s.Ownership()
	if failure != nil {
		return protocol.PoolOwnershipReport{}, failure
	}
	cpus := make([]int, 0, len(s.poolCPUs))
	for cpu := range s.poolCPUs {
		cpus = append(cpus, cpu)
	}
	sort.Ints(cpus)
	report := protocol.PoolOwnershipReport{Version: 1, Source: "configured-pool-and-durable-lifecycle", Lifecycle: ownership, ConfiguredCPUs: cpus, ConfiguredMemoryBytes: s.poolMemoryBytes, MemoryReserveBytes: s.poolMemoryReserve, Initialized: s.poolReady}
	if report.Validate() != nil {
		return fail("OPERATOR_ACTION")
	}
	if ctx.Err() != nil {
		return fail("UNAVAILABLE")
	}
	return report, nil
}
