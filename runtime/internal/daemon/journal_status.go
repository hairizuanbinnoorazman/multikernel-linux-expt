package daemon

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"slices"
	"time"
)

var ErrJournalStatusUnsupported = errors.New("daemon does not advertise journal-status-v1")

// InspectJournalStatus negotiates and reads under one five-second caller budget.
// No legacy journal stream or local-state fallback is permitted.
func InspectJournalStatus(ctx context.Context, caller Caller) (protocol.JournalStatusReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	fail := func(f *protocol.Error) (protocol.JournalStatusReport, error) {
		if f.Code == "UNSUPPORTED" {
			return protocol.JournalStatusReport{}, ErrJournalStatusUnsupported
		}
		return protocol.JournalStatusReport{}, errors.New("daemon journal status request failed")
	}
	if ctx.Err() != nil {
		return protocol.JournalStatusReport{}, errors.New("daemon journal status request canceled")
	}
	var caps protocol.DaemonCapabilities
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "journal-status-capabilities-v1", Method: "Capabilities"}, &caps); failure != nil {
		return fail(failure)
	}
	if caps.Validate() != nil {
		return protocol.JournalStatusReport{}, errors.New("invalid daemon journal status capabilities")
	}
	if !slices.Contains(caps.Features, protocol.JournalStatusCapability) {
		return protocol.JournalStatusReport{}, ErrJournalStatusUnsupported
	}
	if ctx.Err() != nil {
		return protocol.JournalStatusReport{}, errors.New("daemon journal status request canceled")
	}
	var r protocol.JournalStatusReport
	if failure := caller.Call(ctx, protocol.Request{Version: protocol.Version, RequestID: "inspect-journal-status-v1", Method: "InspectJournalStatus"}, &r); failure != nil {
		return fail(failure)
	}
	if ctx.Err() != nil || r.Validate() != nil {
		return protocol.JournalStatusReport{}, errors.New("invalid daemon journal status report")
	}
	return r, nil
}
