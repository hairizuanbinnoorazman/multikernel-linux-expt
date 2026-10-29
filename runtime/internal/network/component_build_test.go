package network

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestComponentBuildIndependentOfNetworkLocksStoreAndBackend(t *testing.T) {
	s, b, e := ownershipFixture(t)
	path := filepath.Join(s.Store.dir, "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := []int{len(b.adds), len(b.checks), len(b.deletes)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orchestrate.Lock()
	defer s.orchestrate.Unlock()
	s.Store.mu.Lock()
	defer s.Store.mu.Unlock()
	done := make(chan Response, 1)
	go func() {
		done <- s.Dispatch(t.Context(), Request{Version: 1, RequestID: "inspect-build", Method: "INSPECT_COMPONENT_BUILD"})
	}()
	var r Response
	select {
	case r = <-done:
	case <-time.After(time.Second):
		t.Fatal("build inspection took mutation/store lock")
	}
	if r.Error != nil || r.ComponentBuild == nil || r.ComponentBuild.Component != "mknetd" || r.ComponentBuild.Validate() != nil || r.Ownership != nil || r.Endpoint != nil {
		t.Fatalf("%+v", r)
	}
	raw, _ := json.Marshal(r)
	for _, private := range []string{"private-workload", "private-credential", e.NetNS, e.Address} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private endpoint leaked")
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || !reflect.DeepEqual(calls, []int{len(b.adds), len(b.checks), len(b.deletes)}) {
		t.Fatal("inspection mutated state/backend")
	}
	for _, request := range []Request{{Endpoint: &Endpoint{}}, {Generation: "private"}} {
		request.Version = 1
		request.RequestID = "bad-build"
		request.Method = "INSPECT_COMPONENT_BUILD"
		out := s.Dispatch(t.Context(), request)
		if out.Error == nil || out.Error.Code != "INVALID_ARGUMENT" || out.ComponentBuild != nil {
			t.Fatal("mutation-shaped input accepted")
		}
	}
}
func TestComponentBuildClientRefusesLegacyMixedInvalidAndPrivateReplies(t *testing.T) {
	for _, mode := range []string{"matched", "legacy", "duplicate", "private-error", "unsupported-after-discovery", "mixed-capabilities", "mixed-report", "wrong-component", "artifact-claim", "invalid-revision"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			c := scriptedOwnershipClient(t, func(r Request) Response {
				calls.Add(1)
				v := Response{Version: 1, RequestID: r.RequestID}
				if mode == "private-error" {
					v.Error = &APIError{Code: "UNAVAILABLE", Message: "synthetic-private-token /private/path"}
					return v
				}
				if r.Method == "CAPABILITIES" {
					v.Capabilities = &Capabilities{Version: 1, Features: []string{protocol.ComponentBuildCapability}}
					if mode == "legacy" {
						v.Capabilities.Features = []string{OwnershipCapability}
					}
					if mode == "duplicate" {
						v.Capabilities.Features = append(v.Capabilities.Features, v.Capabilities.Features[0])
					}
					if mode == "mixed-capabilities" {
						v.ComponentBuild = &protocol.ComponentBuildReport{}
					}
				} else {
					if mode == "unsupported-after-discovery" {
						v.Error = &APIError{Code: "UNSUPPORTED", Message: "synthetic-private-token"}
						return v
					}
					v.ComponentBuild = &protocol.ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: "mknetd", BuildVersion: "0.0.0-dev", BuildRevision: "abcdef0"}
					if mode == "mixed-report" {
						v.Endpoint = &Endpoint{DNS: DNS{Domain: "synthetic-private-token"}}
					}
					if mode == "wrong-component" {
						v.ComponentBuild.Component = "mkruntimed"
					}
					if mode == "artifact-claim" {
						v.ComponentBuild.ArtifactVerified = true
					}
					if mode == "invalid-revision" {
						v.ComponentBuild.BuildRevision = "synthetic-private-token"
					}
				}
				return v
			}, time.Second)
			r, err := InspectComponentBuild(t.Context(), c)
			if mode == "matched" {
				if err != nil || r.Validate() != nil || calls.Load() != 2 {
					t.Fatalf("%+v %v", r, err)
				}
				return
			}
			if err == nil || r.Component != "" || strings.Contains(err.Error(), "synthetic-private-token") || strings.Contains(err.Error(), "/private/path") {
				t.Fatal("unsafe reply accepted/reflected", r, err)
			}
			if (mode == "legacy" || mode == "unsupported-after-discovery") && !errors.Is(err, ErrComponentBuildUnsupported) {
				t.Fatal("compatibility lost")
			}
			if (mode == "legacy" || mode == "duplicate" || mode == "private-error" || mode == "mixed-capabilities") && calls.Load() != 1 {
				t.Fatal("legacy/invalid discovery fallback")
			}
		})
	}
}
func TestComponentBuildNegotiationAndReadShareDeadline(t *testing.T) {
	c := scriptedOwnershipClient(t, func(r Request) Response {
		time.Sleep(40 * time.Millisecond)
		v := Response{Version: 1, RequestID: r.RequestID}
		if r.Method == "CAPABILITIES" {
			v.Capabilities = &Capabilities{Version: 1, Features: []string{protocol.ComponentBuildCapability}}
		} else {
			v.ComponentBuild = &protocol.ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: "mknetd", BuildVersion: "0.0.0-dev", BuildRevision: "abcdef0"}
		}
		return v
	}, 60*time.Millisecond)
	started := time.Now()
	if _, err := InspectComponentBuild(t.Context(), c); err == nil {
		t.Fatal("negotiation reset deadline")
	}
	if time.Since(started) > time.Second {
		t.Fatal("deadline not bounded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := InspectComponentBuild(ctx, Client{}); err == nil {
		t.Fatal("canceled request accepted")
	}
}
