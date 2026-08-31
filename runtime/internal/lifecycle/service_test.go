package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type fake struct {
	mu     sync.Mutex
	states map[string]string
	calls  []string
	fail   string
}

func (f *fake) call(n, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, n+":"+id)
	if f.fail == n {
		return errors.New("injected")
	}
	return nil
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
	if _, e = s.Start(ctx, r.Sandbox.ID, r.Sandbox.Generation, "s1"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Stop(ctx, r.Sandbox.ID, r.Sandbox.Generation, "x1"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Delete(ctx, r.Sandbox.ID, r.Sandbox.Generation, "d1"); e != nil {
		t.Fatal(e)
	}
	if len(f.states) != 0 {
		t.Fatal("backend leaked state")
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
