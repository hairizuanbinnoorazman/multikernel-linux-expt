package doctor

import (
	"context"
	"reflect"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// HealthInputs use the protected, negotiated read-only APIs. Callbacks must
// honor the shared context. No filesystem writability probe or repair is used.
type HealthInputs struct {
	Journal func(context.Context) (protocol.JournalStatusReport, error)
	Network func(context.Context) (network.HealthReport, error)
}

// HealthReport binds sampled journal and recorded host-network CHECK state to
// the full operational view. It is not service readiness or admission authority.
type HealthReport struct {
	Version             int                           `json:"version"`
	Scope               string                        `json:"scope"`
	Status              string                        `json:"status"`
	Revision            string                        `json:"revision"`
	BootID              string                        `json:"boot_id,omitempty"`
	BootStable          bool                          `json:"boot_stable"`
	Operational         *OperationalReport            `json:"operational,omitempty"`
	Journal             *protocol.JournalStatusReport `json:"journal,omitempty"`
	Network             *network.HealthReport         `json:"network_health,omitempty"`
	Findings            []Finding                     `json:"findings"`
	AdmissionGuaranteed bool                          `json:"admission_guaranteed"`
	Unobserved          []string                      `json:"unobserved"`
}

// InspectHealth brackets two health samples with complete v7 observations under
// the original caller context. Observed drift suppresses all payloads. Matching
// samples cannot exclude same-sequence restarts, transient failures or other ABA.
func InspectHealth(ctx context.Context, combined CombinedInputs, pool PoolInputs, builds ComponentInputs, health HealthInputs, revision string) HealthReport {
	r := HealthReport{Version: 8, Scope: "operational-journal-and-recorded-network-health-observations", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"executable-artifact-identity", "daemon-process-incarnation", "component-compatibility", "kernel-build-identity", "kernel-incarnation", "cpu-transition-health", "allocator-internal-metadata", "native-storage-health", "end-to-end-network-health", "journal-current-writability", "journal-byte-integrity", "service-readiness", "docker-engine", "containment"}}
	fail := func(code string) HealthReport {
		if ctx.Err() != nil {
			code = "DEADLINE_OR_CANCELED"
		}
		r.Findings = []Finding{{Code: code}}
		return r
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	if health.Journal == nil || health.Network == nil {
		return fail("HEALTH_OBSERVATION_UNAVAILABLE")
	}
	var views [2]OperationalReport
	var journals [2]protocol.JournalStatusReport
	var networks [2]network.HealthReport
	for n := range 2 {
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		if n == 0 {
			views[n] = InspectOperational(ctx, combined, pool, builds, revision)
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		if n == 0 && (views[n].Unified == nil || views[n].Components == nil) {
			return fail("OPERATIONAL_OBSERVATION_UNAVAILABLE")
		}
		var err error
		journals[n], err = health.Journal(ctx)
		if err != nil || journals[n].Validate() != nil {
			return fail("JOURNAL_OBSERVATION_UNAVAILABLE")
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		networks[n], err = health.Network(ctx)
		if err != nil || networks[n].Validate() != nil {
			return fail("NETWORK_HEALTH_OBSERVATION_UNAVAILABLE")
		}
		// Freeze callback-owned slices before another callback can mutate them.
		networks[n].Endpoints = append(make([]network.EndpointHealth, 0, len(networks[n].Endpoints)), networks[n].Endpoints...)
		if n == 1 {
			views[n] = InspectOperational(ctx, combined, pool, builds, revision)
		}
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	for _, view := range views {
		if view.Unified == nil || view.Unified.Combined == nil || view.Unified.Combined.Resources == nil || view.Unified.Combined.Network == nil || view.Unified.Pool == nil || view.Components == nil || !view.BootStable {
			return fail("OPERATIONAL_OBSERVATION_UNAVAILABLE")
		}
	}
	if !reflect.DeepEqual(views[0], views[1]) || journals[0] != journals[1] || !reflect.DeepEqual(networks[0], networks[1]) {
		return fail("OBSERVATION_CHANGED")
	}
	v, j, h := views[1], journals[1], networks[1]
	r.BootID = v.BootID
	if h.PrimaryBootID != v.BootID {
		return fail("NETWORK_HEALTH_BOOT_MISMATCH")
	}
	// A repeated health report must cover exactly the independently recorded
	// endpoints, with the same descriptor digests, never only an idle subset.
	endpoints := v.Unified.Combined.Network.Endpoints
	if len(endpoints) != len(h.Endpoints) {
		return fail("NETWORK_HEALTH_OWNERSHIP_MISMATCH")
	}
	for n, e := range endpoints {
		if e.EndpointGeneration != h.Endpoints[n].EndpointGeneration || e.DescriptorSHA256 != h.Endpoints[n].DescriptorSHA256 {
			return fail("NETWORK_HEALTH_OWNERSHIP_MISMATCH")
		}
	}
	if j.Sequence != v.Unified.Combined.Resources.Lifecycle.Sequence {
		return fail("JOURNAL_LIFECYCLE_MISMATCH")
	}
	r.BootStable = true
	r.Status = v.Status
	r.Findings = append(r.Findings, v.Findings...)
	add := func(code, status string) {
		r.Findings = append(r.Findings, Finding{Code: code})
		if status == "degraded" || r.Status != "degraded" {
			r.Status = status
		}
	}
	if j.Status == "failed" {
		add("JOURNAL_FAILED", "degraded")
	} else if j.Status == "pending" {
		add("JOURNAL_PENDING", "inconclusive")
	}
	if h.Status == "degraded" {
		add("RECORDED_NETWORK_CHECK_DEGRADED", "degraded")
	}
	if r.Status != "consistent" {
		// Never edit the original observer's capacity snapshot.
		u := *v.Unified
		p := *u.Pool
		p.CPUCandidates = nil
		p.MemoryCandidateUpperBoundBytes = 0
		u.Pool = &p
		v.Unified = &u
	}
	r.Operational = &v
	r.Journal = &j
	r.Network = &h
	return r
}
