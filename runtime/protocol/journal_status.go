package protocol

import (
	"encoding/json"
	"errors"
)

const JournalStatusCapability = "journal-status-v1"

// JournalStatusReport observes fail-closed in-process accounting. It never
// asserts disk writability, recovery completion or service/allocation readiness.
type JournalStatusReport struct {
	Version            int    `json:"version"`
	Source             string `json:"source"`
	Status             string `json:"status"`
	Sequence           uint64 `json:"sequence"`
	PendingCompletions uint64 `json:"pending_completions"`
	JournalFailed      bool   `json:"journal_failed"`
}

func (r JournalStatusReport) Validate() error {
	if r.Version != 1 || r.Source != "in-process-lifecycle-journal" || r.PendingCompletions > r.Sequence {
		return errors.New("invalid lifecycle journal status")
	}
	status := "idle"
	if r.PendingCompletions > 0 {
		status = "pending"
	}
	if r.JournalFailed {
		status = "failed"
	}
	if r.Status != status {
		return errors.New("invalid lifecycle journal status")
	}
	return nil
}

// Require every field on the wire. In particular, omitted/null failure flags
// and counters must not inherit Go zero values and masquerade as idle.
func (r *JournalStatusReport) UnmarshalJSON(data []byte) error {
	*r = JournalStatusReport{}
	if len(data) > 4096 {
		return errors.New("lifecycle journal status exceeds its wire bound")
	}
	// encoding/json matches struct fields without case sensitivity. Require
	// exact keys so a second case variant cannot mask the failure flag.
	var fields map[string]json.RawMessage
	if err := StrictDecode(data, &fields); err != nil || len(fields) != 6 {
		return errors.New("invalid lifecycle journal status fields")
	}
	for _, name := range []string{"version", "source", "status", "sequence", "pending_completions", "journal_failed"} {
		if _, found := fields[name]; !found {
			return errors.New("invalid lifecycle journal status fields")
		}
	}
	var wire struct {
		Version            *int    `json:"version"`
		Source             *string `json:"source"`
		Status             *string `json:"status"`
		Sequence           *uint64 `json:"sequence"`
		PendingCompletions *uint64 `json:"pending_completions"`
		JournalFailed      *bool   `json:"journal_failed"`
	}
	if err := StrictDecode(data, &wire); err != nil {
		return errors.New("invalid lifecycle journal status JSON")
	}
	if wire.Version == nil || wire.Source == nil || wire.Status == nil || wire.Sequence == nil || wire.PendingCompletions == nil || wire.JournalFailed == nil {
		return errors.New("incomplete lifecycle journal status")
	}
	v := JournalStatusReport{Version: *wire.Version, Source: *wire.Source, Status: *wire.Status, Sequence: *wire.Sequence, PendingCompletions: *wire.PendingCompletions, JournalFailed: *wire.JournalFailed}
	if err := v.Validate(); err != nil {
		return err
	}
	*r = v
	return nil
}
