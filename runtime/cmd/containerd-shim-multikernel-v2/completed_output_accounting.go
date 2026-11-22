//go:build linux

package main

import (
	"errors"

	tasktypes "github.com/containerd/containerd/api/types/task"
)

const completedOutputAccountingLimit = 16

// A bounded counter-only diagnostic tail survives ordinary exec deletion while
// init is retained. It is not process incarnation or cleanup authority.
type completedOutputAccounting struct {
	Version             uint32                  `json:"version"`
	DeletedCount        uint64                  `json:"deleted_count"`
	EvictedCount        uint64                  `json:"evicted_count"`
	AccountingExhausted bool                    `json:"accounting_exhausted"`
	Records             []completedOutputRecord `json:"records"`
}

type completedOutputRecord struct {
	Sequence              uint64                    `json:"sequence"`
	ID                    string                    `json:"id"`
	PID                   uint32                    `json:"pid"`
	Status                tasktypes.Status          `json:"status"`
	Exit                  uint32                    `json:"exit"`
	StdoutOffset          uint64                    `json:"stdout_offset"`
	StderrOffset          uint64                    `json:"stderr_offset"`
	FinalOutputAccounting *finalOutputAccounting    `json:"final_output_accounting,omitempty"`
	OutputDisposition     *processOutputDisposition `json:"output_disposition,omitempty"`
}

func (r *completedOutputAccounting) validate() error {
	if r == nil {
		return nil
	}
	if r.Version != 1 || len(r.Records) > completedOutputAccountingLimit || uint64(len(r.Records)) > r.DeletedCount || r.EvictedCount != r.DeletedCount-uint64(len(r.Records)) || r.DeletedCount > uint64(len(r.Records)) && len(r.Records) != completedOutputAccountingLimit || r.AccountingExhausted && r.DeletedCount != ^uint64(0) {
		return errors.New("invalid completed output accounting tail")
	}
	for index, record := range r.Records {
		if record.Sequence != r.EvictedCount+uint64(index)+1 || !guestProcessIdentifier.MatchString(record.ID) || record.Exit > 255 || record.PID >= 1<<31 {
			return errors.New("invalid completed output accounting record")
		}
		switch record.Status {
		case tasktypes.Status_CREATED:
			if record.PID != 0 || record.Exit != 0 || record.StdoutOffset != 0 || record.StderrOffset != 0 || record.FinalOutputAccounting != nil || record.OutputDisposition != nil {
				return errors.New("created exec has completed output")
			}
		case tasktypes.Status_STOPPED:
			// Legacy unavailable state may omit PID; it never qualifies a report.
		default:
			return errors.New("completed output has nonterminal process state")
		}
		if err := record.FinalOutputAccounting.validate(record.ID, record.PID, record.Status, record.StdoutOffset, record.StderrOffset); err != nil {
			return err
		}
		if err := record.OutputDisposition.validate(record.StdoutOffset, record.StderrOffset); err != nil {
			return err
		}
	}
	return nil
}

func (r *completedOutputAccounting) append(id string, p *process) *completedOutputAccounting {
	next := &completedOutputAccounting{Version: 1}
	if r != nil {
		*next = *r
		next.Records = append([]completedOutputRecord(nil), r.Records...)
	}
	if next.DeletedCount == ^uint64(0) {
		next.AccountingExhausted = true
		return next
	}
	next.DeletedCount++
	value := completedOutputRecord{Sequence: next.DeletedCount, ID: id, PID: p.pid, Status: p.status, Exit: p.exit, StdoutOffset: p.stdoutOffset, StderrOffset: p.stderrOffset}
	if p.finalOutputAccounting != nil {
		snapshot := *p.finalOutputAccounting
		value.FinalOutputAccounting = &snapshot
		if snapshot.Report != nil {
			report := *snapshot.Report
			snapshot.Report = &report
		}
	}
	if p.outputDisposition != nil {
		snapshot := *p.outputDisposition
		value.OutputDisposition = &snapshot
	}
	if len(next.Records) == completedOutputAccountingLimit {
		copy(next.Records, next.Records[1:])
		next.Records = next.Records[:completedOutputAccountingLimit-1]
		next.EvictedCount++
	}
	next.Records = append(next.Records, value)
	return next
}
