package doctor

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
)

type ResourceInputs struct {
	Resources func(context.Context) (protocol.ResourceOwnershipReport, error)
	Kernel    func(context.Context) ([]string, error)
	Boot      func() (string, error)
}

// InspectResources compares stable repeated metadata, not live native processes.
// It requires the negotiated combined view, never a partial legacy fallback.
func InspectResources(ctx context.Context, in ResourceInputs, revision string) Report {
	var records []protocol.ResourceOwnershipReport
	var readFailure error
	r := Inspect(ctx, Inputs{Ownership: func(ctx context.Context) (protocol.OwnershipReport, error) {
		record, err := in.Resources(ctx)
		if err == nil && record.Validate() != nil {
			err = errors.New("invalid resource ownership")
		}
		if err != nil {
			readFailure = err
			return protocol.OwnershipReport{}, err
		}
		records = append(records, record)
		return record.Lifecycle, nil
	}, Kernel: in.Kernel, Boot: in.Boot}, revision)
	r.Version = 2
	r.Scope = "durable-resource-ownership-and-kernel-presence"
	r.Unobserved = []string{"kernel-incarnation", "native-storage-health", "network-endpoints", "pool-capacity", "service-readiness", "docker-engine", "containment"}
	if readFailure != nil {
		code := "RESOURCE_OWNERSHIP_UNAVAILABLE"
		if errors.Is(readFailure, daemon.ErrResourceOwnershipUnsupported) {
			code = "RESOURCE_OWNERSHIP_UNSUPPORTED"
		}
		r.Findings = []Finding{{Code: code}}
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
	r.Resources = &records[1]
	correlateStorage(&r)
	return r
}
func correlateStorage(r *Report) {
	resources := r.Resources
	owners := map[string]protocol.SandboxOwnership{}
	bindings := map[string]protocol.LifecycleStorageBinding{}
	for i, owner := range resources.Lifecycle.Sandboxes {
		owners[owner.SandboxID] = owner
		bindings[owner.SandboxID] = resources.StorageBindings[i]
	}
	leases := map[string]protocol.StorageExportOwnership{}
	descriptors := map[string]string{}
	add := func(code, id, generation, export, expected string) {
		r.Findings = append(r.Findings, Finding{Code: code, SandboxID: id, Generation: generation, ExportGeneration: export, ExpectedGeneration: expected})
		r.Status = "degraded"
	}
	for i, lease := range resources.Storage.Exports {
		key := lease.SandboxID + "\x00" + lease.SandboxGeneration
		leases[key] = lease
		descriptors[key] = resources.StorageDescriptors[i].DescriptorSHA256
		if lease.State == "RELEASED" {
			continue
		} // Historical records hold no live authority.
		owner, exists := owners[lease.SandboxID]
		if !exists {
			add("STORAGE_ORPHAN_CANDIDATE", lease.SandboxID, lease.SandboxGeneration, lease.ExportGeneration, "")
			continue
		}
		if owner.Generation != lease.SandboxGeneration {
			add("STORAGE_GENERATION_MISMATCH", lease.SandboxID, lease.SandboxGeneration, lease.ExportGeneration, owner.Generation)
			continue
		}
		if owner.State == "ABSENT" {
			add("STORAGE_AFTER_OWNER_RELEASE", lease.SandboxID, lease.SandboxGeneration, lease.ExportGeneration, "")
		}
		if !bindings[owner.SandboxID].Required {
			add("STORAGE_UNEXPECTED_LEASE", lease.SandboxID, lease.SandboxGeneration, lease.ExportGeneration, "")
		}
		if lease.PeerCID != owner.ChildCID {
			add("STORAGE_PEER_MISMATCH", lease.SandboxID, lease.SandboxGeneration, lease.ExportGeneration, "")
		}
	}
	for _, binding := range resources.StorageBindings {
		if !binding.Required {
			continue
		}
		owner := owners[binding.SandboxID]
		transition := owner.State == "ALLOCATING" || owner.State == "STOPPING" || owner.State == "RELEASING"
		key := binding.SandboxID + "\x00" + binding.Generation
		lease, exists := leases[key]
		if !exists {
			if !transition && owner.State != "ABSENT" {
				add("STORAGE_LEASE_MISSING", binding.SandboxID, binding.Generation, binding.ExportGeneration, "")
			}
			continue
		}
		if binding.ExportGeneration == "" {
			if !transition && owner.State != "ABSENT" {
				add("STORAGE_BINDING_MISSING", binding.SandboxID, binding.Generation, lease.ExportGeneration, "")
			}
		} else if binding.ExportGeneration != lease.ExportGeneration {
			add("STORAGE_EXPORT_GENERATION_MISMATCH", binding.SandboxID, binding.Generation, lease.ExportGeneration, "")
		}
		if binding.DescriptorSHA256 != descriptors[key] {
			add("STORAGE_DESCRIPTOR_MISMATCH", binding.SandboxID, binding.Generation, lease.ExportGeneration, "")
		}
		if owner.State == "ABSENT" {
			continue
		}
		if lease.State == "RELEASED" && !transition {
			add("STORAGE_RELEASED_WHILE_OWNED", binding.SandboxID, binding.Generation, lease.ExportGeneration, "")
		}
		if lease.State == "PREPARING" || lease.State == "QUIESCING" {
			r.Findings = append(r.Findings, Finding{Code: "STORAGE_TRANSITION_REQUIRES_RECHECK", SandboxID: binding.SandboxID, Generation: binding.Generation, ExportGeneration: lease.ExportGeneration})
			if r.Status == "consistent" {
				r.Status = "inconclusive"
			}
		}
	}
}
