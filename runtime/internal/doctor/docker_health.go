package doctor

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"sort"

	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DockerHealthScope = "operational-health-and-all-running-child-engine-observations"

type DockerHealthInputs struct {
	Health  func(context.Context) HealthReport
	Targets func(context.Context) ([]EngineTarget, error)
	Engine  func(context.Context, EngineTarget) (inspection.Report, error)
}
type OwnedEngineObservation struct {
	SandboxID   string                           `json:"sandbox_id"`
	Generation  string                           `json:"generation"`
	Observation protocol.DockerEngineObservation `json:"observation"`
}
type DockerHealthReport struct {
	Version                  int                      `json:"version"`
	Scope                    string                   `json:"scope"`
	Status                   string                   `json:"status"`
	Revision                 string                   `json:"revision"`
	BootID                   string                   `json:"boot_id,omitempty"`
	BootStable               bool                     `json:"boot_stable"`
	Health                   *HealthReport            `json:"health,omitempty"`
	Engines                  []OwnedEngineObservation `json:"engines"`
	AllRunningOwnersObserved bool                     `json:"all_running_owners_observed"`
	Findings                 []Finding                `json:"findings"`
	AdmissionGuaranteed      bool                     `json:"admission_guaranteed"`
	Unobserved               []string                 `json:"unobserved"`
}

func dockerLifecycle(h HealthReport) (protocol.OwnershipReport, bool) {
	_, valid := HealthObservationStatus(h)
	if !valid || !h.BootStable || !bootPattern.MatchString(h.BootID) || h.Operational == nil || h.Operational.Unified == nil || h.Operational.Unified.Combined == nil || h.Operational.Unified.Combined.Resources == nil {
		return protocol.OwnershipReport{}, false
	}
	u := h.Operational.Unified
	c := u.Combined
	if u.Pool == nil || u.Pool.Ownership == nil || c.Ownership == nil || !u.BootStable || !c.BootStable || !u.Pool.BootStable || u.BootID != h.BootID || c.BootID != h.BootID || u.Pool.BootID != h.BootID || c.Resources.Validate() != nil || u.Pool.Ownership.Validate() != nil || !reflect.DeepEqual(*c.Ownership, c.Resources.Lifecycle) || !reflect.DeepEqual(u.Pool.Ownership.Lifecycle, c.Resources.Lifecycle) {
		return protocol.OwnershipReport{}, false
	}
	owners := c.Resources.Lifecycle
	return owners, owners.Validate() == nil
}
func freezeHealth(h HealthReport) (HealthReport, bool) {
	data, err := json.Marshal(h)
	if err != nil || len(data) > 8<<20 {
		return HealthReport{}, false
	}
	var copy HealthReport
	if json.Unmarshal(data, &copy) != nil {
		return HealthReport{}, false
	}
	_, ok := dockerLifecycle(copy)
	return copy, ok
}
func dockerUnobserved() []string {
	return []string{"executable-artifact-identity", "daemon-process-incarnation", "component-compatibility", "kernel-build-identity", "kernel-incarnation", "cpu-transition-health", "allocator-internal-metadata", "native-storage-health", "end-to-end-network-health", "journal-current-writability", "journal-byte-integrity", "service-readiness", "docker-engine", "containment"}
}

// InspectDockerHealth requires exact target coverage of all RUNNING lifecycle
// owners. Two engine samples are bracketed by complete v8 observations under
// the original context. No paths, namespace/task IDs, credentials or raw RPC
// errors enter the report. Equal samples are not atomicity or ABA proof.
func InspectDockerHealth(ctx context.Context, in DockerHealthInputs, revision string) DockerHealthReport {
	r := DockerHealthReport{Version: 9, Scope: DockerHealthScope, Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: dockerUnobserved()}
	fail := func(code string) DockerHealthReport {
		if ctx == nil || ctx.Err() != nil {
			code = "DEADLINE_OR_CANCELED"
		}
		r.Status = "inconclusive"
		r.BootStable = false
		r.Health = nil
		r.Engines = nil
		r.AllRunningOwnersObserved = false
		r.Unobserved = dockerUnobserved()
		r.Findings = []Finding{{Code: code}}
		return r
	}
	if ctx == nil || ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	if in.Health == nil || in.Targets == nil || in.Engine == nil {
		return fail("DOCKER_HEALTH_OBSERVATION_UNAVAILABLE")
	}
	targets, err := in.Targets(ctx)
	if err != nil || ctx.Err() != nil {
		return fail("ENGINE_TARGETS_UNAVAILABLE")
	}
	if targets == nil || len(targets) > protocol.MaxOwnershipRecords {
		return fail("ENGINE_TARGETS_INVALID")
	}
	targets = append([]EngineTarget{}, targets...)
	sort.Slice(targets, func(i, j int) bool { return targets[i].Owner.SandboxID < targets[j].Owner.SandboxID })
	for i, t := range targets {
		if !t.valid() || i > 0 && targets[i-1].Owner.SandboxID == t.Owner.SandboxID {
			return fail("ENGINE_TARGETS_INVALID")
		}
	}
	before, ok := freezeHealth(in.Health(ctx))
	if !ok || ctx.Err() != nil {
		return fail("HEALTH_OBSERVATION_UNAVAILABLE")
	}
	r.BootID = before.BootID
	lifecycle, _ := dockerLifecycle(before)
	var running []protocol.SandboxOwnership
	for _, owner := range lifecycle.Sandboxes {
		switch owner.State {
		case "RUNNING":
			running = append(running, owner)
		case "ABSENT":
		default:
			return fail("ENGINE_OWNER_NOT_RUNNING")
		}
	}
	if len(targets) != len(running) {
		return fail("ENGINE_OWNER_COVERAGE_MISMATCH")
	}
	for i, owner := range running {
		if targets[i].Owner.SandboxID != owner.SandboxID || targets[i].Owner.Generation != owner.Generation {
			return fail("ENGINE_OWNER_COVERAGE_MISMATCH")
		}
	}
	samples := [2][]OwnedEngineObservation{}
	for n := range 2 {
		samples[n] = make([]OwnedEngineObservation, 0, len(targets))
		for _, target := range targets {
			if ctx.Err() != nil {
				return fail("DEADLINE_OR_CANCELED")
			}
			observed, err := in.Engine(ctx, target)
			if err != nil {
				if status.Code(err) == codes.Unimplemented {
					return fail("DOCKER_ENGINE_OBSERVATION_UNSUPPORTED")
				}
				return fail("DOCKER_ENGINE_OBSERVATION_UNAVAILABLE")
			}
			if ctx.Err() != nil {
				return fail("DEADLINE_OR_CANCELED")
			}
			if observed.Version != 1 || observed.Owner != target.Owner || observed.Observation.Validate() != nil {
				return fail("DOCKER_ENGINE_OBSERVATION_INVALID")
			}
			samples[n] = append(samples[n], OwnedEngineObservation{SandboxID: target.Owner.SandboxID, Generation: target.Owner.Generation, Observation: observed.Observation})
		}
	}
	after, ok := freezeHealth(in.Health(ctx))
	if !ok || ctx.Err() != nil {
		return fail("HEALTH_OBSERVATION_UNAVAILABLE")
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(samples[0], samples[1]) {
		return fail("OBSERVATION_CHANGED")
	}
	r.BootStable = true
	r.AllRunningOwnersObserved = true
	r.Health = &after
	r.Engines = samples[1]
	r.Status = after.Status
	r.Findings = append(r.Findings, after.Findings...)
	r.Unobserved = slices.DeleteFunc(r.Unobserved, func(s string) bool { return s == "docker-engine" })
	for _, engine := range r.Engines {
		if engine.Observation.Status != "responding" {
			r.Status = "degraded"
			r.Findings = append(r.Findings, Finding{Code: "DOCKER_ENGINE_NOT_RESPONDING", SandboxID: engine.SandboxID, Generation: engine.Generation})
		}
	}
	if r.Status != "consistent" {
		// after is a frozen owned copy. Suppress usable capacity on aggregate failure
		// without mutating any shared observer or upgrading the v8 component status.
		pool := r.Health.Operational.Unified.Pool
		pool.CPUCandidates = nil
		pool.MemoryCandidateUpperBoundBytes = 0
	}
	if _, valid := DockerHealthObservationStatus(r); !valid {
		return fail("DOCKER_ENGINE_OBSERVATION_INVALID")
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	return r
}

// DockerHealthObservationStatus validates the typed aggregate publication
// contract. Failure reports have no partial payload; complete reports must cover
// every running owner exactly and cannot advertise capacity on degradation.
func DockerHealthObservationStatus(r DockerHealthReport) (string, bool) {
	if r.Version != 9 || r.Scope != DockerHealthScope || r.AdmissionGuaranteed || r.BootID != "" && !bootPattern.MatchString(r.BootID) {
		return "", false
	}
	if !r.BootStable || !r.AllRunningOwnersObserved || r.Health == nil {
		if r.BootStable || r.AllRunningOwnersObserved || r.Health != nil || r.Engines != nil || r.Status != "inconclusive" || !reflect.DeepEqual(r.Unobserved, dockerUnobserved()) || len(r.Findings) != 1 {
			return "", false
		}
		finding := r.Findings[0]
		if finding != (Finding{Code: finding.Code}) {
			return "", false
		}
		switch finding.Code {
		case "DEADLINE_OR_CANCELED", "DOCKER_HEALTH_OBSERVATION_UNAVAILABLE", "ENGINE_TARGETS_UNAVAILABLE", "ENGINE_TARGETS_INVALID", "HEALTH_OBSERVATION_UNAVAILABLE", "ENGINE_OWNER_NOT_RUNNING", "ENGINE_OWNER_COVERAGE_MISMATCH", "DOCKER_ENGINE_OBSERVATION_UNSUPPORTED", "DOCKER_ENGINE_OBSERVATION_UNAVAILABLE", "DOCKER_ENGINE_OBSERVATION_INVALID", "OBSERVATION_CHANGED":
			return r.Status, true
		default:
			return "", false
		}
	}
	owners, valid := dockerLifecycle(*r.Health)
	if !valid || r.BootID != r.Health.BootID || r.Engines == nil {
		return "", false
	}
	wantUnobserved := slices.DeleteFunc(dockerUnobserved(), func(s string) bool { return s == "docker-engine" })
	if !reflect.DeepEqual(r.Unobserved, wantUnobserved) {
		return "", false
	}
	status := r.Health.Status
	findings := append([]Finding{}, r.Health.Findings...)
	index := 0
	for _, owner := range owners.Sandboxes {
		if owner.State == "ABSENT" {
			continue
		}
		if owner.State != "RUNNING" || index >= len(r.Engines) {
			return "", false
		}
		e := r.Engines[index]
		index++
		if e.SandboxID != owner.SandboxID || e.Generation != owner.Generation || e.Observation.Validate() != nil {
			return "", false
		}
		if e.Observation.Status != "responding" {
			status = "degraded"
			findings = append(findings, Finding{Code: "DOCKER_ENGINE_NOT_RESPONDING", SandboxID: e.SandboxID, Generation: e.Generation})
		}
	}
	if index != len(r.Engines) || status != r.Status || !reflect.DeepEqual(findings, r.Findings) {
		return "", false
	}
	pool := r.Health.Operational.Unified.Pool
	if status != "consistent" && (len(pool.CPUCandidates) != 0 || pool.MemoryCandidateUpperBoundBytes != 0) {
		return "", false
	}
	return status, true
}
