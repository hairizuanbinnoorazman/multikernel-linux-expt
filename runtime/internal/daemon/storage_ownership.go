package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
)

var ErrStorageOwnershipUnsupported = errors.New("daemon does not advertise storage-ownership-v1")

func InspectStorageOwnership(ctx context.Context, caller Caller) (protocol.StorageOwnershipReport, error) {
	var capabilities protocol.DaemonCapabilities
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "storage-ownership-capabilities-v1", Method: "Capabilities"}, &capabilities); failure != nil {
		return protocol.StorageOwnershipReport{}, storageOwnershipRPCError(failure)
	}
	if capabilities.Validate() != nil {
		return protocol.StorageOwnershipReport{}, errors.New("invalid daemon storage capabilities")
	}
	if !slices.Contains(capabilities.Features, protocol.StorageOwnershipCapability) {
		return protocol.StorageOwnershipReport{}, ErrStorageOwnershipUnsupported
	}
	var report protocol.StorageOwnershipReport
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-storage-ownership-v1", Method: "InspectStorageOwnership"}, &report); failure != nil {
		return protocol.StorageOwnershipReport{}, storageOwnershipRPCError(failure)
	}
	if report.Validate() != nil {
		return protocol.StorageOwnershipReport{}, errors.New("invalid daemon storage ownership report")
	}
	return report, nil
}
func storageOwnershipRPCError(failure *protocol.Error) error {
	err := auditRPCError(failure).(*MutationError)
	err.Failure.Message = "daemon storage ownership request failed"
	return err
}
