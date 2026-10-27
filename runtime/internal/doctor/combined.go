package doctor

import (
	"context"
	"errors"
	"reflect"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// CombinedInputs share one caller context across all three negotiated views.
// None of the callbacks may reconcile state or substitute a legacy projection.
type CombinedInputs struct {
	Native       func(context.Context) (protocol.NativeResourceReport, error)
	Expectations func(context.Context) (protocol.NetworkExpectationsReport, error)
	Network      func(context.Context) (network.OwnershipReport, error)
	Kernel       func(context.Context) ([]string, error)
	Boot         func() (string, error)
}

// InspectCombined brackets kernel/boot observations with repeated complete
// native/intent/endpoint samples. Matching samples do not imply cross-daemon
// atomicity, native network health or kernel/endpoint incarnation attestation.
func InspectCombined(ctx context.Context, in CombinedInputs, revision string) Report {
	type sample struct {
		expected protocol.NetworkExpectationsReport
		network  network.OwnershipReport
	}
	var records []sample
	failure := ""
	canceled := func() error {
		if err := ctx.Err(); err != nil {
			failure = "DEADLINE_OR_CANCELED"
			return err
		}
		return nil
	}
	r := InspectNativeResources(ctx, NativeInputs{Native: func(ctx context.Context) (protocol.NativeResourceReport, error) {
		if err := canceled(); err != nil {
			return protocol.NativeResourceReport{}, err
		}
		native, err := in.Native(ctx)
		if err := canceled(); err != nil {
			return protocol.NativeResourceReport{}, err
		}
		if err != nil || native.Validate() != nil {
			// The native inspector preserves its existing sanitized compatibility
			// and availability codes. Stop before reading other components.
			if err == nil {
				err = errors.New("invalid native identity report")
			}
			return protocol.NativeResourceReport{}, err
		}
		expected, err := in.Expectations(ctx)
		if err := canceled(); err != nil {
			return protocol.NativeResourceReport{}, err
		}
		if err != nil || expected.Validate() != nil {
			failure = "NETWORK_EXPECTATIONS_UNAVAILABLE"
			if errors.Is(err, daemon.ErrNetworkExpectationsUnsupported) {
				failure = "NETWORK_EXPECTATIONS_UNSUPPORTED"
			}
			return protocol.NativeResourceReport{}, errors.New("network expectation inspection failed")
		}
		if !reflect.DeepEqual(native.Resources.Lifecycle, expected.Lifecycle) {
			failure = "RESOURCE_NETWORK_LIFECYCLE_CHANGED"
			return protocol.NativeResourceReport{}, errors.New("lifecycle inspection changed")
		}
		endpoints, err := in.Network(ctx)
		if err := canceled(); err != nil {
			return protocol.NativeResourceReport{}, err
		}
		if err != nil || endpoints.Validate() != nil {
			failure = "NETWORK_OWNERSHIP_UNAVAILABLE"
			if errors.Is(err, network.ErrOwnershipUnsupported) {
				failure = "NETWORK_OWNERSHIP_UNSUPPORTED"
			}
			return protocol.NativeResourceReport{}, errors.New("network ownership inspection failed")
		}
		records = append(records, sample{expected, endpoints})
		return native, nil
	}, Kernel: in.Kernel, Boot: in.Boot}, revision)
	r.Version = 5
	r.Scope = "durable-resource-network-ownership-native-process-identity-and-kernel-presence"
	r.Unobserved = []string{"kernel-incarnation", "native-storage-health", "native-network-health", "network-descriptor-expectations", "pool-capacity", "service-readiness", "docker-engine", "containment"}
	if failure != "" {
		incompleteCombined(&r, failure)
		return r
	}
	if r.Resources == nil || r.NativeProcesses == nil {
		return r
	}
	if len(records) != 2 || !reflect.DeepEqual(records[0], records[1]) {
		incompleteCombined(&r, "OBSERVATION_CHANGED")
		return r
	}
	r.Network = &records[1].network
	r.NetworkExpectations = &records[1].expected.Expectations
	// Preserve storage/native findings and their severity when adding network
	// findings. A matching endpoint never erases a degraded storage observation.
	correlateNetwork(&r)
	return r
}

func incompleteCombined(r *Report, code string) {
	r.Status = "inconclusive"
	r.Ownership = nil
	r.Resources = nil
	r.NativeProcesses = nil
	r.KernelInstances = nil
	r.Network = nil
	r.NetworkExpectations = nil
	r.Findings = []Finding{{Code: code}}
}
