//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A continuously drained writer removes queue saturation as a possible bound.
// Only byte counts and acknowledgements are retained, never diagnostic bodies.
type diagnosticBudgetWriter struct {
	bytes     atomic.Uint64
	completed chan struct{}
}

func (w *diagnosticBudgetWriter) Write(p []byte) (int, error) {
	w.bytes.Add(uint64(len(p)))
	if w.completed != nil {
		w.completed <- struct{}{}
	}
	return len(p), nil
}

func TestDiagnosticLifetimeBudgetFreshMonitorsWithDrainedWriter(t *testing.T) {
	writer := &diagnosticBudgetWriter{completed: make(chan struct{}, 1)}
	sink := newProcessDiagnosticSink(writer)
	defer sink.close()
	accepted, rejected := 0, 0
	for i := 0; i < 10000; i++ {
		reporter := newProcessDiagnosticReporter("box", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", fmt.Sprintf("exec-%d", i), sink)
		if reporter.emit(diagnosticOutputRead, 0) {
			accepted++
			select {
			case <-writer.completed:
			case <-time.After(time.Second):
				t.Fatal("drained writer stalled")
			}
		} else {
			rejected++
		}
	}
	if accepted == 0 || rejected == 0 || writer.bytes.Load() > processDiagnosticLifetimeByteLimit || sink.reservedBytes.Load() > processDiagnosticLifetimeByteLimit {
		t.Fatal("fresh monitor identities bypassed lifetime budget", accepted, rejected, writer.bytes.Load(), sink.reservedBytes.Load())
	}
	if sink.dropped.Load() != uint64(rejected) {
		t.Fatal("budget refusals missing from drop accounting")
	}
	t.Logf("fresh_monitors=10000 accepted=%d refused=%d writer_bytes=%d limit=%d", accepted, rejected, writer.bytes.Load(), processDiagnosticLifetimeByteLimit)
}

func TestDiagnosticLifetimeBudgetConcurrentAdmissionAndExactBoundary(t *testing.T) {
	writer := &diagnosticBudgetWriter{}
	sink := newProcessDiagnosticSink(writer)
	defer sink.close()
	var producers sync.WaitGroup
	for i := 0; i < 32; i++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for n := 0; n < 2048; n++ {
				sink.tryRecord(diagnosticTestRecord())
			}
		}()
	}
	producers.Wait()
	if sink.reservedBytes.Load() > processDiagnosticLifetimeByteLimit || writer.bytes.Load() > processDiagnosticLifetimeByteLimit {
		t.Fatal("concurrent reservation exceeded byte budget")
	}
	exactWriter := &diagnosticBudgetWriter{completed: make(chan struct{}, 1)}
	exact := newProcessDiagnosticSink(exactWriter)
	defer exact.close()
	record := diagnosticTestRecord()
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	exact.reservedBytes.Store(processDiagnosticLifetimeByteLimit - uint64(len(encoded)+1))
	if !exact.tryRecord(record) {
		t.Fatal("last fitting frame refused")
	}
	select {
	case <-exactWriter.completed:
	case <-time.After(time.Second):
		t.Fatal("last frame stalled")
	}
	if exact.reservedBytes.Load() != processDiagnosticLifetimeByteLimit || exact.tryRecord(record) {
		t.Fatal("exhausted budget reset")
	}
	// Every typed producer must use the same fully exhausted allowance.
	supervisor := newSupervisorDiagnostics(exact)
	if supervisor.emit(supervisorStartFailed, 0, 0, 0) {
		t.Fatal("supervisor bypassed shared budget")
	}
	logger, _ := libraryTestLogger(t, exact)
	before := exact.dropped.Load()
	logger.Error("SYNTHETIC_PRIVATE_DIAGNOSTIC_PRESSURE")
	if exact.dropped.Load() != before+1 {
		t.Fatal("library bypassed shared budget")
	}
	exact.reservedBytes.Store(^uint64(0))
	if exact.tryRecord(record) || exact.reservedBytes.Load() != ^uint64(0) {
		t.Fatal("overflowed counter regained allowance")
	}
}
