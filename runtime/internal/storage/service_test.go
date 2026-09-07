package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type fakeBackend struct {
	active   map[string]string
	fail     map[string]error
	calls    []string
	counters Counters
	closed   bool
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{active: map[string]string{}, fail: map[string]error{}}
}
func (f *fakeBackend) Inspect(context.Context, PreparedImage) error {
	f.calls = append(f.calls, "inspect")
	return f.fail["inspect"]
}
func (f *fakeBackend) Start(_ context.Context, value Export) error {
	f.calls = append(f.calls, "start")
	if err := f.fail["start"]; err != nil {
		return err
	}
	f.active[value.Path] = value.ExportGeneration
	if err := f.fail["start-after-active"]; err != nil {
		return err
	}
	return nil
}
func (f *fakeBackend) Observe(_ context.Context, value Export) (Observation, error) {
	f.calls = append(f.calls, "observe")
	if err := f.fail["observe"]; err != nil {
		return Observation{}, err
	}
	generation, active := f.active[value.Path]
	return Observation{Active: active, Closed: !active && f.closed, Generation: generation, Counters: f.counters}, nil
}
func (f *fakeBackend) Stop(_ context.Context, value Export) (Counters, error) {
	f.calls = append(f.calls, "stop")
	if err := f.fail["stop"]; err != nil {
		return Counters{}, err
	}
	delete(f.active, value.Path)
	f.closed = true
	return f.counters, nil
}
func (f *fakeBackend) OfflineCheck(context.Context, Export) (string, error) {
	f.calls = append(f.calls, "check")
	return "e2fsck-clean-sha256:" + strings.Repeat("c", 64), f.fail["check"]
}

func fixture(t *testing.T) (*Service, *Store, *fakeBackend, PreparedImage) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := newFakeBackend()
	image := PreparedImage{Path: filepath.Join(t.TempDir(), "root.ext4"), ImageID: "busybox-root",
		FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
		QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("a", 64)}
	return NewService(store, backend), store, backend, image
}

const sandboxGeneration = "11111111222233334444555555555555"

func TestProvisionIsGenerationBoundIdempotentAndSingleOwner(t *testing.T) {
	service, _, backend, image := fixture(t)
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
	if err != nil {
		t.Fatal(err)
	}
	if value.State != "ACTIVE" || value.ExportGeneration == "" || backend.active[image.Path] != value.ExportGeneration {
		t.Fatalf("provisioned export = %+v active=%v", value, backend.active)
	}
	replayed, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
	if err != nil || replayed.ExportGeneration != value.ExportGeneration {
		t.Fatalf("idempotent provision = %+v, %v", replayed, err)
	}
	other := image
	other.ImageID = "other"
	if _, err = service.Provision(context.Background(), "box-a", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", other); err == nil {
		t.Fatal("same sandbox obtained two writable exports")
	}
	other = image
	other.Path = filepath.Join(t.TempDir(), "other.ext4")
	if _, err = service.Provision(context.Background(), "box-b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", other); err == nil {
		t.Fatal("duplicate port/UUID was accepted")
	}
}

func TestReleaseRequiresExactGenerationAndOfflineCheck(t *testing.T) {
	service, _, backend, image := fixture(t)
	backend.counters = Counters{Reads: 2, Writes: 3, Flushes: 4}
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Release(context.Background(), "box-a", sandboxGeneration, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("stale export generation released storage")
	}
	released, err := service.Release(context.Background(), "box-a", sandboxGeneration, value.ExportGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if released.State != "RELEASED" || !offlineCheckRE.MatchString(released.OfflineCheck) || released.Counters.Flushes != 4 || !released.ReleasedAt.Equal(released.UpdatedAt) {
		t.Fatalf("released export = %+v", released)
	}
	if _, ok := backend.active[image.Path]; ok {
		t.Fatal("backend remained active")
	}
	if replayed, err := service.Release(context.Background(), "box-a", sandboxGeneration, value.ExportGeneration); err != nil || replayed.State != "RELEASED" {
		t.Fatalf("idempotent release = %+v, %v", replayed, err)
	}
}

func TestProvisionAndReleaseFailuresRemainFailClosed(t *testing.T) {
	for _, point := range []string{"inspect", "start"} {
		t.Run(point, func(t *testing.T) {
			service, store, backend, image := fixture(t)
			backend.fail[point] = errors.New("injected")
			if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image); err == nil {
				t.Fatal("injected provision failure was ignored")
			}
			retained := store.List()
			if point == "inspect" && len(retained) != 0 {
				t.Fatalf("inspection failure created ownership: %+v", retained)
			}
			if point == "start" && (len(retained) != 1 || retained[0].State != "PREPARING") {
				t.Fatalf("start failure did not retain preparation ownership: %+v", retained)
			}
			if len(backend.active) != 0 {
				t.Fatalf("partial export remained: state=%+v active=%v", store.List(), backend.active)
			}
		})
	}
	for _, point := range []string{"stop", "check"} {
		t.Run(point, func(t *testing.T) {
			service, store, backend, image := fixture(t)
			value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
			if err != nil {
				t.Fatal(err)
			}
			backend.fail[point] = errors.New("injected")
			if _, err = service.Release(context.Background(), "box-a", sandboxGeneration, value.ExportGeneration); err == nil {
				t.Fatal("injected release failure was ignored")
			}
			remaining, ok := store.Get("box-a", sandboxGeneration)
			if !ok || remaining.State != "QUIESCING" {
				t.Fatalf("failure was not retained for recovery: %+v", remaining)
			}
		})
	}
}

func TestReconcileRestartsRetainedPreparationWithExactGeneration(t *testing.T) {
	for _, retryMethod := range []string{"provision", "reconcile"} {
		for _, activeBeforeFailure := range []bool{false, true} {
			name := retryMethod + "-" + map[bool]string{false: "absent", true: "ambiguous-active"}[activeBeforeFailure]
			t.Run(name, func(t *testing.T) {
				service, store, backend, image := fixture(t)
				failurePoint := "start"
				if activeBeforeFailure {
					failurePoint = "start-after-active"
				}
				backend.fail[failurePoint] = errors.New("injected ambiguous start")
				if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image); err == nil {
					t.Fatal("injected start failure was ignored")
				}
				retained, ok := store.Get("box-a", sandboxGeneration)
				if !ok || retained.State != "PREPARING" || retained.ExportGeneration == "" {
					t.Fatalf("retained preparation = %+v, %v", retained, ok)
				}
				delete(backend.fail, failurePoint)
				if retryMethod == "provision" {
					if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image); err != nil {
						t.Fatal(err)
					}
				} else if err := service.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
				recovered, ok := store.Get("box-a", sandboxGeneration)
				if !ok || recovered.State != "ACTIVE" || recovered.ExportGeneration != retained.ExportGeneration ||
					backend.active[image.Path] != retained.ExportGeneration {
					t.Fatalf("recovered preparation = %+v active=%v", recovered, backend.active)
				}
				if activeBeforeFailure && !backend.closed {
					t.Fatal("ambiguous live export was not gracefully stopped before restart")
				}
			})
		}
	}
}

func TestRetainedPreparationRefusesConflictingLiveGeneration(t *testing.T) {
	service, store, backend, image := fixture(t)
	backend.fail["start"] = errors.New("injected start failure")
	if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image); err == nil {
		t.Fatal("injected start failure was ignored")
	}
	retained, ok := store.Get("box-a", sandboxGeneration)
	if !ok || retained.State != "PREPARING" {
		t.Fatalf("retained preparation = %+v, %v", retained, ok)
	}
	conflicting := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	backend.active[image.Path] = conflicting
	delete(backend.fail, "start")
	if _, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image); err == nil ||
		!strings.Contains(err.Error(), "conflicting generation") {
		t.Fatalf("conflicting preparation retry error = %v", err)
	}
	if backend.active[image.Path] != conflicting || backend.closed {
		t.Fatalf("conflicting process was mutated: active=%v closed=%v", backend.active, backend.closed)
	}
	if current, ok := store.Get("box-a", sandboxGeneration); !ok || current.State != "PREPARING" {
		t.Fatalf("conflict lost retained ownership: %+v, %v", current, ok)
	}
}

func TestReconcileRestartsOnlyAbsentExactActiveExport(t *testing.T) {
	service, store, backend, image := fixture(t)
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
	if err != nil {
		t.Fatal(err)
	}
	delete(backend.active, image.Path)
	if err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if backend.active[image.Path] != value.ExportGeneration {
		t.Fatalf("export was not restarted: %v", backend.active)
	}
	backend.active[image.Path] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err = service.Reconcile(context.Background()); err == nil {
		t.Fatal("conflicting active generation was accepted")
	}

	value.State = "QUIESCING"
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	if err = service.Reconcile(context.Background()); err == nil {
		t.Fatal("incomplete quiescence was guessed away")
	}
}

func TestReconcileCompletesExactQuiescingExport(t *testing.T) {
	for _, serverActive := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "already-stopped"}[serverActive], func(t *testing.T) {
			service, store, backend, image := fixture(t)
			backend.counters = Counters{Reads: 2, ReadBytes: 8192, Writes: 3, WrittenBytes: 12288, Flushes: 4}
			value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
			if err != nil {
				t.Fatal(err)
			}
			value.State = "QUIESCING"
			if err = store.Put(value); err != nil {
				t.Fatal(err)
			}
			if !serverActive {
				delete(backend.active, image.Path)
				backend.closed = true
			}
			if err = service.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			recovered, ok := store.Get("box-a", sandboxGeneration)
			if !ok || recovered.State != "RELEASED" || !offlineCheckRE.MatchString(recovered.OfflineCheck) || recovered.Counters != backend.counters {
				t.Fatalf("recovered export = %+v", recovered)
			}
			if _, active := backend.active[image.Path]; active {
				t.Fatal("recovered export remained active")
			}
		})
	}
}

func TestReconcileRetainsQuiescingExportWithoutGracefulCloseProof(t *testing.T) {
	service, store, backend, image := fixture(t)
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, image)
	if err != nil {
		t.Fatal(err)
	}
	value.State = "QUIESCING"
	if err = store.Put(value); err != nil {
		t.Fatal(err)
	}
	delete(backend.active, image.Path)
	if err = service.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "without a graceful close record") {
		t.Fatalf("reconcile error = %v", err)
	}
	retained, ok := store.Get("box-a", sandboxGeneration)
	if !ok || retained.State != "QUIESCING" {
		t.Fatalf("diagnosable quiescing state was lost: %+v", retained)
	}
}
