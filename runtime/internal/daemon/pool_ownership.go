package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
)

var ErrPoolOwnershipUnsupported = errors.New("daemon does not advertise pool-ownership-v1")

func InspectPoolOwnership(ctx context.Context, caller Caller) (protocol.PoolOwnershipReport, error) {
	var caps protocol.DaemonCapabilities
	fail := func(f *protocol.Error) (protocol.PoolOwnershipReport, error) {
		e := auditRPCError(f).(*MutationError)
		e.Failure.Message = "daemon pool ownership request failed"
		return protocol.PoolOwnershipReport{}, e
	}
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "pool-capabilities-v1", Method: "Capabilities"}, &caps); f != nil {
		return fail(f)
	}
	if caps.Validate() != nil {
		return protocol.PoolOwnershipReport{}, errors.New("invalid daemon pool capabilities")
	}
	if !slices.Contains(caps.Features, protocol.PoolOwnershipCapability) {
		return protocol.PoolOwnershipReport{}, ErrPoolOwnershipUnsupported
	}
	var report protocol.PoolOwnershipReport
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-pool-ownership-v1", Method: "InspectPoolOwnership"}, &report); f != nil {
		return fail(f)
	}
	if report.Validate() != nil {
		return protocol.PoolOwnershipReport{}, errors.New("invalid daemon pool ownership report")
	}
	return report, nil
}
