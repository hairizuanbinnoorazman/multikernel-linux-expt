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
	mu                sync.Mutex
	adds              []Endpoint
	checks            []Endpoint
	deletes           []Endpoint
	failAdd           error
	failCheck         error
	failDelete        error
	addHook           func()
	namespacesCreated []string
	namespacesDeleted []string
}

func (f *fakeBackend) Add(_ context.Context, endpoint Endpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds = append(f.adds, endpoint)
	if f.addHook != nil {
		f.addHook()
	}
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
func (f *fakeBackend) CreateNamespace(_ context.Context, generation string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := "/run/netns/mk-" + generation[:12]
	f.namespacesCreated = append(f.namespacesCreated, path)
	return path, nil
}
func (f *fakeBackend) DeleteNamespace(_ context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.namespacesDeleted = append(f.namespacesDeleted, path)
	return nil
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

func TestFinalAddStateFailureRollsBackDurableAllocatingRecord(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	backend.addHook = func() {
		journaled, ok := s.Store.Get("multikernel", "failed", "eth0")
		if !ok || journaled.State != "ALLOCATING" || journaled.Generation == "" {
			t.Fatalf("pre-mutation allocation record = %+v, %v", journaled, ok)
		}
		s.Store.persistFault = func() error {
			s.Store.persistFault = nil
			return errors.New("injected final state persistence failure")
		}
	}
	if _, issue := s.Add(context.Background(), endpoint("failed")); issue == nil || !strings.Contains(issue.Message, "persistence") {
		t.Fatalf("final state issue = %+v", issue)
	}
	if len(backend.adds) != 1 || len(backend.deletes) != 1 || len(s.List()) != 0 {
		t.Fatalf("final state rollback adds=%d deletes=%d state=%+v", len(backend.adds), len(backend.deletes), s.List())
	}
}

func TestReconcileRemovesInterruptedAllocatingGeneration(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	value := endpoint("interrupted")
	value.Owner, value.Generation, value.Address, value.Gateway, value.MTU, value.State =
		"cni", "0123456789abcdef0123456789abcdef", "172.31.0.2/30", "172.31.0.1", 1400, "ALLOCATING"
	if err := s.Store.Put(value); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(backend.deletes) != 1 || len(backend.checks) != 0 || len(s.List()) != 0 {
		t.Fatalf("reconcile deletes=%d checks=%d state=%+v", len(backend.deletes), len(backend.checks), s.List())
	}
}

func TestDeleteJournalsTransitionAndReconcileCompletesFailure(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	allocated, issue := s.Add(context.Background(), endpoint("deleting"))
	if issue != nil {
		t.Fatal(issue)
	}
	backend.failDelete = errors.New("injected endpoint delete failure")
	if issue = s.Delete(context.Background(), allocated); issue == nil {
		t.Fatal("endpoint delete failure was accepted")
	}
	journaled, ok := s.Store.Get(allocated.NetworkName, allocated.ContainerID, allocated.IfName)
	if !ok || journaled.State != "DELETING" {
		t.Fatalf("failed deletion record = %+v, %v", journaled, ok)
	}
	backend.failDelete = nil
	backend.deletes = nil
	if err := s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(backend.deletes) != 1 || len(s.List()) != 0 {
		t.Fatalf("deletion reconcile deletes=%d state=%+v", len(backend.deletes), s.List())
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

func TestRuntimeProvisionOwnsLifecycleWhileExternalCNIRetainsIt(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/29", backend)
	sandboxGeneration := "11111111111111111111111111111111"
	runtimeEndpoint, issue := s.Provision(context.Background(), Endpoint{ContainerID: "runtime-one", NetworkName: "multikernel", IfName: "mktun0", SandboxID: "sandbox-one", SandboxGeneration: sandboxGeneration})
	if issue != nil || runtimeEndpoint.Owner != "runtime" || !runtimeEndpoint.ManagedNamespace || len(backend.namespacesCreated) != 1 {
		t.Fatalf("runtime endpoint=%+v issue=%v namespaces=%v", runtimeEndpoint, issue, backend.namespacesCreated)
	}
	if issue = s.Release(context.Background(), Endpoint{SandboxID: "sandbox-one", SandboxGeneration: sandboxGeneration}); issue != nil || len(backend.namespacesDeleted) != 1 || len(s.List()) != 0 {
		t.Fatalf("runtime release issue=%v namespace deletes=%v endpoints=%v", issue, backend.namespacesDeleted, s.List())
	}
	external, issue := s.Add(context.Background(), endpoint("external"))
	if issue != nil {
		t.Fatal(issue)
	}
	bound, issue := s.Provision(context.Background(), Endpoint{ContainerID: external.ContainerID, NetworkName: external.NetworkName, IfName: external.IfName, NetNS: external.NetNS, SandboxID: "sandbox-two", SandboxGeneration: sandboxGeneration})
	if issue != nil || bound.Owner != "cni" {
		t.Fatalf("external bind=%+v issue=%v", bound, issue)
	}
	if issue = s.Release(context.Background(), Endpoint{SandboxID: "sandbox-two", SandboxGeneration: sandboxGeneration}); issue != nil || len(s.List()) != 1 {
		t.Fatalf("external release issue=%v endpoints=%v", issue, s.List())
	}
}

func TestRuntimeReleaseRetainsSandboxIdentityUntilRetryCompletes(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	sandboxGeneration := "11111111111111111111111111111111"
	allocated, issue := s.Provision(context.Background(), Endpoint{ContainerID: "runtime", NetworkName: "multikernel", IfName: "mktun0", SandboxID: "sandbox", SandboxGeneration: sandboxGeneration})
	if issue != nil {
		t.Fatal(issue)
	}
	backend.failDelete = errors.New("injected runtime release failure")
	request := Endpoint{SandboxID: "sandbox", SandboxGeneration: sandboxGeneration}
	if issue = s.Release(context.Background(), request); issue == nil {
		t.Fatal("runtime release failure was accepted")
	}
	retained, ok := s.Store.Get(allocated.NetworkName, allocated.ContainerID, allocated.IfName)
	if !ok || retained.State != "DELETING" || retained.SandboxID != "sandbox" || retained.SandboxGeneration != sandboxGeneration {
		t.Fatalf("failed runtime release record = %+v, %v", retained, ok)
	}
	backend.failDelete = nil
	if issue = s.Release(context.Background(), request); issue != nil {
		t.Fatal(issue)
	}
	if len(s.List()) != 0 {
		t.Fatalf("retried runtime release retained state: %+v", s.List())
	}
}
