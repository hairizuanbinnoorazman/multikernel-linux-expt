package network

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"slices"
	"sort"
	"time"
)

const HealthCapability = "network-health-v1"
const MaxHealthEndpoints = 32
const HealthTimeout = 5 * time.Second

// HealthReport describes host backend CHECK observations of recorded endpoints.
// It is not an end-to-end probe, service readiness or allocation authority.
type HealthReport struct {
	Version       int              `json:"version"`
	Source        string           `json:"source"`
	PrimaryBootID string           `json:"primary_boot_id"`
	Status        string           `json:"status"`
	Endpoints     []EndpointHealth `json:"endpoints"`
}
type EndpointHealth struct {
	EndpointGeneration string `json:"endpoint_generation"`
	DescriptorSHA256   string `json:"descriptor_sha256"`
	Code               string `json:"code"`
}

func (r HealthReport) Validate() error {
	invalid := errors.New("invalid network health report")
	if r.Version != 1 || r.Source != "host-network-backend-check" || !primaryBootID.MatchString(r.PrimaryBootID) || r.Endpoints == nil || len(r.Endpoints) > MaxHealthEndpoints {
		return invalid
	}
	status := "idle"
	previous := ""
	if len(r.Endpoints) > 0 {
		status = "checked"
	}
	for _, e := range r.Endpoints {
		digest, err := hex.DecodeString(e.DescriptorSHA256)
		if !endpointGeneration.MatchString(e.EndpointGeneration) || e.EndpointGeneration <= previous || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != e.DescriptorSHA256 {
			return invalid
		}
		switch e.Code {
		case "CHECK_PASSED":
		case "ENDPOINT_NOT_READY", "ENDPOINT_BOOT_MISMATCH", "BACKEND_CHECK_FAILED":
			status = "degraded"
		default:
			return invalid
		}
		previous = e.EndpointGeneration
	}
	if r.Status != status {
		return invalid
	}
	return nil
}

// healthSnapshot refuses busy, invalid or oversized state without backend calls.
func (s *Service) healthSnapshot() ([]Endpoint, *APIError) {
	if !s.Store.mu.TryLock() {
		return nil, &APIError{Code: "UNAVAILABLE", Message: "network health snapshot is busy", Retryable: true}
	}
	defer s.Store.mu.Unlock()
	if s.Store.data.Version != 1 || s.Store.data.Endpoints == nil {
		return nil, &APIError{Code: "OPERATOR_ACTION", Message: "network health metadata is invalid"}
	}
	if len(s.Store.data.Endpoints) > MaxHealthEndpoints {
		return nil, &APIError{Code: "RESOURCE_EXHAUSTED", Message: "network health endpoint limit exceeded"}
	}
	seen := map[string]bool{}
	endpoints := make([]Endpoint, 0, len(s.Store.data.Endpoints))
	for key, e := range s.Store.data.Endpoints {
		_, descriptorErr := endpointDescriptor(e)
		if validateStoredEndpoint(key, e) != nil || descriptorErr != nil || seen[e.Generation] {
			return nil, &APIError{Code: "OPERATOR_ACTION", Message: "network health metadata is invalid"}
		}
		seen[e.Generation] = true
		endpoints = append(endpoints, cloneEndpoint(e))
	}
	sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].Generation < endpoints[j].Generation })
	return endpoints, nil
}

// Health holds the existing orchestration/mutation locks only after TryLock and
// bounds all backend checks together. It never calls ADD, DEL, reconciliation,
// counter reporting or persistence. Unrecorded resources and ABA remain outside
// its scope. The production Linux backend uses bounded context-aware commands.
func (s *Service) Health(ctx context.Context) (HealthReport, *APIError) {
	ctx, cancel := context.WithTimeout(ctx, HealthTimeout)
	defer cancel()
	unavailable := func() (HealthReport, *APIError) {
		return HealthReport{}, &APIError{Code: "UNAVAILABLE", Message: "network health observation unavailable", Retryable: true}
	}
	if ctx.Err() != nil || !s.orchestrate.TryLock() {
		return unavailable()
	}
	defer s.orchestrate.Unlock()
	if !s.mu.TryLock() {
		return unavailable()
	}
	defer s.mu.Unlock()
	if s.Store == nil || s.Backend == nil || !primaryBootID.MatchString(s.PrimaryBootID) {
		return HealthReport{}, &APIError{Code: "OPERATOR_ACTION", Message: "network health service is invalid"}
	}
	endpoints, failure := s.healthSnapshot()
	if failure != nil {
		return HealthReport{}, failure
	}
	r := HealthReport{Version: 1, Source: "host-network-backend-check", PrimaryBootID: s.PrimaryBootID, Status: "idle", Endpoints: make([]EndpointHealth, 0, len(endpoints))}
	if len(endpoints) > 0 {
		r.Status = "checked"
	}
	for _, e := range endpoints {
		if ctx.Err() != nil {
			return unavailable()
		}
		digest, err := endpointDescriptor(e)
		if err != nil {
			return HealthReport{}, &APIError{Code: "OPERATOR_ACTION", Message: "network health descriptor is invalid"}
		}
		item := EndpointHealth{EndpointGeneration: e.Generation, DescriptorSHA256: digest, Code: "CHECK_PASSED"}
		switch {
		case e.PrimaryBootID != s.PrimaryBootID:
			item.Code = "ENDPOINT_BOOT_MISMATCH"
		case e.State != "READY":
			item.Code = "ENDPOINT_NOT_READY"
		default:
			if s.Backend.Check(ctx, e) != nil {
				item.Code = "BACKEND_CHECK_FAILED"
			}
		}
		if ctx.Err() != nil {
			return unavailable()
		}
		if item.Code != "CHECK_PASSED" {
			r.Status = "degraded"
		}
		r.Endpoints = append(r.Endpoints, item)
	}
	after, failure := s.healthSnapshot()
	if failure != nil {
		return HealthReport{}, failure
	}
	if ctx.Err() != nil || !reflect.DeepEqual(endpoints, after) {
		return unavailable()
	}
	if r.Validate() != nil {
		return HealthReport{}, &APIError{Code: "OPERATOR_ACTION", Message: "network health report is invalid"}
	}
	return r, nil
}

var ErrHealthUnsupported = errors.New("mknetd does not advertise network-health-v1")

// InspectHealth negotiates and reads with one budget, without legacy fallback.
func InspectHealth(ctx context.Context, client Client) (HealthReport, error) {
	timeout := HealthTimeout
	if client.Timeout > 0 && client.Timeout < timeout {
		timeout = client.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	fail := func(err error) (HealthReport, error) {
		var api *APIError
		if errors.As(err, &api) && api.Code == "UNSUPPORTED" {
			return HealthReport{}, ErrHealthUnsupported
		}
		return HealthReport{}, errors.New("network health request failed")
	}
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	cap, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "network-health-capabilities-v1", Method: "CAPABILITIES"})
	if err != nil {
		return fail(err)
	}
	if !validInspectionCapabilities(cap) {
		return HealthReport{}, errors.New("invalid network health capabilities")
	}
	if !slices.Contains(cap.Capabilities.Features, HealthCapability) {
		return HealthReport{}, ErrHealthUnsupported
	}
	response, err := client.Call(ctx, Request{Version: ProtocolVersion, RequestID: "inspect-network-health-v1", Method: "INSPECT_HEALTH"})
	if err != nil {
		return fail(err)
	}
	if ctx.Err() != nil || response.Health == nil || response.Endpoint != nil || response.Ownership != nil || response.ComponentBuild != nil || response.Capabilities != nil || response.Health.Validate() != nil {
		return HealthReport{}, errors.New("invalid network health response")
	}
	return *response.Health, nil
}
