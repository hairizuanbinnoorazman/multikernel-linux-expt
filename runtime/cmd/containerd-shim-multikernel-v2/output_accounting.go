//go:build linux

package main

import (
	"errors"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// This record qualifies guest-buffer counters only. It does not qualify host
// delivery, logger retention, process incarnation, or lifecycle authority.
type finalOutputAccounting struct {
	State  string                            `json:"state"`
	Report *protocol.ProcessOutputAccounting `json:"report,omitempty"`
}

func (r *finalOutputAccounting) validate(id string, pid uint32, status tasktypes.Status, stdout, stderr uint64) error {
	if r == nil {
		return nil
	} // Older recovery records made no observation.
	if status != tasktypes.Status_STOPPED {
		return errors.New("final output accounting requires stopped recovery state")
	}
	switch r.State {
	case "unsupported", "unavailable":
		if r.Report == nil {
			return nil
		}
	case "validated":
		if r.Report == nil {
			break
		}
		if err := r.Report.Validate(id, int(pid)); err != nil {
			return err
		}
		if r.Report.Status != "STOPPED" {
			break
		}
		for _, stream := range []struct {
			report protocol.OutputBufferAccounting
			offset uint64
		}{{r.Report.Stdout, stdout}, {r.Report.Stderr, stderr}} {
			if stream.report.PendingBytes != 0 || stream.report.BufferedBytes != 0 || stream.report.AcknowledgedBytes != stream.offset {
				return errors.New("final guest output counters differ from drained offsets")
			}
		}
		return nil
	}
	return errors.New("invalid final guest output accounting")
}

// Use only the owning authenticated client, after the separate WaitProcess
// identity/exit check and final empty output read. Observation is bounded by the
// ordinary agent I/O deadline. A failed optional report never fabricates zeros
// and cannot replace or prevent the verified exit transition.
func (s *service) collectFinalOutputAccounting(id string, pid uint32, stdout, stderr uint64) *finalOutputAccounting {
	result := &finalOutputAccounting{State: "unsupported"}
	if !s.agentOutputAccounting.Load() {
		return result
	}
	result.State = "unavailable"
	var report protocol.ProcessOutputAccounting
	if err := s.callAgentWithReconnect("ProcessOutputAccounting", map[string]string{"id": id}, &report); err != nil {
		return result
	}
	candidate := &finalOutputAccounting{State: "validated", Report: &report}
	if candidate.validate(id, pid, tasktypes.Status_STOPPED, stdout, stderr) == nil {
		return candidate
	}
	return result
}
