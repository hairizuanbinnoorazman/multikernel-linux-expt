package state

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// JournalStatus does no filesystem I/O, journal replay, payload cloning or
// mutation. Pending counts reflect committed intents minus completions; after a
// failed append/sync the sequence is only the last acknowledged sequence.
func (s *Store) JournalStatus(ctx context.Context) (protocol.JournalStatusReport, *protocol.Error) {
	unavailable := func() (protocol.JournalStatusReport, *protocol.Error) {
		return protocol.JournalStatusReport{}, &protocol.Error{Code: "UNAVAILABLE", Message: "journal status snapshot unavailable", Retryable: true}
	}
	if ctx.Err() != nil || !s.mu.TryLock() {
		return unavailable()
	}
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return unavailable()
	}
	if s.pendingCompletions < 0 {
		return protocol.JournalStatusReport{}, &protocol.Error{Code: "OPERATOR_ACTION", Message: "journal status accounting is invalid"}
	}
	r := protocol.JournalStatusReport{Version: 1, Source: "in-process-lifecycle-journal", Status: "idle", Sequence: s.data.Sequence, PendingCompletions: uint64(s.pendingCompletions), JournalFailed: s.journalFailed}
	if r.PendingCompletions > 0 {
		r.Status = "pending"
	}
	if r.JournalFailed {
		r.Status = "failed"
	}
	if r.Validate() != nil {
		return protocol.JournalStatusReport{}, &protocol.Error{Code: "OPERATOR_ACTION", Message: "journal status accounting is invalid"}
	}
	return r, nil
}
