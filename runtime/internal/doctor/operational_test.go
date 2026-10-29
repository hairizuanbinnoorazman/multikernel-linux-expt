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

func TestOperationalReportCombinesScopesAndPreservesSeverityWithoutAttestation(t *testing.T) {
	for _, mode := range []string{"matched", "known-different-builds", "unknown-build", "native-unavailable", "network-degraded", "degraded-with-unknown-build"} {
		t.Run(mode, func(t *testing.T) {
			c, p := unifiedFixture()
			b := componentFixture()
			want := "consistent"
			if mode == "known-different-builds" || mode == "unknown-build" || mode == "degraded-with-unknown-build" {
				read := b.Network
				b.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					v, e := read(ctx)
					v.BuildRevision = "1234567"
					if mode != "known-different-builds" {
						v.BuildRevision = "unknown"
					}
					return v, e
				}
				if mode != "known-different-builds" {
					want = "inconclusive"
				}
			}
			if mode == "native-unavailable" {
				read := c.Native
				c.Native = func(ctx context.Context) (protocol.NativeResourceReport, error) {
					v, e := read(ctx)
					v.Processes = append([]protocol.StorageProcessIdentity(nil), v.Processes...)
					v.Processes[0].Status = "unavailable"
					return v, e
				}
				want = "inconclusive"
			}
			if mode == "network-degraded" || mode == "degraded-with-unknown-build" {
				read := c.Expectations
				c.Expectations = func(ctx context.Context) (protocol.NetworkExpectationsReport, error) {
					v, e := read(ctx)
					v.Expectations = append([]protocol.LifecycleNetworkExpectation(nil), v.Expectations...)
					v.Expectations[0].Expectation = "none"
					return v, e
				}
				want = "degraded"
			}
			r := InspectOperational(t.Context(), c, p, b, "inspector")
			if r.Version != 7 || r.Status != want || !r.BootStable || r.Unified == nil || r.Components == nil || r.Unified.Version != 6 || r.Components.Version != 1 || r.Revision != "inspector" || r.AdmissionGuaranteed || r.ArtifactVerified || r.KernelVerified {
				t.Fatalf("%+v", r)
			}
			if mode == "known-different-builds" && r.Components.CompiledRevisionsMatch {
				t.Fatal("different builds laundered as identical")
			}
			if want == "consistent" {
				if r.Unified.Pool.MemoryCandidateUpperBoundBytes != 4<<30 || len(r.Unified.Pool.CPUCandidates) != 2 {
					t.Fatal(r.Unified.Pool)
				}
			} else if r.Unified.Pool.MemoryCandidateUpperBoundBytes != 0 || len(r.Unified.Pool.CPUCandidates) != 0 {
				t.Fatal("aggregate exposed unsafe candidates")
			}
			if !strings.Contains(strings.Join(r.Unobserved, ","), "component-compatibility") || !strings.Contains(strings.Join(r.Unobserved, ","), "service-readiness") {
				t.Fatal(r.Unobserved)
			}
		})
	}
}
func TestOperationalReportRefusesOuterDriftLegacyPrivateFailuresAndCrossBoot(t *testing.T) {
	for _, mode := range []string{"resource-drift", "pool-drift", "build-drift", "legacy-build", "private-build-error", "boot"} {
		t.Run(mode, func(t *testing.T) {
			c, p := unifiedFixture()
			b := componentFixture()
			code := "OBSERVATION_CHANGED"
			switch mode {
			case "resource-drift":
				calls := 0
				read := c.Native
				c.Native = func(ctx context.Context) (protocol.NativeResourceReport, error) {
					v, e := read(ctx)
					calls++
					if calls > 4 {
						v.Processes = append([]protocol.StorageProcessIdentity(nil), v.Processes...)
						v.Processes[0].Status = "process-absent"
					}
					return v, e
				}
			case "pool-drift":
				calls := 0
				read := p.Kernel
				p.Kernel = func(ctx context.Context) (poolinspect.Snapshot, error) {
					v, e := read(ctx)
					calls++
					if calls > 4 {
						v.Memory.LargestFreeExtentBytes--
					}
					return v, e
				}
			case "build-drift":
				calls := 0
				read := b.Runtime
				b.Runtime = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					v, e := read(ctx)
					calls++
					if calls > 2 {
						v.BuildRevision = "1234567"
					}
					return v, e
				}
			case "legacy-build":
				b.Runtime = func(context.Context) (protocol.ComponentBuildReport, error) {
					return protocol.ComponentBuildReport{}, daemon.ErrComponentBuildUnsupported
				}
				code = "COMPONENT_BUILD_OBSERVATION_UNAVAILABLE"
			case "private-build-error":
				b.Runtime = func(context.Context) (protocol.ComponentBuildReport, error) {
					return protocol.ComponentBuildReport{}, errors.New("TOKEN-private /private/path")
				}
				code = "COMPONENT_BUILD_OBSERVATION_UNAVAILABLE"
			case "boot":
				b.Boot = func() (string, error) { return "11111111-1111-4111-8111-111111111111", nil }
				code = "BOOT_CHANGED"
			}
			r := InspectOperational(t.Context(), c, p, b, "inspector")
			if r.Status != "inconclusive" || r.Unified != nil || r.Components != nil || len(r.Findings) == 0 || r.Findings[0].Code != code {
				t.Fatalf("%+v", r)
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), "TOKEN-private") || strings.Contains(string(raw), "/private/path") {
				t.Fatal("private error leaked")
			}
		})
	}
}
func TestOperationalReportStopsAfterUnavailableResourceAndSharesDeadline(t *testing.T) {
	c, p := unifiedFixture()
	b := componentFixture()
	c.Native = func(context.Context) (protocol.NativeResourceReport, error) {
		return protocol.NativeResourceReport{}, errors.New("unavailable")
	}
	b.Runtime = func(context.Context) (protocol.ComponentBuildReport, error) {
		t.Fatal("build read after resource failure")
		return protocol.ComponentBuildReport{}, nil
	}
	r := InspectOperational(t.Context(), c, p, b, "inspector")
	if r.Findings[0].Code != "RESOURCE_OBSERVATION_UNAVAILABLE" {
		t.Fatal(r)
	}
	c, p = unifiedFixture()
	b = componentFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	b.Runtime = func(got context.Context) (protocol.ComponentBuildReport, error) {
		if got != ctx {
			t.Error("original context replaced")
		}
		<-got.Done()
		return protocol.ComponentBuildReport{}, got.Err()
	}
	r = InspectOperational(ctx, c, p, b, "inspector")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" || r.Unified != nil || r.Components != nil {
		t.Fatal(r)
	}
	c.Boot = func() (string, error) { t.Fatal("read after canceled"); return "", nil }
	r = InspectOperational(ctx, c, p, b, "inspector")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal(r)
	}
}
func TestOperationalReportStopsBeforeBootReadAfterLastBuildCancellation(t *testing.T) {
	c, p := unifiedFixture()
	b := componentFixture()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	originalBoot := c.Boot
	c.Boot = func() (string, error) {
		if ctx.Err() != nil {
			t.Fatal("boot read after canceled")
		}
		return originalBoot()
	}
	calls := 0
	read := b.Network
	b.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
		v, e := read(ctx)
		calls++
		if calls == 4 {
			cancel()
		}
		return v, e
	}
	r := InspectOperational(ctx, c, p, b, "inspector")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" || r.Unified != nil || r.Components != nil || calls != 4 {
		t.Fatalf("%+v calls=%d", r, calls)
	}
}
