package doctor

import (
	"context"
	"errors"
	"reflect"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type NetworkInputs struct {
	Expectations func(context.Context) (protocol.NetworkExpectationsReport, error)
	Network      func(context.Context) (network.OwnershipReport, error)
	Kernel       func(context.Context) ([]string, error)
	Boot         func() (string, error)
}

// InspectNetwork brackets kernel/boot observations with repeated complete
// expectation/endpoint claims. Equality is sampled, not cross-daemon atomicity.
func InspectNetwork(ctx context.Context, in NetworkInputs, revision string) Report {
	type pair struct {
		expected  protocol.NetworkExpectationsReport
		endpoints network.OwnershipReport
	}
	var records []pair
	failure := ""
	r := Inspect(ctx, Inputs{Ownership: func(ctx context.Context) (protocol.OwnershipReport, error) {
		expected, err := in.Expectations(ctx)
		if err != nil || expected.Validate() != nil {
			failure = "NETWORK_EXPECTATIONS_UNAVAILABLE"
			if errors.Is(err, daemon.ErrNetworkExpectationsUnsupported) {
				failure = "NETWORK_EXPECTATIONS_UNSUPPORTED"
			}
			return protocol.OwnershipReport{}, errors.New("network expectation inspection failed")
		}
		endpoints, err := in.Network(ctx)
		if err != nil || endpoints.Validate() != nil {
			failure = "NETWORK_OWNERSHIP_UNAVAILABLE"
			if errors.Is(err, network.ErrOwnershipUnsupported) {
				failure = "NETWORK_OWNERSHIP_UNSUPPORTED"
			}
			return protocol.OwnershipReport{}, errors.New("network ownership inspection failed")
		}
		records = append(records, pair{expected, endpoints})
		return expected.Lifecycle, nil
	}, Kernel: in.Kernel, Boot: in.Boot}, revision)
	r.Version = 4
	r.Scope = "durable-lifecycle-network-ownership-and-kernel-presence"
	r.Unobserved = []string{"kernel-incarnation", "native-network-health", "network-descriptor-expectations", "storage-exports", "pool-capacity", "service-readiness", "docker-engine", "containment"}
	if failure != "" {
		r.Findings = []Finding{{Code: failure}}
		return r
	}
	if r.Ownership == nil {
		return r
	}
	if len(records) != 2 || !reflect.DeepEqual(records[0], records[1]) {
		r.Status = "inconclusive"
		r.Ownership = nil
		r.KernelInstances = nil
		r.Findings = []Finding{{Code: "OBSERVATION_CHANGED"}}
		return r
	}
	r.Network = &records[1].endpoints
	r.NetworkExpectations = &records[1].expected.Expectations
	correlateNetwork(&r)
	return r
}

func correlateNetwork(r *Report) {
	add := func(code, id, generation, endpoint, expected, status string) {
		r.Findings = append(r.Findings, Finding{Code: code, SandboxID: id, Generation: generation, EndpointGeneration: endpoint, ExpectedGeneration: expected})
		if status == "degraded" || r.Status != "degraded" {
			r.Status = status
		}
	}
	if r.Network.PrimaryBootID != r.BootID {
		add("NETWORK_DAEMON_BOOT_MISMATCH", "", "", "", "", "inconclusive")
		return
	}
	owners := map[string]protocol.SandboxOwnership{}
	intent := map[string]string{}
	counts := map[string]int{}
	for i, owner := range r.Ownership.Sandboxes {
		owners[owner.SandboxID] = owner
		intent[owner.SandboxID] = (*r.NetworkExpectations)[i].Expectation
	}
	for _, e := range r.Network.Endpoints {
		if e.PrimaryBootID == "" {
			add("NETWORK_ENDPOINT_BOOT_UNOBSERVED", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "inconclusive")
		} else if e.PrimaryBootID != r.BootID {
			add("NETWORK_ENDPOINT_BOOT_MISMATCH", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "degraded")
		}
		if e.SandboxID == "" {
			if e.Owner == "runtime" {
				add("NETWORK_UNBOUND_RUNTIME_ENDPOINT", "", "", e.EndpointGeneration, "", "inconclusive")
			}
			continue
		}
		owner, ok := owners[e.SandboxID]
		if !ok {
			add("NETWORK_ORPHAN_CANDIDATE", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "degraded")
			continue
		}
		if e.SandboxGeneration != owner.Generation {
			add("NETWORK_GENERATION_MISMATCH", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, owner.Generation, "degraded")
			continue
		}
		counts[e.SandboxID]++
		if owner.State == "ABSENT" {
			add("NETWORK_AFTER_OWNER_RELEASE", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "degraded")
			continue
		}
		if intent[e.SandboxID] == "none" {
			add("NETWORK_UNEXPECTED_ENDPOINT", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "degraded")
		}
		switch e.State {
		case "DEGRADED", "DISCONNECTED":
			add("NETWORK_RECORDED_STATE_DEGRADED", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "degraded")
		case "ALLOCATING", "DELETING":
			add("NETWORK_TRANSITION_REQUIRES_RECHECK", e.SandboxID, e.SandboxGeneration, e.EndpointGeneration, "", "inconclusive")
		}
	}
	for _, owner := range r.Ownership.Sandboxes {
		if owner.State == "ABSENT" {
			continue
		}
		switch intent[owner.SandboxID] {
		case "unknown":
			add("NETWORK_EXPECTATION_UNOBSERVED", owner.SandboxID, owner.Generation, "", "", "inconclusive")
		case "required":
			if counts[owner.SandboxID] == 0 {
				status, code := "degraded", "NETWORK_ENDPOINT_MISSING"
				if owner.State != "RUNNING" {
					status = "inconclusive"
					code = "NETWORK_TRANSITION_REQUIRES_RECHECK"
				}
				add(code, owner.SandboxID, owner.Generation, "", "", status)
			}
		}
		if counts[owner.SandboxID] > 1 {
			add("NETWORK_MULTIPLE_BOUND_ENDPOINTS", owner.SandboxID, owner.Generation, "", "", "degraded")
		}
	}
}
