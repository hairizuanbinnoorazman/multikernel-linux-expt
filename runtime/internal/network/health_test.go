package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type healthBackend struct {
	check func(context.Context, Endpoint) error
	calls int
}

func (b *healthBackend) Add(context.Context, Endpoint) error    { panic("health called ADD") }
func (b *healthBackend) Delete(context.Context, Endpoint) error { panic("health called DEL") }
func (b *healthBackend) Check(ctx context.Context, e Endpoint) error {
	b.calls++
	return b.check(ctx, e)
}

func TestHealthBackendChecksAreBoundedRedactedAndReadOnly(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			s, _, e := ownershipFixture(t)
			path := filepath.Join(s.Store.dir, "state.json")
			before, _ := os.ReadFile(path)
			b := &healthBackend{check: func(_ context.Context, got Endpoint) error {
				if !reflect.DeepEqual(got, e) {
					t.Fatal("backend checked another descriptor")
				}
				if failure {
					return errors.New("private-credential /private/namespace")
				}
				return nil
			}}
			s.Backend = b
			r, issue := s.Health(t.Context())
			if issue != nil || r.Validate() != nil || b.calls != 1 || len(r.Endpoints) != 1 {
				t.Fatal(r, issue, b.calls)
			}
			want := "checked"
			code := "CHECK_PASSED"
			if failure {
				want = "degraded"
				code = "BACKEND_CHECK_FAILED"
			}
			digest, _ := endpointDescriptor(e)
			if r.Status != want || r.Endpoints[0].Code != code || r.Endpoints[0].DescriptorSHA256 != digest || r.Endpoints[0].EndpointGeneration != e.Generation {
				t.Fatal(r)
			}
			raw, _ := json.Marshal(r)
			for _, secret := range []string{e.NetNS, e.Address, "private-workload", "private-credential", "/private/namespace"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("health leaked private data")
				}
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("inspection persisted state")
			}
		})
	}
}

func TestHealthRefusesIneligibleEndpointsWithoutBackendCalls(t *testing.T) {
	for _, mode := range []string{"idle", "stale-boot", "legacy-boot", "allocating", "deleting", "disconnected", "degraded"} {
		t.Run(mode, func(t *testing.T) {
			s, b, e := ownershipFixture(t)
			switch mode {
			case "idle":
				if err := s.Store.Delete(e.NetworkName, e.ContainerID, e.IfName); err != nil {
					t.Fatal(err)
				}
			case "stale-boot":
				e.PrimaryBootID = "00000000-0000-0000-0000-000000000000"
			case "legacy-boot":
				e.PrimaryBootID = ""
			default:
				e.State = strings.ToUpper(mode)
			}
			if mode != "idle" {
				if err := s.Store.Put(e); err != nil {
					t.Fatal(err)
				}
			}
			r, issue := s.Health(t.Context())
			if issue != nil || r.Validate() != nil || len(b.checks) != 0 {
				t.Fatal(r, issue)
			}
			if mode == "idle" {
				if r.Status != "idle" || len(r.Endpoints) != 0 {
					t.Fatal(r)
				}
			} else if r.Status != "degraded" {
				t.Fatal("unready endpoint passed", r)
			}
		})
	}
}

func TestHealthBusyCancellationDriftAndInvalidStateRefuse(t *testing.T) {
	for _, mode := range []string{"mutation", "orchestration", "store", "canceled", "backend-deadline", "changed", "oversize", "invalid", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			s, _, e := ownershipFixture(t)
			b := &healthBackend{check: func(ctx context.Context, _ Endpoint) error {
				if mode == "backend-deadline" {
					<-ctx.Done()
					return ctx.Err()
				}
				if mode == "changed" {
					e.RXPackets++
					if err := s.Store.Put(e); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			}}
			s.Backend = b
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			defer cancel()
			switch mode {
			case "mutation":
				s.mu.Lock()
				defer s.mu.Unlock()
			case "orchestration":
				s.orchestrate.Lock()
				defer s.orchestrate.Unlock()
			case "store":
				s.Store.mu.Lock()
				defer s.Store.mu.Unlock()
			case "canceled":
				cancel()
			case "invalid":
				s.Store.data.Version = 2
			case "duplicate", "oversize":
				count := 2
				if mode == "oversize" {
					count = MaxHealthEndpoints + 1
				}
				for i := 1; i < count; i++ {
					copy := e
					copy.ContainerID = fmt.Sprintf("endpoint-%d", i)
					copy.Generation = fmt.Sprintf("%032x", i)
					if mode == "duplicate" {
						copy.Generation = e.Generation
					}
					s.Store.data.Endpoints[endpointKey(copy.NetworkName, copy.ContainerID, copy.IfName)] = copy
				}
			}
			started := time.Now()
			r, issue := s.Health(ctx)
			if issue == nil || r.Version != 0 || time.Since(started) > time.Second {
				t.Fatal("unsafe health accepted or blocked", r, issue)
			}
			if mode != "changed" && mode != "backend-deadline" && b.calls != 0 {
				t.Fatal("refusal invoked backend")
			}
		})
	}
}

func TestHealthClientRefusesLegacyMixedMalformedAndPrivateReplies(t *testing.T) {
	for _, mode := range []string{"checked", "idle", "degraded", "legacy", "mixed-cap", "mixed-report", "private-error", "invalid-code", "invalid-status", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := scriptedOwnershipClient(t, func(req Request) Response {
				calls++
				v := Response{Version: 1, RequestID: req.RequestID}
				if mode == "private-error" {
					v.Error = &APIError{Code: "UNAVAILABLE", Message: "private-credential /private/path"}
					return v
				}
				if req.Method == "CAPABILITIES" {
					v.Capabilities = &Capabilities{Version: 1, Features: []string{HealthCapability}}
					if mode == "legacy" {
						v.Capabilities.Features = []string{OwnershipCapability}
					}
					if mode == "mixed-cap" {
						v.Health = &HealthReport{}
					}
				} else {
					v.Health = &HealthReport{Version: 1, Source: "host-network-backend-check", PrimaryBootID: "00000000-0000-0000-0000-000000000001", Status: "checked", Endpoints: []EndpointHealth{{EndpointGeneration: strings.Repeat("a", 32), DescriptorSHA256: strings.Repeat("b", 64), Code: "CHECK_PASSED"}}}
					switch mode {
					case "idle":
						v.Health.Status = "idle"
						v.Health.Endpoints = []EndpointHealth{}
					case "degraded":
						v.Health.Status = "degraded"
						v.Health.Endpoints[0].Code = "BACKEND_CHECK_FAILED"
					case "mixed-report":
						v.Ownership = &OwnershipReport{}
					case "invalid-code":
						v.Health.Endpoints[0].Code = "private-credential"
					case "invalid-status":
						v.Health.Status = "ready"
					case "duplicate":
						v.Health.Endpoints = append(v.Health.Endpoints, v.Health.Endpoints[0])
					}
				}
				return v
			}, time.Second)
			r, err := InspectHealth(t.Context(), c)
			if mode == "checked" || mode == "idle" || mode == "degraded" {
				if err != nil || r.Validate() != nil || calls != 2 {
					t.Fatal(r, err, calls)
				}
				return
			}
			if err == nil || r.Version != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe reply accepted or leaked", r, err)
			}
			if mode == "legacy" && (!errors.Is(err, ErrHealthUnsupported) || calls != 1) {
				t.Fatal("legacy fallback")
			}
		})
	}
}

func TestHealthNegotiationAndChecksShareClientDeadline(t *testing.T) {
	c := scriptedOwnershipClient(t, func(req Request) Response {
		time.Sleep(40 * time.Millisecond)
		return Response{Version: 1, RequestID: req.RequestID, Capabilities: &Capabilities{Version: 1, Features: []string{HealthCapability}}}
	}, 60*time.Millisecond)
	start := time.Now()
	if _, err := InspectHealth(t.Context(), c); err == nil {
		t.Fatal("deadline reset")
	}
	if time.Since(start) > time.Second {
		t.Fatal("unbounded health client")
	}
}

func TestHealthDispatchRejectsMutationShapedInput(t *testing.T) {
	s, b, _ := ownershipFixture(t)
	for _, req := range []Request{{Endpoint: &Endpoint{}}, {Generation: "private"}} {
		req.Version = 1
		req.RequestID = "health"
		req.Method = "INSPECT_HEALTH"
		r := s.Dispatch(t.Context(), req)
		if r.Error == nil || r.Error.Code != "INVALID_ARGUMENT" || r.Health != nil {
			t.Fatal(r)
		}
	}
	if len(b.checks) != 0 {
		t.Fatal("bad request invoked backend")
	}
	r := s.Dispatch(t.Context(), Request{Version: 1, RequestID: "health", Method: "INSPECT_HEALTH"})
	if r.Error != nil || r.Health == nil || r.Health.Validate() != nil {
		t.Fatal(r)
	}
}
