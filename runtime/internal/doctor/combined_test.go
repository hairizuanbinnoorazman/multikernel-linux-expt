package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func combinedInputs(native protocol.NativeResourceReport, expected protocol.NetworkExpectationsReport, endpoints network.OwnershipReport, ids []string) CombinedInputs {
	return CombinedInputs{
		Native:       func(context.Context) (protocol.NativeResourceReport, error) { return native, nil },
		Expectations: func(context.Context) (protocol.NetworkExpectationsReport, error) { return expected, nil },
		Network:      func(context.Context) (network.OwnershipReport, error) { return endpoints, nil },
		Kernel:       func(context.Context) ([]string, error) { return ids, nil },
		Boot:         func() (string, error) { return boot, nil },
	}
}

func combinedHas(t *testing.T, report Report, codes ...string) {
	t.Helper()
	for _, code := range codes {
		found := false
		for _, finding := range report.Findings {
			found = found || finding.Code == code
		}
		if !found {
			t.Fatalf("missing %s: %+v", code, report)
		}
	}
}

func TestCombinedDoctorCorrelatesBothComponentsAndPreservesDegradedSeverity(t *testing.T) {
	for _, mode := range []string{"matched", "empty", "released", "storage-mismatch", "network-missing", "native-absent", "both-degraded", "degraded-plus-transition", "native-unavailable", "native-unavailable-network-degraded", "endpoint-boot-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			native := nativeFixture()
			expected, endpoints := networkFixture()
			ids := []string{"box-a"}
			status := "consistent"
			var codes []string
			switch mode {
			case "empty", "released":
				native.Resources.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				native.Resources.StorageBindings = []protocol.LifecycleStorageBinding{}
				if mode == "empty" {
					native.Resources.Storage.Exports = []protocol.StorageExportOwnership{}
					native.Resources.StorageDescriptors = []protocol.StorageDescriptorBinding{}
					native.Processes = []protocol.StorageProcessIdentity{}
				} else {
					native.Resources.Storage.Exports[0].State = "RELEASED"
					native.Resources.Storage.Exports[0].OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("a", 64)
					native.Processes[0].Status = "not-probed-released"
				}
				expected.Lifecycle = native.Resources.Lifecycle
				expected.Expectations = []protocol.LifecycleNetworkExpectation{}
				endpoints.Endpoints = []network.EndpointOwnership{}
				ids = []string{}
			case "storage-mismatch":
				native.Resources.StorageBindings[0].DescriptorSHA256 = strings.Repeat("e", 64)
				status = "degraded"
				codes = []string{"STORAGE_DESCRIPTOR_MISMATCH"}
			case "network-missing":
				endpoints.Endpoints = []network.EndpointOwnership{}
				status = "degraded"
				codes = []string{"NETWORK_ENDPOINT_MISSING"}
			case "native-absent", "both-degraded", "degraded-plus-transition":
				native.Processes[0].Status = "process-absent"
				status = "degraded"
				codes = []string{"NATIVE_STORAGE_PROCESS_ABSENT"}
				if mode == "both-degraded" {
					endpoints.Endpoints = []network.EndpointOwnership{}
					codes = append(codes, "NETWORK_ENDPOINT_MISSING")
				}
				if mode == "degraded-plus-transition" {
					endpoints.Endpoints[0].State = "DELETING"
					codes = append(codes, "NETWORK_TRANSITION_REQUIRES_RECHECK")
				}
			case "native-unavailable", "native-unavailable-network-degraded":
				native.Processes[0].Status = "unavailable"
				status = "inconclusive"
				codes = []string{"NATIVE_STORAGE_IDENTITY_UNAVAILABLE"}
				if mode == "native-unavailable-network-degraded" {
					endpoints.Endpoints = []network.EndpointOwnership{}
					status = "degraded"
					codes = append(codes, "NETWORK_ENDPOINT_MISSING")
				}
			case "endpoint-boot-mismatch":
				endpoints.Endpoints[0].PrimaryBootID = "11111111-1111-4111-8111-111111111111"
				status = "degraded"
				codes = []string{"NETWORK_ENDPOINT_BOOT_MISMATCH"}
			}
			r := InspectCombined(t.Context(), combinedInputs(native, expected, endpoints, ids), "revision")
			if r.Version != 5 || r.Status != status || r.Scope != "durable-resource-network-ownership-native-process-identity-and-kernel-presence" || !r.BootStable || r.KernelGenerationVerified || r.Resources == nil || r.NativeProcesses == nil || r.Network == nil || r.NetworkExpectations == nil {
				t.Fatalf("report=%+v", r)
			}
			if !reflect.DeepEqual(*r.Resources, native.Resources) || !reflect.DeepEqual(*r.Network, endpoints) {
				t.Fatal("lost component claims")
			}
			combinedHas(t, r, codes...)
			if mode == "matched" && len(r.Findings) != 0 {
				t.Fatal(r.Findings)
			}
			for _, missing := range []string{"kernel-incarnation", "native-storage-health", "native-network-health", "network-descriptor-expectations", "pool-capacity", "service-readiness", "docker-engine", "containment"} {
				found := false
				for _, item := range r.Unobserved {
					found = found || item == missing
				}
				if !found {
					t.Fatalf("claimed %s", missing)
				}
			}
		})
	}
}

func TestCombinedDoctorRefusesPartialUnsupportedInvalidOrDriftingViews(t *testing.T) {
	for _, mode := range []string{"native-unsupported", "native-invalid", "expectations-unsupported", "expectations-unavailable", "expectations-invalid", "network-unsupported", "network-unavailable", "network-invalid", "lifecycle-sequence", "lifecycle-generation", "network-counter-drift", "intent-drift", "native-drift", "boot-change", "kernel-change"} {
		t.Run(mode, func(t *testing.T) {
			native := nativeFixture()
			expected, endpoints := networkFixture()
			in := combinedInputs(native, expected, endpoints, []string{"box-a"})
			code := "OBSERVATION_CHANGED"
			calls := 0
			switch mode {
			case "native-unsupported":
				code = "NATIVE_RESOURCE_IDENTITY_UNSUPPORTED"
				in.Native = func(context.Context) (protocol.NativeResourceReport, error) {
					return native, daemon.ErrNativeResourcesUnsupported
				}
			case "native-invalid":
				code = "NATIVE_RESOURCE_IDENTITY_UNAVAILABLE"
				native.Version = 0
				in.Native = func(context.Context) (protocol.NativeResourceReport, error) { return native, nil }
			case "expectations-unsupported":
				code = "NETWORK_EXPECTATIONS_UNSUPPORTED"
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					return expected, daemon.ErrNetworkExpectationsUnsupported
				}
			case "expectations-unavailable":
				code = "NETWORK_EXPECTATIONS_UNAVAILABLE"
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					return expected, errors.New("PRIVATE_SECRET")
				}
			case "expectations-invalid":
				code = "NETWORK_EXPECTATIONS_UNAVAILABLE"
				expected.Version = 0
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) { return expected, nil }
			case "network-unsupported":
				code = "NETWORK_OWNERSHIP_UNSUPPORTED"
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					return endpoints, network.ErrOwnershipUnsupported
				}
			case "network-unavailable":
				code = "NETWORK_OWNERSHIP_UNAVAILABLE"
				in.Network = func(context.Context) (network.OwnershipReport, error) { return endpoints, errors.New("PRIVATE_SECRET") }
			case "network-invalid":
				code = "NETWORK_OWNERSHIP_UNAVAILABLE"
				endpoints.Version = 0
				in.Network = func(context.Context) (network.OwnershipReport, error) { return endpoints, nil }
			case "lifecycle-sequence", "lifecycle-generation":
				code = "RESOURCE_NETWORK_LIFECYCLE_CHANGED"
				if mode == "lifecycle-sequence" {
					expected.Lifecycle.Sequence++
				} else {
					expected.Lifecycle.Sandboxes[0].Generation = strings.Repeat("e", 32)
					expected.Expectations[0].Generation = strings.Repeat("e", 32)
				}
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) { return expected, nil }
			case "network-counter-drift":
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					calls++
					v := endpoints
					v.Endpoints = append([]network.EndpointOwnership{}, endpoints.Endpoints...)
					v.Endpoints[0].Counters.RXPackets = uint64(calls)
					return v, nil
				}
			case "intent-drift":
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					calls++
					v := expected
					v.Expectations = append([]protocol.LifecycleNetworkExpectation{}, expected.Expectations...)
					if calls == 2 {
						v.Expectations[0].Expectation = "none"
					}
					return v, nil
				}
			case "native-drift":
				in.Native = func(context.Context) (protocol.NativeResourceReport, error) {
					calls++
					v := native
					v.Processes = append([]protocol.StorageProcessIdentity{}, native.Processes...)
					if calls == 2 {
						v.Processes[0].Status = "process-absent"
					}
					return v, nil
				}
			case "boot-change":
				code = "BOOT_CHANGED"
				in.Boot = func() (string, error) {
					calls++
					if calls == 2 {
						return "11111111-1111-4111-8111-111111111111", nil
					}
					return boot, nil
				}
			case "kernel-change":
				in.Kernel = func(context.Context) ([]string, error) {
					calls++
					if calls == 2 {
						return []string{}, nil
					}
					return []string{"box-a"}, nil
				}
			}
			// Unsupported/malformed earlier components cannot trigger later reads.
			if strings.HasPrefix(mode, "native-") && mode != "native-drift" {
				in.Expectations = func(context.Context) (protocol.NetworkExpectationsReport, error) {
					t.Fatal("read after native refusal")
					return expected, nil
				}
			}
			if strings.HasPrefix(mode, "expectations-") || strings.HasPrefix(mode, "lifecycle-") {
				in.Network = func(context.Context) (network.OwnershipReport, error) {
					t.Fatal("read after expectation refusal")
					return endpoints, nil
				}
			}
			r := InspectCombined(t.Context(), in, "revision")
			if r.Status != "inconclusive" || r.Ownership != nil || r.Resources != nil || r.NativeProcesses != nil || r.Network != nil || r.NetworkExpectations != nil || len(r.KernelInstances) != 0 {
				t.Fatalf("partial success: %+v", r)
			}
			combinedHas(t, r, code)
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "PRIVATE_SECRET") {
				t.Fatal("private error leaked")
			}
		})
	}
}

func TestCombinedDoctorSharesAbsoluteDeadlineAndStopsAfterCancellation(t *testing.T) {
	for _, phase := range []string{"before", "native", "expectations", "network", "complete"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			deadline, _ := ctx.Deadline()
			native := nativeFixture()
			expected, endpoints := networkFixture()
			in := combinedInputs(native, expected, endpoints, []string{"box-a"})
			var reads []string
			check := func(got context.Context, name string) {
				t.Helper()
				d, ok := got.Deadline()
				if !ok || d != deadline {
					t.Fatal("deadline renewed")
				}
				if got.Err() != nil {
					t.Fatal("read after cancellation")
				}
				reads = append(reads, name)
				if phase == name {
					cancel()
				}
			}
			in.Native = func(c context.Context) (protocol.NativeResourceReport, error) { check(c, "native"); return native, nil }
			in.Expectations = func(c context.Context) (protocol.NetworkExpectationsReport, error) {
				check(c, "expectations")
				return expected, nil
			}
			in.Network = func(c context.Context) (network.OwnershipReport, error) { check(c, "network"); return endpoints, nil }
			if phase == "before" {
				cancel()
			}
			r := InspectCombined(ctx, in, "revision")
			if phase == "complete" {
				if r.Status != "consistent" || r.Network == nil || r.Resources == nil {
					t.Fatal(r)
				}
			} else {
				if r.Status != "inconclusive" || r.Network != nil || r.Resources != nil {
					t.Fatal(r)
				}
				combinedHas(t, r, "DEADLINE_OR_CANCELED")
			}
			want := map[string]int{"before": 0, "native": 1, "expectations": 2, "network": 3, "complete": 6}[phase]
			if len(reads) != want {
				t.Fatalf("reads=%v", reads)
			}
		})
	}
}

func TestCombinedDoctorBlockedNetworkReadHonorsOriginalBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	native := nativeFixture()
	expected, endpoints := networkFixture()
	in := combinedInputs(native, expected, endpoints, []string{"box-a"})
	in.Network = func(c context.Context) (network.OwnershipReport, error) {
		<-c.Done()
		return network.OwnershipReport{}, c.Err()
	}
	in.Kernel = func(context.Context) ([]string, error) { t.Fatal("read kernel after deadline"); return nil, nil }
	started := time.Now()
	r := InspectCombined(ctx, in, "revision")
	if time.Since(started) > time.Second || r.Status != "inconclusive" || r.Network != nil || r.Resources != nil {
		t.Fatal(r)
	}
	combinedHas(t, r, "DEADLINE_OR_CANCELED")
}
