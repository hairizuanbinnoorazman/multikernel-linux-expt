package network

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
)

const OwnershipCapability = "network-ownership-v1"
const MaxOwnershipRecords = 1024

type OwnershipCounters struct {
	RXPackets uint64 `json:"rx_packets"`
	TXPackets uint64 `json:"tx_packets"`
	RXDrops   uint64 `json:"rx_drops"`
	TXDrops   uint64 `json:"tx_drops"`
	Errors    uint64 `json:"errors"`
}
type EndpointOwnership struct {
	EndpointGeneration string            `json:"endpoint_generation"`
	SandboxID          string            `json:"sandbox_id,omitempty"`
	SandboxGeneration  string            `json:"sandbox_generation,omitempty"`
	PrimaryBootID      string            `json:"primary_boot_id,omitempty"`
	Owner              string            `json:"owner"`
	ManagedNamespace   bool              `json:"managed_namespace"`
	State              string            `json:"state"`
	DescriptorSHA256   string            `json:"descriptor_sha256"`
	Counters           OwnershipCounters `json:"counters"`
}
type OwnershipReport struct {
	Version       int                 `json:"version"`
	Source        string              `json:"source"`
	PrimaryBootID string              `json:"primary_boot_id"`
	Endpoints     []EndpointOwnership `json:"endpoints"`
}
type Capabilities struct {
	Version  int      `json:"version"`
	Features []string `json:"features"`
}

func (r OwnershipReport) Validate() error {
	if r.Version != 1 || r.Source != "durable-network" || !primaryBootID.MatchString(r.PrimaryBootID) || r.Endpoints == nil || len(r.Endpoints) > MaxOwnershipRecords {
		return errors.New("invalid network ownership report")
	}
	previous := ""
	for _, v := range r.Endpoints {
		digest, err := hex.DecodeString(v.DescriptorSHA256)
		if !endpointGeneration.MatchString(v.EndpointGeneration) || v.EndpointGeneration <= previous || err != nil || len(digest) != 32 || hex.EncodeToString(digest) != v.DescriptorSHA256 ||
			(v.Owner != "runtime" && v.Owner != "cni") || v.ManagedNamespace != (v.Owner == "runtime") ||
			(v.PrimaryBootID != "" && !primaryBootID.MatchString(v.PrimaryBootID)) ||
			((v.SandboxID == "") != (v.SandboxGeneration == "")) ||
			(v.SandboxID != "" && (!identifier.MatchString(v.SandboxID) || !endpointGeneration.MatchString(v.SandboxGeneration))) ||
			(v.State != "ALLOCATING" && v.State != "READY" && v.State != "DELETING" && v.State != "DEGRADED" && v.State != "DISCONNECTED") ||
			(v.State == "ALLOCATING" && v.SandboxID != "") {
			return errors.New("invalid network ownership metadata")
		}
		previous = v.EndpointGeneration
	}
	return nil
}

// Hash raw bytes with explicit lengths, rather than normalizing private strings
// through JSON. No private names, namespace paths, addresses or DNS are returned.
func endpointDescriptor(v Endpoint) (string, error) {
	h := sha256.New()
	h.Write([]byte("mkruntime-network-descriptor-v1\x00"))
	total := 0
	number := func(n uint64) { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); h.Write(b[:]) }
	put := func(s string) bool {
		total += len(s)
		if len(s) > 4096 || total > 65536 {
			return false
		}
		number(uint64(len(s)))
		h.Write([]byte(s))
		return true
	}
	for _, s := range []string{v.ContainerID, v.NetworkName, v.IfName, v.NetNS, v.Address, v.Gateway} {
		if !put(s) {
			return "", errors.New("network descriptor exceeds its bound")
		}
	}
	number(uint64(v.MTU))
	if !put(v.DNS.Domain) {
		return "", errors.New("network descriptor exceeds its bound")
	}
	for _, list := range [][]string{v.DNS.Nameservers, v.DNS.Search, v.DNS.Options} {
		number(uint64(len(list)))
		for _, s := range list {
			if !put(s) {
				return "", errors.New("network descriptor exceeds its bound")
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Ownership samples only in-memory durable claims. It never takes the service
// mutation lock, invokes a backend, reconciles, or initializes files/directories.
func (s *Service) Ownership() (OwnershipReport, *APIError) {
	if !s.Store.mu.TryLock() {
		return OwnershipReport{}, &APIError{Code: "UNAVAILABLE", Message: "network ownership snapshot is busy", Retryable: true}
	}
	defer s.Store.mu.Unlock()
	invalid := func() (OwnershipReport, *APIError) {
		return OwnershipReport{}, &APIError{Code: "OPERATOR_ACTION", Message: "durable network ownership metadata is invalid"}
	}
	if s.Store.data.Version != 1 || s.Store.data.Endpoints == nil {
		return invalid()
	}
	if len(s.Store.data.Endpoints) > MaxOwnershipRecords {
		return OwnershipReport{}, &APIError{Code: "RESOURCE_EXHAUSTED", Message: "network ownership exceeds the record limit"}
	}
	r := OwnershipReport{Version: 1, Source: "durable-network", PrimaryBootID: s.PrimaryBootID, Endpoints: make([]EndpointOwnership, 0, len(s.Store.data.Endpoints))}
	for key, v := range s.Store.data.Endpoints {
		if validateStoredEndpoint(key, v) != nil {
			return invalid()
		}
		digest, err := endpointDescriptor(v)
		if err != nil {
			return invalid()
		}
		r.Endpoints = append(r.Endpoints, EndpointOwnership{EndpointGeneration: v.Generation, SandboxID: v.SandboxID, SandboxGeneration: v.SandboxGeneration, PrimaryBootID: v.PrimaryBootID, Owner: v.Owner, ManagedNamespace: v.ManagedNamespace, State: v.State, DescriptorSHA256: digest, Counters: OwnershipCounters{RXPackets: v.RXPackets, TXPackets: v.TXPackets, RXDrops: v.RXDrops, TXDrops: v.TXDrops, Errors: v.Errors}})
	}
	sort.Slice(r.Endpoints, func(i, j int) bool { return r.Endpoints[i].EndpointGeneration < r.Endpoints[j].EndpointGeneration })
	if r.Validate() != nil {
		return invalid()
	}
	return r, nil
}
