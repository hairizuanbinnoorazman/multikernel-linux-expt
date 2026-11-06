package diagnostics

import (
	"context"
	"errors"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
)

const HealthScope = "operational-health-and-component-observation-snapshot"

type HealthSnapshot struct {
	Components doctor.ComponentsReport `json:"components"`
	Health     doctor.HealthReport     `json:"health"`
}
type HealthBundle struct {
	Version          int            `json:"version"`
	Scope            string         `json:"scope"`
	CollectionStatus string         `json:"collection_status"`
	StartedAt        string         `json:"started_at"`
	FinishedAt       string         `json:"finished_at"`
	Snapshot         HealthSnapshot `json:"snapshot"`
	Excluded         []string       `json:"excluded"`
}

// CollectHealth retains the validated doctor's v8 health observation in a v2
// bundle. The callback is trusted internal code, not arbitrary JSON. The shared
// publisher preserves legacy path, locking, bounds and exclusive sync policy.
func CollectHealth(ctx context.Context, path string, inspect func(context.Context) HealthSnapshot) (Receipt, error) {
	return collectHealth(ctx, path, nil, inspect)
}
func CollectHealthWithBudget(ctx context.Context, path string, budget DirectoryBudget, inspect func(context.Context) HealthSnapshot) (Receipt, error) {
	if !budget.Valid() {
		return Receipt{}, errors.New("invalid diagnostic directory budget")
	}
	return collectHealth(ctx, path, &budget, inspect)
}

func healthCaptureStatus(r doctor.HealthReport) (string, bool) {
	if r.Version != 8 || r.Scope != "operational-journal-and-recorded-network-health-observations" || r.AdmissionGuaranteed {
		return "", false
	}
	if r.Operational == nil && r.Journal == nil && r.Network == nil {
		return r.Status, r.Status == "inconclusive" && !r.BootStable
	}
	if r.Operational == nil || r.Journal == nil || r.Network == nil || !r.BootStable || r.Journal.Validate() != nil || r.Network.Validate() != nil {
		return "", false
	}
	op := r.Operational
	if op.Version != 7 || op.Scope != "resource-network-pool-and-compiled-component-observations" || !op.BootStable || op.BootID != r.BootID || r.Network.PrimaryBootID != r.BootID || op.AdmissionGuaranteed || op.ArtifactVerified || op.KernelVerified {
		return "", false
	}
	status := op.Status
	if status != "consistent" && status != "degraded" && status != "inconclusive" {
		return "", false
	}
	if r.Journal.Status == "failed" || r.Network.Status == "degraded" {
		status = "degraded"
	} else if r.Journal.Status == "pending" && status != "degraded" {
		status = "inconclusive"
	}
	return r.Status, r.Status == status
}

func collectHealth(ctx context.Context, path string, budget *DirectoryBudget, inspect func(context.Context) HealthSnapshot) (Receipt, error) {
	if inspect == nil {
		return Receipt{}, errors.New("invalid diagnostic bundle destination or collector")
	}
	return collectSnapshot(ctx, path, budget, func(ctx context.Context) (capturedSnapshot, error) {
		snapshot := inspect(ctx)
		status, valid := healthCaptureStatus(snapshot.Health)
		if snapshot.Components.Version != 1 || snapshot.Components.Scope != "compiled-runtime-network-build-self-reports" || snapshot.Components.ArtifactVerified || snapshot.Components.KernelVerified {
			valid = false
		}
		aggregate, aggregateValid := aggregateStatus(snapshot.Components.Status, status)
		if !valid || !aggregateValid {
			return capturedSnapshot{}, errors.New("diagnostic collector returned an invalid health report contract")
		}
		return capturedSnapshot{version: 2, scope: HealthScope, status: aggregate, snapshot: snapshot}, nil
	})
}
