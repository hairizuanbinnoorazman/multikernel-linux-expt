package doctor

import (
	"context"
	"errors"
	"reflect"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type ComponentInputs struct {
	Runtime func(context.Context) (protocol.ComponentBuildReport, error)
	Network func(context.Context) (protocol.ComponentBuildReport, error)
	Boot    func() (string, error)
}
type ComponentsReport struct {
	Version                int                            `json:"version"`
	Scope                  string                         `json:"scope"`
	Status                 string                         `json:"status"`
	Revision               string                         `json:"revision"`
	BootID                 string                         `json:"boot_id,omitempty"`
	BootStable             bool                           `json:"boot_stable"`
	Runtime                *protocol.ComponentBuildReport `json:"runtime,omitempty"`
	Network                *protocol.ComponentBuildReport `json:"network,omitempty"`
	CompiledRevisionsMatch bool                           `json:"compiled_revisions_match"`
	Findings               []Finding                      `json:"findings"`
	ArtifactVerified       bool                           `json:"artifact_verified"`
	KernelVerified         bool                           `json:"kernel_verified"`
	Unobserved             []string                       `json:"unobserved"`
}

// InspectComponents reads only negotiated compiled labels. Repeated equality
// detects observed drift, not process restart/ABA or executable attestation.
// Known differing revisions remain visible; compatibility is not inferred.
func InspectComponents(ctx context.Context, in ComponentInputs, revision string) ComponentsReport {
	r := ComponentsReport{Version: 1, Scope: "compiled-runtime-network-build-self-reports", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"executable-artifact-identity", "daemon-process-incarnation", "component-compatibility", "other-component-builds", "kernel-build-identity", "kernel-incarnation", "service-readiness", "native-storage-health", "native-network-health", "docker-engine", "containment"}}
	fail := func(code string) ComponentsReport {
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
	var runtime, net [2]protocol.ComponentBuildReport
	for n := range 2 {
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		runtime[n], err = in.Runtime(ctx)
		if err != nil || runtime[n].Validate() != nil || runtime[n].Component != "mkruntimed" {
			if errors.Is(err, daemon.ErrComponentBuildUnsupported) {
				return fail("RUNTIME_BUILD_UNSUPPORTED")
			}
			return fail("RUNTIME_BUILD_UNAVAILABLE")
		}
		if ctx.Err() != nil {
			return fail("DEADLINE_OR_CANCELED")
		}
		net[n], err = in.Network(ctx)
		if err != nil || net[n].Validate() != nil || net[n].Component != "mknetd" {
			if errors.Is(err, network.ErrComponentBuildUnsupported) {
				return fail("NETWORK_BUILD_UNSUPPORTED")
			}
			return fail("NETWORK_BUILD_UNAVAILABLE")
		}
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	end, err := in.Boot()
	if err != nil || end != boot {
		return fail("BOOT_CHANGED")
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	r.BootStable = true
	if !reflect.DeepEqual(runtime[0], runtime[1]) || !reflect.DeepEqual(net[0], net[1]) {
		return fail("OBSERVATION_CHANGED")
	}
	r.Runtime = &runtime[1]
	r.Network = &net[1]
	r.Status = "consistent"
	for _, entry := range []struct {
		name  string
		value protocol.ComponentBuildReport
	}{{"RUNTIME", runtime[1]}, {"NETWORK", net[1]}} {
		if entry.value.BuildRevision == "unknown" {
			r.Status = "inconclusive"
			r.Findings = append(r.Findings, Finding{Code: entry.name + "_BUILD_REVISION_UNKNOWN"})
		}
	}
	r.CompiledRevisionsMatch = runtime[1].BuildRevision != "unknown" && runtime[1].BuildRevision == net[1].BuildRevision
	return r
}
