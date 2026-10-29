package daemon

import (
	"context"
	"errors"
	"slices"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var ErrComponentBuildUnsupported = errors.New("daemon does not advertise component-build-v1")

// InspectComponentBuild never substitutes the client revision or a state-file
// read for an unsupported or unavailable server build identity.
func InspectComponentBuild(ctx context.Context, caller Caller) (protocol.ComponentBuildReport, error) {
	fail := func(f *protocol.Error) (protocol.ComponentBuildReport, error) {
		if f.Code == "UNSUPPORTED" {
			return protocol.ComponentBuildReport{}, ErrComponentBuildUnsupported
		}
		return protocol.ComponentBuildReport{}, errors.New("daemon component build request failed")
	}
	if ctx.Err() != nil {
		return protocol.ComponentBuildReport{}, errors.New("daemon component build request canceled")
	}
	var caps protocol.DaemonCapabilities
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "component-build-capabilities-v1", Method: "Capabilities"}, &caps); f != nil {
		return fail(f)
	}
	if caps.Validate() != nil {
		return protocol.ComponentBuildReport{}, errors.New("invalid daemon component build capabilities")
	}
	if !slices.Contains(caps.Features, protocol.ComponentBuildCapability) {
		return protocol.ComponentBuildReport{}, ErrComponentBuildUnsupported
	}
	if ctx.Err() != nil {
		return protocol.ComponentBuildReport{}, errors.New("daemon component build request canceled")
	}
	var report protocol.ComponentBuildReport
	if f := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-component-build-v1", Method: "InspectComponentBuild"}, &report); f != nil {
		return fail(f)
	}
	if ctx.Err() != nil || report.Validate() != nil || report.Component != "mkruntimed" {
		return protocol.ComponentBuildReport{}, errors.New("invalid daemon component build report")
	}
	return report, nil
}
