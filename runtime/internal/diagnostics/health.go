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
	return doctor.HealthObservationStatus(r)
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
