//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type dispositionWriter struct{ write func([]byte) (int, error) }

func (w dispositionWriter) Write(p []byte) (int, error) { return w.write(p) }
func (dispositionWriter) Close() error                  { return nil }

func TestOutputDispositionMonitorPartitionsAndPartialExposure(t *testing.T) {
	for _, mode := range []string{"written", "no-destination", "pressure-drop", "partial-retry"} {
		t.Run(mode, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
				t.Fatal(err)
			}
			client := &outputAckAgent{offsets: make(chan uint64, 100)}
			p := &process{pid: 41, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
			calls := 0
			if mode != "no-destination" {
				p.stdoutWriter = dispositionWriter{write: func(data []byte) (int, error) {
					calls++
					if mode == "pressure-drop" {
						return 0, syscall.EAGAIN
					}
					if mode == "partial-retry" && calls == 1 {
						return 2, nil
					}
					return len(data), nil
				}}
			}
			if mode == "pressure-drop" {
				p.stdoutPressure = time.Now().Add(-31 * time.Second)
			}
			s := &service{id: "task", namespace: "tests", bundle: bundle, agent: client, sandbox: protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: strings.Repeat("a", 32)}, processes: map[string]*process{"": p}, publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			defer s.closeBundleIdentity()
			go s.waitProcess("init", "", p)
			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				t.Fatal("monitor did not finish")
			}
			saved, found, err := loadPersistedRecovery(filepath.Join(bundle, ".multikernel/sandbox.json"), "tests", "task")
			if err != nil || !found {
				t.Fatal(err)
			}
			r := saved.Processes[0].OutputDisposition
			if r == nil || r.validate(4, 0) != nil || r.Stdout.InFlightBytes != 0 || r.Stdout.UnknownPrefixBytes != 0 {
				t.Fatal("durable disposition missing or unqualified")
			}
			switch mode {
			case "written":
				if r.Stdout.WrittenBytes != 4 || r.Stdout.ReplayExposureBytes != 0 || calls != 1 {
					t.Fatal("written disposition differs")
				}
			case "no-destination":
				if r.Stdout.NoDestinationBytes != 4 || r.Stdout.ReplayExposureBytes != 0 || calls != 0 {
					t.Fatal("unconfigured discard not counted")
				}
			case "pressure-drop":
				if r.Stdout.PressureDroppedBytes != 4 || r.Stdout.ReplayExposureBytes != 0 {
					t.Fatal("bounded drop not counted")
				}
			case "partial-retry":
				if r.Stdout.WrittenBytes != 4 || r.Stdout.ReplayExposureBytes != 4 || calls != 2 {
					t.Fatal("partial retry lost conservative exposure")
				}
			}
			t.Logf("mode=%s written=%d no_destination=%d pressure_drop=%d replay_exposure=%d", mode, r.Stdout.WrittenBytes, r.Stdout.NoDestinationBytes, r.Stdout.PressureDroppedBytes, r.Stdout.ReplayExposureBytes)
		})
	}
}

func TestOutputDispositionIntentCannotBypassPersistence(t *testing.T) {
	p := &process{stdoutOffset: 7, stdoutWriter: dispositionWriter{write: func([]byte) (int, error) { t.Fatal("unexpected write"); return 0, nil }}}
	bundle := privateTestDirectory(t)
	s := &service{bundle: bundle, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, processes: map[string]*process{"": p}}
	output := processOutput{Stdout: []byte("once"), StdoutOffset: 11}
	if _, _, err := s.prepareOutputDelivery(p, output); err == nil || p.outputDisposition != nil {
		t.Fatal("unpublished intent escaped")
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	defer s.closeBundleIdentity()
	if _, _, err := s.prepareOutputDelivery(p, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel/sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value persisted
	if json.Unmarshal(data, &value) != nil {
		t.Fatal("intent unavailable")
	}
	first := value.Processes[0].OutputDisposition
	if first.Stdout.UnknownPrefixBytes != 7 || first.Stdout.InFlightBytes != 4 || first.Stdout.ReplayExposureBytes != 0 {
		t.Fatal("legacy prefix or bounded intent differs")
	}
	// Reconstruct from a durable in-flight record. No counter observation claims
	// that the prior worker's write actually occurred, even if it might have.
	p.outputDisposition = first
	if _, _, err = s.prepareOutputDelivery(p, output); err != nil {
		t.Fatal(err)
	}
	if first.Stdout.ReplayExposureBytes != 0 || p.outputDisposition.Stdout.ReplayExposureBytes != 4 || p.outputDisposition.Stdout.InFlightBytes != 4 {
		t.Fatal("uncertain intent mutated old snapshot or disappeared")
	}
	t.Log("intent_bytes=4 recovered_replay_exposure=4 legacy_unknown_prefix=7 payload_persisted=false")
}

func TestOutputDispositionRefusesForgedCountersAndNeverWraps(t *testing.T) {
	valid := outputDisposition{WrittenBytes: 7, NoDestinationBytes: 2, PressureDroppedBytes: 1, InFlightBytes: 4}
	if valid.validate(10) != nil {
		t.Fatal("valid partition refused")
	}
	for _, r := range []outputDisposition{{WrittenBytes: 11}, {WrittenBytes: 9}, {WrittenBytes: ^uint64(0), NoDestinationBytes: 11}, {WrittenBytes: 10, InFlightBytes: 4097}, {WrittenBytes: 10, ExposureExhausted: true}} {
		if r.validate(10) == nil {
			t.Fatal("forged conservation/bound/exhaustion accepted")
		}
	}
	r := outputDisposition{ReplayExposureBytes: ^uint64(0) - 1}
	r.expose(4)
	if r.ReplayExposureBytes != ^uint64(0) || !r.ExposureExhausted {
		t.Fatal("exposure wrapped")
	}
	r.expose(1)
	if r.ReplayExposureBytes != ^uint64(0) || !r.ExposureExhausted {
		t.Fatal("exposure regained qualification")
	}
	if (outputDisposition{WrittenBytes: ^uint64(0), InFlightBytes: 1}).validate(^uint64(0)) == nil {
		t.Fatal("intent offset overflow accepted")
	}
	value := validPersistedRecovery("tests", "task")
	value.Processes[0].StdoutOffset = 10
	value.Processes[0].OutputDisposition = &processOutputDisposition{Version: 1, Stdout: valid}
	if validatePersistedRecovery(value, "tests", "task") != nil {
		t.Fatal("valid recovery refused")
	}
	value.Processes[0].OutputDisposition.Stdout.WrittenBytes++
	if validatePersistedRecovery(value, "tests", "task") == nil {
		t.Fatal("forged recovery partition accepted")
	}
}
