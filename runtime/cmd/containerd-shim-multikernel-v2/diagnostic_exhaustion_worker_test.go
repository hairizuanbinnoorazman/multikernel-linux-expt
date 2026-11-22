//go:build linux && diagnostic_exhaustion_worker

package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
)

const diagnosticExhaustionFixtureReceipt = "diagnostic-exhaustion.fixture.json"

func init() { diagnosticExhaustionTestEntry = runDiagnosticExhaustionTestEntry }

// This test executable consumes the default sink's actual allowance before
// entering the production shim. It does not change the cap or replace transport,
// monitor, guest responses, journals or lifecycle code. Go build excludes it.
func runDiagnosticExhaustionTestEntry() bool {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.") {
			return false
		}
	}
	probe := len(os.Args) == 2 && os.Args[1] == "--diagnostic-exhaustion-probe"
	if probe || os.Getenv("MK_SHIM_WORKER") == "1" && serverInvocation() {
		if !exhaustDiagnosticTestWorker() {
			os.Exit(93)
		}
	}
	if !probe {
		main()
	}
	return true
}

func exhaustDiagnosticTestWorker() bool {
	sink := processDiagnosticDefault()
	record := processDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2", Operation: processDiagnosticSpecs[diagnosticOutputRead].operation, Code: processDiagnosticSpecs[diagnosticOutputRead].code, SandboxID: "qualification", Generation: strings.Repeat("0", 32), ProcessID: "synthetic-preload", At: "2026-01-01T00:00:00Z"}
	attempts, admitted := 0, 0
	for attempts < 10000 && !sink.budgetClosed.Load() {
		attempts++
		if sink.tryRecord(record) {
			admitted++
		}
	}
	if !sink.budgetClosed.Load() || sink.disabled.Load() || sink.reservedBytes.Load() > processDiagnosticLifetimeByteLimit {
		return false
	}
	receipt := struct {
		Version        int    `json:"version"`
		Scope          string `json:"scope"`
		SourceRevision string `json:"source_revision"`
		WorkerPID      int    `json:"worker_pid"`
		Limit          uint64 `json:"limit_bytes"`
		Reserved       uint64 `json:"reserved_bytes"`
		Attempts       int    `json:"synthetic_preload_attempts"`
		Admitted       int    `json:"synthetic_preload_admitted"`
		Closed         bool   `json:"allowance_closed_before_production_entry"`
		Instrumented   bool   `json:"instrumented_test_executable"`
	}{1, "diagnostic-exhaustion-test-worker", buildinfo.Revision, os.Getpid(), processDiagnosticLifetimeByteLimit, sink.reservedBytes.Load(), attempts, admitted, true, true}
	data, err := json.Marshal(receipt)
	if err != nil {
		return false
	}
	// The supervisor pins the private bundle cwd. Never follow or replace an
	// existing receipt: a restarted worker must refuse stale proof.
	file, err := os.OpenFile(diagnosticExhaustionFixtureReceipt, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false
	}
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	return err == nil && closeErr == nil
}
