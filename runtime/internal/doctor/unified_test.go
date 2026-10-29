package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/poolinspect"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func unifiedFixture() (CombinedInputs, PoolInputs) {
	native := nativeFixture()
	expected, endpoints := networkFixture()
	owner, kernel := poolFixture()
	owner.Lifecycle = native.Resources.Lifecycle
	return combinedInputs(native, expected, endpoints, []string{"box-a"}), poolInputs(owner, kernel)
}
func TestUnifiedCapacityCorrelatesAllComponentsAndSuppressesUnsafeCandidates(t *testing.T) {
	for _, mode := range []string{"matched", "storage-degraded", "network-degraded", "native-unavailable", "pool-degraded", "both-degraded"} {
		t.Run(mode, func(t *testing.T) {
			c, p := unifiedFixture()
			want := "consistent"
			if mode == "storage-degraded" || mode == "native-unavailable" || mode == "both-degraded" {
				read := c.Native
				c.Native = func(ctx context.Context) (protocol.NativeResourceReport, error) {
					v, e := read(ctx)
					v.Processes = append([]protocol.StorageProcessIdentity(nil), v.Processes...)
					v.Processes[0].Status = "process-absent"
					if mode == "native-unavailable" {
						v.Processes[0].Status = "unavailable"
					}
					return v, e
				}
				want = "degraded"
				if mode == "native-unavailable" {
					want = "inconclusive"
				}
			}
			if mode == "network-degraded" {
				read := c.Expectations
				c.Expectations = func(ctx context.Context) (protocol.NetworkExpectationsReport, error) {
					v, e := read(ctx)
					v.Expectations = append([]protocol.LifecycleNetworkExpectation(nil), v.Expectations...)
					v.Expectations[0].Expectation = "none"
					return v, e
				}
				want = "degraded"
			}
			if mode == "pool-degraded" || mode == "both-degraded" {
				read := p.Kernel
				p.Kernel = func(ctx context.Context) (poolinspect.Snapshot, error) {
					v, e := read(ctx)
					v.CPUs.Available = []uint64{11}
					return v, e
				}
				want = "degraded"
			}
			r := InspectUnified(t.Context(), c, p, "test-revision")
			if r.Status != want || r.Version != 6 || !r.BootStable || r.Combined == nil || r.Pool == nil || r.AdmissionGuaranteed || r.Combined.Version != 5 || r.Pool.Version != 1 {
				t.Fatalf("%+v", r)
			}
			if want == "consistent" {
				if r.Pool.MemoryCandidateUpperBoundBytes != 4<<30 || len(r.Pool.CPUCandidates) != 2 {
					t.Fatal(r.Pool)
				}
			} else if r.Pool.MemoryCandidateUpperBoundBytes != 0 || len(r.Pool.CPUCandidates) != 0 {
				t.Fatal("unsafe candidate", r.Pool)
			}
			if !strings.Contains(strings.Join(r.Unobserved, ","), "service-readiness") || strings.Contains(strings.Join(r.Unobserved, ","), "pool-capacity") {
				t.Fatal(r.Unobserved)
			}
		})
	}
}
func TestUnifiedRefusesDriftLegacyPartialPrivateFailuresAndCrossBoot(t *testing.T) {
	for _, mode := range []string{"lifecycle-sequence", "same-sequence-claims", "pool-between-rounds", "native-between-rounds", "boot", "legacy", "private-error", "missing-free-list"} {
		t.Run(mode, func(t *testing.T) {
			c, p := unifiedFixture()
			code := "RESOURCE_POOL_LIFECYCLE_CHANGED"
			switch mode {
			case "lifecycle-sequence", "same-sequence-claims":
				read := p.Ownership
				p.Ownership = func(ctx context.Context) (protocol.PoolOwnershipReport, error) {
					v, e := read(ctx)
					if mode == "lifecycle-sequence" {
						v.Lifecycle.Sequence++
					} else {
						v.Lifecycle.Sandboxes = append([]protocol.SandboxOwnership(nil), v.Lifecycle.Sandboxes...)
						v.Lifecycle.Sandboxes[0].AgentPort++
					}
					return v, e
				}
			case "pool-between-rounds":
				calls := 0
				read := p.Kernel
				p.Kernel = func(ctx context.Context) (poolinspect.Snapshot, error) {
					v, e := read(ctx)
					calls++
					if calls > 2 {
						v.Memory.LargestFreeExtentBytes--
					}
					return v, e
				}
				code = "OBSERVATION_CHANGED"
			case "native-between-rounds":
				calls := 0
				read := c.Native
				c.Native = func(ctx context.Context) (protocol.NativeResourceReport, error) {
					v, e := read(ctx)
					calls++
					if calls > 2 {
						v.Processes = append([]protocol.StorageProcessIdentity(nil), v.Processes...)
						v.Processes[0].Status = "process-absent"
					}
					return v, e
				}
				code = "OBSERVATION_CHANGED"
			case "boot":
				p.Boot = func() (string, error) { return "11111111-1111-4111-8111-111111111111", nil }
				code = "BOOT_CHANGED"
			case "legacy":
				p.Ownership = func(context.Context) (protocol.PoolOwnershipReport, error) {
					return protocol.PoolOwnershipReport{}, daemon.ErrPoolOwnershipUnsupported
				}
				code = "POOL_OBSERVATION_UNAVAILABLE"
			case "private-error":
				c.Native = func(context.Context) (protocol.NativeResourceReport, error) {
					return protocol.NativeResourceReport{}, errors.New("TOKEN-private /private/path")
				}
				code = "COMBINED_OBSERVATION_UNAVAILABLE"
			case "missing-free-list":
				p.Kernel = func(context.Context) (poolinspect.Snapshot, error) {
					return poolinspect.Snapshot{}, poolinspect.ErrCPUFreeListUnavailable
				}
				code = "POOL_OBSERVATION_UNAVAILABLE"
			}
			r := InspectUnified(t.Context(), c, p, "revision")
			if r.Status != "inconclusive" || r.Combined != nil || r.Pool != nil || len(r.Findings) == 0 || r.Findings[0].Code != code {
				t.Fatalf("%+v", r)
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "TOKEN-private") || strings.Contains(string(data), "/private/path") {
				t.Fatal("private metadata leaked")
			}
		})
	}
}
func TestUnifiedSharesCallerDeadlineAndStopsAfterCancellation(t *testing.T) {
	c, p := unifiedFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	p.Ownership = func(got context.Context) (protocol.PoolOwnershipReport, error) {
		calls++
		if got != ctx {
			t.Error("replaced caller context")
		}
		<-got.Done()
		return protocol.PoolOwnershipReport{}, got.Err()
	}
	r := InspectUnified(ctx, c, p, "revision")
	if calls != 1 || r.Status != "inconclusive" || r.Findings[0].Code != "DEADLINE_OR_CANCELED" || r.Combined != nil || r.Pool != nil {
		t.Fatalf("%+v calls=%d", r, calls)
	}
	c, p = unifiedFixture()
	c.Boot = func() (string, error) { t.Fatal("read after canceled"); return "", nil }
	r = InspectUnified(ctx, c, p, "revision")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal(r)
	}
}
