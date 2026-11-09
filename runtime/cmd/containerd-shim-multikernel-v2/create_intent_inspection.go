//go:build linux

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

const intentObservationBudget = 2 * time.Second
const intentOutputBudget = 50 * time.Millisecond

type createIntentInspection struct {
	Version             int    `json:"version"`
	Component           string `json:"component"`
	Scope               string `json:"scope"`
	Status              string `json:"status"`
	Code                string `json:"code"`
	ContentsObserved    bool   `json:"contents_observed"`
	CleanupAuthorized   bool   `json:"cleanup_authorized"`
	AdmissionGuaranteed bool   `json:"admission_guaranteed"`
	RecoveryComplete    bool   `json:"recovery_complete"`
}

func intentInspection(status, code string) createIntentInspection {
	return createIntentInspection{Version: 1, Component: "containerd-shim-multikernel-v2", Scope: "create-intent-marker", Status: status, Code: code}
}

// The standalone command owns one observation worker. A blocked filesystem can
// retain it until process exit; this helper is not for resident polling loops.
func observeIntentBudget(ctx context.Context, source func() createIntentInspection) createIntentInspection {
	if ctx.Err() != nil {
		return intentInspection("inconclusive", "CREATE_INTENT_TIMEOUT")
	}
	done := make(chan createIntentInspection, 1)
	go func() { done <- source() }()
	select {
	case report := <-done:
		if ctx.Err() != nil {
			return intentInspection("inconclusive", "CREATE_INTENT_TIMEOUT")
		}
		return report
	case <-ctx.Done():
		return intentInspection("inconclusive", "CREATE_INTENT_TIMEOUT")
	}
}

func observeCreateIntent(bundle string, afterObservation func()) createIntentInspection {
	fail := func() createIntentInspection { return intentInspection("inconclusive", "CREATE_INTENT_UNAVAILABLE") }
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		return fail()
	}
	defer directory.Close()
	before, presentBefore, err := directory.EntryIdentity(createIntentName)
	if err != nil {
		return fail()
	}
	pending, err := directory.EntryOrRemovalPending(createIntentName, 256)
	if err != nil {
		return fail()
	}
	if afterObservation != nil {
		afterObservation()
	}
	after, presentAfter, err := directory.EntryIdentity(createIntentName)
	if err != nil {
		return fail()
	}
	current, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		return intentInspection("inconclusive", "CREATE_INTENT_CHANGED")
	}
	defer current.Close()
	if current.Identity() != directory.Identity() || presentBefore != presentAfter || before != after {
		return intentInspection("inconclusive", "CREATE_INTENT_CHANGED")
	}
	if pending || presentAfter {
		return intentInspection("pending", "CREATE_INTENT_PENDING")
	}
	return intentInspection("absent", "CREATE_INTENT_NOT_OBSERVED")
}

type intentStdoutWriter struct{}

func (intentStdoutWriter) Write(data []byte) (int, error) {
	fd, err := unix.FcntlInt(1, unix.F_DUPFD_CLOEXEC, 3)
	if err != nil {
		return 0, err
	}
	file := os.NewFile(uintptr(fd), "intent-inspection-stdout")
	defer file.Close()
	return file.Write(data)
}

// One bounded record/write attempt, with no retry or stderr fallback. The CLI
// exits after this call; a blocked writer never holds up that process exit.
func writeIntentInspection(out io.Writer, report createIntentInspection) bool {
	data, err := json.Marshal(report)
	if err != nil || len(data)+1 > 1024 {
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

func runCreateIntentInspection(args []string, out io.Writer) int {
	if len(args) != 2 || args[0] != "--bundle" || len(args[1]) > 4096 || !filepath.IsAbs(args[1]) || filepath.Clean(args[1]) != args[1] {
		if !writeIntentInspection(out, intentInspection("inconclusive", "CREATE_INTENT_ARGUMENTS_INVALID")) {
			return 1
		}
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), intentObservationBudget)
	defer cancel()
	report := observeIntentBudget(ctx, func() createIntentInspection { return observeCreateIntent(args[1], nil) })
	if !writeIntentInspection(out, report) || report.Status == "inconclusive" {
		return 1
	}
	return 0
}
