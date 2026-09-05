package network

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"sync"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type Backend interface {
	Add(context.Context, Endpoint) error
	Check(context.Context, Endpoint) error
	Delete(context.Context, Endpoint) error
}

type NamespaceBackend interface {
	Backend
	CreateNamespace(context.Context, string) (string, error)
	DeleteNamespace(context.Context, string) error
}

type Service struct {
	Store       *Store
	Backend     Backend
	Subnet      *net.IPNet
	MTU         int
	DNS         DNS
	now         func() time.Time
	mu          sync.Mutex
	orchestrate sync.Mutex
}

func NewService(store *Store, backend Backend, subnet string, mtu int, dns DNS) (*Service, error) {
	ip, network, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() == nil || network.String() != subnet {
		return nil, errors.New("mknetd subnet must be a canonical IPv4 CIDR")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones < 16 || ones > 30 {
		return nil, errors.New("mknetd subnet must be between /16 and /30")
	}
	if !ip.IsPrivate() {
		return nil, errors.New("mknetd guest subnet must use RFC1918 private space")
	}
	if mtu < 576 || mtu > 65515 {
		return nil, errors.New("mknetd MTU must be between 576 and 65515")
	}
	if store == nil || backend == nil {
		return nil, errors.New("mknetd store and backend are required")
	}
	if len(dns.Nameservers) > 8 || len(dns.Search) > 8 || len(dns.Options) > 8 {
		return nil, errors.New("mknetd DNS lists are bounded to eight entries")
	}
	for _, server := range dns.Nameservers {
		if net.ParseIP(server) == nil {
			return nil, errors.New("mknetd DNS server must be an IP address")
		}
	}
	return &Service{Store: store, Backend: backend, Subnet: network, MTU: mtu, DNS: dns, now: func() time.Time { return time.Now().UTC() }}, nil
}

func validateEndpoint(endpoint *Endpoint, requireNetNS bool) *APIError {
	if endpoint == nil || !identifier.MatchString(endpoint.ContainerID) || !identifier.MatchString(endpoint.NetworkName) || !identifier.MatchString(endpoint.IfName) {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "network, container ID, and interface name must be valid identifiers"}
	}
	if len(endpoint.IfName) > 15 {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "interface name exceeds the Linux 15-byte limit"}
	}
	if requireNetNS && !endpoint.ManagedNamespace && (endpoint.NetNS == "" || endpoint.NetNS[0] != '/') {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "ADD and CHECK require an absolute network namespace path"}
	}
	return nil
}

func generation() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func addIP(ip net.IP, value int) net.IP {
	result := append(net.IP(nil), ip.To4()...)
	number := int(result[0])<<24 | int(result[1])<<16 | int(result[2])<<8 | int(result[3])
	number += value
	return net.IPv4(byte(number>>24), byte(number>>16), byte(number>>8), byte(number))
}

func (s *Service) allocate() (address, gateway string, err error) {
	ones, _ := s.Subnet.Mask.Size()
	count := 1 << (30 - ones)
	used := map[string]bool{}
	for _, endpoint := range s.Store.List() {
		used[endpoint.Address] = true
	}
	for index := 0; index < count; index++ {
		base := addIP(s.Subnet.IP, index*4)
		candidate := addIP(base, 2).String() + "/30"
		if !used[candidate] {
			return candidate, addIP(base, 1).String(), nil
		}
	}
	return "", "", errors.New("network address pool is exhausted")
}

func sameAdd(a, b Endpoint) bool {
	return a.ContainerID == b.ContainerID && a.NetworkName == b.NetworkName && a.IfName == b.IfName && a.NetNS == b.NetNS && a.Owner == b.Owner
}

func (s *Service) Add(ctx context.Context, requested Endpoint) (Endpoint, *APIError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if issue := validateEndpoint(&requested, true); issue != nil {
		return Endpoint{}, issue
	}
	if requested.Owner == "" {
		requested.Owner = "cni"
	}
	if requested.Owner != "cni" && requested.Owner != "runtime" {
		return Endpoint{}, &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint owner must be cni or runtime"}
	}
	if requested.ManagedNamespace && requested.Owner != "runtime" {
		return Endpoint{}, &APIError{Code: "INVALID_ARGUMENT", Message: "only runtime-owned endpoints may create a namespace"}
	}
	if existing, ok := s.Store.Get(requested.NetworkName, requested.ContainerID, requested.IfName); ok {
		if !sameAdd(existing, requested) {
			return Endpoint{}, &APIError{Code: "ALREADY_EXISTS", Message: "endpoint identity is already bound to different input"}
		}
		if err := s.Backend.Check(ctx, existing); err != nil {
			return Endpoint{}, &APIError{Code: "FAILED_PRECONDITION", Message: err.Error(), Retryable: true}
		}
		return existing, nil
	}
	address, gateway, err := s.allocate()
	if err != nil {
		return Endpoint{}, &APIError{Code: "RESOURCE_EXHAUSTED", Message: err.Error()}
	}
	gen, err := generation()
	if err != nil {
		return Endpoint{}, &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	now := s.now()
	requested.Generation, requested.Address, requested.Gateway = gen, address, gateway
	requested.MTU, requested.DNS, requested.State = s.MTU, s.DNS, "READY"
	requested.CreatedAt, requested.UpdatedAt = now, now
	if requested.ManagedNamespace {
		namespaces, ok := s.Backend.(NamespaceBackend)
		if !ok {
			return Endpoint{}, &APIError{Code: "UNSUPPORTED", Message: "network backend cannot create namespaces"}
		}
		requested.NetNS, err = namespaces.CreateNamespace(ctx, requested.Generation)
		if err != nil {
			return Endpoint{}, &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
		}
	}
	if err = s.Backend.Add(ctx, requested); err != nil {
		_ = s.Backend.Delete(context.WithoutCancel(ctx), requested)
		if requested.ManagedNamespace {
			_ = s.Backend.(NamespaceBackend).DeleteNamespace(context.WithoutCancel(ctx), requested.NetNS)
		}
		return Endpoint{}, &APIError{Code: "INTERNAL", Message: "endpoint ADD rolled back: " + err.Error(), Retryable: true}
	}
	if err = s.Store.Put(requested); err != nil {
		rollbackErr := s.Backend.Delete(context.WithoutCancel(ctx), requested)
		if requested.ManagedNamespace {
			rollbackErr = errors.Join(rollbackErr, s.Backend.(NamespaceBackend).DeleteNamespace(context.WithoutCancel(ctx), requested.NetNS))
		}
		return Endpoint{}, &APIError{Code: "INTERNAL", Message: errors.Join(err, rollbackErr).Error(), Retryable: true}
	}
	return requested, nil
}

func (s *Service) Check(ctx context.Context, requested Endpoint) (Endpoint, *APIError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if issue := validateEndpoint(&requested, true); issue != nil {
		return Endpoint{}, issue
	}
	if requested.Owner == "" {
		requested.Owner = "cni"
	}
	existing, ok := s.Store.Get(requested.NetworkName, requested.ContainerID, requested.IfName)
	if !ok {
		return Endpoint{}, &APIError{Code: "NOT_FOUND", Message: "endpoint does not exist"}
	}
	if !sameAdd(existing, requested) {
		return Endpoint{}, &APIError{Code: "STALE_IDENTITY", Message: "endpoint input no longer matches its allocation"}
	}
	if requested.Generation != "" && requested.Generation != existing.Generation {
		return Endpoint{}, &APIError{Code: "STALE_GENERATION", Message: "endpoint generation does not match"}
	}
	if err := s.Backend.Check(ctx, existing); err != nil {
		return Endpoint{}, &APIError{Code: "FAILED_PRECONDITION", Message: err.Error(), Retryable: true}
	}
	return existing, nil
}

func (s *Service) Delete(ctx context.Context, requested Endpoint) *APIError {
	s.mu.Lock()
	defer s.mu.Unlock()
	if issue := validateEndpoint(&requested, false); issue != nil {
		return issue
	}
	existing, ok := s.Store.Get(requested.NetworkName, requested.ContainerID, requested.IfName)
	if !ok {
		return nil
	}
	if requested.Generation == "" || requested.Generation != existing.Generation {
		return &APIError{Code: "STALE_GENERATION", Message: "endpoint generation does not match"}
	}
	if requested.NetNS != "" && requested.NetNS != existing.NetNS {
		return &APIError{Code: "STALE_IDENTITY", Message: "endpoint namespace no longer matches its allocation"}
	}
	if existing.SandboxID != "" {
		return &APIError{Code: "FAILED_PRECONDITION", Message: "endpoint is still bound to a sandbox generation", Retryable: true}
	}
	if err := s.Backend.Delete(ctx, existing); err != nil {
		return &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	if existing.ManagedNamespace {
		namespaces, ok := s.Backend.(NamespaceBackend)
		if !ok {
			return &APIError{Code: "INTERNAL", Message: "managed endpoint backend cannot delete namespaces"}
		}
		if err := namespaces.DeleteNamespace(ctx, existing.NetNS); err != nil {
			return &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
		}
	}
	if err := s.Store.Delete(existing.NetworkName, existing.ContainerID, existing.IfName); err != nil {
		return &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	return nil
}

func (s *Service) Bind(ctx context.Context, requested Endpoint) (Endpoint, *APIError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requested.NetNS == "" || !identifier.MatchString(requested.SandboxID) || !endpointGeneration.MatchString(requested.SandboxGeneration) {
		return Endpoint{}, &APIError{Code: "INVALID_ARGUMENT", Message: "BIND requires namespace and valid sandbox identity/generation"}
	}
	var match *Endpoint
	for _, candidate := range s.Store.List() {
		if candidate.NetNS == requested.NetNS {
			if match != nil {
				return Endpoint{}, &APIError{Code: "FAILED_PRECONDITION", Message: "namespace has multiple endpoint allocations"}
			}
			copy := candidate
			match = &copy
		}
	}
	if match == nil {
		return Endpoint{}, &APIError{Code: "NOT_FOUND", Message: "namespace has no CNI endpoint"}
	}
	if requested.Generation != "" && requested.Generation != match.Generation {
		return Endpoint{}, &APIError{Code: "STALE_GENERATION", Message: "endpoint generation does not match"}
	}
	if match.SandboxID != "" && (match.SandboxID != requested.SandboxID || match.SandboxGeneration != requested.SandboxGeneration) {
		return Endpoint{}, &APIError{Code: "ALREADY_EXISTS", Message: "endpoint is bound to a different sandbox generation"}
	}
	if err := s.Backend.Check(ctx, *match); err != nil {
		return Endpoint{}, &APIError{Code: "FAILED_PRECONDITION", Message: err.Error(), Retryable: true}
	}
	match.SandboxID, match.SandboxGeneration = requested.SandboxID, requested.SandboxGeneration
	match.UpdatedAt = s.now()
	if err := s.Store.Put(*match); err != nil {
		return Endpoint{}, &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	return *match, nil
}

func (s *Service) Provision(ctx context.Context, requested Endpoint) (Endpoint, *APIError) {
	s.orchestrate.Lock()
	defer s.orchestrate.Unlock()
	if !identifier.MatchString(requested.SandboxID) || !endpointGeneration.MatchString(requested.SandboxGeneration) {
		return Endpoint{}, &APIError{Code: "INVALID_ARGUMENT", Message: "PROVISION requires sandbox identity/generation"}
	}
	for _, candidate := range s.Store.List() {
		identityMatch := candidate.ContainerID == requested.ContainerID && candidate.NetworkName == requested.NetworkName && candidate.IfName == requested.IfName
		if identityMatch || (requested.NetNS != "" && candidate.NetNS == requested.NetNS) {
			if !identityMatch || (requested.NetNS != "" && candidate.NetNS != requested.NetNS) {
				return Endpoint{}, &APIError{Code: "ALREADY_EXISTS", Message: "namespace endpoint identity belongs to a different workload"}
			}
			return s.Bind(ctx, Endpoint{NetNS: candidate.NetNS, Generation: candidate.Generation, SandboxID: requested.SandboxID, SandboxGeneration: requested.SandboxGeneration})
		}
	}
	requested.Owner = "runtime"
	requested.ManagedNamespace = requested.NetNS == ""
	sandboxID, sandboxGeneration := requested.SandboxID, requested.SandboxGeneration
	requested.SandboxID, requested.SandboxGeneration = "", ""
	allocated, issue := s.Add(ctx, requested)
	if issue != nil {
		return Endpoint{}, issue
	}
	bound, issue := s.Bind(ctx, Endpoint{NetNS: allocated.NetNS, Generation: allocated.Generation, SandboxID: sandboxID, SandboxGeneration: sandboxGeneration})
	if issue != nil {
		_ = s.Delete(context.WithoutCancel(ctx), allocated)
	}
	return bound, issue
}

func (s *Service) Release(ctx context.Context, requested Endpoint) *APIError {
	s.orchestrate.Lock()
	defer s.orchestrate.Unlock()
	var match *Endpoint
	for _, candidate := range s.Store.List() {
		if candidate.SandboxID == requested.SandboxID {
			copy := candidate
			match = &copy
			break
		}
	}
	if issue := s.Unbind(requested); issue != nil || match == nil || match.Owner != "runtime" {
		return issue
	}
	return s.Delete(ctx, *match)
}

func (s *Service) Unbind(requested Endpoint) *APIError {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !identifier.MatchString(requested.SandboxID) || !endpointGeneration.MatchString(requested.SandboxGeneration) {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "UNBIND requires valid sandbox identity/generation"}
	}
	var match *Endpoint
	for _, candidate := range s.Store.List() {
		if candidate.SandboxID == requested.SandboxID {
			copy := candidate
			match = &copy
			break
		}
	}
	if match == nil {
		return nil
	}
	if match.SandboxGeneration != requested.SandboxGeneration {
		return &APIError{Code: "STALE_GENERATION", Message: "sandbox generation does not match endpoint binding"}
	}
	match.SandboxID, match.SandboxGeneration = "", ""
	match.UpdatedAt = s.now()
	if err := s.Store.Put(*match); err != nil {
		return &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	return nil
}

func (s *Service) Report(requested Endpoint) *APIError {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !identifier.MatchString(requested.SandboxID) || !endpointGeneration.MatchString(requested.SandboxGeneration) || !endpointGeneration.MatchString(requested.Generation) {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "REPORT requires endpoint and sandbox generations"}
	}
	var match *Endpoint
	for _, candidate := range s.Store.List() {
		if candidate.SandboxID == requested.SandboxID {
			copy := candidate
			match = &copy
			break
		}
	}
	if match == nil {
		return &APIError{Code: "NOT_FOUND", Message: "sandbox has no network endpoint"}
	}
	if match.Generation != requested.Generation || match.SandboxGeneration != requested.SandboxGeneration {
		return &APIError{Code: "STALE_GENERATION", Message: "counter report generation does not match"}
	}
	if requested.RXPackets < match.RXPackets || requested.TXPackets < match.TXPackets || requested.RXDrops < match.RXDrops || requested.TXDrops < match.TXDrops || requested.Errors < match.Errors {
		return &APIError{Code: "STALE_COUNTER", Message: "network counters cannot decrease"}
	}
	if requested.State != "READY" && requested.State != "DEGRADED" && requested.State != "DISCONNECTED" {
		return &APIError{Code: "INVALID_ARGUMENT", Message: "invalid endpoint link state"}
	}
	match.RXPackets, match.TXPackets = requested.RXPackets, requested.TXPackets
	match.RXDrops, match.TXDrops, match.Errors = requested.RXDrops, requested.TXDrops, requested.Errors
	match.State, match.UpdatedAt = requested.State, s.now()
	if err := s.Store.Put(*match); err != nil {
		return &APIError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
	}
	return nil
}

func (s *Service) List() []Endpoint {
	result := s.Store.List()
	sort.Slice(result, func(i, j int) bool {
		if result[i].Address == result[j].Address {
			return result[i].ContainerID < result[j].ContainerID
		}
		return result[i].Address < result[j].Address
	})
	return result
}

// Reconcile verifies that every durable allocation still has its primary-side
// resources. It deliberately fails startup instead of silently duplicating or
// reallocating an endpoint after a daemon restart.
func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var failures []error
	for _, endpoint := range s.Store.List() {
		if err := s.Backend.Check(ctx, endpoint); err != nil {
			failures = append(failures, fmt.Errorf("endpoint %s/%s/%s generation %s: %w",
				endpoint.NetworkName, endpoint.ContainerID, endpoint.IfName, endpoint.Generation, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Service) Dispatch(ctx context.Context, request Request) Response {
	response := Response{Version: ProtocolVersion, RequestID: request.RequestID}
	if request.Version != ProtocolVersion || !identifier.MatchString(request.RequestID) {
		response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "invalid protocol version or request ID"}
		return response
	}
	switch request.Method {
	case "ADD":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		endpoint, issue := s.Add(ctx, *request.Endpoint)
		response.Endpoint, response.Error = &endpoint, issue
	case "CHECK":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		endpoint, issue := s.Check(ctx, *request.Endpoint)
		response.Endpoint, response.Error = &endpoint, issue
	case "DEL":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		response.Error = s.Delete(ctx, *request.Endpoint)
	case "BIND", "ATTACH":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		endpoint, issue := s.Bind(ctx, *request.Endpoint)
		response.Endpoint, response.Error = &endpoint, issue
	case "UNBIND":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		response.Error = s.Unbind(*request.Endpoint)
	case "REPORT":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		response.Error = s.Report(*request.Endpoint)
	case "PROVISION":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		endpoint, issue := s.Provision(ctx, *request.Endpoint)
		response.Endpoint, response.Error = &endpoint, issue
	case "RELEASE":
		if request.Endpoint == nil {
			response.Error = &APIError{Code: "INVALID_ARGUMENT", Message: "endpoint is required"}
			break
		}
		response.Error = s.Release(ctx, *request.Endpoint)
	case "LIST":
		response.Error = &APIError{Code: "UNSUPPORTED", Message: fmt.Sprintf("LIST has %d endpoints; use the administrative API", len(s.List()))}
	default:
		response.Error = &APIError{Code: "UNSUPPORTED", Message: "unsupported network method"}
	}
	return response
}
