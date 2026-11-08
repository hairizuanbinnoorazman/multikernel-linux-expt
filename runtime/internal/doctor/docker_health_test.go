package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func dockerHealthFixture(t *testing.T) (DockerHealthInputs, HealthReport, []EngineTarget) {
	t.Helper()
	c, p, b, h := healthFixture(t)
	base := InspectHealth(t.Context(), c, p, b, h, "fixture")
	owners, ok := dockerLifecycle(base)
	if !ok {
		t.Fatal("invalid fixture")
	}
	targets := make([]EngineTarget, 0)
	for _, owner := range owners.Sandboxes {
		if owner.State == "RUNNING" {
			targets = append(targets, EngineTarget{Socket: "/private/secret.sock", Owner: inspection.Request{Version: 1, Namespace: "moby", ContainerID: "task-" + owner.SandboxID, SandboxID: owner.SandboxID, Generation: owner.Generation}})
		}
	}
	if len(targets) == 0 {
		t.Fatal("fixture has no running owners")
	}
	return DockerHealthInputs{Health: func(context.Context) HealthReport { return base }, Targets: func(context.Context) ([]EngineTarget, error) { return targets, nil }, Engine: func(ctx context.Context, target EngineTarget) (inspection.Report, error) {
		return inspection.Report{Version: 1, Owner: target.Owner, Observation: protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}}, nil
	}}, base, targets
}
func TestDockerHealthCoverageSamplesAndCapacity(t *testing.T) {
	for _, state := range []string{"responding", "unavailable", "exited", "stopping", "stopped"} {
		t.Run(state, func(t *testing.T) {
			in, base, targets := dockerHealthFixture(t)
			original, _ := json.Marshal(base)
			calls := 0
			read := in.Engine
			in.Engine = func(ctx context.Context, target EngineTarget) (inspection.Report, error) {
				calls++
				v, e := read(ctx, target)
				v.Observation.Status = state
				return v, e
			}
			r := InspectDockerHealth(t.Context(), in, "inspector")
			want := "consistent"
			if state != "responding" {
				want = "degraded"
			}
			if r.Status != want || !r.BootStable || !r.AllRunningOwnersObserved || r.AdmissionGuaranteed || len(r.Engines) != len(targets) || calls != 2*len(targets) {
				t.Fatalf("bad aggregate: %+v calls %d", r, calls)
			}
			if got, valid := DockerHealthObservationStatus(r); !valid || got != want {
				t.Fatal("invalid publication contract", got, valid)
			}
			if state != "responding" && (len(r.Health.Operational.Unified.Pool.CPUCandidates) != 0 || r.Health.Operational.Unified.Pool.MemoryCandidateUpperBoundBytes != 0) {
				t.Fatal("degraded engine exposed capacity")
			}
			after, _ := json.Marshal(base)
			if string(original) != string(after) {
				t.Fatal("aggregate mutated shared v8 observation")
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "secret.sock") || strings.Contains(string(data), `"container_id":`) || strings.Contains(string(data), `"namespace":`) {
				t.Fatal("private target leaked")
			}
		})
	}
}
func TestDockerHealthRefusesPartialStaleDriftingAndPrivateFailures(t *testing.T) {
	for _, mode := range []string{"missing", "extra", "generation", "duplicate", "bad-socket", "private-error", "unsupported", "foreign-owner", "invalid-source", "sample-drift", "health-drift", "callback-mutation", "unknown-owner-state", "invalid-base"} {
		t.Run(mode, func(t *testing.T) {
			in, base, targets := dockerHealthFixture(t)
			calls := 0
			read := in.Engine
			health := in.Health
			in.Engine = func(ctx context.Context, target EngineTarget) (inspection.Report, error) {
				calls++
				v, e := read(ctx, target)
				switch mode {
				case "private-error":
					return inspection.Report{}, errors.New("token=secret /private/path")
				case "unsupported":
					return inspection.Report{}, status.Error(codes.Unimplemented, "token=secret")
				case "foreign-owner":
					v.Owner.Generation = strings.Repeat("f", 32)
				case "invalid-source":
					v.Observation.Source = "secret"
				case "sample-drift":
					if calls > len(targets) {
						v.Observation.Status = "unavailable"
					}
				case "callback-mutation":
					base.Operational.Unified.Combined.Resources.Lifecycle.Sequence++
				}
				return v, e
			}
			switch mode {
			case "missing":
				in.Targets = func(context.Context) ([]EngineTarget, error) { return []EngineTarget{}, nil }
			case "extra":
				extra := targets[0]
				extra.Owner.SandboxID = "foreign"
				in.Targets = func(context.Context) ([]EngineTarget, error) {
					return append(append([]EngineTarget{}, targets...), extra), nil
				}
			case "generation":
				targets[0].Owner.Generation = strings.Repeat("f", 32)
			case "duplicate":
				in.Targets = func(context.Context) ([]EngineTarget, error) {
					return append(append([]EngineTarget{}, targets...), targets[0]), nil
				}
			case "bad-socket":
				targets[0].Socket = "relative/secret"
			case "health-drift":
				n := 0
				in.Health = func(ctx context.Context) HealthReport {
					n++
					v := health(ctx)
					if n == 2 {
						v.Revision = "drift"
					}
					return v
				}
			case "unknown-owner-state":
				base.Operational.Unified.Combined.Resources.Lifecycle.Sandboxes[0].State = "CREATED"
			case "invalid-base":
				base.Operational.Unified.Pool = nil
			}
			r := InspectDockerHealth(t.Context(), in, "inspector")
			if r.Status != "inconclusive" || r.Health != nil || r.Engines != nil || r.AllRunningOwnersObserved || r.BootStable {
				t.Fatalf("partial success %s: %+v", mode, r)
			}
			if _, ok := DockerHealthObservationStatus(r); !ok {
				t.Fatal("invalid failure report")
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "token=") || strings.Contains(string(data), "/private/") {
				t.Fatal("private failure leaked")
			}
			if (mode == "missing" || mode == "extra" || mode == "generation" || mode == "duplicate" || mode == "bad-socket" || mode == "unknown-owner-state" || mode == "invalid-base") && calls != 0 {
				t.Fatal("invalid target coverage queried an engine")
			}
		})
	}
}
func TestDockerHealthCallerBudgetAndFinalCancellation(t *testing.T) {
	for _, where := range []string{"before", "engine", "after"} {
		t.Run(where, func(t *testing.T) {
			in, _, _ := dockerHealthFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
			defer cancel()
			calls := 0
			read := in.Engine
			health := in.Health
			in.Engine = func(ctx context.Context, target EngineTarget) (inspection.Report, error) {
				calls++
				if where == "engine" {
					<-ctx.Done()
					return inspection.Report{}, ctx.Err()
				}
				return read(ctx, target)
			}
			n := 0
			in.Health = func(ctx context.Context) HealthReport {
				n++
				v := health(ctx)
				if where == "after" && n == 2 {
					cancel()
				}
				return v
			}
			if where == "before" {
				cancel()
			}
			r := InspectDockerHealth(ctx, in, "inspector")
			if r.Status != "inconclusive" || len(r.Findings) != 1 || r.Findings[0].Code != "DEADLINE_OR_CANCELED" || r.Health != nil {
				t.Fatal("late success", r)
			}
			if where == "before" && calls != 0 {
				t.Fatal("expired caller reached engine")
			}
		})
	}
}
func TestDockerHealthPublicationRefusesCoverageForgery(t *testing.T) {
	in, _, _ := dockerHealthFixture(t)
	good := InspectDockerHealth(t.Context(), in, "inspector")
	for _, mode := range []string{"missing", "generation", "capacity", "claim", "findings"} {
		t.Run(mode, func(t *testing.T) {
			bytes, _ := json.Marshal(good)
			var r DockerHealthReport
			_ = json.Unmarshal(bytes, &r)
			switch mode {
			case "missing":
				r.Engines = r.Engines[:0]
			case "generation":
				r.Engines[0].Generation = strings.Repeat("f", 32)
			case "capacity":
				r.Status = "degraded"
			case "claim":
				r.AdmissionGuaranteed = true
			case "findings":
				r.Findings = append(r.Findings, Finding{Code: "PRIVATE"})
			}
			if _, ok := DockerHealthObservationStatus(r); ok {
				t.Fatal("forged publication accepted", mode)
			}
		})
	}
	if !reflect.DeepEqual(good.Health.Findings, []Finding{}) {
		t.Fatal("unexpected baseline findings")
	}
}

func TestDockerHealthIdleCoverageOverProductionRPC(t *testing.T) {
	f := newHealthServices(t)
	before := f.files(t)
	stateBefore := f.store.Snapshot()
	calls := 0
	in := DockerHealthInputs{Health: func(ctx context.Context) HealthReport {
		return InspectHealth(ctx, f.combined, f.pool, f.builds, f.health, "inspector")
	}, Targets: func(context.Context) ([]EngineTarget, error) { return []EngineTarget{}, nil }, Engine: func(context.Context, EngineTarget) (inspection.Report, error) {
		calls++
		return inspection.Report{}, errors.New("must not query absent history")
	}}
	r := InspectDockerHealth(t.Context(), in, "inspector")
	if !r.AllRunningOwnersObserved || !r.BootStable || r.Health == nil || r.Status != r.Health.Status || r.Engines == nil || len(r.Engines) != 0 || calls != 0 {
		t.Fatalf("idle coverage lost: %+v", r)
	}
	data, _ := json.Marshal(r)
	var decoded DockerHealthReport
	if protocol.StrictDecode(data, &decoded) != nil {
		t.Fatal("idle aggregate not decodable")
	}
	if _, ok := DockerHealthObservationStatus(decoded); !ok {
		t.Fatal("idle wire report invalid")
	}
	if !reflect.DeepEqual(before, f.files(t)) || !reflect.DeepEqual(stateBefore, f.store.Snapshot()) {
		t.Fatal("inspection mutated production services")
	}
}

func TestDockerHealthRequiresBothConcurrentOwners(t *testing.T) {
	native := nativeFixture()
	second := sandbox("box-b", "RUNNING")
	second.Generation = strings.Repeat("b", 32)
	second.CPUs = []int{10, 11}
	second.MemoryBytes = 1 << 30
	second.ChildCID = 41
	second.AgentPort = 1001
	native.Resources.Lifecycle.Sandboxes = append(native.Resources.Lifecycle.Sandboxes, second)
	native.Resources.StorageBindings = append(native.Resources.StorageBindings, protocol.LifecycleStorageBinding{SandboxID: second.SandboxID, Generation: second.Generation})
	expected, endpoints := networkFixture()
	expected.Lifecycle = native.Resources.Lifecycle
	expected.Expectations = append(expected.Expectations, protocol.LifecycleNetworkExpectation{SandboxID: second.SandboxID, Generation: second.Generation, Expectation: "none"})
	pool, kernel := poolFixture()
	pool.Lifecycle = native.Resources.Lifecycle
	kernel.CPUs.Available = []uint64{}
	kernel.Memory.AllocatedBytes = 4 << 30
	kernel.Memory.AvailableBytes = 4 << 30
	kernel.Memory.LargestFreeExtentBytes = 4 << 30
	c := combinedInputs(native, expected, endpoints, []string{"box-a", "box-b"})
	p := poolInputs(pool, kernel)
	b := componentFixture()
	_, _, _, h := healthFixture(t)
	base := InspectHealth(t.Context(), c, p, b, h, "fixture")
	if base.Status != "consistent" {
		t.Fatalf("two-owner base inconsistent: %+v", base.Findings)
	}
	targets := []EngineTarget{}
	for _, owner := range native.Resources.Lifecycle.Sandboxes {
		targets = append(targets, EngineTarget{Socket: "/private/secret.sock", Owner: inspection.Request{Version: 1, Namespace: "moby", ContainerID: "task-" + owner.SandboxID, SandboxID: owner.SandboxID, Generation: owner.Generation}})
	}
	for _, count := range []int{1, 2} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			calls := 0
			in := DockerHealthInputs{Health: func(context.Context) HealthReport { return base }, Targets: func(context.Context) ([]EngineTarget, error) { return targets[:count], nil }, Engine: func(_ context.Context, target EngineTarget) (inspection.Report, error) {
				calls++
				return inspection.Report{Version: 1, Owner: target.Owner, Observation: protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}}, nil
			}}
			r := InspectDockerHealth(t.Context(), in, "inspector")
			if count == 1 {
				if r.Status != "inconclusive" || r.Findings[0].Code != "ENGINE_OWNER_COVERAGE_MISMATCH" || calls != 0 {
					t.Fatal("hidden second owner accepted", r)
				}
			} else if r.Status != "consistent" || len(r.Engines) != 2 || calls != 4 {
				t.Fatal("complete two-owner coverage refused", r)
			}
		})
	}
}
