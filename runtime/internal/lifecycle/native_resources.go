package lifecycle

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
)

func (s *Service) NativeResourcesAvailable() bool {
	return s.storage != nil && s.storage.ProcessInspectionAvailable()
}
func (s *Service) NativeResources(ctx context.Context) (protocol.NativeResourceReport, *protocol.Error) {
	empty := protocol.NativeResourceReport{}
	if !s.NativeResourcesAvailable() {
		return empty, &protocol.Error{Code: "UNSUPPORTED", Message: "native resource inspection is unavailable"}
	}
	before, failure := s.store.ResourceOwnershipSnapshot()
	if failure != nil {
		return empty, failure
	}
	storage, descriptors, processes, failure := s.storage.ProcessOwnership(ctx)
	if failure != nil {
		return empty, failure
	}
	after, failure := s.store.ResourceOwnershipSnapshot()
	if failure != nil {
		return empty, failure
	}
	if !reflect.DeepEqual(before, after) || ctx.Err() != nil {
		return empty, &protocol.Error{Code: "UNAVAILABLE", Message: "native resource ownership changed during observation", Retryable: true}
	}
	after.Storage = storage
	after.StorageDescriptors = descriptors
	report := protocol.NativeResourceReport{Version: 1, Source: "durable-resources-and-native-process-identity", Resources: after, Processes: processes}
	if report.Validate() != nil {
		return empty, &protocol.Error{Code: "OPERATOR_ACTION", Message: "native resource metadata is invalid"}
	}
	return report, nil
}
