package lifecycle

import "github.com/hairizuan/multikernel-linux-expt/runtime/protocol"

// AuditEvents exposes only bounded lifecycle metadata. Legacy WatchEvents stays
// wire compatible; neither journal configuration nor free-form error text is
// copied into this explicitly negotiated view.
func (s *Service) AuditEvents(query protocol.AuditQuery) (protocol.AuditPage, *protocol.Error) {
	page := protocol.AuditPage{Version: 1, Records: make([]protocol.AuditRecord, 0), NextSequence: query.AfterSequence}
	if query.Version != 1 {
		return protocol.AuditPage{}, apierr("UNSUPPORTED", "audit version must be 1", false)
	}
	if query.Validate() != nil {
		return protocol.AuditPage{}, apierr("INVALID_ARGUMENT", "audit limit must be at most 512", false)
	}
	entries, err := s.store.JournalEntries()
	if err != nil {
		return protocol.AuditPage{}, apierr("INTERNAL", "audit journal is unavailable", true)
	}
	if len(entries) > 0 {
		page.HighWaterSequence = entries[len(entries)-1].Sequence
	}
	if query.AfterSequence > page.HighWaterSequence {
		return protocol.AuditPage{}, apierr("INVALID_ARGUMENT", "audit cursor is ahead of the journal", false)
	}
	for _, entry := range entries {
		if entry.Sequence <= query.AfterSequence {
			continue
		}
		record := protocol.AuditRecord{Version: 1, Component: "mkruntimed", Sequence: entry.Sequence, At: entry.At, OperationID: entry.OperationID, SandboxID: entry.SandboxID, Generation: entry.Generation, Method: entry.Method, Phase: entry.Phase, Outcome: "pending", State: entry.State}
		if entry.Phase == "complete" {
			record.Outcome = "succeeded"
			if entry.Error != nil {
				record.Outcome = "failed"
				record.ErrorCode = entry.Error.Code
			}
		}
		if record.Validate() != nil {
			return protocol.AuditPage{}, apierr("INTERNAL", "audit journal metadata is invalid", false)
		}
		page.Records = append(page.Records, record)
		page.NextSequence = record.Sequence
		if len(page.Records) == int(query.PageLimit()) {
			break
		}
	}
	page.HasMore = page.NextSequence < page.HighWaterSequence
	return page, nil
}
