//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

const outputSampleLimit = 64 << 10

type outputObservation struct {
	LimitPerStream  int      `json:"limit_per_stream"`
	StdoutBytes     int      `json:"stdout_sample_bytes"`
	StderrBytes     int      `json:"stderr_sample_bytes"`
	StdoutStart     uint64   `json:"stdout_start_offset"`
	StderrStart     uint64   `json:"stderr_start_offset"`
	StdoutEnd       uint64   `json:"stdout_end_offset"`
	StderrEnd       uint64   `json:"stderr_end_offset"`
	StdoutTruncated bool     `json:"stdout_upstream_truncated"`
	StderrTruncated bool     `json:"stderr_upstream_truncated"`
	StateStable     bool     `json:"process_state_stable"`
	Markers         []string `json:"stdout_markers_seen"`
}
type outputSample struct {
	Stdout          []byte `json:"stdout"`
	Stderr          []byte `json:"stderr"`
	StdoutOffset    uint64 `json:"stdout_offset"`
	StderrOffset    uint64 `json:"stderr_offset"`
	Status          string `json:"status"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
}
type outputWire struct {
	Stdout          json.RawMessage `json:"stdout"`
	Stderr          json.RawMessage `json:"stderr"`
	StdoutOffset    *uint64         `json:"stdout_offset"`
	StderrOffset    *uint64         `json:"stderr_offset"`
	Status          *string         `json:"status"`
	StdoutTruncated *bool           `json:"stdout_truncated"`
	StderrTruncated *bool           `json:"stderr_truncated"`
}

func (w outputWire) decode() (s outputSample, valid bool) {
	if w.StdoutOffset == nil || w.StderrOffset == nil || w.Status == nil || w.StdoutTruncated == nil || w.StderrTruncated == nil {
		return s, false
	}
	decode := func(raw json.RawMessage, dst *[]byte) bool {
		if len(raw) == 0 || len(raw) > ((outputSampleLimit+2)/3)*4+2 {
			return false
		}
		if !bytes.Equal(raw, []byte("null")) && raw[0] != '"' {
			return false
		}
		return json.Unmarshal(raw, dst) == nil && len(*dst) <= outputSampleLimit
	}
	if !decode(w.Stdout, &s.Stdout) || !decode(w.Stderr, &s.Stderr) {
		clear(s.Stdout)
		clear(s.Stderr)
		return outputSample{}, false
	}
	s.StdoutOffset, s.StderrOffset, s.Status, s.StdoutTruncated, s.StderrTruncated = *w.StdoutOffset, *w.StderrOffset, *w.Status, *w.StdoutTruncated, *w.StderrTruncated
	return s, true
}

// Only these constant, whole, newline-terminated stdout lines may be projected.
// Markers are clues from guest output, never independent build attestation.
var dockerProbeMarkers = []string{
	"G8_BUILD_WORKLOAD_BEGIN", "G8_STORAGE_PRESSURE_FULL", "G8_STORAGE_PRESSURE_FREED",
	"G8_MULTISTAGE_BUILD_PASS", "G8_REPEAT_BUILD_PASS", "G8_BUILT_IMAGE_RUN_PASS",
	"G8_STORAGE_PRESSURE_IMAGE_READY", "G8_FAILED_BUILD_REJECTED", "G8_BUILD_RESOURCES_REMOVED",
	"G8_STORAGE_PRESSURE_BUILD_RECOVERED", "G8_BUILD_WORKLOAD_PASS",
}

func projectOutput(sample outputSample) *outputObservation {
	if sample.Status != "STOPPED" || len(sample.Stdout) > outputSampleLimit || len(sample.Stderr) > outputSampleLimit || sample.StdoutOffset < uint64(len(sample.Stdout)) || sample.StderrOffset < uint64(len(sample.Stderr)) {
		return nil
	}
	r := &outputObservation{LimitPerStream: outputSampleLimit, StdoutBytes: len(sample.Stdout), StderrBytes: len(sample.Stderr), StdoutStart: sample.StdoutOffset - uint64(len(sample.Stdout)), StderrStart: sample.StderrOffset - uint64(len(sample.Stderr)), StdoutEnd: sample.StdoutOffset, StderrEnd: sample.StderrOffset, StdoutTruncated: sample.StdoutTruncated, StderrTruncated: sample.StderrTruncated, Markers: []string{}}
	seen := map[string]bool{}
	for i, line := range bytes.SplitAfter(sample.Stdout, []byte{'\n'}) {
		// A nonzero retained base may start in the middle of a line. Never accept
		// that first fragment or an unterminated last fragment as a whole marker.
		if len(line) == 0 || line[len(line)-1] != '\n' || i == 0 && r.StdoutStart != 0 {
			continue
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		for _, marker := range dockerProbeMarkers {
			if bytes.Equal(line, []byte(marker)) {
				seen[marker] = true
			}
		}
	}
	for _, marker := range dockerProbeMarkers {
		if seen[marker] {
			r.Markers = append(r.Markers, marker)
		}
	}
	return r
}
func inspectStoppedOutput(ctx context.Context, client *agent.Client, o options, r report, before agent.ProcessState) report {
	r.Status = "inconclusive"
	if before.Status != "STOPPED" {
		r.Code = "PROCESS_NOT_STOPPED"
		return r
	}
	r.OutputQueryAttempted = true
	var sample outputSample
	var wire outputWire
	defer func() { clear(sample.Stdout); clear(sample.Stderr); clear(wire.Stdout); clear(wire.Stderr) }()
	// Offset zero is deliberate: the existing API can acknowledge/discard older
	// buffer bytes when given a positive offset. Zero never advances that base.
	err := client.CallContext(ctx, "ReadProcessOutput", map[string]any{"id": o.processID, "stdout_offset": uint64(0), "stderr_offset": uint64(0), "limit": uint64(outputSampleLimit)}, &wire)
	failure := func(err error, code string) report {
		r.Code = code
		var timed net.Error
		if ctx.Err() != nil || errors.As(err, &timed) && timed.Timeout() {
			r.Code = "DEADLINE_OR_CANCELED"
		}
		return r
	}
	if err != nil {
		return failure(err, "OUTPUT_SAMPLE_UNAVAILABLE")
	}
	var decoded bool
	sample, decoded = wire.decode()
	if !decoded {
		r.Code = "OUTPUT_SAMPLE_MISMATCH"
		return r
	}
	projected := projectOutput(sample)
	if projected == nil {
		r.Code = "OUTPUT_SAMPLE_MISMATCH"
		return r
	}
	var after agent.ProcessState
	if err = client.CallContext(ctx, "StateProcess", map[string]string{"id": o.processID}, &after); err != nil {
		return failure(err, "FINAL_PROCESS_STATE_UNAVAILABLE")
	}
	if !validProcessState(after, o.processID) || after.Status != before.Status || after.PID != before.PID || after.ExitCode != before.ExitCode {
		r.Code = "PROCESS_STATE_CHANGED"
		r.Process = nil
		return r
	}
	projected.StateStable = true
	r.Output = projected
	r.Status = "observed"
	r.Code = "STOPPED_OUTPUT_SUMMARY_OBSERVED"
	return r
}
