package diagnostics

import (
	"context"
	"errors"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
)

const DockerHealthScope = "operational-health-child-engine-and-component-observation-snapshot"

type DockerHealthSnapshot struct {
	Components   doctor.ComponentsReport   `json:"components"`
	DockerHealth doctor.DockerHealthReport `json:"docker_health"`
}

func CollectDockerHealth(ctx context.Context, path string, inspect func(context.Context) DockerHealthSnapshot) (Receipt, error) {
	return collectDockerHealth(ctx, path, nil, inspect)
}
func CollectDockerHealthWithBudget(ctx context.Context, path string, budget DirectoryBudget, inspect func(context.Context) DockerHealthSnapshot) (Receipt, error) {
	if !budget.Valid() {
		return Receipt{}, errors.New("invalid diagnostic directory budget")
	}
	return collectDockerHealth(ctx, path, &budget, inspect)
}
func collectDockerHealth(ctx context.Context, path string, budget *DirectoryBudget, inspect func(context.Context) DockerHealthSnapshot) (Receipt, error) {
	if inspect == nil {
		return Receipt{}, errors.New("invalid diagnostic bundle destination or collector")
	}
	return collectSnapshot(ctx, path, budget, func(ctx context.Context) (capturedSnapshot, error) {
		snapshot := inspect(ctx)
		state, valid := doctor.DockerHealthObservationStatus(snapshot.DockerHealth)
		c := snapshot.Components
		if c.Version != 1 || c.Scope != "compiled-runtime-network-build-self-reports" || c.ArtifactVerified || c.KernelVerified {
			valid = false
		}
		aggregate, ok := aggregateStatus(c.Status, state)
		if !valid || !ok {
			return capturedSnapshot{}, errors.New("diagnostic collector returned an invalid Docker health report contract")
		}
		return capturedSnapshot{version: 3, scope: DockerHealthScope, status: aggregate, snapshot: snapshot}, nil
	})
}
