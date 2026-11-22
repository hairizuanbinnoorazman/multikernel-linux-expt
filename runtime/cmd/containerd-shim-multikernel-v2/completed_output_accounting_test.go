//go:build linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func completedAccountingProcess(id string) *process {
	return &process{id: id, pid: 41, status: tasktypes.Status_STOPPED, exit: 37, stdoutOffset: 7, exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{}), outputDisposition: &processOutputDisposition{Version: 1, Stdout: outputDisposition{WrittenBytes: 7}}, finalOutputAccounting: &finalOutputAccounting{State: "validated", Report: &protocol.ProcessOutputAccounting{Version: 1, ID: id, PID: 41, Status: "STOPPED", Stdout: protocol.OutputBufferAccounting{ObservedBytes: 10, AcknowledgedBytes: 7, DroppedBytes: 3, BufferedPeakBytes: 7, LimitBytes: protocol.ProcessOutputBufferLimit, Truncated: true}, Stderr: protocol.OutputBufferAccounting{LimitBytes: protocol.ProcessOutputBufferLimit}}}}
}

func TestCompletedOutputAccountingDeleteAndRollbackAreAtomic(t *testing.T) {
	bundle := privateTestDirectory(t)
	p := completedAccountingProcess("exec")
	s := &service{id: "task", namespace: "tests", bundle: bundle, sandbox: protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: strings.Repeat("a", 32)}, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	defer s.closeBundleIdentity()
	request := &taskapi.DeleteRequest{ID: "task", ExecID: "exec"}
	if _, err := s.Delete(context.Background(), request); err == nil || s.processes["exec"] != p || p.deleting || s.completedOutputAccounting != nil {
		t.Fatal("failed deletion exposed archive or dropped process")
	}
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	saved, found, err := loadPersistedRecovery(filepath.Join(bundle, ".multikernel/sandbox.json"), "tests", "task")
	if err != nil || !found {
		t.Fatal(err)
	}
	tail := saved.CompletedOutputAccounting
	if len(saved.Processes) != 1 || saved.Processes[0].ID != "" || tail == nil || tail.DeletedCount != 1 || tail.EvictedCount != 0 || len(tail.Records) != 1 || tail.Records[0].ID != "exec" || tail.Records[0].Exit != 37 || tail.Records[0].OutputDisposition.Stdout.WrittenBytes != 7 || tail.Records[0].FinalOutputAccounting.Report.Stdout.DroppedBytes != 3 {
		t.Fatal("deleted exec lost final counters")
	}
	p.finalOutputAccounting.Report.Stdout.DroppedBytes = 999
	p.outputDisposition.Stdout.WrittenBytes = 999
	if s.completedOutputAccounting.Records[0].FinalOutputAccounting.Report.Stdout.DroppedBytes != 3 || s.completedOutputAccounting.Records[0].OutputDisposition.Stdout.WrittenBytes != 7 {
		t.Fatal("archive aliases deleted mutable process")
	}
	if _, err = s.Delete(context.Background(), request); err == nil || s.completedOutputAccounting.DeletedCount != 1 {
		t.Fatal("repeat delete appended duplicate")
	}
	// Loading/reconstruction preserves the tail, including subsequent deletes.
	other := &service{completedOutputAccounting: saved.CompletedOutputAccounting}
	other.completedOutputAccounting = other.completedOutputAccounting.append("next", completedAccountingProcess("next"))
	if other.completedOutputAccounting.validate() != nil || other.completedOutputAccounting.DeletedCount != 2 || saved.CompletedOutputAccounting.DeletedCount != 1 {
		t.Fatal("reconstructed archive lost sequence or mutated old snapshot")
	}
	t.Log("deleted_exec_absent=true retained_tail_records=1 verified_exit=37 written=7 guest_drop=3 rollback_no_duplicate=true")
}

func TestCompletedOutputAccountingBoundedTailEvictionAndOverflow(t *testing.T) {
	var tail *completedOutputAccounting
	for i := 1; i <= 1000; i++ {
		tail = tail.append(fmt.Sprintf("exec-%d", i), completedAccountingProcess(fmt.Sprintf("exec-%d", i)))
		if err := tail.validate(); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(tail)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Records) != 16 || tail.DeletedCount != 1000 || tail.EvictedCount != 984 || tail.Records[0].Sequence != 985 || tail.Records[15].Sequence != 1000 || len(raw) > 65536 {
		t.Fatal("tail exceeded retention bound")
	}
	old := tail
	tail = tail.append("last", completedAccountingProcess("last"))
	if old.Records[0].Sequence != 985 || tail.Records[0].Sequence != 986 {
		t.Fatal("eviction mutated prior snapshot")
	}
	tail.DeletedCount = ^uint64(0)
	tail.EvictedCount = tail.DeletedCount - 16
	for i := range tail.Records {
		tail.Records[i].Sequence = tail.EvictedCount + uint64(i) + 1
	}
	exhausted := tail.append("overflow", completedAccountingProcess("overflow"))
	if exhausted.validate() != nil || !exhausted.AccountingExhausted || exhausted.DeletedCount != ^uint64(0) || exhausted.Records[15].ID != "last" || tail.AccountingExhausted {
		t.Fatal("overflow wrapped or mutated prior tail")
	}
	t.Logf("recorded_deletions=1000 retained=16 evicted=984 encoded_bytes=%d overflow_sticky=true", len(raw))
}

func TestCompletedOutputAccountingStrictRecoveryRefusals(t *testing.T) {
	for name, mutate := range map[string]func(*completedOutputAccounting){
		"version": func(r *completedOutputAccounting) { r.Version = 2 }, "count": func(r *completedOutputAccounting) { r.DeletedCount++ }, "eviction": func(r *completedOutputAccounting) { r.EvictedCount++ }, "sequence": func(r *completedOutputAccounting) { r.Records[0].Sequence = 0 }, "id": func(r *completedOutputAccounting) { r.Records[0].ID = "/private" }, "pid": func(r *completedOutputAccounting) { r.Records[0].PID = 42 }, "exit": func(r *completedOutputAccounting) { r.Records[0].Exit = 256 }, "state": func(r *completedOutputAccounting) { r.Records[0].Status = tasktypes.Status_RUNNING }, "offset": func(r *completedOutputAccounting) { r.Records[0].StdoutOffset++ }, "exhausted": func(r *completedOutputAccounting) { r.AccountingExhausted = true }, "created-with-output": func(r *completedOutputAccounting) { r.Records[0].Status = tasktypes.Status_CREATED }, "oversize": func(r *completedOutputAccounting) {
			r.Records = append(r.Records, make([]completedOutputRecord, 16)...)
		}} {
		t.Run(name, func(t *testing.T) {
			var empty *completedOutputAccounting
			value := validPersistedRecovery("tests", "task")
			value.CompletedOutputAccounting = empty.append("exec", completedAccountingProcess("exec"))
			mutate(value.CompletedOutputAccounting)
			if validatePersistedRecovery(value, "tests", "task") == nil {
				t.Fatal("forged deleted-exec accounting accepted")
			}
		})
	}
	var empty *completedOutputAccounting
	if empty.validate() != nil || empty.append("created", &process{status: tasktypes.Status_CREATED}).validate() != nil {
		t.Fatal("legacy absence/never-started exec refused")
	}
}
