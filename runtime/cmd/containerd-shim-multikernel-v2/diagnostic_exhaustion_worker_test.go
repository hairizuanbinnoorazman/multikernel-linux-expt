//go:build linux && diagnostic_exhaustion_worker

package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	hold := len(os.Args) == 2 && os.Args[1] == "--diagnostic-exhaustion-binding-hold"
	if probe || hold || os.Getenv("MK_SHIM_WORKER") == "1" && serverInvocation() {
		if !exhaustDiagnosticTestWorker() {
			os.Exit(93)
		}
	}
	if hold {
		if _, err := os.Stdout.WriteString("DIAGNOSTIC_EXHAUSTION_BINDING_READY\n"); err != nil {
			os.Exit(94)
		}
		for {
			time.Sleep(time.Hour)
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
	processStat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return false
	}
	end := strings.LastIndexByte(string(processStat), ')')
	if end < 0 {
		return false
	}
	fields := strings.Fields(string(processStat)[end+1:])
	if len(fields) < 20 {
		return false
	}
	startTicks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || startTicks == 0 {
		return false
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || len(strings.TrimSpace(string(boot))) != 36 {
		return false
	}
	bundle, err := os.Stat(".")
	if err != nil {
		return false
	}
	identity, ok := bundle.Sys().(*syscall.Stat_t)
	if !ok || !bundle.IsDir() || identity.Uid != uint32(os.Geteuid()) || bundle.Mode().Perm()&0022 != 0 {
		return false
	}
	receipt := struct {
		Version          int    `json:"version"`
		Scope            string `json:"scope"`
		SourceRevision   string `json:"source_revision"`
		BootID           string `json:"boot_id"`
		WorkerStartTicks uint64 `json:"worker_start_ticks"`
		BundleDevice     uint64 `json:"bundle_device"`
		BundleInode      uint64 `json:"bundle_inode"`
		BundleUID        uint32 `json:"bundle_uid"`
		WorkerPID        int    `json:"worker_pid"`
		Limit            uint64 `json:"limit_bytes"`
		Reserved         uint64 `json:"reserved_bytes"`
		Attempts         int    `json:"synthetic_preload_attempts"`
		Admitted         int    `json:"synthetic_preload_admitted"`
		Closed           bool   `json:"allowance_closed_before_production_entry"`
		Instrumented     bool   `json:"instrumented_test_executable"`
	}{2, "diagnostic-exhaustion-test-worker", buildinfo.Revision, strings.TrimSpace(string(boot)), startTicks, uint64(identity.Dev), identity.Ino, identity.Uid, os.Getpid(), processDiagnosticLifetimeByteLimit, sink.reservedBytes.Load(), attempts, admitted, true, true}
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
