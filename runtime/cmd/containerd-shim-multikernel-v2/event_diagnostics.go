//go:build linux

package main

import "github.com/hairizuan/multikernel-linux-expt/runtime/protocol"

// Lifecycle setup installs only validated ownership after create or recovered
// bundle identity validation; successful rollback clears it. The immutable
// snapshot avoids taking s.mu from publish callers that already hold it, or
// racing a startup retry with initial sandbox assignment. It identifies the
// worker's observed owner, not the identity of any particular pending event.
func (s *service) setEventDiagnosticOwner(sandbox protocol.Sandbox) {
	r := newProcessDiagnosticReporter(sandbox.ID, sandbox.Generation, "init", s.processDiagnosticSink)
	if r.sink == nil {
		s.eventDiagnosticOwner.Store(nil)
		return
	}
	s.eventDiagnosticOwner.Store(&r)
}

// Publication and replay each get at most one enqueue attempt per worker,
// including full/disabled-sink rejection. This quota is independent of changes
// in observed ownership. Unknown ownership is suppressed without consuming the
// attempt, so later validated ownership can still produce its first record.
// Neither service nor event journal locks, event payloads, broker errors,
// topics, sequences, filesystem paths or RPCs are used by this interface.
func (s *service) reportEventDiagnostic(code processDiagnosticCode) bool {
	if code != diagnosticEventPublicationDeferred && code != diagnosticEventReplayDeferred {
		return false
	}
	owner := s.eventDiagnosticOwner.Load()
	if owner == nil {
		return false
	}
	bit := uint64(1) << code
	for {
		attempted := s.eventLogAttempts.Load()
		if attempted&bit != 0 {
			return false
		}
		if s.eventLogAttempts.CompareAndSwap(attempted, attempted|bit) {
			break
		}
	}
	reporter := *owner
	return reporter.emit(code, 0)
}
