package network

import (
	"context"
	"errors"
	"slices"
	"time"
)

var ErrOwnershipUnsupported = errors.New("mknetd does not advertise network-ownership-v1")

// InspectOwnership negotiates the projection with one shared caller budget.
// Unsupported daemons never fall back to a raw endpoint or state-file dump.
func InspectOwnership(ctx context.Context, client Client) (OwnershipReport, error) {
	timeout := client.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	failure := func(err error) (OwnershipReport, error) {
		var api *APIError
		if errors.As(err, &api) {
			if api.Code == "UNSUPPORTED" {
				return OwnershipReport{}, ErrOwnershipUnsupported
			}
			for _, code := range []string{"UNAVAILABLE", "OPERATOR_ACTION", "RESOURCE_EXHAUSTED"} {
				if api.Code == code {
					return OwnershipReport{}, &APIError{Code: code, Message: "network ownership request failed", Retryable: api.Retryable}
				}
			}
		}
		return OwnershipReport{}, errors.New("network ownership request failed")
	}
	cap, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "network-ownership-capabilities-v1", Method: "CAPABILITIES"})
	if err != nil {
		return failure(err)
	}
	if !validInspectionCapabilities(cap) {
		return OwnershipReport{}, errors.New("invalid network ownership capabilities")
	}
	if !slices.Contains(cap.Capabilities.Features, OwnershipCapability) {
		return OwnershipReport{}, ErrOwnershipUnsupported
	}
	response, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "inspect-network-ownership-v1", Method: "INSPECT_OWNERSHIP"})
	if err != nil {
		return failure(err)
	}
	if response.Health != nil || response.Ownership == nil || response.Endpoint != nil || response.Capabilities != nil || response.ComponentBuild != nil || response.Ownership.Validate() != nil {
		return OwnershipReport{}, errors.New("invalid network ownership report")
	}
	return *response.Ownership, nil
}
