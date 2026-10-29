package network

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var ErrComponentBuildUnsupported = errors.New("mknetd does not advertise component-build-v1")

func validInspectionCapabilities(cap Response) bool {
	if cap.Capabilities == nil || cap.Endpoint != nil || cap.Ownership != nil || cap.ComponentBuild != nil || cap.Capabilities.Version != 1 || len(cap.Capabilities.Features) == 0 || len(cap.Capabilities.Features) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, feature := range cap.Capabilities.Features {
		if len(feature) == 0 || len(feature) > 64 || seen[feature] {
			return false
		}
		for _, r := range feature {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
		seen[feature] = true
	}
	return true
}

// InspectComponentBuild uses the existing authenticated socket and a single
// negotiation budget. It never opens state files or reconciles network state.
func InspectComponentBuild(ctx context.Context, client Client) (protocol.ComponentBuildReport, error) {
	timeout := client.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	fail := func(err error) (protocol.ComponentBuildReport, error) {
		var api *APIError
		if errors.As(err, &api) && api.Code == "UNSUPPORTED" {
			return protocol.ComponentBuildReport{}, ErrComponentBuildUnsupported
		}
		return protocol.ComponentBuildReport{}, errors.New("network component build request failed")
	}
	if ctx.Err() != nil {
		return protocol.ComponentBuildReport{}, errors.New("network component build request canceled")
	}
	cap, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "component-build-capabilities-v1", Method: "CAPABILITIES"})
	if err != nil {
		return fail(err)
	}
	if !validInspectionCapabilities(cap) {
		return protocol.ComponentBuildReport{}, errors.New("invalid network component build capabilities")
	}
	if !slices.Contains(cap.Capabilities.Features, protocol.ComponentBuildCapability) {
		return protocol.ComponentBuildReport{}, ErrComponentBuildUnsupported
	}
	if ctx.Err() != nil {
		return protocol.ComponentBuildReport{}, errors.New("network component build request canceled")
	}
	response, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "inspect-component-build-v1", Method: "INSPECT_COMPONENT_BUILD"})
	if err != nil {
		return fail(err)
	}
	if ctx.Err() != nil || response.ComponentBuild == nil || response.Endpoint != nil || response.Ownership != nil || response.Capabilities != nil || response.ComponentBuild.Validate() != nil || response.ComponentBuild.Component != "mknetd" {
		return protocol.ComponentBuildReport{}, errors.New("invalid network component build report")
	}
	return *response.ComponentBuild, nil
}
