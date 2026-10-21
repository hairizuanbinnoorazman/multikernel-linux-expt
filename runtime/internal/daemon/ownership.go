package daemon

import (
	"context"
	"errors"
	"slices"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func InspectOwnership(ctx context.Context, caller Caller) (protocol.OwnershipReport, error) {
	var capabilities protocol.DaemonCapabilities
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "ownership-capabilities-v1", Method: "Capabilities"}, &capabilities); failure != nil {
		return protocol.OwnershipReport{}, ownershipRPCError(failure)
	}
	if capabilities.Validate() != nil || !slices.Contains(capabilities.Features, protocol.OwnershipCapability) {
		return protocol.OwnershipReport{}, errors.New("daemon does not advertise a valid ownership-v1 capability")
	}
	var report protocol.OwnershipReport
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-ownership-v1", Method: "InspectOwnership"}, &report); failure != nil {
		return protocol.OwnershipReport{}, ownershipRPCError(failure)
	}
	if report.Validate() != nil {
		return protocol.OwnershipReport{}, errors.New("invalid daemon ownership report")
	}
	return report, nil
}

func ownershipRPCError(failure *protocol.Error) error {
	// Reuse the stable-code allowlist, but never copy remote error text.
	err := auditRPCError(failure).(*MutationError)
	err.Failure.Message = "daemon ownership request failed"
	return err
}
