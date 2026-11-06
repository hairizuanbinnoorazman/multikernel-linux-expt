package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func healthFixture(t *testing.T) (CombinedInputs, PoolInputs, ComponentInputs, HealthInputs) {
	t.Helper()
	c, p := unifiedFixture()
	b := componentFixture()
	v := InspectOperational(t.Context(), c, p, b, "fixture")
	if v.Status != "consistent" {
		t.Fatalf("baseline %v", v.Findings)
	}
	j := protocol.JournalStatusReport{Version: 1, Source: "in-process-lifecycle-journal", Status: "idle", Sequence: v.Unified.Combined.Resources.Lifecycle.Sequence}
	h := network.HealthReport{Version: 1, Source: "host-network-backend-check", PrimaryBootID: v.BootID, Status: "checked", Endpoints: []network.EndpointHealth{}}
	for _, e := range v.Unified.Combined.Network.Endpoints {
		h.Endpoints = append(h.Endpoints, network.EndpointHealth{EndpointGeneration: e.EndpointGeneration, DescriptorSHA256: e.DescriptorSHA256, Code: "CHECK_PASSED"})
	}
	return c, p, b, HealthInputs{Journal: func(context.Context) (protocol.JournalStatusReport, error) { return j, nil }, Network: func(context.Context) (network.HealthReport, error) {
		h.Endpoints = append([]network.EndpointHealth{}, h.Endpoints...)
		return h, nil
	}}
}

func TestHealthReportBindsJournalNetworkAndCapacity(t *testing.T) {
	for _, mode := range []string{"matched", "pending", "failed", "network-failed", "failed-and-pending", "operational-inconclusive", "network-failed-operational-inconclusive"} {
		t.Run(mode, func(t *testing.T) {
			c, p, b, h := healthFixture(t)
			want := "consistent"
			jr, nr := h.Journal, h.Network
			if mode == "pending" || mode == "failed" || mode == "failed-and-pending" {
				h.Journal = func(ctx context.Context) (protocol.JournalStatusReport, error) {
					j, e := jr(ctx)
					if mode != "failed" {
						j.PendingCompletions = 1
						j.Status = "pending"
					}
					if mode != "pending" {
						j.JournalFailed = true
						j.Status = "failed"
					}
					return j, e
				}
				want = "inconclusive"
				if mode != "pending" {
					want = "degraded"
				}
			}
			if strings.Contains(mode, "operational-inconclusive") {
				read := b.Network
				b.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					v, e := read(ctx)
					v.BuildRevision = "unknown"
					return v, e
				}
				want = "inconclusive"
			}
			if strings.HasPrefix(mode, "network-failed") {
				h.Network = func(ctx context.Context) (network.HealthReport, error) {
					v, e := nr(ctx)
					v.Status = "degraded"
					v.Endpoints[0].Code = "BACKEND_CHECK_FAILED"
					return v, e
				}
				want = "degraded"
			}
			r := InspectHealth(t.Context(), c, p, b, h, "inspector")
			if r.Version != 8 || r.Status != want || !r.BootStable || r.Operational == nil || r.Journal == nil || r.Network == nil || r.AdmissionGuaranteed || r.Revision != "inspector" {
				t.Fatalf("%+v", r)
			}
			pool := r.Operational.Unified.Pool
			if want == "consistent" {
				if len(pool.CPUCandidates) != 2 || pool.MemoryCandidateUpperBoundBytes != 4<<30 {
					t.Fatal("lost observed capacity")
				}
			} else if len(pool.CPUCandidates) != 0 || pool.MemoryCandidateUpperBoundBytes != 0 {
				t.Fatal("health failure exposed candidates")
			}
			for _, field := range []string{"service-readiness", "docker-engine", "journal-current-writability", "end-to-end-network-health", "containment"} {
				if !strings.Contains(strings.Join(r.Unobserved, ","), field) {
					t.Fatal("missing scope limit", field)
				}
			}
			if mode == "failed-and-pending" && !hasFinding(r.Findings, "JOURNAL_FAILED") {
				t.Fatal("journal failure masked by pending")
			}
		})
	}
}
func hasFinding(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestHealthReportRefusesPartialDriftCrossBindingAndPrivateFailures(t *testing.T) {
	for _, mode := range []string{"journal-unavailable", "network-unavailable", "journal-invalid", "network-invalid", "journal-drift", "network-drift", "sequence", "boot", "missing-endpoint", "descriptor", "generation", "operational-drift", "missing-callback"} {
		t.Run(mode, func(t *testing.T) {
			c, p, b, h := healthFixture(t)
			jr, nr := h.Journal, h.Network
			calls := 0
			code := ""
			switch mode {
			case "journal-unavailable":
				h.Journal = func(context.Context) (protocol.JournalStatusReport, error) {
					return protocol.JournalStatusReport{}, errors.New("SYNTHETIC_PRIVATE_FAILURE")
				}
				code = "JOURNAL_OBSERVATION_UNAVAILABLE"
			case "network-unavailable":
				h.Network = func(context.Context) (network.HealthReport, error) {
					return network.HealthReport{}, errors.New("SYNTHETIC_PRIVATE_FAILURE")
				}
				code = "NETWORK_HEALTH_OBSERVATION_UNAVAILABLE"
			case "journal-invalid":
				h.Journal = func(ctx context.Context) (protocol.JournalStatusReport, error) {
					v, e := jr(ctx)
					v.JournalFailed = true
					return v, e
				}
				code = "JOURNAL_OBSERVATION_UNAVAILABLE"
			case "network-invalid":
				h.Network = func(ctx context.Context) (network.HealthReport, error) {
					v, e := nr(ctx)
					v.Status = "idle"
					return v, e
				}
				code = "NETWORK_HEALTH_OBSERVATION_UNAVAILABLE"
			case "journal-drift", "sequence":
				h.Journal = func(ctx context.Context) (protocol.JournalStatusReport, error) {
					v, e := jr(ctx)
					calls++
					if mode == "sequence" || calls == 2 {
						v.Sequence++
					}
					return v, e
				}
				code = "JOURNAL_LIFECYCLE_MISMATCH"
				if mode == "journal-drift" {
					code = "OBSERVATION_CHANGED"
				}
			case "network-drift", "boot", "missing-endpoint", "descriptor", "generation":
				h.Network = func(ctx context.Context) (network.HealthReport, error) {
					v, e := nr(ctx)
					calls++
					switch mode {
					case "network-drift":
						if calls == 2 {
							v.Status = "degraded"
							v.Endpoints[0].Code = "BACKEND_CHECK_FAILED"
						}
					case "boot":
						v.PrimaryBootID = "22222222-2222-4222-8222-222222222222"
					case "missing-endpoint":
						v.Endpoints = []network.EndpointHealth{}
						v.Status = "idle"
					case "descriptor":
						v.Endpoints[0].DescriptorSHA256 = strings.Repeat("f", 64)
					case "generation":
						v.Endpoints[0].EndpointGeneration = "ffffffffffffffffffffffffffffffff"
					}
					return v, e
				}
				code = "NETWORK_HEALTH_OWNERSHIP_MISMATCH"
				if mode == "network-drift" {
					code = "OBSERVATION_CHANGED"
				}
				if mode == "boot" {
					code = "NETWORK_HEALTH_BOOT_MISMATCH"
				}
			case "operational-drift":
				read := b.Network
				b.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					v, e := read(ctx)
					calls++
					if calls > 4 {
						v.BuildRevision = "1234567"
					}
					return v, e
				}
				code = "OBSERVATION_CHANGED"
			case "missing-callback":
				h.Journal = nil
				code = "HEALTH_OBSERVATION_UNAVAILABLE"
			}
			r := InspectHealth(t.Context(), c, p, b, h, "inspector")
			if r.Status != "inconclusive" || r.Operational != nil || r.Journal != nil || r.Network != nil || r.BootStable || !hasFinding(r.Findings, code) {
				t.Fatalf("%s: %+v", code, r)
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "SYNTHETIC_PRIVATE") {
				t.Fatal("raw error leaked")
			}
		})
	}
}

func TestHealthReportCancellationSharesBudgetAndStopsFurtherObservers(t *testing.T) {
	for target := 0; target <= 4; target++ {
		c, p, b, h := healthFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		jr, nr := h.Journal, h.Network
		observe := func(got context.Context) {
			if got != ctx || got.Err() != nil {
				t.Error("observer got extended or expired context")
			}
			calls++
			if calls == target {
				cancel()
			}
		}
		h.Journal = func(got context.Context) (protocol.JournalStatusReport, error) { observe(got); return jr(got) }
		h.Network = func(got context.Context) (network.HealthReport, error) { observe(got); return nr(got) }
		if target == 0 {
			cancel()
		}
		r := InspectHealth(ctx, c, p, b, h, "inspector")
		cancel()
		if calls != target || r.Operational != nil || r.Journal != nil || r.Network != nil || !hasFinding(r.Findings, "DEADLINE_OR_CANCELED") {
			t.Fatalf("target=%d calls=%d %+v", target, calls, r)
		}
	}
}

func TestHealthReportSnapshotsCallbackOwnedEndpointSlices(t *testing.T) {
	c, p, b, h := healthFixture(t)
	nr := h.Network
	jr := h.Journal
	shared, _ := nr(t.Context())
	calls := 0
	h.Network = func(context.Context) (network.HealthReport, error) { return shared, nil }
	h.Journal = func(ctx context.Context) (protocol.JournalStatusReport, error) {
		calls++
		if calls == 2 {
			shared.Endpoints[0].Code = "BACKEND_CHECK_FAILED"
			shared.Status = "degraded"
		}
		return jr(ctx)
	}
	r := InspectHealth(t.Context(), c, p, b, h, "inspector")
	if r.Network != nil || !hasFinding(r.Findings, "OBSERVATION_CHANGED") {
		t.Fatal("shared callback slice concealed drift", r)
	}
}

func TestHealthReportIdleNetworkDoesNotInventEndpointHealth(t *testing.T) {
	c, p, b, h := healthFixture(t)
	read := c.Expectations
	c.Expectations = func(ctx context.Context) (protocol.NetworkExpectationsReport, error) {
		v, e := read(ctx)
		v.Expectations = append([]protocol.LifecycleNetworkExpectation{}, v.Expectations...)
		for n := range v.Expectations {
			v.Expectations[n].Expectation = "none"
		}
		return v, e
	}
	nr := c.Network
	c.Network = func(ctx context.Context) (network.OwnershipReport, error) {
		v, e := nr(ctx)
		v.Endpoints = []network.EndpointOwnership{}
		return v, e
	}
	hr := h.Network
	h.Network = func(ctx context.Context) (network.HealthReport, error) {
		v, e := hr(ctx)
		v.Status = "idle"
		v.Endpoints = []network.EndpointHealth{}
		return v, e
	}
	r := InspectHealth(t.Context(), c, p, b, h, "inspector")
	if r.Status != "consistent" || r.Network == nil || r.Network.Status != "idle" || len(r.Network.Endpoints) != 0 {
		t.Fatal("idle network became endpoint readiness", r)
	}
}
