package doctor

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
)

type NativeInputs struct {
	Native func(context.Context) (protocol.NativeResourceReport, error)
	Kernel func(context.Context) ([]string, error)
	Boot   func() (string, error)
}

// InspectNativeResources brackets kernel/boot observations with complete native
// identity reports. Sampled equality is neither session health nor an ABA proof.
func InspectNativeResources(ctx context.Context, in NativeInputs, revision string) Report {
	var records []protocol.NativeResourceReport
	var failure error
	r := InspectResources(ctx, ResourceInputs{Resources: func(ctx context.Context) (protocol.ResourceOwnershipReport, error) {
		report, err := in.Native(ctx)
		if err == nil && report.Validate() != nil {
			err = errors.New("invalid native identity report")
		}
		if err != nil {
			failure = err
			return protocol.ResourceOwnershipReport{}, err
		}
		records = append(records, report)
		return report.Resources, nil
	}, Kernel: in.Kernel, Boot: in.Boot}, revision)
	r.Version = 3
	r.Scope = "durable-resource-ownership-native-process-identity-and-kernel-presence"
	if failure != nil {
		code := "NATIVE_RESOURCE_IDENTITY_UNAVAILABLE"
		if errors.Is(failure, daemon.ErrNativeResourcesUnsupported) {
			code = "NATIVE_RESOURCE_IDENTITY_UNSUPPORTED"
		}
		r.Findings = []Finding{{Code: code}}
		return r
	}
	if r.Resources == nil {
		return r
	}
	if len(records) != 2 || !reflect.DeepEqual(records[0], records[1]) {
		r.Status = "inconclusive"
		r.Ownership = nil
		r.Resources = nil
		r.KernelInstances = nil
		r.Findings = []Finding{{Code: "OBSERVATION_CHANGED"}}
		return r
	}
	r.NativeProcesses = &records[1].Processes
	for i, p := range records[1].Processes {
		lease := records[1].Resources.Storage.Exports[i]
		if p.Status == "process-matched" || p.Status == "not-probed-released" {
			continue
		}
		code := "NATIVE_STORAGE_IDENTITY_UNAVAILABLE"
		status := "inconclusive"
		if lease.State == "ACTIVE" {
			switch p.Status {
			case "record-missing":
				code = "NATIVE_STORAGE_RECORD_MISSING"
				status = "degraded"
			case "process-absent":
				code = "NATIVE_STORAGE_PROCESS_ABSENT"
				status = "degraded"
			}
		}
		r.Findings = append(r.Findings, Finding{Code: code, SandboxID: p.SandboxID, Generation: p.Generation, ExportGeneration: p.ExportGeneration})
		if r.Status != "degraded" || status == "degraded" {
			r.Status = status
		}
	}
	return r
}
