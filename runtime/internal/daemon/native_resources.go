package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
)

var ErrNativeResourcesUnsupported = errors.New("daemon does not advertise native-resource-identity-v1")

func InspectNativeResources(ctx context.Context, caller Caller) (protocol.NativeResourceReport, error) {
	var capabilities protocol.DaemonCapabilities
	fail := func(f *protocol.Error) (protocol.NativeResourceReport, error) {
		e := auditRPCError(f).(*MutationError)
		e.Failure.Message = "daemon native resource identity request failed"
		return protocol.NativeResourceReport{}, e
	}
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "native-resource-capabilities-v1", Method: "Capabilities"}, &capabilities); f != nil {
		return fail(f)
	}
	if capabilities.Validate() != nil {
		return protocol.NativeResourceReport{}, errors.New("invalid daemon resource capabilities")
	}
	if !slices.Contains(capabilities.Features, protocol.NativeResourcesCapability) {
		return protocol.NativeResourceReport{}, ErrNativeResourcesUnsupported
	}
	var report protocol.NativeResourceReport
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-native-resource-identity-v1", Method: "InspectNativeResources"}, &report); f != nil {
		return fail(f)
	}
	if report.Validate() != nil {
		return protocol.NativeResourceReport{}, errors.New("invalid daemon native resource identity report")
	}
	return report, nil
}
