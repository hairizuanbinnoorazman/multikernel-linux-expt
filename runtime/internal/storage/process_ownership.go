package storage

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"sort"
)

type processInspector interface {
	InspectProcess(context.Context, Export) (ProcessInspection, error)
}

func (s *Service) ProcessInspectionAvailable() bool { _, ok := s.backend.(processInspector); return ok }

// processSnapshot holds only the persistence lock, never the operation lock.
// Private descriptors remain internal and are compared again after inspection.
func (s *Service) processSnapshot() (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, []Export, *protocol.Error) {
	if !s.store.mu.TryLock() {
		return protocol.StorageOwnershipReport{}, nil, nil, &protocol.Error{Code: "UNAVAILABLE", Message: "storage process snapshot is busy", Retryable: true}
	}
	defer s.store.mu.Unlock()
	report, descriptors, failure := s.ownershipLocked(true)
	if failure != nil {
		return protocol.StorageOwnershipReport{}, nil, nil, failure
	}
	exports := make([]Export, 0, len(report.Exports))
	for _, v := range s.store.data.Exports {
		exports = append(exports, v)
	}
	sort.Slice(exports, func(i, j int) bool {
		if exports[i].SandboxID == exports[j].SandboxID {
			return exports[i].SandboxGeneration < exports[j].SandboxGeneration
		}
		return exports[i].SandboxID < exports[j].SandboxID
	})
	return report, descriptors, exports, nil
}
func (s *Service) ProcessOwnership(ctx context.Context) (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, []protocol.StorageProcessIdentity, *protocol.Error) {
	fail := func(code string) (protocol.StorageOwnershipReport, []protocol.StorageDescriptorBinding, []protocol.StorageProcessIdentity, *protocol.Error) {
		return protocol.StorageOwnershipReport{}, nil, nil, &protocol.Error{Code: code, Message: "native storage process observation is unavailable", Retryable: code == "UNAVAILABLE"}
	}
	inspector, ok := s.backend.(processInspector)
	if !ok {
		return fail("UNSUPPORTED")
	}
	if ctx.Err() != nil {
		return fail("UNAVAILABLE")
	}
	report, descriptors, before, failure := s.processSnapshot()
	if failure != nil {
		return protocol.StorageOwnershipReport{}, nil, nil, failure
	}
	processes := make([]protocol.StorageProcessIdentity, 0, len(before))
	for _, v := range before {
		if ctx.Err() != nil {
			return fail("UNAVAILABLE")
		}
		p := protocol.StorageProcessIdentity{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, Status: "not-probed-released"}
		if v.State != "RELEASED" {
			observation, err := inspector.InspectProcess(ctx, v)
			p.Status = "unavailable"
			if err == nil && observation.Generation == v.ExportGeneration {
				switch observation.Status {
				case "process-matched", "process-absent", "record-missing":
					p.Status = observation.Status
				}
			}
		}
		processes = append(processes, p)
	}
	_, _, after, failure := s.processSnapshot()
	if failure != nil {
		return protocol.StorageOwnershipReport{}, nil, nil, failure
	}
	if ctx.Err() != nil || !reflect.DeepEqual(before, after) {
		return fail("UNAVAILABLE")
	}
	return report, descriptors, processes, nil
}
