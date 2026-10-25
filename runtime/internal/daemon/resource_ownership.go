package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
)

var ErrResourceOwnershipUnsupported = errors.New("daemon does not advertise resource-ownership-v1")

func InspectResourceOwnership(ctx context.Context, caller Caller) (protocol.ResourceOwnershipReport, error) {
	var capabilities protocol.DaemonCapabilities
	fail := func(f *protocol.Error) (protocol.ResourceOwnershipReport, error) {
		e := auditRPCError(f).(*MutationError)
		e.Failure.Message = "daemon resource ownership request failed"
		return protocol.ResourceOwnershipReport{}, e
	}
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "resource-ownership-capabilities-v1", Method: "Capabilities"}, &capabilities); f != nil {
		return fail(f)
	}
	if capabilities.Validate() != nil {
		return protocol.ResourceOwnershipReport{}, errors.New("invalid daemon resource capabilities")
	}
	if !slices.Contains(capabilities.Features, protocol.ResourceOwnershipCapability) {
		return protocol.ResourceOwnershipReport{}, ErrResourceOwnershipUnsupported
	}
	var report protocol.ResourceOwnershipReport
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-resource-ownership-v1", Method: "InspectResourceOwnership"}, &report); f != nil {
		return fail(f)
	}
	if report.Validate() != nil {
		return protocol.ResourceOwnershipReport{}, errors.New("invalid daemon resource ownership report")
	}
	return report, nil
}
