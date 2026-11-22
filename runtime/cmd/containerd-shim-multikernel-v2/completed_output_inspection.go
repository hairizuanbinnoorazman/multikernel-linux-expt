//go:build linux

package main

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"time"

	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
)

const completedOutputInspectionLimit = 64 << 10

type completedOutputInspection struct {
	Version        uint32                       `json:"version"`
	Scope          string                       `json:"scope"`
	Status         string                       `json:"status"`
	Code           string                       `json:"code"`
	SandboxID      string                       `json:"sandbox_id,omitempty"`
	Generation     string                       `json:"generation,omitempty"`
	TaskID         string                       `json:"task_id,omitempty"`
	Namespace      string                       `json:"namespace,omitempty"`
	BundleIdentity *rootfspkg.DirectoryIdentity `json:"bundle_identity,omitempty"`
	Tail           *completedOutputAccounting   `json:"tail,omitempty"`
}

func completedInspection(status, code string) completedOutputInspection {
	return completedOutputInspection{Version: 1, Scope: "private-shim-completed-output", Status: status, Code: code}
}

// A consistent private recovery-file snapshot, not a live worker/kernel probe.
func observeCompletedOutput(bundle, namespace, task string) completedOutputInspection {
	fail := func() completedOutputInspection {
		return completedInspection("inconclusive", "COMPLETED_OUTPUT_UNAVAILABLE")
	}
	s := &service{bundle: bundle, namespace: namespace, id: task}
	defer s.closeBundleIdentity()
	directory, err := s.ensureRuntimeDirectory()
	if err != nil {
		return fail()
	}
	saved, found, _, err := loadPersistedRecoveryFromDirectory(directory, namespace, task)
	if err != nil || !found || saved.BundleIdentity != s.bundleIdentity {
		return fail()
	}
	if _, err = s.ensureRuntimeDirectory(); err != nil {
		return fail()
	}
	report := completedInspection("observed", "COMPLETED_OUTPUT_OBSERVED")
	report.SandboxID, report.Generation, report.TaskID, report.Namespace = saved.ID, saved.Generation, task, namespace
	identity := saved.BundleIdentity
	report.BundleIdentity = &identity
	report.Tail = saved.CompletedOutputAccounting
	if report.Tail == nil {
		report.Status, report.Code = "unobserved", "COMPLETED_OUTPUT_NOT_OBSERVED"
	} else if report.Tail.AccountingExhausted {
		report.Status, report.Code = "unqualified", "COMPLETED_OUTPUT_EXHAUSTED"
	}
	return report
}
func observeCompletedOutputBudget(ctx context.Context, source func() completedOutputInspection) completedOutputInspection {
	timeout := func() completedOutputInspection {
		return completedInspection("inconclusive", "COMPLETED_OUTPUT_TIMEOUT")
	}
	if ctx.Err() != nil {
		return timeout()
	}
	result := make(chan completedOutputInspection, 1)
	go func() { result <- source() }()
	select {
	case value := <-result:
		if ctx.Err() != nil {
			return timeout()
		}
		return value
	case <-ctx.Done():
		return timeout()
	}
}
func writeCompletedOutputInspection(out io.Writer, report completedOutputInspection) bool {
	data, err := json.Marshal(report)
	if err != nil || len(data)+1 > completedOutputInspectionLimit {
		return false
	}
	data = append(data, '\n')
	done := make(chan bool, 1)
	timer := time.NewTimer(intentOutputBudget)
	defer timer.Stop()
	go func() { n, err := out.Write(data); done <- err == nil && n == len(data) }()
	select {
	case ok := <-done:
		return ok
	case <-timer.C:
		return false
	}
}

// Standalone only: process exit bounds a blocked filesystem or output worker.
func runCompletedOutputInspection(args []string, out io.Writer) int {
	if len(args) != 6 || args[0] != "--bundle" || len(args[1]) > 4096 || !filepath.IsAbs(args[1]) || filepath.Clean(args[1]) != args[1] || args[2] != "--namespace" || !runtimeIdentifier.MatchString(args[3]) || args[4] != "--task" || !runtimeIdentifier.MatchString(args[5]) {
		if !writeCompletedOutputInspection(out, completedInspection("inconclusive", "COMPLETED_OUTPUT_ARGUMENTS_INVALID")) {
			return 1
		}
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), intentObservationBudget)
	defer cancel()
	report := observeCompletedOutputBudget(ctx, func() completedOutputInspection { return observeCompletedOutput(args[1], args[3], args[5]) })
	if !writeCompletedOutputInspection(out, report) || report.Status == "inconclusive" {
		return 1
	}
	return 0
}
