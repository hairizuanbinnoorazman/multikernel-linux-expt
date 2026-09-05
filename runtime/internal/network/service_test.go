package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type fakeBackend struct {
	mu         sync.Mutex
	adds       []Endpoint
	checks     []Endpoint
	deletes    []Endpoint
	failAdd    error
	failCheck  error
	failDelete error
}

func (f *fakeBackend) Add(_ context.Context, endpoint Endpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, endpoint)
	return f.failAdd
}
func (f *fakeBackend) Check(_ context.Context, endpoint Endpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, endpoint)
	return f.failCheck
}
func (f *fakeBackend) Delete(_ context.Context, endpoint Endpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, endpoint)
	return f.failDelete
}

func service(t *testing.T, subnet string, backend *fakeBackend) *Service {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(store, backend, subnet, 1400, DNS{Nameservers: []string{"169.254.169.254"}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func endpoint(id string) Endpoint {
	return Endpoint{ContainerID: id, NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/" + id}
}

func TestAddCheckDeleteAreGenerationBoundAndIdempotent(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/24", backend)
	first, issue := s.Add(context.Background(), endpoint("one"))
	if issue != nil {
		t.Fatal(issue)
	}
	if first.Address != "172.31.0.2/30" || first.Gateway != "172.31.0.1" || len(first.Generation) != 32 {
		t.Fatalf("first endpoint = %+v", first)
	}
	replayed, issue := s.Add(context.Background(), endpoint("one"))
	if issue != nil || replayed.Generation != first.Generation || len(backend.adds) != 1 {
		t.Fatalf("idempotent ADD = %+v, issue=%v, backend adds=%d", replayed, issue, len(backend.adds))
	}
	second, issue := s.Add(context.Background(), endpoint("two"))
	if issue != nil || second.Address != "172.31.0.6/30" {
		t.Fatalf("second endpoint = %+v, issue=%v", second, issue)
	}
	check := endpoint("one")
	check.Generation = first.Generation
	if _, issue = s.Check(context.Background(), check); issue != nil {
		t.Fatal(issue)
	}
	check.Generation = "ffffffffffffffffffffffffffffffff"
	if _, issue = s.Check(context.Background(), check); issue == nil || issue.Code != "STALE_GENERATION" {
		t.Fatalf("stale CHECK issue = %+v", issue)
	}
	if issue = s.Delete(context.Background(), check); issue == nil || issue.Code != "STALE_GENERATION" {
		t.Fatalf("stale DEL issue = %+v", issue)
	}
	check.Generation = first.Generation
	if issue = s.Delete(context.Background(), check); issue != nil {
		t.Fatal(issue)
	}
	if issue = s.Delete(context.Background(), check); issue != nil || len(backend.deletes) != 1 {
		t.Fatalf("idempotent DEL issue=%v backend deletes=%d", issue, len(backend.deletes))
	}
}

func TestPartialAddFailureRollsBackWithoutState(t *testing.T) {
	backend := &fakeBackend{failAdd: errors.New("injected link failure")}
	s := service(t, "172.31.0.0/30", backend)
	_, issue := s.Add(context.Background(), endpoint("failed"))
	if issue == nil || len(backend.deletes) != 1 || len(s.List()) != 0 {
		t.Fatalf("failed ADD issue=%v deletes=%d state=%v", issue, len(backend.deletes), s.List())
	}
}

func TestConcurrentAllocationIsCollisionFreeAndDurable(t *testing.T) {
	directory := t.TempDir()
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	s, err := NewService(store, backend, "172.31.0.0/24", 1400, DNS{})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	issues := make(chan error, 32)
	for index := 0; index < 32; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if _, issue := s.Add(context.Background(), endpoint(fmt.Sprintf("box-%d", index))); issue != nil {
				issues <- issue
			}
		}(index)
	}
	group.Wait()
	close(issues)
	for issue := range issues {
		t.Fatal(issue)
	}
	addresses := map[string]bool{}
	for _, item := range s.List() {
		if addresses[item.Address] {
			t.Fatalf("duplicate allocation %s", item.Address)
		}
		addresses[item.Address] = true
	}
	if len(addresses) != 32 {
		t.Fatalf("allocated %d addresses", len(addresses))
	}
	reopened, err := OpenStore(directory)
	if err != nil || len(reopened.List()) != 32 {
		t.Fatalf("reopened endpoints=%d error=%v", len(reopened.List()), err)
	}
}

func TestAddressPoolExhaustionIsBounded(t *testing.T) {
	s := service(t, "172.31.0.0/30", &fakeBackend{})
	if _, issue := s.Add(context.Background(), endpoint("one")); issue != nil {
		t.Fatal(issue)
	}
	if _, issue := s.Add(context.Background(), endpoint("two")); issue == nil || issue.Code != "RESOURCE_EXHAUSTED" {
		t.Fatalf("pool exhaustion issue = %+v", issue)
	}
}

func TestDispatchRejectsMissingEndpointInsteadOfPanicking(t *testing.T) {
	s := service(t, "172.31.0.0/30", &fakeBackend{})
	for _, method := range []string{"ADD", "CHECK", "DEL"} {
		response := s.Dispatch(context.Background(), Request{Version: 1, RequestID: "request-1", Method: method})
		if response.Error == nil || response.Error.Code != "INVALID_ARGUMENT" {
			t.Fatalf("%s response = %+v", method, response)
		}
	}
}

func TestRestartReconciliationChecksEveryDurableEndpoint(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/29", backend)
	for _, id := range []string{"one", "two"} {
		if _, issue := s.Add(context.Background(), endpoint(id)); issue != nil {
			t.Fatal(issue)
		}
	}
	backend.checks = nil
	if err := s.Reconcile(context.Background()); err != nil || len(backend.checks) != 2 {
		t.Fatalf("reconcile error=%v checks=%d", err, len(backend.checks))
	}
	backend.failCheck = errors.New("injected missing route")
	if err := s.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("reconcile error=%v", err)
	}
}

func TestBindAndUnbindRejectCrossGenerationOwnership(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	allocated, issue := s.Add(context.Background(), endpoint("one"))
	if issue != nil {
		t.Fatal(issue)
	}
	firstGeneration := "11111111111111111111111111111111"
	bound, issue := s.Bind(context.Background(), Endpoint{NetNS: allocated.NetNS, Generation: allocated.Generation, SandboxID: "sandbox-one", SandboxGeneration: firstGeneration})
	if issue != nil || bound.SandboxID != "sandbox-one" {
		t.Fatalf("bind=%+v issue=%v", bound, issue)
	}
	if _, issue = s.Bind(context.Background(), Endpoint{NetNS: allocated.NetNS, SandboxID: "sandbox-two", SandboxGeneration: "22222222222222222222222222222222"}); issue == nil || issue.Code != "ALREADY_EXISTS" {
		t.Fatalf("cross-generation bind issue=%+v", issue)
	}
	if issue = s.Delete(context.Background(), Endpoint{ContainerID: "one", NetworkName: "multikernel", IfName: "eth0", Generation: allocated.Generation}); issue == nil || issue.Code != "FAILED_PRECONDITION" {
		t.Fatalf("delete bound endpoint issue=%+v", issue)
	}
	if issue = s.Unbind(Endpoint{SandboxID: "sandbox-one", SandboxGeneration: "22222222222222222222222222222222"}); issue == nil || issue.Code != "STALE_GENERATION" {
		t.Fatalf("stale unbind issue=%+v", issue)
	}
	if issue = s.Unbind(Endpoint{SandboxID: "sandbox-one", SandboxGeneration: firstGeneration}); issue != nil {
		t.Fatal(issue)
	}
	if issue = s.Delete(context.Background(), Endpoint{ContainerID: "one", NetworkName: "multikernel", IfName: "eth0", Generation: allocated.Generation}); issue != nil {
		t.Fatal(issue)
	}
}

func TestExistingEndpointRequiresGenerationForDelete(t *testing.T) {
	s := service(t, "172.31.0.0/30", &fakeBackend{})
	if _, issue := s.Add(context.Background(), endpoint("one")); issue != nil {
		t.Fatal(issue)
	}
	if issue := s.Delete(context.Background(), endpoint("one")); issue == nil || issue.Code != "STALE_GENERATION" {
		t.Fatalf("generationless delete issue=%+v", issue)
	}
}

func TestCounterReportsAreGenerationBoundAndMonotonic(t *testing.T) {
	s := service(t, "172.31.0.0/30", &fakeBackend{})
	allocated, issue := s.Add(context.Background(), endpoint("one"))
	if issue != nil {
		t.Fatal(issue)
	}
	sandboxGeneration := "11111111111111111111111111111111"
	bound, issue := s.Bind(context.Background(), Endpoint{NetNS: allocated.NetNS, SandboxID: "sandbox-one", SandboxGeneration: sandboxGeneration})
	if issue != nil {
		t.Fatal(issue)
	}
	report := bound
	report.State, report.RXPackets, report.TXPackets, report.RXDrops = "READY", 20, 10, 2
	if issue = s.Report(report); issue != nil {
		t.Fatal(issue)
	}
	report.RXPackets = 19
	if issue = s.Report(report); issue == nil || issue.Code != "STALE_COUNTER" {
		t.Fatalf("decreasing report issue=%+v", issue)
	}
	report.RXPackets, report.SandboxGeneration = 21, "22222222222222222222222222222222"
	if issue = s.Report(report); issue == nil || issue.Code != "STALE_GENERATION" {
		t.Fatalf("stale report issue=%+v", issue)
	}
}
