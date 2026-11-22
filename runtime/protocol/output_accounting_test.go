package protocol

import (
	"encoding/json"
	"testing"
)

func TestOutputAccountingRejectsImpossibleConservationAndIdentity(t *testing.T) {
	empty := OutputBufferAccounting{LimitBytes: ProcessOutputBufferLimit}
	report := ProcessOutputAccounting{Version: 1, ID: "init", PID: 41, Status: "STOPPED", Stdout: empty, Stderr: empty}
	if err := report.Validate("init", 41); err != nil {
		t.Fatal(err)
	}
	cases := []ProcessOutputAccounting{}
	for _, change := range []func(*ProcessOutputAccounting){
		func(r *ProcessOutputAccounting) { r.Version = 0 }, func(r *ProcessOutputAccounting) { r.ID = "other" }, func(r *ProcessOutputAccounting) { r.PID = 42 },
		func(r *ProcessOutputAccounting) { r.Status = "UNKNOWN" }, func(r *ProcessOutputAccounting) { r.Status = "CREATED" },
		func(r *ProcessOutputAccounting) { r.Stdout.LimitBytes = 0 }, func(r *ProcessOutputAccounting) { r.Stdout.ObservedBytes = 1 },
		func(r *ProcessOutputAccounting) { r.Stdout.AccountingExhausted = true }, func(r *ProcessOutputAccounting) { r.Stdout.Truncated = true },
		func(r *ProcessOutputAccounting) { r.Stdout.ObservedBytes = 1; r.Stdout.DroppedBytes = 1 },
		func(r *ProcessOutputAccounting) { r.Stdout.ObservedBytes = 1; r.Stdout.BufferedBytes = 1 },
		func(r *ProcessOutputAccounting) {
			r.Stdout.ObservedBytes = 1
			r.Stdout.BufferedPeakBytes = 2
			r.Stdout.AcknowledgedBytes = 1
		},
		func(r *ProcessOutputAccounting) {
			r.Stdout.ObservedBytes = ^uint64(0)
			r.Stdout.AcknowledgedBytes = ^uint64(0)
			r.Stdout.DroppedBytes = 1
			r.Stdout.Truncated = true
		},
	} {
		r := report
		change(&r)
		cases = append(cases, r)
	}
	for i, r := range cases {
		if r.Validate("init", 41) == nil {
			t.Fatal("impossible accounting accepted", i)
		}
	}
	// Both streams use the same validator.
	r := report
	r.Stderr = cases[len(cases)-1].Stdout
	if r.Validate("init", 41) == nil {
		t.Fatal("stderr overflow accepted")
	}
	for _, wire := range []string{`{"version":true}`, `{"pid":1.5}`, `{"stdout":{"observed_bytes":-1}}`, `{"stdout":{"observed_bytes":1.0}}`} {
		var r ProcessOutputAccounting
		if json.Unmarshal([]byte(wire), &r) == nil {
			t.Fatal("numeric alias accepted")
		}
	}
}
