package doctor

import (
	"context"
	"reflect"
)

// OperationalReport is scoped observation, not health, artifact attestation,
// allocation admission, or a cross-daemon atomic transaction.
type OperationalReport struct {
	Version             int               `json:"version"`
	Scope               string            `json:"scope"`
	Status              string            `json:"status"`
	Revision            string            `json:"revision"`
	BootID              string            `json:"boot_id,omitempty"`
	BootStable          bool              `json:"boot_stable"`
	Unified             *UnifiedReport    `json:"unified,omitempty"`
	Components          *ComponentsReport `json:"components,omitempty"`
	Findings            []Finding         `json:"findings"`
	AdmissionGuaranteed bool              `json:"admission_guaranteed"`
	ArtifactVerified    bool              `json:"artifact_verified"`
	KernelVerified      bool              `json:"kernel_verified"`
	Unobserved          []string          `json:"unobserved"`
}

// InspectOperational repeats both complete v6 resource/pool and v1 build-label
// views under the original caller context. Matching labels do not establish
// compatibility, executable identity or daemon incarnation; same-build restarts
// and other ABA changes can escape detection.
func InspectOperational(ctx context.Context, combined CombinedInputs, pool PoolInputs, builds ComponentInputs, revision string) OperationalReport {
	r := OperationalReport{Version: 7, Scope: "resource-network-pool-and-compiled-component-observations", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"executable-artifact-identity", "daemon-process-incarnation", "component-compatibility", "other-component-builds", "kernel-build-identity", "kernel-incarnation", "cpu-transition-health", "allocator-internal-metadata", "native-storage-health", "native-network-health", "network-descriptor-expectations", "service-readiness", "docker-engine", "containment"}}
	fail := func(code string, findings []Finding) OperationalReport {
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
	var resources [2]UnifiedReport
	var components [2]ComponentsReport
	for n := range 2 {
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED", nil)
		}
		resources[n] = InspectUnified(ctx, combined, pool, revision)
		u := resources[n]
		if u.Combined == nil || u.Pool == nil || !u.BootStable {
			return fail("RESOURCE_OBSERVATION_UNAVAILABLE", u.Findings)
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED", nil)
		}
		components[n] = InspectComponents(ctx, builds, revision)
		c := components[n]
		if c.Runtime == nil || c.Network == nil || !c.BootStable {
			return fail("COMPONENT_BUILD_OBSERVATION_UNAVAILABLE", c.Findings)
		}
		if u.BootID != boot || c.BootID != boot {
			return fail("BOOT_CHANGED", nil)
		}
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED", nil)
	}
	end, err := combined.Boot()
	if err != nil || end != boot {
		return fail("BOOT_CHANGED", nil)
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED", nil)
	}
	r.BootStable = true
	if !reflect.DeepEqual(resources[0], resources[1]) || !reflect.DeepEqual(components[0], components[1]) {
		return fail("OBSERVATION_CHANGED", nil)
	}
	u, c := resources[1], components[1]
	r.Status = "consistent"
	for _, status := range []string{u.Status, c.Status} {
		if status == "degraded" {
			r.Status = "degraded"
		} else if status != "consistent" && r.Status != "degraded" {
			r.Status = "inconclusive"
		}
	}
	r.Findings = append(r.Findings, u.Findings...)
	r.Findings = append(r.Findings, c.Findings...)
	if r.Status != "consistent" {
		// Copy before suppression so the report never edits a component snapshot
		// shared with another observer or retained as an independently read view.
		p := *u.Pool
		p.CPUCandidates = nil
		p.MemoryCandidateUpperBoundBytes = 0
		u.Pool = &p
	}
	r.Unified = &u
	r.Components = &c
	return r
}
