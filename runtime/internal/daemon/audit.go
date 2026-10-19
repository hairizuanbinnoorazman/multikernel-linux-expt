package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// ReadAuditEvents negotiates before requesting the metadata-only view. It never
// falls back to the legacy stream, whose error bodies have different semantics.
func ReadAuditEvents(ctx context.Context, caller Caller, query protocol.AuditQuery) (protocol.AuditPage, error) {
	if err := query.Validate(); err != nil {
		return protocol.AuditPage{}, err
	}
	var capabilities protocol.DaemonCapabilities
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "audit-capabilities-v1", Method: "Capabilities"}, &capabilities); failure != nil {
		return protocol.AuditPage{}, auditRPCError(failure)
	}
	if capabilities.Validate() != nil {
		return protocol.AuditPage{}, errors.New("invalid daemon audit capabilities")
	}
	if !slices.Contains(capabilities.Features, protocol.LifecycleAuditCapability) {
		return protocol.AuditPage{}, &MutationError{Failure: protocol.Error{Code: "UNSUPPORTED", Message: "daemon does not advertise lifecycle-audit-v1"}}
	}
	body, err := json.Marshal(query)
	if err != nil {
		return protocol.AuditPage{}, err
	}
	var page protocol.AuditPage
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "audit-events-v1", Method: "ReadAuditEvents", Body: body}, &page); failure != nil {
		return protocol.AuditPage{}, auditRPCError(failure)
	}
	if err = page.Validate(query); err != nil {
		return protocol.AuditPage{}, err
	}
	return page, nil
}

// Collector-facing errors preserve stable classifications without copying a
// remote message, nested operation ID or unknown code into diagnostic output.
func auditRPCError(failure *protocol.Error) error {
	code := failure.Code
	switch code {
	case "INVALID_ARGUMENT", "UNSUPPORTED", "NOT_FOUND", "ALREADY_EXISTS", "STALE_GENERATION", "IDEMPOTENCY_CONFLICT", "FAILED_PRECONDITION", "RESOURCE_EXHAUSTED", "BACKEND_TIMEOUT", "BACKEND_FAILURE", "UNAUTHENTICATED", "ABORTED", "OPERATOR_ACTION", "INTERNAL", "UNAVAILABLE":
	default:
		code = "INTERNAL"
	}
	return &MutationError{Failure: protocol.Error{Code: code, Message: "daemon audit request failed", Retryable: failure.Retryable}}
}
