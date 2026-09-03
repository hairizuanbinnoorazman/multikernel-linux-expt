package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type fake struct {
	mu      sync.Mutex
	states  map[string]string
	calls   []string
	fail    string
	failErr error
	onCall  func()
}

func (f *fake) call(n, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, n+":"+id)
	if f.onCall != nil {
		f.onCall()
	}
	if f.fail == n {
		if f.failErr != nil {
			return f.failErr
		}
		return errors.New("injected")
	}
	return nil
}

func TestBackendTimeoutHasStableOperationID(t *testing.T) {
	s, st, backend := setup(t)
	defer st.Close()
	backend.fail = "create"
	backend.failErr = context.DeadlineExceeded
	_, apiErr := s.Create(context.Background(), config("box-a", 8, 7001), "timeout-create")
	if apiErr == nil || apiErr.Code != "BACKEND_TIMEOUT" {
		t.Fatalf("Create() error = %+v, want BACKEND_TIMEOUT", apiErr)
	}
	if len(apiErr.OperationID) != 32 {
		t.Fatalf("operation ID = %q, want 32 hex characters", apiErr.OperationID)
	}
	stored, ok := s.Get("box-a")
	if !ok || stored.Error == nil || stored.Error.OperationID != apiErr.OperationID {
		t.Fatalf("stored error = %+v, want operation ID %q", stored.Error, apiErr.OperationID)
	}
	entries, err := st.JournalEntries()
	if err != nil {
		t.Fatal(err)
	}
	last := entries[len(entries)-1]
	if last.Phase != "complete" || last.Error == nil || last.Error.OperationID != apiErr.OperationID {
		t.Fatalf("completion entry = %+v", last)
	}
}

func TestBackendErrorsDoNotExposeSecretsOrPaths(t *testing.T) {
	s, st, backend := setup(t)
	defer st.Close()
	backend.fail = "create"
	backend.failErr = errors.New("token=super-secret /private/host/path OCI_PASSWORD=hunter2")
	_, apiErr := s.Create(context.Background(), config("box-a", 8, 7001), "safe-error")
	if apiErr == nil || apiErr.Code != "BACKEND_FAILURE" {
		t.Fatalf("Create() error = %+v, want BACKEND_FAILURE", apiErr)
	}
	for _, secret := range []string{"super-secret", "/private/host/path", "hunter2"} {
		if strings.Contains(apiErr.Message, secret) {
			t.Fatalf("API error exposed %q: %+v", secret, apiErr)
		}
	}
	if apiErr.Message != "backend operation failed" {
		t.Fatalf("API error message = %q", apiErr.Message)
	}
}

func TestFailurePathStoreErrorIsPropagated(t *testing.T) {
	s, st, backend := setup(t)
	backend.fail = "create"
	backend.onCall = func() { _ = st.Close() }
	_, apiErr := s.Create(context.Background(), config("box-a", 8, 7001), "closed-store")
	if apiErr == nil || apiErr.Code != "INTERNAL" || len(apiErr.OperationID) != 32 {
		t.Fatalf("Create() error = %+v, want operation-scoped INTERNAL", apiErr)
	}
}
func (f *fake) EnsurePool(c context.Context) error { return f.call("pool", "") }
func (f *fake) Observe(c context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if x, ok := f.states[id]; ok {
		return x, nil
	}
	return "ABSENT", nil
}
func (f *fake) Create(c context.Context, s protocol.Sandbox) error {
	if e := f.call("create", s.ID); e != nil {
		return e
	}
	f.mu.Lock()
	f.states[s.ID] = "CREATED"
	f.mu.Unlock()
	return nil
}
func (f *fake) Load(c context.Context, s protocol.Sandbox, a, b, d string) error {
	if e := f.call("load", s.ID); e != nil {
		return e
	}
	f.mu.Lock()
	f.states[s.ID] = "LOADED"
	f.mu.Unlock()
	return nil
}
func (f *fake) Start(c context.Context, s protocol.Sandbox) error {
	if e := f.call("start", s.ID); e != nil {
		return e
	}
	f.mu.Lock()
	f.states[s.ID] = "RUNNING"
	f.mu.Unlock()
	return nil
}
func (f *fake) Stop(c context.Context, s protocol.Sandbox) error {
	if e := f.call("stop", s.ID); e != nil {
		return e
	}
	f.mu.Lock()
	f.states[s.ID] = "STOPPED"
	f.mu.Unlock()
	return nil
}
func (f *fake) Delete(c context.Context, s protocol.Sandbox) error {
	if e := f.call("delete", s.ID); e != nil {
		return e
	}
	f.mu.Lock()
	delete(f.states, s.ID)
	f.mu.Unlock()
	return nil
}
func (f *fake) ReleasePool(c context.Context) error { return f.call("release", "") }
func setup(t *testing.T) (*Service, *state.Store, *fake) {
	t.Helper()
	st, e := state.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	f := &fake{states: map[string]string{}}
	return New(st, f, Artifacts{}), st, f
}
func config(id string, cpu, port int) protocol.SandboxConfig {
	return protocol.SandboxConfig{SchemaVersion: 1, ID: id, CPUs: []int{cpu}, MemoryBytes: 1 << 30, KernelManifest: "test", Bundle: "/bundle/" + id, AgentPort: uint32(port), ChildCID: uint32(port - 7000)}
}
func TestLifecycleAndReplay(t *testing.T) {
	s, st, f := setup(t)
	defer st.Close()
	ctx := context.Background()
	r, e := s.Create(ctx, config("box-a", 8, 7001), "c1")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create(ctx, config("box-a", 8, 7001), "c1")
	if e != nil || !again.Replayed {
		t.Fatalf("replay: %+v %+v", again, e)
	}
	if _, e = s.Load(ctx, r.Sandbox.ID, r.Sandbox.Generation, "l1"); e != nil {
		t.Fatal(e)
	}
	if replay, replayErr := s.Load(ctx, r.Sandbox.ID, r.Sandbox.Generation, "l1"); replayErr != nil || !replay.Replayed {
		t.Fatalf("load replay: %+v %+v", replay, replayErr)
	}
	if retry, retryErr := s.Load(ctx, r.Sandbox.ID, r.Sandbox.Generation, "l2"); retryErr != nil || retry.Replayed || retry.Sandbox.State != "LOADED" {
		t.Fatalf("load terminal retry: %+v %+v", retry, retryErr)
	}
	if _, e = s.Start(ctx, r.Sandbox.ID, r.Sandbox.Generation, "s1"); e != nil {
		t.Fatal(e)
	}
	if replay, replayErr := s.Start(ctx, r.Sandbox.ID, r.Sandbox.Generation, "s1"); replayErr != nil || !replay.Replayed {
		t.Fatalf("start replay: %+v %+v", replay, replayErr)
	}
	if retry, retryErr := s.Start(ctx, r.Sandbox.ID, r.Sandbox.Generation, "s2"); retryErr != nil || retry.Replayed || retry.Sandbox.State != "RUNNING" {
		t.Fatalf("start terminal retry: %+v %+v", retry, retryErr)
	}
	if _, e = s.Stop(ctx, r.Sandbox.ID, r.Sandbox.Generation, "x1"); e != nil {
		t.Fatal(e)
	}
	if replay, replayErr := s.Stop(ctx, r.Sandbox.ID, r.Sandbox.Generation, "x1"); replayErr != nil || !replay.Replayed {
		t.Fatalf("stop replay: %+v %+v", replay, replayErr)
	}
	if retry, retryErr := s.Stop(ctx, r.Sandbox.ID, r.Sandbox.Generation, "x2"); retryErr != nil || retry.Replayed || retry.Sandbox.State != "STOPPED" {
		t.Fatalf("stop terminal retry: %+v %+v", retry, retryErr)
	}
	if _, e = s.Delete(ctx, r.Sandbox.ID, r.Sandbox.Generation, "d1"); e != nil {
		t.Fatal(e)
	}
	if replay, replayErr := s.Delete(ctx, r.Sandbox.ID, r.Sandbox.Generation, "d1"); replayErr != nil || !replay.Replayed || replay.Sandbox.State != "ABSENT" {
		t.Fatalf("delete replay: %+v %+v", replay, replayErr)
	}
	if _, newKeyErr := s.Delete(ctx, r.Sandbox.ID, r.Sandbox.Generation, "d2"); newKeyErr == nil || newKeyErr.Code != "NOT_FOUND" {
		t.Fatalf("new-key absent delete: %+v", newKeyErr)
	}
	if len(f.states) != 0 {
		t.Fatal("backend leaked state")
	}
	wantCalls := map[string]int{"load:box-a": 1, "start:box-a": 1, "stop:box-a": 1, "delete:box-a": 1}
	for _, call := range f.calls {
		if _, ok := wantCalls[call]; ok {
			wantCalls[call]--
		}
	}
	for call, remaining := range wantCalls {
		if remaining != 0 {
			t.Fatalf("backend call %s count mismatch: remaining=%d calls=%v", call, remaining, f.calls)
		}
	}
}
func TestStaleAndOverlap(t *testing.T) {
	s, st, _ := setup(t)
	defer st.Close()
	r, e := s.Create(context.Background(), config("box-a", 8, 7001), "a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Load(context.Background(), "box-a", "old", "b"); e == nil || e.Code != "STALE_GENERATION" {
		t.Fatalf("bad stale error: %+v", e)
	}
	if _, e = s.Create(context.Background(), config("box-b", 8, 7002), "c"); e == nil || e.Code != "RESOURCE_EXHAUSTED" {
		t.Fatalf("overlap accepted: %+v", e)
	}
	_ = r
}

func TestCreateInputValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*protocol.SandboxConfig)
		key    string
	}{
		{"manifest", func(c *protocol.SandboxConfig) { c.KernelManifest = "Bad Manifest" }, "key"},
		{"bundle", func(c *protocol.SandboxConfig) { c.Bundle = "relative/bundle" }, "key"},
		{"label key", func(c *protocol.SandboxConfig) { c.Labels = map[string]string{"Bad Label": "x"} }, "key"},
		{"label value", func(c *protocol.SandboxConfig) { c.Labels = map[string]string{"app": strings.Repeat("x", 257)} }, "key"},
		{"idempotency empty", func(*protocol.SandboxConfig) {}, ""},
		{"idempotency long", func(*protocol.SandboxConfig) {}, strings.Repeat("x", 129)},
		{"idempotency control", func(*protocol.SandboxConfig) {}, "bad\nkey"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, st, _ := setup(t)
			defer st.Close()
			candidate := config("box-a", 8, 7001)
			test.mutate(&candidate)
			if _, apiErr := s.Create(context.Background(), candidate, test.key); apiErr == nil || apiErr.Code != "INVALID_ARGUMENT" {
				t.Fatalf("Create() error = %+v, want INVALID_ARGUMENT", apiErr)
			}
		})
	}
}

func TestSandboxCPUsMustBelongToConfiguredPool(t *testing.T) {
	s, st, _ := setup(t)
	defer st.Close()
	s.SetPoolCPUs([]int{8, 10})
	if _, apiErr := s.Create(context.Background(), config("box-a", 12, 7001), "outside-pool"); apiErr == nil || apiErr.Code != "INVALID_ARGUMENT" {
		t.Fatalf("Create() error = %+v, want outside-pool INVALID_ARGUMENT", apiErr)
	}
	if _, apiErr := s.Create(context.Background(), config("box-b", 10, 7002), "inside-pool"); apiErr != nil {
		t.Fatalf("inside-pool Create() error = %+v", apiErr)
	}
}
func TestSandboxMemoryMustFitUsablePool(t *testing.T) {
	s, st, backend := setup(t)
	defer st.Close()
	s.SetPoolMemory(8_000_000_000, 1_000_000_000)
	first := config("box-a", 8, 7001)
	first.MemoryBytes = 4 << 30
	if _, apiErr := s.Create(context.Background(), first, "first"); apiErr != nil {
		t.Fatalf("first Create() error = %+v", apiErr)
	}
	second := config("box-b", 10, 7002)
	second.MemoryBytes = 4 << 30
	if _, apiErr := s.Create(context.Background(), second, "second"); apiErr == nil || apiErr.Code != "RESOURCE_EXHAUSTED" {
		t.Fatalf("second Create() error = %+v, want RESOURCE_EXHAUSTED", apiErr)
	}
	for _, call := range backend.calls {
		if call == "create:box-b" {
			t.Fatal("memory-exhausted sandbox reached backend")
		}
	}
}

func TestAbsentSandboxDoesNotConsumeUsablePoolMemory(t *testing.T) {
	s, st, _ := setup(t)
	defer st.Close()
	s.SetPoolMemory(3<<30, 1<<30)
	first := config("box-a", 8, 7001)
	r, apiErr := s.Create(context.Background(), first, "first")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, apiErr = s.Delete(context.Background(), r.Sandbox.ID, r.Sandbox.Generation, "delete-first"); apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, apiErr = s.Create(context.Background(), config("box-b", 10, 7002), "second"); apiErr != nil {
		t.Fatalf("memory was not reclaimed after delete: %+v", apiErr)
	}
}
func TestConcurrentAllocation(t *testing.T) {
	s, st, _ := setup(t)
	defer st.Close()
	var wg sync.WaitGroup
	results := make(chan *protocol.Error, 2)
	for i, id := range []string{"box-a", "box-b"} {
		wg.Add(1)
		go func(id string, k int) {
			defer wg.Done()
			_, e := s.Create(context.Background(), config(id, 8, 7001+k), id)
			results <- e
		}(id, i)
	}
	wg.Wait()
	close(results)
	ok, fail := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else {
			fail++
		}
	}
	if ok != 1 || fail != 1 {
		t.Fatalf("ok=%d fail=%d", ok, fail)
	}
}

func TestSecondSandboxDoesNotReinitializePool(t *testing.T) {
	s, st, f := setup(t)
	defer st.Close()
	if _, e := s.Create(context.Background(), config("box-a", 8, 7001), "a"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Create(context.Background(), config("box-b", 10, 7002), "b"); e != nil {
		t.Fatal(e)
	}
	poolCalls := 0
	for _, call := range f.calls {
		if call == "pool:" {
			poolCalls++
		}
	}
	if poolCalls != 1 {
		t.Fatalf("pool initialized %d times", poolCalls)
	}
}
func TestRestartReconciliation(t *testing.T) {
	dir := t.TempDir()
	st, e := state.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	f := &fake{states: map[string]string{}}
	s := New(st, f, Artifacts{})
	r, pe := s.Create(context.Background(), config("box-a", 8, 7001), "a")
	if pe != nil {
		t.Fatal(pe)
	}
	st.Close()
	f.states[r.Sandbox.ID] = "RUNNING"
	st2, e := state.Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer st2.Close()
	s2 := New(st2, f, Artifacts{})
	if e = s2.Reconcile(context.Background()); e != nil {
		t.Fatal(e)
	}
	x, _ := s2.Get("box-a")
	if x.Error == nil || x.Error.Code != "OPERATOR_ACTION" {
		t.Fatalf("missing reconciliation error: %+v", x)
	}
}
