package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ownershipFixture(t *testing.T) (*Service, *fakeBackend, Endpoint) {
	t.Helper()
	b := &fakeBackend{}
	s := service(t, "172.31.0.0/24", b)
	e, issue := s.Add(t.Context(), endpoint("private-workload"))
	if issue != nil {
		t.Fatal(issue)
	}
	e.DNS.Domain = "private-credential"
	e.DNS.Search = []string{"private-search"}
	e.DNS.Options = []string{"private-option"}
	if err := s.Store.Put(e); err != nil {
		t.Fatal(err)
	}
	return s, b, e
}
func TestOwnershipProjectionIsReadOnlyPrivateAndIndependentOfMutationLock(t *testing.T) {
	s, b, e := ownershipFixture(t)
	path := filepath.Join(s.Store.dir, "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := []int{len(b.adds), len(b.checks), len(b.deletes), len(b.namespacesCreated), len(b.namespacesDeleted)}
	s.mu.Lock()
	s.orchestrate.Lock()
	defer s.mu.Unlock()
	defer s.orchestrate.Unlock()
	done := make(chan struct{})
	var r OwnershipReport
	var issue *APIError
	go func() { r, issue = s.Ownership(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ownership waited on the mutation lock")
	}
	if issue != nil || r.Validate() != nil || len(r.Endpoints) != 1 || r.Endpoints[0].EndpointGeneration != e.Generation {
		t.Fatalf("projection=%+v issue=%v", r, issue)
	}
	data, _ := json.Marshal(r)
	for _, secret := range []string{"private-workload", "private-credential", "private-search", "private-option", e.NetNS, e.Address, e.Gateway} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("private descriptor value was serialized")
		}
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(calls, []int{len(b.adds), len(b.checks), len(b.deletes), len(b.namespacesCreated), len(b.namespacesDeleted)}) {
		t.Fatal("inspection mutated disk or invoked backend")
	}
	digest := r.Endpoints[0].DescriptorSHA256
	e.DNS.Domain = "different-private-credential"
	if err := s.Store.Put(e); err != nil {
		t.Fatal(err)
	}
	changed, issue := s.Ownership()
	if issue != nil || changed.Endpoints[0].DescriptorSHA256 == digest {
		t.Fatal("private descriptor drift not fingerprinted")
	}
}
func TestOwnershipBusyLimitsAndInvalidRecordsAreExplicit(t *testing.T) {
	s, _, e := ownershipFixture(t)
	s.Store.mu.Lock()
	_, issue := s.Ownership()
	s.Store.mu.Unlock()
	if issue == nil || issue.Code != "UNAVAILABLE" || !issue.Retryable {
		t.Fatal("busy snapshot not refused")
	}
	key := endpointKey(e.NetworkName, e.ContainerID, e.IfName)
	e.DNS.Domain = strings.Repeat("secret", 1000)
	s.Store.data.Endpoints[key] = e
	if _, issue = s.Ownership(); issue == nil || issue.Code != "OPERATOR_ACTION" || strings.Contains(issue.Message, "secret") {
		t.Fatal("invalid oversized private record accepted or disclosed")
	}
	s.Store.data.Endpoints = make(map[string]Endpoint)
	for n := 0; n <= MaxOwnershipRecords; n++ {
		s.Store.data.Endpoints[fmt.Sprint(n)] = Endpoint{}
	}
	if _, issue = s.Ownership(); issue == nil || issue.Code != "RESOURCE_EXHAUSTED" {
		t.Fatal("oversized inventory accepted")
	}
	s.Store.data.Endpoints = nil
	if _, issue = s.Ownership(); issue == nil || issue.Code != "OPERATOR_ACTION" {
		t.Fatal("invalid null store claimed empty")
	}
}
func TestOwnershipDescriptorRetainsRawByteAndFieldBoundaries(t *testing.T) {
	_, _, e := ownershipFixture(t)
	a, err := endpointDescriptor(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range []string{"\xff", "\ufffd"} {
		e.DNS.Domain = domain
		v, err := endpointDescriptor(e)
		if err != nil || v == a {
			t.Fatal("descriptor collision")
		}
		a = v
	}
	e.DNS.Search = []string{"ab", "c"}
	a, _ = endpointDescriptor(e)
	e.DNS.Search = []string{"a", "bc"}
	b, _ := endpointDescriptor(e)
	if a == b {
		t.Fatal("list/string boundaries collide")
	}
}
func TestOwnershipClientNegotiatesAndPreservesStoreOverWire(t *testing.T) {
	s, _, _ := ownershipFixture(t)
	server := &Server{Service: s}
	client := Client{Path: "memory", dial: func(context.Context, string, string) (net.Conn, error) {
		c, r := net.Pipe()
		go server.handle(t.Context(), r)
		return c, nil
	}}
	r, err := InspectOwnership(t.Context(), client)
	if err != nil || r.Validate() != nil || len(r.Endpoints) != 1 {
		t.Fatalf("ownership=%+v err=%v", r, err)
	}
	request := Request{Version: 1, RequestID: "bad-inspect", Method: "INSPECT_OWNERSHIP", Endpoint: &Endpoint{}}
	response := s.Dispatch(t.Context(), request)
	if response.Error == nil || response.Error.Code != "INVALID_ARGUMENT" || response.Ownership != nil {
		t.Fatal("inspection accepted mutation-shaped input")
	}
}
func scriptedOwnershipClient(t *testing.T, reply func(Request) Response, timeout time.Duration) Client {
	t.Helper()
	return Client{Path: "memory", Timeout: timeout, dial: func(context.Context, string, string) (net.Conn, error) {
		c, s := net.Pipe()
		go func() {
			defer s.Close()
			var r Request
			if err := json.NewDecoder(s).Decode(&r); err != nil {
				return
			}
			_ = json.NewEncoder(s).Encode(reply(r))
		}()
		return c, nil
	}}
}
func TestOwnershipClientRefusesUnsupportedAndMalformedWithoutFallback(t *testing.T) {
	var calls int
	c := scriptedOwnershipClient(t, func(r Request) Response {
		calls++
		return Response{Version: 1, RequestID: r.RequestID, Error: &APIError{Code: "UNSUPPORTED", Message: "private-credential"}}
	}, time.Second)
	if _, err := InspectOwnership(t.Context(), c); !errors.Is(err, ErrOwnershipUnsupported) || calls != 1 {
		t.Fatal("legacy server did not refuse without fallback")
	}
	for _, kind := range []string{"no feature", "duplicate feature", "raw endpoint", "invalid report"} {
		t.Run(kind, func(t *testing.T) {
			c := scriptedOwnershipClient(t, func(r Request) Response {
				v := Response{Version: 1, RequestID: r.RequestID}
				if r.Method == "CAPABILITIES" {
					v.Capabilities = &Capabilities{Version: 1, Features: []string{OwnershipCapability}}
					switch kind {
					case "no feature":
						v.Capabilities.Features = []string{"other-feature"}
					case "duplicate feature":
						v.Capabilities.Features = append(v.Capabilities.Features, OwnershipCapability)
					case "raw endpoint":
						v.Endpoint = &Endpoint{DNS: DNS{Domain: "private-credential"}}
					}
				} else {
					v.Ownership = &OwnershipReport{Version: 1, Source: "durable-network"}
				}
				return v
			}, time.Second)
			if _, err := InspectOwnership(t.Context(), c); err == nil || strings.Contains(err.Error(), "private-credential") {
				t.Fatalf("malformed report accepted/disclosed: %v", err)
			}
		})
	}
}
func TestOwnershipClientUsesOneDeadlineForNegotiationAndRead(t *testing.T) {
	c := scriptedOwnershipClient(t, func(r Request) Response {
		time.Sleep(40 * time.Millisecond)
		v := Response{Version: 1, RequestID: r.RequestID}
		if r.Method == "CAPABILITIES" {
			v.Capabilities = &Capabilities{Version: 1, Features: []string{OwnershipCapability}}
		} else {
			v.Ownership = &OwnershipReport{Version: 1, Source: "durable-network", PrimaryBootID: testPrimaryBootID, Endpoints: []EndpointOwnership{}}
		}
		return v
	}, 60*time.Millisecond)
	started := time.Now()
	if _, err := InspectOwnership(t.Context(), c); err == nil {
		t.Fatal("each RPC received a fresh budget")
	}
	if time.Since(started) > time.Second {
		t.Fatal("shared deadline did not bound inspection")
	}
}

// Explicit lab opt-in. Read the retained failed ledger without starting a
// daemon or reconciling its prior-boot endpoint; export only the projection.
func TestOwnershipReadsRetainedFailedLedgerWithoutReconcile(t *testing.T) {
	dir := os.Getenv("MK_G8_NETWORK_STATE_FIXTURE")
	if dir == "" {
		t.Skip("requires explicit retained private GCE fixture")
	}
	if dir != "/opt/mkruntime/g8-host-017c52f/integration/network-state" {
		t.Fatal("unexpected retained fixture")
	}
	path := filepath.Join(dir, "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	s, err := NewService(store, backend, "172.30.0.0/24", 1400, DNS{}, strings.TrimSpace(string(boot)))
	if err != nil {
		t.Fatal(err)
	}
	r, issue := s.Ownership()
	if issue != nil || r.Validate() != nil || len(r.Endpoints) != 1 {
		t.Fatalf("retained projection: %+v %v", r, issue)
	}
	if len(backend.adds)+len(backend.checks)+len(backend.deletes) != 0 {
		t.Fatal("retained snapshot invoked backend")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("retained original state changed")
	}
	v := r.Endpoints[0]
	if v.State != "DISCONNECTED" || v.SandboxID == "" || v.PrimaryBootID == r.PrimaryBootID {
		t.Fatal("prior-boot failed endpoint was silently retired or rebound")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("NETWORK_OWNERSHIP_RETAINED_REPORT=" + string(encoded))
}

func TestOwnershipCancellationClosesPendingCapabilityRead(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	read := make(chan struct{})
	go func() {
		var request Request
		_ = json.NewDecoder(serverConn).Decode(&request)
		close(read)
		_, _ = serverConn.Read(make([]byte, 1))
	}()
	client := Client{Path: "memory", Timeout: time.Second, dial: func(context.Context, string, string) (net.Conn, error) { return clientConn, nil }}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := InspectOwnership(ctx, client); done <- err }()
	<-read
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled ownership read succeeded")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("canceled ownership read waited for the socket timeout")
	}
}

// Shared with the independent Python live-checkpoint projector. A digest
// mismatch must stop qualification rather than silently ignoring descriptor drift.
func TestEndpointDescriptorCrossLanguageVector(t *testing.T) {
	data, err := os.ReadFile("testdata/endpoint-descriptor.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Endpoint         Endpoint `json:"endpoint"`
		DescriptorSHA256 string   `json:"descriptor_sha256"`
	}
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	got, err := endpointDescriptor(vector.Endpoint)
	if err != nil || got != vector.DescriptorSHA256 {
		t.Fatalf("descriptor=%s err=%v", got, err)
	}
}
