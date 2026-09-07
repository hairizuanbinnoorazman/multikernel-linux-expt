package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	storagepkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type fake struct {
	mu         sync.Mutex
	states     map[string]string
	calls      []string
	fail       string
	failErr    error
	failures   map[string]error
	onCall     func()
	observeErr error
}

type failingResolver struct{ calls int }

type lifecycleStorageBackend struct {
	active map[string]string
	fail   error
	calls  []string
}

func (b *lifecycleStorageBackend) Inspect(context.Context, storagepkg.PreparedImage) error {
	b.calls = append(b.calls, "inspect")
	return b.fail
}
func (b *lifecycleStorageBackend) Start(_ context.Context, value storagepkg.Export) error {
	b.calls = append(b.calls, "storage-start")
	if b.fail != nil {
		return b.fail
	}
	b.active[value.Path] = value.ExportGeneration
	return nil
}
func (b *lifecycleStorageBackend) Observe(_ context.Context, value storagepkg.Export) (storagepkg.Observation, error) {
	generation, ok := b.active[value.Path]
	return storagepkg.Observation{Active: ok, Generation: generation}, nil
}
func (b *lifecycleStorageBackend) Stop(_ context.Context, value storagepkg.Export) (storagepkg.Counters, error) {
	b.calls = append(b.calls, "storage-stop")
	delete(b.active, value.Path)
	return storagepkg.Counters{Writes: 2, Flushes: 1}, nil
}
func (b *lifecycleStorageBackend) OfflineCheck(context.Context, storagepkg.Export) (string, error) {
	b.calls = append(b.calls, "offline-check")
	return "clean", nil
}

func (r *failingResolver) Resolve(string) (Artifacts, error) {
	r.calls++
	return Artifacts{}, errors.New("invalid manifest")
}

func TestCrashInjectionAtEveryOperationBoundary(t *testing.T) {
	points := []string{"before-intent", "after-intent", "before-external-mutation", "after-external-mutation", "after-observation", "before-snapshot", "after-snapshot", "before-completion", "after-completion"}
	methods := []struct {
		name, initial, terminal string
		invoke                  func(*Service, protocol.Sandbox) *protocol.Error
	}{
		{"create", "ABSENT", "CREATED", func(service *Service, _ protocol.Sandbox) *protocol.Error {
			_, err := service.Create(context.Background(), config("box-a", 8, 7001), "create")
			return err
		}},
		{"load", "CREATED", "LOADED", func(service *Service, sandbox protocol.Sandbox) *protocol.Error {
			_, err := service.Load(context.Background(), sandbox.ID, sandbox.Generation, "load")
			return err
		}},
		{"start", "LOADED", "RUNNING", func(service *Service, sandbox protocol.Sandbox) *protocol.Error {
			_, err := service.Start(context.Background(), sandbox.ID, sandbox.Generation, "start")
			return err
		}},
		{"stop", "RUNNING", "STOPPED", func(service *Service, sandbox protocol.Sandbox) *protocol.Error {
			_, err := service.Stop(context.Background(), sandbox.ID, sandbox.Generation, "stop")
			return err
		}},
		{"delete", "STOPPED", "ABSENT", func(service *Service, sandbox protocol.Sandbox) *protocol.Error {
			_, err := service.Delete(context.Background(), sandbox.ID, sandbox.Generation, "delete")
			return err
		}},
	}
	for _, method := range methods {
		for _, point := range points {
			t.Run(method.name+"/"+point, func(t *testing.T) {
				store, err := state.Open(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				backend := &fake{states: map[string]string{}}
				sandbox := protocol.Sandbox{ID: "box-a", Generation: "0123456789abcdef0123456789abcdef", State: method.initial, Config: config("box-a", 8, 7001)}
				if method.initial != "ABSENT" {
					if err = store.SetSandbox(sandbox); err != nil {
						t.Fatal(err)
					}
					backend.states[sandbox.ID] = method.initial
				}
				service := New(store, backend, Artifacts{})
				injected := false
				service.SetFaultInjector(func(observed string) error {
					if observed == point && !injected {
						injected = true
						return errors.New("injected crash")
					}
					return nil
				})
				if apiErr := method.invoke(service, sandbox); apiErr == nil || apiErr.Code != "INTERNAL" {
					t.Fatalf("injected operation error = %+v", apiErr)
				}
				if !injected {
					t.Fatalf("checkpoint %s was not reached", point)
				}
				service.SetFaultInjector(nil)
				if point == "before-intent" {
					if incomplete := state.Incomplete(mustJournal(t, store)); len(incomplete) != 0 {
						t.Fatalf("unexpected intent: %+v", incomplete)
					}
					return
				}
				if err = service.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
				recovered, exists := service.Get("box-a")
				if method.terminal == "ABSENT" {
					if exists {
						t.Fatalf("deleted sandbox remains: %+v", recovered)
					}
				} else if !exists || recovered.State != method.terminal || recovered.Error != nil {
					t.Fatalf("recovered = %+v, %v; want %s", recovered, exists, method.terminal)
				}
				if incomplete := state.Incomplete(mustJournal(t, store)); len(incomplete) != 0 {
					t.Fatalf("incomplete intents remain: %+v", incomplete)
				}
			})
		}
	}
}

func TestCancelAmbiguousCreateAtEveryBoundary(t *testing.T) {
	points := []string{"before-intent", "after-intent", "before-external-mutation", "after-external-mutation", "after-observation", "before-snapshot", "after-snapshot", "before-completion", "after-completion"}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			service, store, backend := setup(t)
			defer store.Close()
			candidate := config("box-a", 8, 7001)
			injected := false
			service.SetFaultInjector(func(observed string) error {
				if observed == point && !injected {
					injected = true
					return errors.New("injected lost create response")
				}
				return nil
			})
			if _, apiErr := service.Create(context.Background(), candidate, "create-key"); apiErr == nil {
				t.Fatal("ambiguous create unexpectedly succeeded")
			}
			service.SetFaultInjector(nil)
			safe, apiErr := service.CancelCreate(context.Background(), candidate, "create-key")
			if apiErr != nil || !safe {
				t.Fatalf("CancelCreate() = %v, %+v", safe, apiErr)
			}
			if _, exists := service.Get(candidate.ID); exists {
				t.Fatal("canceled sandbox remains in durable state")
			}
			if actual, err := backend.Observe(context.Background(), candidate.ID); err != nil || actual != "ABSENT" {
				t.Fatalf("canceled backend state = %q, %v", actual, err)
			}
			if incomplete := state.Incomplete(mustJournal(t, store)); len(incomplete) != 0 {
				t.Fatalf("canceled create intent remains incomplete: %+v", incomplete)
			}
			if _, replayErr := service.Create(context.Background(), candidate, "create-key"); replayErr == nil || replayErr.Code != "ABORTED" || replayErr.Retryable {
				t.Fatalf("delayed create replay = %+v, want terminal ABORTED", replayErr)
			}
			safe, apiErr = service.CancelCreate(context.Background(), candidate, "create-key")
			if apiErr != nil || !safe {
				t.Fatalf("repeated CancelCreate() = %v, %+v", safe, apiErr)
			}
		})
	}
}

func TestCancelCreateRefusesDifferentOrProgressedOwner(t *testing.T) {
	service, store, _ := setup(t)
	defer store.Close()
	candidate := config("box-a", 8, 7001)
	created, apiErr := service.Create(context.Background(), candidate, "create-key")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	different := candidate
	different.CPUs = []int{10}
	if safe, cancelErr := service.CancelCreate(context.Background(), different, "create-key"); cancelErr == nil || cancelErr.Code != "IDEMPOTENCY_CONFLICT" || safe {
		t.Fatalf("different cancellation = %v, %+v", safe, cancelErr)
	}
	if _, apiErr = service.Load(context.Background(), candidate.ID, created.Sandbox.Generation, "load-key"); apiErr != nil {
		t.Fatal(apiErr)
	}
	if safe, cancelErr := service.CancelCreate(context.Background(), candidate, "create-key"); cancelErr == nil || cancelErr.Code != "FAILED_PRECONDITION" || safe {
		t.Fatalf("progressed cancellation = %v, %+v", safe, cancelErr)
	}
	if sandbox, exists := service.Get(candidate.ID); !exists || sandbox.State != "LOADED" {
		t.Fatalf("progressed sandbox was changed: %+v, %v", sandbox, exists)
	}
}

func TestCancelCreateTombstonesBeforeExternalCleanup(t *testing.T) {
	service, store, backend := setup(t)
	defer store.Close()
	candidate := config("box-a", 8, 7001)
	if _, apiErr := service.Create(context.Background(), candidate, "create-key"); apiErr != nil {
		t.Fatal(apiErr)
	}
	backend.observeErr = errors.New("injected observation outage")
	if safe, cancelErr := service.CancelCreate(context.Background(), candidate, "create-key"); cancelErr == nil || cancelErr.Code != "BACKEND_FAILURE" || safe {
		t.Fatalf("interrupted cancellation = %v, %+v", safe, cancelErr)
	}
	if _, replayErr := service.Create(context.Background(), candidate, "create-key"); replayErr == nil || replayErr.Code != "ABORTED" {
		t.Fatalf("create replay before cleanup = %+v, want ABORTED", replayErr)
	}
	if _, exists := service.Get(candidate.ID); !exists {
		t.Fatal("ownership disappeared before external cleanup was proven")
	}
	backend.observeErr = nil
	if safe, cancelErr := service.CancelCreate(context.Background(), candidate, "create-key"); cancelErr != nil || !safe {
		t.Fatalf("retried cancellation = %v, %+v", safe, cancelErr)
	}
	if _, exists := service.Get(candidate.ID); exists {
		t.Fatal("canceled ownership survived successful retry")
	}
}

func TestManifestIsResolvedBeforeBackendMutation(t *testing.T) {
	service, store, backend := setup(t)
	defer store.Close()
	resolver := &failingResolver{}
	service.SetArtifactResolver(resolver)
	if _, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "create"); apiErr == nil || apiErr.Code != "FAILED_PRECONDITION" {
		t.Fatalf("Create() error = %+v", apiErr)
	}
	if resolver.calls != 1 || len(backend.calls) != 0 {
		t.Fatalf("resolver calls=%d backend calls=%v", resolver.calls, backend.calls)
	}
}

func TestBackendFailuresRestoreObservedRetryableState(t *testing.T) {
	service, store, backend := setup(t)
	defer store.Close()
	created, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "create")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	sandbox := created.Sandbox
	for _, test := range []struct {
		name        string
		backendCall string
		state       string
		invoke      func(string) *protocol.Error
	}{
		{"load", "load", "CREATED", func(key string) *protocol.Error {
			_, err := service.Load(context.Background(), sandbox.ID, sandbox.Generation, key)
			return err
		}},
		{"start", "start", "LOADED", func(key string) *protocol.Error {
			_, err := service.Start(context.Background(), sandbox.ID, sandbox.Generation, key)
			return err
		}},
		{"stop", "stop", "RUNNING", func(key string) *protocol.Error {
			_, err := service.Stop(context.Background(), sandbox.ID, sandbox.Generation, key)
			return err
		}},
	} {
		backend.fail = test.backendCall
		backend.failErr = errors.New("injected backend failure")
		if err := test.invoke(test.name + "-failure"); err == nil || err.Code != "BACKEND_FAILURE" {
			t.Fatalf("%s error = %+v", test.name, err)
		}
		got, ok := service.Get(sandbox.ID)
		if !ok || got.State != test.state || got.Error == nil {
			t.Fatalf("%s failed state = %+v, exists=%v; want %s with error", test.name, got, ok, test.state)
		}
		backend.fail = ""
		backend.failErr = nil
		if err := test.invoke(test.name + "-retry"); err != nil {
			t.Fatalf("%s retry: %+v", test.name, err)
		}
		sandbox, _ = service.Get(sandbox.ID)
	}
}

func TestFirstCreateFailureReleasesNewPool(t *testing.T) {
	service, store, backend := setup(t)
	defer store.Close()
	backend.fail = "create"
	if _, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "create-key"); apiErr == nil || apiErr.Code != "BACKEND_FAILURE" {
		t.Fatalf("Create() error = %+v", apiErr)
	}
	if !reflect.DeepEqual(backend.calls, []string{"pool:", "create:box-a", "release:"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
	if _, exists := service.Get("box-a"); exists {
		t.Fatal("failed first create retained sandbox ownership")
	}
}

func TestCancelCreateRetriesUncertainFirstPoolRelease(t *testing.T) {
	service, store, backend := setup(t)
	defer store.Close()
	candidate := config("box-a", 8, 7001)
	backend.failures = map[string]error{
		"create":  errors.New("injected create failure"),
		"release": errors.New("injected pool release failure"),
	}
	if _, apiErr := service.Create(context.Background(), candidate, "create-key"); apiErr == nil || apiErr.Code != "BACKEND_FAILURE" {
		t.Fatalf("Create() error = %+v", apiErr)
	}
	delete(backend.failures, "release")
	if safe, apiErr := service.CancelCreate(context.Background(), candidate, "create-key"); apiErr != nil || !safe {
		t.Fatalf("CancelCreate() = %v, %+v", safe, apiErr)
	}
	if !reflect.DeepEqual(backend.calls, []string{"pool:", "create:box-a", "release:", "release:"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
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
	if err := f.failures[n]; err != nil {
		return err
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
	if stored, ok := s.Get("box-a"); ok {
		t.Fatalf("failed uncommitted create left a blocking sandbox record: %+v", stored)
	}
	entries, err := st.JournalEntries()
	if err != nil {
		t.Fatal(err)
	}
	last := entries[len(entries)-1]
	if last.Phase != "complete" || last.Error == nil || last.Error.OperationID != apiErr.OperationID {
		t.Fatalf("completion entry = %+v", last)
	}
	backend.fail = ""
	backend.failErr = nil
	if result, retryErr := s.Create(context.Background(), config("box-a", 8, 7001), "retry-create"); retryErr != nil || result.Sandbox.State != "CREATED" {
		t.Fatalf("Create() retry = %+v, %+v", result, retryErr)
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
	if f.observeErr != nil {
		return "", f.observeErr
	}
	if x, ok := f.states[id]; ok {
		return x, nil
	}
	return "ABSENT", nil
}
func (f *fake) ListInstances(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	instances := make([]string, 0, len(f.states))
	for id := range f.states {
		instances = append(instances, id)
	}
	return instances, nil
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

func TestStorageOwnershipParticipatesInCreateDeleteAndRollback(t *testing.T) {
	newService := func(t *testing.T) (*Service, *state.Store, *fake, *lifecycleStorageBackend) {
		t.Helper()
		service, lifecycleStore, kerfBackend := setup(t)
		storageStore, err := storagepkg.OpenStore(filepath.Join(t.TempDir(), "storage"))
		if err != nil {
			t.Fatal(err)
		}
		storageBackend := &lifecycleStorageBackend{active: map[string]string{}}
		service.SetStorage(storagepkg.NewService(storageStore, storageBackend))
		return service, lifecycleStore, kerfBackend, storageBackend
	}
	withStorage := func() protocol.SandboxConfig {
		value := config("box-a", 8, 7001)
		value.Storage = &protocol.StorageConfig{Path: "/srv/storage/box-a.ext4", ImageID: "busybox-root",
			FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
			QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("b", 64)}
		return value
	}

	t.Run("lifecycle", func(t *testing.T) {
		service, lifecycleStore, _, storageBackend := newService(t)
		defer lifecycleStore.Close()
		created, apiErr := service.Create(context.Background(), withStorage(), "create-storage")
		if apiErr != nil {
			t.Fatal(apiErr)
		}
		if created.Sandbox.Storage == nil || created.Sandbox.Storage.State != "ACTIVE" || created.Sandbox.Storage.ExportGeneration == "" {
			t.Fatalf("created storage = %+v", created.Sandbox.Storage)
		}
		if _, apiErr = service.Load(context.Background(), created.Sandbox.ID, created.Sandbox.Generation, "load-storage"); apiErr != nil {
			t.Fatal(apiErr)
		}
		if _, apiErr = service.Start(context.Background(), created.Sandbox.ID, created.Sandbox.Generation, "start-storage"); apiErr != nil {
			t.Fatal(apiErr)
		}
		deleted, apiErr := service.Delete(context.Background(), created.Sandbox.ID, created.Sandbox.Generation, "delete-storage")
		if apiErr != nil {
			t.Fatal(apiErr)
		}
		if deleted.Sandbox.Storage == nil || deleted.Sandbox.Storage.State != "RELEASED" || deleted.Sandbox.Storage.OfflineCheck != "clean" || deleted.Sandbox.Storage.Writes != 2 {
			t.Fatalf("deleted storage = %+v", deleted.Sandbox.Storage)
		}
		if strings.Join(storageBackend.calls, ",") != "inspect,storage-start,storage-stop,offline-check" {
			t.Fatalf("storage call order = %v", storageBackend.calls)
		}
	})

	t.Run("missing", func(t *testing.T) {
		service, lifecycleStore, _, _ := newService(t)
		defer lifecycleStore.Close()
		if _, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "missing-storage"); apiErr == nil || apiErr.Code != "INVALID_ARGUMENT" {
			t.Fatalf("missing storage error = %+v", apiErr)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		service, lifecycleStore, kerfBackend, storageBackend := newService(t)
		defer lifecycleStore.Close()
		storageBackend.fail = errors.New("injected storage inspection failure")
		if _, apiErr := service.Create(context.Background(), withStorage(), "failed-storage"); apiErr == nil || apiErr.Code != "BACKEND_FAILURE" {
			t.Fatalf("storage failure = %+v", apiErr)
		}
		if len(kerfBackend.states) != 0 {
			t.Fatalf("Kerf allocation leaked: %v", kerfBackend.states)
		}
		if got := strings.Join(kerfBackend.calls, ","); !strings.Contains(got, "create:box-a,delete:box-a,release:") {
			t.Fatalf("rollback calls = %v", kerfBackend.calls)
		}
	})
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

func TestVersionedResumableEvents(t *testing.T) {
	service, store, _ := setup(t)
	defer store.Close()
	if _, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "create"); apiErr != nil {
		t.Fatal(apiErr)
	}
	events, apiErr := service.Events(0, 1)
	if apiErr != nil || len(events) != 1 || events[0].Version != 1 || events[0].Method != "CreateSandbox" || events[0].State != "CREATED" {
		t.Fatalf("events = %+v, error = %+v", events, apiErr)
	}
	resumed, apiErr := service.Events(events[0].Sequence, 128)
	if apiErr != nil || len(resumed) != 0 {
		t.Fatalf("resumed events = %+v, error = %+v", resumed, apiErr)
	}
	if _, apiErr = service.Events(0, 1025); apiErr == nil || apiErr.Code != "INVALID_ARGUMENT" {
		t.Fatalf("oversized event limit error = %+v", apiErr)
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

func TestIntermediateStatesAreDurableAndObservable(t *testing.T) {
	t.Run("allocating", func(t *testing.T) {
		s, st, backend := setup(t)
		defer st.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		backend.onCall = func() { once.Do(func() { close(entered); <-release }) }
		done := make(chan *protocol.Error, 1)
		go func() {
			_, apiErr := s.Create(context.Background(), config("box-a", 8, 7001), "create")
			done <- apiErr
		}()
		<-entered
		if sandbox, ok := s.Get("box-a"); !ok || sandbox.State != "ALLOCATING" {
			t.Fatalf("state during create = %+v, %v", sandbox, ok)
		}
		close(release)
		if apiErr := <-done; apiErr != nil {
			t.Fatal(apiErr)
		}
	})

	t.Run("stopping-and-releasing", func(t *testing.T) {
		s, st, backend := setup(t)
		defer st.Close()
		result, apiErr := s.Create(context.Background(), config("box-a", 8, 7001), "create")
		if apiErr != nil {
			t.Fatal(apiErr)
		}
		if _, apiErr = s.Load(context.Background(), "box-a", result.Sandbox.Generation, "load"); apiErr != nil {
			t.Fatal(apiErr)
		}
		if _, apiErr = s.Start(context.Background(), "box-a", result.Sandbox.Generation, "start"); apiErr != nil {
			t.Fatal(apiErr)
		}
		for _, transition := range []struct {
			name, state string
			call        func() *protocol.Error
		}{
			{"stop", "STOPPING", func() *protocol.Error {
				_, err := s.Stop(context.Background(), "box-a", result.Sandbox.Generation, "stop")
				return err
			}},
			{"delete", "RELEASING", func() *protocol.Error {
				_, err := s.Delete(context.Background(), "box-a", result.Sandbox.Generation, "delete")
				return err
			}},
		} {
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			backend.onCall = func() { once.Do(func() { close(entered); <-release }) }
			done := make(chan *protocol.Error, 1)
			go func() { done <- transition.call() }()
			<-entered
			if sandbox, ok := s.Get("box-a"); !ok || sandbox.State != transition.state {
				t.Fatalf("state during %s = %+v, %v", transition.name, sandbox, ok)
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			backend.onCall = nil
		}
	})
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

func TestReconcileCreateIntentBeforeSnapshot(t *testing.T) {
	st, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	backend := &fake{states: map[string]string{}}
	sandbox := protocol.Sandbox{ID: "box-a", Generation: "0123456789abcdef0123456789abcdef", State: "ALLOCATING", Config: config("box-a", 8, 7001)}
	intent := state.JournalEntry{OperationID: "operation", IdempotencyKey: "create", Fingerprint: "fingerprint", SandboxID: sandbox.ID, Generation: sandbox.Generation, Method: "CreateSandbox", Phase: "intent", State: sandbox.State, Sandbox: &sandbox}
	if err = st.Append(intent); err != nil {
		t.Fatal(err)
	}
	service := New(st, backend, Artifacts{})
	if err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, ok := service.Get("box-a")
	if !ok || recovered.State != "CREATED" || recovered.Error != nil {
		t.Fatalf("recovered sandbox = %+v, %v", recovered, ok)
	}
	result, ok := st.Result("create")
	if !ok || result.Result.Sandbox.State != "CREATED" {
		t.Fatalf("recovered replay result = %+v, %v", result, ok)
	}
	if incomplete := state.Incomplete(mustJournal(t, st)); len(incomplete) != 0 {
		t.Fatalf("incomplete intents remain: %+v", incomplete)
	}
}

func TestReconcileResumesEveryIncompleteTransition(t *testing.T) {
	tests := []struct {
		method, durable, actual, want string
	}{
		{"LoadSandbox", "CREATED", "CREATED", "LOADED"},
		{"StartSandbox", "LOADED", "LOADED", "RUNNING"},
		{"StopSandbox", "RUNNING", "RUNNING", "STOPPED"},
		{"DeleteSandbox", "STOPPED", "STOPPED", "ABSENT"},
	}
	for _, test := range tests {
		t.Run(test.method, func(t *testing.T) {
			st, err := state.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			sandbox := protocol.Sandbox{ID: "box-a", Generation: "0123456789abcdef0123456789abcdef", State: test.durable, Config: config("box-a", 8, 7001)}
			if err = st.SetSandbox(sandbox); err != nil {
				t.Fatal(err)
			}
			intent := state.JournalEntry{OperationID: "operation", IdempotencyKey: test.method, Fingerprint: "fingerprint", SandboxID: sandbox.ID, Generation: sandbox.Generation, Method: test.method, Phase: "intent", State: sandbox.State, Sandbox: &sandbox}
			if err = st.Append(intent); err != nil {
				t.Fatal(err)
			}
			backend := &fake{states: map[string]string{"box-a": test.actual}}
			service := New(st, backend, Artifacts{})
			if err = service.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			recovered, exists := service.Get("box-a")
			if test.want == "ABSENT" {
				if exists {
					t.Fatalf("deleted sandbox remains: %+v", recovered)
				}
			} else if !exists || recovered.State != test.want || recovered.Error != nil {
				t.Fatalf("recovered sandbox = %+v, %v; want %s", recovered, exists, test.want)
			}
			result, ok := st.Result(test.method)
			if !ok || result.Result.Sandbox.State != test.want {
				t.Fatalf("replay result = %+v, %v; want %s", result, ok, test.want)
			}
		})
	}
}

func TestReconcileRejectsUnknownBackendInstance(t *testing.T) {
	service, st, backend := setup(t)
	defer st.Close()
	backend.states["operator-owned"] = "RUNNING"
	if err := service.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "OPERATOR_ACTION") {
		t.Fatalf("Reconcile() error = %v, want OPERATOR_ACTION", err)
	}
}

func TestStoppedMapsToKerfLoadedOnRestart(t *testing.T) {
	service, st, backend := setup(t)
	defer st.Close()
	result, apiErr := service.Create(context.Background(), config("box-a", 8, 7001), "create")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, apiErr = service.Load(context.Background(), "box-a", result.Sandbox.Generation, "load"); apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, apiErr = service.Start(context.Background(), "box-a", result.Sandbox.Generation, "start"); apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, apiErr = service.Stop(context.Background(), "box-a", result.Sandbox.Generation, "stop"); apiErr != nil {
		t.Fatal(apiErr)
	}
	backend.states["box-a"] = "LOADED"
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sandbox, _ := service.Get("box-a"); sandbox.Error != nil || sandbox.State != "STOPPED" {
		t.Fatalf("stopped sandbox reconciliation = %+v", sandbox)
	}
}

func mustJournal(t *testing.T, store *state.Store) []state.JournalEntry {
	t.Helper()
	entries, err := store.JournalEntries()
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
