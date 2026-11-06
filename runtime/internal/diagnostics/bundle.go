// Package diagnostics retains bounded structured observations for an operator.
// It never collects raw logs, credentials, environment or guest output.
package diagnostics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

const MaxBundleBytes = 8 << 20
const Scope = "operational-and-component-observation-snapshot"

// Snapshot contains only reports produced by the validated, redacting doctor
// APIs. The two reports are separate sampled observations, not an atomic view.
type Snapshot struct {
	Components  doctor.ComponentsReport  `json:"components"`
	Operational doctor.OperationalReport `json:"operational"`
}

type Bundle struct {
	Version          int      `json:"version"`
	Scope            string   `json:"scope"`
	CollectionStatus string   `json:"collection_status"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at"`
	Snapshot         Snapshot `json:"snapshot"`
	Excluded         []string `json:"excluded"`
}

// Receipt attests only to publication of these bytes. Neither collection_status
// nor a consistent observation status establishes readiness or containment.
type Receipt struct {
	Version           int    `json:"version"`
	Scope             string `json:"scope"`
	CollectionStatus  string `json:"collection_status"`
	ObservationStatus string `json:"observation_status"`
	Bytes             int    `json:"bytes"`
	SHA256            string `json:"sha256"`
}

var destinationName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,122}\.json$`)

func ValidDestination(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && destinationName.MatchString(filepath.Base(path))
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if len(data) > MaxBundleBytes-b.Len() {
		return 0, errors.New("diagnostic bundle exceeds the byte limit")
	}
	return b.Buffer.Write(data)
}

func observationStatus(snapshot Snapshot) (string, bool) {
	return aggregateStatus(snapshot.Components.Status, snapshot.Operational.Status)
}

func aggregateStatus(values ...string) (string, bool) {
	status := "consistent"
	for _, value := range values {
		switch value {
		case "degraded":
			status = "degraded"
		case "inconclusive":
			if status != "degraded" {
				status = "inconclusive"
			}
		case "consistent":
		default:
			return "", false
		}
	}
	return status, true
}

// Collect uses one caller budget for all observations. Expired or unavailable
// observations remain diagnostic results and can still be saved. Publication is
// a local bounded write; it does not retry inspection or extend its deadline.
//
// The parent must already exist, be caller-owned and mode 0700. OpenDirectory
// rejects symlinks throughout the path without creating/chmodding anything.
// Publication is descriptor-relative, synced and exclusive: an existing file,
// directory, symlink or raced competing collector is never overwritten. Errors
// are fixed strings, excluding private paths and raw filesystem/RPC errors.
func Collect(ctx context.Context, path string, inspect func(context.Context) Snapshot) (Receipt, error) {
	return collect(ctx, path, nil, inspect)
}

// CollectWithBudget adds an opt-in aggregate file/count guard. The directory is
// dedicated to private JSON artifacts. Other files, unsafe metadata or observed
// drift refuse capture. Existing evidence is never removed to make room.
func CollectWithBudget(ctx context.Context, path string, budget DirectoryBudget, inspect func(context.Context) Snapshot) (Receipt, error) {
	if !budget.Valid() {
		return Receipt{}, errors.New("invalid diagnostic directory budget")
	}
	return collect(ctx, path, &budget, inspect)
}

// capturedSnapshot is constructed only by the typed legacy/health adapters.
type capturedSnapshot struct {
	version       int
	scope, status string
	snapshot      any
}

type bundleEnvelope struct {
	Version          int      `json:"version"`
	Scope            string   `json:"scope"`
	CollectionStatus string   `json:"collection_status"`
	StartedAt        string   `json:"started_at"`
	FinishedAt       string   `json:"finished_at"`
	Snapshot         any      `json:"snapshot"`
	Excluded         []string `json:"excluded"`
}

func collect(ctx context.Context, path string, budget *DirectoryBudget, inspect func(context.Context) Snapshot) (Receipt, error) {
	if inspect == nil {
		return Receipt{}, errors.New("invalid diagnostic bundle destination or collector")
	}
	return collectSnapshot(ctx, path, budget, func(ctx context.Context) (capturedSnapshot, error) {
		snapshot := inspect(ctx)
		status, valid := observationStatus(snapshot)
		if !valid || snapshot.Components.Version != 1 || snapshot.Components.Scope != "compiled-runtime-network-build-self-reports" || snapshot.Operational.Version != 7 || snapshot.Operational.Scope != "resource-network-pool-and-compiled-component-observations" {
			return capturedSnapshot{}, errors.New("diagnostic collector returned an invalid report contract")
		}
		return capturedSnapshot{version: 1, scope: Scope, status: status, snapshot: snapshot}, nil
	})
}

func collectSnapshot(ctx context.Context, path string, budget *DirectoryBudget, inspect func(context.Context) (capturedSnapshot, error)) (Receipt, error) {
	fail := func(message string) (Receipt, error) { return Receipt{}, errors.New(message) }
	if !ValidDestination(path) || inspect == nil {
		return fail("invalid diagnostic bundle destination or collector")
	}
	parent, err := safefile.OpenDirectory(filepath.Dir(path), false)
	if err != nil {
		return fail("diagnostic bundle parent must be an existing private owned directory without symlinks")
	}
	defer parent.Close()
	// All current collectors share this directory-inode lock, including legacy
	// mode. Contention refuses immediately, without a worker, wait or retry.
	locked, err := parent.TryLockExclusive()
	if err != nil || !locked {
		return fail("diagnostic bundle directory is busy or unavailable")
	}
	var before directoryInventory
	if budget != nil {
		before, err = inventory(parent)
		if err != nil {
			return fail("diagnostic directory inventory is unavailable or unsafe")
		}
		if len(before.files) >= budget.MaxFiles || before.bytes >= budget.MaxBytes {
			return fail("diagnostic directory retention limit reached")
		}
	}
	name := filepath.Base(path)
	if _, exists, err := parent.EntryIdentity(name); err != nil || exists {
		return fail("diagnostic bundle destination is unavailable or already exists")
	}
	started := time.Now().UTC().Format(time.RFC3339Nano)
	capture, err := inspect(ctx)
	if err != nil {
		return Receipt{}, err
	}
	bundle := bundleEnvelope{Version: capture.version, Scope: capture.scope, CollectionStatus: "captured", StartedAt: started,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Snapshot: capture.snapshot,
		Excluded: []string{"raw-logs", "guest-output", "credentials", "environment", "configuration-files", "state-files", "process-command-lines", "kernel-console", "service-readiness", "executable-attestation", "kernel-attestation", "containment"}}
	var output boundedBuffer
	if json.NewEncoder(&output).Encode(bundle) != nil {
		return fail("diagnostic bundle encoding failed or exceeded the byte limit")
	}
	if budget != nil {
		after, err := inventory(parent)
		if err != nil || !before.same(after) {
			return fail("diagnostic directory contents changed during collection")
		}
		if int64(output.Len()) > budget.MaxBytes-before.bytes {
			return fail("diagnostic directory retention limit reached")
		}
	}
	parentStillNamed := func() bool {
		current, err := safefile.OpenDirectory(filepath.Dir(path), false)
		if err != nil {
			return false
		}
		defer current.Close()
		return current.Identity() == parent.Identity()
	}
	if !parentStillNamed() {
		return fail("diagnostic bundle parent identity changed")
	}
	created, err := parent.PublishExclusive(name, output.Bytes(), 0600)
	if err != nil {
		// A directory-sync error may leave the completed file present. Never
		// remove it or retry at this name; preserve it for operator inspection.
		return fail("diagnostic bundle publication or synchronization failed; inspect the destination before retrying")
	}
	if !created {
		return fail("diagnostic bundle destination already exists")
	}
	if !parentStillNamed() {
		return fail("diagnostic bundle parent identity changed after publication; inspect the original directory")
	}
	digest := sha256.Sum256(output.Bytes())
	return Receipt{Version: capture.version, Scope: capture.scope, CollectionStatus: "captured", ObservationStatus: capture.status, Bytes: output.Len(), SHA256: hex.EncodeToString(digest[:])}, nil
}
