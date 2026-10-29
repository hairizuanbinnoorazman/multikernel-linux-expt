package doctor

import (
	"context"
	"reflect"
)

// UnifiedReport correlates the existing component contracts. It establishes
// neither an atomic snapshot nor service readiness or allocation admission.
type UnifiedReport struct {
	Version             int         `json:"version"`
	Scope               string      `json:"scope"`
	Status              string      `json:"status"`
	Revision            string      `json:"revision"`
	BootID              string      `json:"boot_id,omitempty"`
	BootStable          bool        `json:"boot_stable"`
	Combined            *Report     `json:"combined,omitempty"`
	Pool                *PoolReport `json:"pool,omitempty"`
	Findings            []Finding   `json:"findings"`
	AdmissionGuaranteed bool        `json:"admission_guaranteed"`
	Unobserved          []string    `json:"unobserved"`
}

// InspectUnified repeats both complete component inspections under the same
// caller context, then compares their full lifecycle (including sequence), boot
// and component snapshots. Callbacks must be read-only and honor ctx. Repeated
// equality detects observed drift, but cannot exclude intervening ABA changes.
func InspectUnified(ctx context.Context, combined CombinedInputs, pool PoolInputs, revision string) UnifiedReport {
	r := UnifiedReport{Version: 6, Scope: "durable-resource-network-native-process-kernel-presence-and-pool-capacity", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"kernel-incarnation", "cpu-transition-health", "allocator-internal-metadata", "native-storage-health", "native-network-health", "network-descriptor-expectations", "service-readiness", "docker-engine", "containment"}}
	fail := func(code string, findings []Finding) UnifiedReport {
		if ctx.Err() != nil {
			code = "DEADLINE_OR_CANCELED"
			findings = nil
		}
		r.Findings = append([]Finding{{Code: code}}, findings...)
		return r
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED", nil)
	}
	boot, err := combined.Boot()
	if err != nil || !bootPattern.MatchString(boot) {
		return fail("BOOT_UNAVAILABLE", nil)
	}
	r.BootID = boot
	var components [2]Report
	var pools [2]PoolReport
	for n := range 2 {
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED", nil)
		}
		components[n] = InspectCombined(ctx, combined, revision)
		c := components[n]
		if c.Resources == nil || c.Ownership == nil || c.NativeProcesses == nil || c.Network == nil || c.NetworkExpectations == nil || !c.BootStable {
			return fail("COMBINED_OBSERVATION_UNAVAILABLE", c.Findings)
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED", nil)
		}
		pools[n] = InspectPool(ctx, pool, revision)
		p := pools[n]
		if p.Ownership == nil || p.Kernel == nil || !p.BootStable {
			return fail("POOL_OBSERVATION_UNAVAILABLE", p.Findings)
		}
		if c.BootID != boot || p.BootID != boot {
			return fail("BOOT_CHANGED", nil)
		}
		if !reflect.DeepEqual(c.Resources.Lifecycle, p.Ownership.Lifecycle) {
			return fail("RESOURCE_POOL_LIFECYCLE_CHANGED", nil)
		}
	}
	end, err := combined.Boot()
	if err != nil || end != boot {
		return fail("BOOT_CHANGED", nil)
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED", nil)
	}
	r.BootStable = true
	if !reflect.DeepEqual(components[0], components[1]) || !reflect.DeepEqual(pools[0], pools[1]) {
		return fail("OBSERVATION_CHANGED", nil)
	}
	c, p := components[1], pools[1]
	r.Status = "consistent"
	// A complete but unavailable native identity must not become a capacity
	// recommendation. Preserve degraded severity over inconclusive severity.
	for _, s := range []string{c.Status, p.Status} {
		if s == "degraded" {
			r.Status = "degraded"
		} else if s != "consistent" && r.Status != "degraded" {
			r.Status = "inconclusive"
		}
	}
	r.Findings = append(r.Findings, c.Findings...)
	r.Findings = append(r.Findings, p.Findings...)
	if r.Status != "consistent" {
		p.CPUCandidates = nil
		p.MemoryCandidateUpperBoundBytes = 0
	}
	r.Combined = &c
	r.Pool = &p
	return r
}
