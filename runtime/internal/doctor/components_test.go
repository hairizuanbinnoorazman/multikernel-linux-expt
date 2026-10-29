package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func componentFixture() ComponentInputs {
	report := func(name string) protocol.ComponentBuildReport {
		return protocol.ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: name, BuildVersion: "0.0.0-dev", BuildRevision: "abcdef0"}
	}
	return ComponentInputs{Runtime: func(context.Context) (protocol.ComponentBuildReport, error) { return report("mkruntimed"), nil }, Network: func(context.Context) (protocol.ComponentBuildReport, error) { return report("mknetd"), nil }, Boot: func() (string, error) { return boot, nil }}
}
func TestComponentInspectionReportsDaemonLabelsSeparatelyFromInspector(t *testing.T) {
	for _, mode := range []string{"matched", "different", "unknown-runtime", "unknown-network", "both-unknown"} {
		t.Run(mode, func(t *testing.T) {
			in := componentFixture()
			if mode == "different" || mode == "unknown-network" || mode == "both-unknown" {
				read := in.Network
				in.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					r, e := read(ctx)
					r.BuildRevision = "1234567"
					if mode != "different" {
						r.BuildRevision = "unknown"
					}
					return r, e
				}
			}
			if mode == "unknown-runtime" || mode == "both-unknown" {
				read := in.Runtime
				in.Runtime = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					r, e := read(ctx)
					r.BuildRevision = "unknown"
					return r, e
				}
			}
			r := InspectComponents(t.Context(), in, "inspector-revision")
			want := "consistent"
			if strings.Contains(mode, "unknown") {
				want = "inconclusive"
			}
			if r.Version != 1 || r.Status != want || !r.BootStable || r.Runtime == nil || r.Network == nil || r.Revision != "inspector-revision" || r.Runtime.BuildRevision == r.Revision || r.ArtifactVerified || r.KernelVerified {
				t.Fatalf("%+v", r)
			}
			if r.CompiledRevisionsMatch != (mode == "matched") {
				t.Fatal("revision comparison inferred compatibility", r)
			}
			if !strings.Contains(strings.Join(r.Unobserved, ","), "component-compatibility") || !strings.Contains(strings.Join(r.Unobserved, ","), "service-readiness") {
				t.Fatal(r.Unobserved)
			}
			if mode == "both-unknown" && (len(r.Findings) != 2 || r.Findings[0].Code != "RUNTIME_BUILD_REVISION_UNKNOWN" || r.Findings[1].Code != "NETWORK_BUILD_REVISION_UNKNOWN") {
				t.Fatal(r.Findings)
			}
		})
	}
}
func TestComponentInspectionRefusesPartialLegacyInvalidAndDriftedViews(t *testing.T) {
	for _, mode := range []string{"runtime-legacy", "network-legacy", "private-error", "invalid", "wrong-component", "runtime-drift", "network-drift", "boot"} {
		t.Run(mode, func(t *testing.T) {
			in := componentFixture()
			code := "OBSERVATION_CHANGED"
			switch mode {
			case "runtime-legacy":
				in.Runtime = func(context.Context) (protocol.ComponentBuildReport, error) {
					return protocol.ComponentBuildReport{}, daemon.ErrComponentBuildUnsupported
				}
				code = "RUNTIME_BUILD_UNSUPPORTED"
			case "network-legacy":
				in.Network = func(context.Context) (protocol.ComponentBuildReport, error) {
					return protocol.ComponentBuildReport{}, network.ErrComponentBuildUnsupported
				}
				code = "NETWORK_BUILD_UNSUPPORTED"
			case "private-error":
				in.Runtime = func(context.Context) (protocol.ComponentBuildReport, error) {
					return protocol.ComponentBuildReport{}, errors.New("TOKEN-private /private/path")
				}
				code = "RUNTIME_BUILD_UNAVAILABLE"
			case "invalid", "wrong-component":
				read := in.Network
				in.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					r, e := read(ctx)
					if mode == "invalid" {
						r.KernelVerified = true
					} else {
						r.Component = "mkruntimed"
					}
					return r, e
				}
				code = "NETWORK_BUILD_UNAVAILABLE"
			case "runtime-drift", "network-drift":
				var read func(context.Context) (protocol.ComponentBuildReport, error)
				if mode == "runtime-drift" {
					read = in.Runtime
				} else {
					read = in.Network
				}
				calls := 0
				changed := func(ctx context.Context) (protocol.ComponentBuildReport, error) {
					r, e := read(ctx)
					calls++
					if calls > 1 {
						r.BuildRevision = "1234567"
					}
					return r, e
				}
				if mode == "runtime-drift" {
					in.Runtime = changed
				} else {
					in.Network = changed
				}
			case "boot":
				calls := 0
				in.Boot = func() (string, error) {
					calls++
					if calls > 1 {
						return "11111111-1111-4111-8111-111111111111", nil
					}
					return boot, nil
				}
				code = "BOOT_CHANGED"
			}
			r := InspectComponents(t.Context(), in, "revision")
			if r.Status != "inconclusive" || r.Runtime != nil || r.Network != nil || r.CompiledRevisionsMatch || len(r.Findings) != 1 || r.Findings[0].Code != code {
				t.Fatalf("%+v", r)
			}
			data, _ := json.Marshal(r)
			if strings.Contains(string(data), "TOKEN-private") || strings.Contains(string(data), "/private/path") {
				t.Fatal("raw error leaked")
			}
		})
	}
}
func TestComponentInspectionSharesOriginalDeadlineAndStopsAfterFailure(t *testing.T) {
	in := componentFixture()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	in.Runtime = func(got context.Context) (protocol.ComponentBuildReport, error) {
		if got != ctx {
			t.Error("caller budget replaced")
		}
		<-got.Done()
		return protocol.ComponentBuildReport{}, got.Err()
	}
	in.Network = func(context.Context) (protocol.ComponentBuildReport, error) {
		t.Fatal("network read after runtime failure")
		return protocol.ComponentBuildReport{}, nil
	}
	r := InspectComponents(ctx, in, "revision")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal(r)
	}
	in.Boot = func() (string, error) { t.Fatal("boot read after canceled"); return "", nil }
	r = InspectComponents(ctx, in, "revision")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal(r)
	}
}

func TestComponentInspectionStopsBeforeBootReadAfterLastCallbackCancellation(t *testing.T) {
	in := componentFixture()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	boots := 0
	in.Boot = func() (string, error) {
		boots++
		if boots > 1 {
			t.Fatal("boot read after last callback canceled")
		}
		return boot, nil
	}
	read := in.Network
	in.Network = func(ctx context.Context) (protocol.ComponentBuildReport, error) {
		r, e := read(ctx)
		calls++
		if calls == 2 {
			cancel()
		}
		return r, e
	}
	r := InspectComponents(ctx, in, "revision")
	if r.Findings[0].Code != "DEADLINE_OR_CANCELED" || r.Runtime != nil || r.Network != nil {
		t.Fatal(r)
	}
}
