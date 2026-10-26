package doctor

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"strings"
	"testing"
)

func networkFixture() (protocol.NetworkExpectationsReport, network.OwnershipReport) {
	owner := sandbox("box-a", "RUNNING")
	e := protocol.NetworkExpectationsReport{Version: 1, Source: "durable-lifecycle-network", Lifecycle: protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: 4, Sandboxes: []protocol.SandboxOwnership{owner}}, Expectations: []protocol.LifecycleNetworkExpectation{{SandboxID: owner.SandboxID, Generation: owner.Generation, Expectation: "required"}}}
	n := network.OwnershipReport{Version: 1, Source: "durable-network", PrimaryBootID: boot, Endpoints: []network.EndpointOwnership{{EndpointGeneration: strings.Repeat("b", 32), SandboxID: owner.SandboxID, SandboxGeneration: owner.Generation, PrimaryBootID: boot, Owner: "runtime", ManagedNamespace: true, State: "READY", DescriptorSHA256: strings.Repeat("c", 64)}}}
	return e, n
}
func networkInputs(e protocol.NetworkExpectationsReport, n network.OwnershipReport, ids []string) NetworkInputs {
	return NetworkInputs{Expectations: func(context.Context) (protocol.NetworkExpectationsReport, error) { return e, nil }, Network: func(context.Context) (network.OwnershipReport, error) { return n, nil }, Kernel: func(context.Context) ([]string, error) { return ids, nil }, Boot: func() (string, error) { return boot, nil }}
}
func TestNetworkDoctorCorrelatesOnlyExplicitStableClaims(t *testing.T) {
	for _, mode := range []string{"matching", "missing", "none", "unexpected", "legacy-unknown", "orphan", "generation", "released", "disconnected", "transition", "boot-missing", "boot-mismatch", "daemon-boot", "duplicate", "unbound-runtime", "unbound-cni", "before-provision"} {
		t.Run(mode, func(t *testing.T) {
			e, n := networkFixture()
			ids := []string{"box-a"}
			status, code := "degraded", ""
			switch mode {
			case "matching":
				status = "consistent"
			case "missing":
				n.Endpoints = []network.EndpointOwnership{}
				code = "NETWORK_ENDPOINT_MISSING"
			case "none":
				e.Expectations[0].Expectation = "none"
				n.Endpoints = []network.EndpointOwnership{}
				status = "consistent"
			case "unexpected":
				e.Expectations[0].Expectation = "none"
				code = "NETWORK_UNEXPECTED_ENDPOINT"
			case "legacy-unknown":
				e.Expectations[0].Expectation = "unknown"
				n.Endpoints = []network.EndpointOwnership{}
				status = "inconclusive"
				code = "NETWORK_EXPECTATION_UNOBSERVED"
			case "orphan":
				e.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				e.Expectations = []protocol.LifecycleNetworkExpectation{}
				ids = []string{}
				code = "NETWORK_ORPHAN_CANDIDATE"
			case "generation":
				n.Endpoints[0].SandboxGeneration = strings.Repeat("f", 32)
				code = "NETWORK_GENERATION_MISMATCH"
			case "released":
				e.Lifecycle.Sandboxes[0].State = "ABSENT"
				ids = []string{}
				code = "NETWORK_AFTER_OWNER_RELEASE"
			case "disconnected":
				n.Endpoints[0].State = "DISCONNECTED"
				code = "NETWORK_RECORDED_STATE_DEGRADED"
			case "transition":
				n.Endpoints[0].State = "DELETING"
				status = "inconclusive"
				code = "NETWORK_TRANSITION_REQUIRES_RECHECK"
			case "boot-missing":
				n.Endpoints[0].PrimaryBootID = ""
				status = "inconclusive"
				code = "NETWORK_ENDPOINT_BOOT_UNOBSERVED"
			case "boot-mismatch":
				n.Endpoints[0].PrimaryBootID = "11111111-1111-4111-8111-111111111111"
				code = "NETWORK_ENDPOINT_BOOT_MISMATCH"
			case "daemon-boot":
				n.PrimaryBootID = "11111111-1111-4111-8111-111111111111"
				status = "inconclusive"
				code = "NETWORK_DAEMON_BOOT_MISMATCH"
			case "duplicate":
				second := n.Endpoints[0]
				second.EndpointGeneration = strings.Repeat("d", 32)
				n.Endpoints = append(n.Endpoints, second)
				code = "NETWORK_MULTIPLE_BOUND_ENDPOINTS"
			case "unbound-runtime":
				e.Expectations[0].Expectation = "none"
				n.Endpoints[0].SandboxID = ""
				n.Endpoints[0].SandboxGeneration = ""
				status = "inconclusive"
				code = "NETWORK_UNBOUND_RUNTIME_ENDPOINT"
			case "unbound-cni":
				e.Expectations[0].Expectation = "none"
				n.Endpoints[0].SandboxID = ""
				n.Endpoints[0].SandboxGeneration = ""
				n.Endpoints[0].Owner = "cni"
				n.Endpoints[0].ManagedNamespace = false
				status = "consistent"
			case "before-provision":
				e.Lifecycle.Sandboxes[0].State = "LOADED"
				n.Endpoints = []network.EndpointOwnership{}
				status = "inconclusive"
				code = "NETWORK_TRANSITION_REQUIRES_RECHECK"
			}
			if e.Validate() != nil || n.Validate() != nil {
				t.Fatal("malformed fixture")
			}
			r := InspectNetwork(t.Context(), networkInputs(e, n, ids), "test")
			if r.Version != 4 || r.Status != status || r.Network == nil || r.NetworkExpectations == nil || !r.BootStable || r.KernelGenerationVerified {
				t.Fatalf("report=%+v", r)
			}
			found := code == ""
			for _, f := range r.Findings {
				found = found || f.Code == code
			}
			if !found {
				t.Fatalf("missing %s: %+v", code, r.Findings)
			}
		})
	}
}
func TestNetworkDoctorUnsupportedUnavailableAndDriftRefusePartialClaims(t *testing.T) {
	for _, mode := range []string{"expectations-unsupported", "network-unsupported", "network-unavailable", "counter-drift", "intent-drift", "boot-drift"} {
		t.Run(mode, func(t *testing.T) {
			e, n := networkFixture()
			in := networkInputs(e, n, []string{"box-a"})
			code := "OBSERVATION_CHANGED"
			calls := 0
			switch mode {
			case "expectations-unsupported":
				code = "NETWORK_EXPECTATIONS_UNSUPPORTED"
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					return protocol.NetworkExpectationsReport{}, daemon.ErrNetworkExpectationsUnsupported
				}
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					t.Fatal("fallback after unsupported expectations")
					return n, nil
				}
			case "network-unsupported":
				code = "NETWORK_OWNERSHIP_UNSUPPORTED"
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					return network.OwnershipReport{}, network.ErrOwnershipUnsupported
				}
			case "network-unavailable":
				code = "NETWORK_OWNERSHIP_UNAVAILABLE"
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					return network.OwnershipReport{}, errors.New("PRIVATE_SECRET")
				}
			case "counter-drift":
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					calls++
					v := n
					v.Endpoints = append([]network.EndpointOwnership{}, n.Endpoints...)
					v.Endpoints[0].Counters.RXPackets = uint64(calls)
					return v, nil
				}
			case "intent-drift":
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					calls++
					v := e
					v.Expectations = append([]protocol.LifecycleNetworkExpectation{}, e.Expectations...)
					if calls == 2 {
						v.Expectations[0].Expectation = "none"
					}
					return v, nil
				}
			case "boot-drift":
				code = "BOOT_CHANGED"
				in.Boot = func() (string, error) {
					calls++
					if calls == 2 {
						return "11111111-1111-4111-8111-111111111111", nil
					}
					return boot, nil
				}
			}
			r := InspectNetwork(t.Context(), in, "test")
			if r.Status != "inconclusive" || r.Network != nil || r.NetworkExpectations != nil || len(r.Findings) != 1 || r.Findings[0].Code != code {
				t.Fatalf("%+v", r)
			}
		})
	}
}
