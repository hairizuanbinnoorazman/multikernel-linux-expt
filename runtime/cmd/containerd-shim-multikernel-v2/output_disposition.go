//go:build linux

package main

import (
	"errors"
	"io"
)

// Counts partition durably acknowledged offsets, not physical logger retention.
// ReplayExposureBytes is an upper bound for uncertain write attempts; a write
// intent survives a worker crash or failed offset commit without its payload.
type outputDisposition struct {
	WrittenBytes         uint64 `json:"written_bytes"`
	NoDestinationBytes   uint64 `json:"no_destination_bytes"`
	PressureDroppedBytes uint64 `json:"pressure_dropped_bytes"`
	UnknownPrefixBytes   uint64 `json:"unknown_prefix_bytes"`
	InFlightBytes        uint64 `json:"in_flight_bytes"`
	ReplayExposureBytes  uint64 `json:"replay_exposure_bytes"`
	ExposureExhausted    bool   `json:"exposure_exhausted"`
}

type processOutputDisposition struct {
	Version uint32            `json:"version"`
	Stdout  outputDisposition `json:"stdout"`
	Stderr  outputDisposition `json:"stderr"`
}

func (r outputDisposition) validate(offset uint64) error {
	remaining := offset
	for _, v := range []uint64{r.WrittenBytes, r.NoDestinationBytes, r.PressureDroppedBytes, r.UnknownPrefixBytes} {
		if v > remaining {
			return errors.New("output disposition exceeds durable offset")
		}
		remaining -= v
	}
	if remaining != 0 || r.InFlightBytes > processOutputChunk || r.InFlightBytes > ^uint64(0)-offset || r.ExposureExhausted && r.ReplayExposureBytes != ^uint64(0) {
		return errors.New("invalid output disposition")
	}
	return nil
}
func (r *processOutputDisposition) validate(stdout, stderr uint64) error {
	if r == nil {
		return nil
	}
	if r.Version != 1 {
		return errors.New("unsupported output disposition version")
	}
	if err := r.Stdout.validate(stdout); err != nil {
		return err
	}
	return r.Stderr.validate(stderr)
}
func (r *outputDisposition) expose(n uint64) {
	if n > ^uint64(0)-r.ReplayExposureBytes {
		r.ReplayExposureBytes = ^uint64(0)
		r.ExposureExhausted = true
		return
	}
	r.ReplayExposureBytes += n
}
func (r *outputDisposition) resolveIntent() { r.expose(r.InFlightBytes); r.InFlightBytes = 0 }

// Called with mu held. The returned value is independent of the previous
// snapshot, including when an offset commit has failed.
func (p *process) nextOutputDisposition() *processOutputDisposition {
	if p.outputDisposition != nil {
		value := *p.outputDisposition
		return &value
	}
	return &processOutputDisposition{Version: 1, Stdout: outputDisposition{UnknownPrefixBytes: p.stdoutOffset}, Stderr: outputDisposition{UnknownPrefixBytes: p.stderrOffset}}
}

func (s *service) prepareOutputDelivery(p *process, output processOutput) (stdout, stderr io.WriteCloser, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stdout, stderr = p.stdoutWriter, p.stderrWriter
	// Empty reads with no unresolved intent need no new disk publication.
	if (len(output.Stdout) == 0 || stdout == nil) && (len(output.Stderr) == 0 || stderr == nil) && (p.outputDisposition == nil || p.outputDisposition.Stdout.InFlightBytes == 0 && p.outputDisposition.Stderr.InFlightBytes == 0) {
		return stdout, stderr, nil
	}
	if s.sandbox.ID == "" {
		return nil, nil, errors.New("output write intent has no durable sandbox identity")
	}
	old := p.outputDisposition
	next := p.nextOutputDisposition()
	next.Stdout.resolveIntent()
	next.Stderr.resolveIntent()
	if stdout != nil {
		next.Stdout.InFlightBytes = uint64(len(output.Stdout))
	}
	if stderr != nil {
		next.Stderr.InFlightBytes = uint64(len(output.Stderr))
	}
	p.outputDisposition = next
	if err := s.persistRecovery(); err != nil {
		p.outputDisposition = old
		return nil, nil, err
	}
	return stdout, stderr, nil
}

func (r *outputDisposition) finish(n uint64, advance, dropped, configured, uncertain bool) {
	r.InFlightBytes = 0
	if uncertain {
		r.expose(n)
	}
	if !advance {
		return
	}
	switch {
	case !configured:
		r.NoDestinationBytes += n
	case dropped:
		r.PressureDroppedBytes += n
	default:
		r.WrittenBytes += n
	}
}
