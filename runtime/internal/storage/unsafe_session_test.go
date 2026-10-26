package storage

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestLostAcceptedPreparationNeverInspectsOrRestarts(t *testing.T) {
	for _, operation := range []string{"provision", "reconcile"} {
		t.Run(operation, func(t *testing.T) {
			service, store, backend, image := fixture(t)
			backend.fail["start"] = errors.New("ambiguous start")
			if _, err := service.Provision(t.Context(), "box-a", sandboxGeneration, 1, image); err == nil {
				t.Fatal("start failure missing")
			}
			delete(backend.fail, "start")
			backend.clientLost = true
			backend.calls = nil
			before := store.List()
			var err error
			if operation == "provision" {
				_, err = service.Provision(t.Context(), "box-a", sandboxGeneration, 1, image)
			} else {
				err = service.Reconcile(t.Context(), fixtureOwners(store))
			}
			if !errors.Is(err, ErrUnsafeSession) {
				t.Fatalf("unsafe accepted preparation error = %v", err)
			}
			if !reflect.DeepEqual(backend.calls, []string{"observe"}) || !reflect.DeepEqual(before, store.List()) {
				t.Fatal("lost accepted preparation inspected, restarted or changed ownership")
			}
		})
	}
}

func TestUnsafeReleaseRetainsQuiescingEvidence(t *testing.T) {
	service, store, backend, image := fixture(t)
	value, err := service.Provision(context.Background(), "box-a", sandboxGeneration, 1, image)
	if err != nil {
		t.Fatal(err)
	}
	backend.fail["stop"] = ErrUnsafeSession
	backend.calls = nil
	_, err = service.Release(t.Context(), value.SandboxID, value.SandboxGeneration, value.ExportGeneration, value.PeerCID)
	if !errors.Is(err, ErrUnsafeSession) {
		t.Fatalf("release = %v", err)
	}
	current, ok := store.Get(value.SandboxID, value.SandboxGeneration)
	if !ok || current.State != "QUIESCING" || current.OfflineCheck != "" || !current.ReleasedAt.IsZero() || current.Counters != (Counters{}) {
		t.Fatalf("unsafe release published clean evidence: %+v", current)
	}
	if !reflect.DeepEqual(backend.calls, []string{"stop"}) {
		t.Fatalf("offline inspection followed unsafe stop: %v", backend.calls)
	}
}
