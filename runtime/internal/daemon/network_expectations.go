package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
)

var ErrNetworkExpectationsUnsupported = errors.New("daemon does not advertise network-expectations-v1")

func InspectNetworkExpectations(ctx context.Context, caller Caller) (protocol.NetworkExpectationsReport, error) {
	var caps protocol.DaemonCapabilities
	fail := func(f *protocol.Error) (protocol.NetworkExpectationsReport, error) {
		e := auditRPCError(f).(*MutationError)
		e.Failure.Message = "daemon network expectations request failed"
		return protocol.NetworkExpectationsReport{}, e
	}
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "network-expectations-capabilities-v1", Method: "Capabilities"}, &caps); f != nil {
		return fail(f)
	}
	if caps.Validate() != nil {
		return protocol.NetworkExpectationsReport{}, errors.New("invalid daemon network capabilities")
	}
	if !slices.Contains(caps.Features, protocol.NetworkExpectationsCapability) {
		return protocol.NetworkExpectationsReport{}, ErrNetworkExpectationsUnsupported
	}
	var r protocol.NetworkExpectationsReport
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-network-expectations-v1", Method: "InspectNetworkExpectations"}, &r); f != nil {
		return fail(f)
	}
	if r.Validate() != nil {
		return protocol.NetworkExpectationsReport{}, errors.New("invalid daemon network expectations report")
	}
	return r, nil
}
