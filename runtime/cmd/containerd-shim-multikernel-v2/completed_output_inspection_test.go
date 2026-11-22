//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCompletedOutputInspectionBindsPrivateRecoveryAndExcludesPayload(t *testing.T) {
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	p := completedAccountingProcess("exec")
	p.stdinPending = []byte("SYNTHETIC_PRIVATE_PAYLOAD")
	p.stdin = "/SYNTHETIC_PRIVATE_PATH"
	s := &service{id: "task", namespace: "tests", bundle: bundle, sandbox: protocol.Sandbox{ID: sandboxID("tests", "task"), Generation: strings.Repeat("a", 32)}, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	defer s.closeBundleIdentity()
	var tail *completedOutputAccounting
	s.completedOutputAccounting = tail.append("exec", p)
	if err := s.ensureBundleIdentity(); err != nil {
		t.Fatal(err)
	}
	if err := s.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bundle, ".multikernel/sandbox.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := runCompletedOutputInspection([]string{"--bundle", bundle, "--namespace", "tests", "--task", "task"}, &output); code != 0 {
		t.Fatal("inspection refused", code)
	}
	var report completedOutputInspection
	if protocol.StrictDecode(output.Bytes(), &report) != nil || report.Status != "observed" || report.TaskID != "task" || report.Generation != s.sandbox.Generation || report.BundleIdentity == nil || *report.BundleIdentity != s.bundleIdentity || report.Tail.Records[0].Exit != 37 {
		t.Fatal("inspection lost identity or counters")
	}
	if bytes.Contains(output.Bytes(), []byte("SYNTHETIC_PRIVATE")) || output.Len() > completedOutputInspectionLimit {
		t.Fatal("inspection exposed payload/path")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("observation mutated recovery")
	}
	if observeCompletedOutput(bundle, "tests", "other").Status != "inconclusive" {
		t.Fatal("wrong task accepted")
	}
	saved := validPersistedRecovery("tests", "task")
	saved.BundleIdentity = s.bundleIdentity
	saved.BundleIdentity.Inode++
	raw, _ := json.Marshal(saved)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if observeCompletedOutput(bundle, "tests", "task").Status != "inconclusive" {
		t.Fatal("foreign bundle identity accepted")
	}
	saved.BundleIdentity = s.bundleIdentity
	raw, _ = json.Marshal(saved)
	os.WriteFile(path, raw, 0600)
	if observeCompletedOutput(bundle, "tests", "task").Status != "unobserved" {
		t.Fatal("legacy absence fabricated zero history")
	}
	os.Chmod(path, 0644)
	if observeCompletedOutput(bundle, "tests", "task").Status != "inconclusive" {
		t.Fatal("public recovery accepted")
	}
	t.Log("private_snapshot_bound=true payload_and_paths_excluded=true observation_read_only=true legacy_absence_unobserved=true")
}

func TestCompletedOutputInspectionBoundsAndClosedFailures(t *testing.T) {
	var output bytes.Buffer
	if runCompletedOutputInspection([]string{"--bundle", "SYNTHETIC_PRIVATE_ARGUMENT"}, &output) != 2 || bytes.Contains(output.Bytes(), []byte("SYNTHETIC_PRIVATE")) {
		t.Fatal("argument refusal leaked input")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if r := observeCompletedOutputBudget(ctx, func() completedOutputInspection { t.Fatal("canceled source ran"); return completedOutputInspection{} }); r.Code != "COMPLETED_OUTPUT_TIMEOUT" {
		t.Fatal("cancellation did not refuse")
	}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	done := make(chan completedOutputInspection, 1)
	go func() {
		done <- observeCompletedOutputBudget(ctx, func() completedOutputInspection {
			close(started)
			<-release
			close(finished)
			return completedInspection("observed", "COMPLETED_OUTPUT_OBSERVED")
		})
	}()
	<-started
	cancel()
	select {
	case r := <-done:
		if r.Status != "inconclusive" {
			t.Fatal("blocked source claimed success")
		}
	case <-time.After(time.Second):
		t.Fatal("source did not cancel")
	}
	close(release)
	<-finished
	for _, writer := range []interface{ Write([]byte) (int, error) }{intentShortWriter{}, intentFailureWriter{}} {
		if writeCompletedOutputInspection(writer, completedInspection("observed", "COMPLETED_OUTPUT_OBSERVED")) {
			t.Fatal("short/failed write accepted")
		}
	}
	writeStarted, writeRelease, writeFinished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	writeDone := make(chan bool, 1)
	go func() {
		writeDone <- writeCompletedOutputInspection(writerFunc(func(data []byte) (int, error) {
			close(writeStarted)
			<-writeRelease
			close(writeFinished)
			return len(data), nil
		}), completedInspection("observed", "COMPLETED_OUTPUT_OBSERVED"))
	}()
	<-writeStarted
	select {
	case ok := <-writeDone:
		if ok {
			t.Fatal("blocked output claimed success")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked output exceeded budget")
	}
	close(writeRelease)
	<-writeFinished
	report := completedInspection("observed", "COMPLETED_OUTPUT_OBSERVED")
	report.TaskID = strings.Repeat("x", completedOutputInspectionLimit)
	if writeCompletedOutputInspection(&output, report) {
		t.Fatal("oversized report written")
	}
}
