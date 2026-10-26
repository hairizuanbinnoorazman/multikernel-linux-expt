package lifecycle

import (
	"context"
	"fmt"
	storagepkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"path/filepath"
	"strings"
	"testing"
)

type unsafeStopStorage struct{ *lifecycleStorageBackend }

func (b unsafeStopStorage) Stop(context.Context, storagepkg.Export) (storagepkg.Counters, error) {
	b.calls = append(b.calls, "unsafe-stop")
	return storagepkg.Counters{}, fmt.Errorf("%w: PRIVATE_EXPORT_PATH", storagepkg.ErrUnsafeSession)
}
func TestUnsafeStorageDeleteIsNonRetryableAndIdempotentlyRetained(t *testing.T) {
	service, store, kernel := setup(t)
	defer store.Close()
	leases, err := storagepkg.OpenStore(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	backend := unsafeStopStorage{&lifecycleStorageBackend{active: map[string]string{}}}
	service.SetStorage(storagepkg.NewService(leases, backend))
	cfg := config("box-a", 8, 7001)
	cfg.Storage = &protocol.StorageConfig{Path: "/PRIVATE_EXPORT_PATH/root.ext4", ImageID: "root", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("b", 64)}
	created, failure := service.Create(t.Context(), cfg, "create-unsafe")
	if failure != nil {
		t.Fatal(failure)
	}
	id, gen := created.Sandbox.ID, created.Sandbox.Generation
	if _, failure = service.Load(t.Context(), id, gen, "load-unsafe"); failure != nil {
		t.Fatal(failure)
	}
	if _, failure = service.Start(t.Context(), id, gen, "start-unsafe"); failure != nil {
		t.Fatal(failure)
	}
	_, failure = service.Delete(t.Context(), id, gen, "delete-unsafe")
	if failure == nil || failure.Code != "OPERATOR_ACTION" || failure.Retryable || strings.Contains(failure.Message, "PRIVATE") {
		t.Fatalf("delete failure = %+v", failure)
	}
	operation := failure.OperationID
	current, ok := service.Get(id)
	if !ok || current.State != "RELEASING" || current.Error == nil || current.Error.Code != "OPERATOR_ACTION" || current.Error.Retryable {
		t.Fatalf("ownership lost: %+v", current)
	}
	lease, ok := leases.Get(id, gen)
	if !ok || lease.State != "QUIESCING" || lease.OfflineCheck != "" || !lease.ReleasedAt.IsZero() || lease.Counters != (storagepkg.Counters{}) {
		t.Fatalf("unsafe release falsely completed: %+v", lease)
	}
	calls := len(kernel.calls)
	storageCalls := len(backend.calls)
	_, failure = service.Delete(t.Context(), id, gen, "delete-unsafe")
	if failure == nil || failure.Code != "OPERATOR_ACTION" || failure.Retryable || failure.OperationID != operation || len(kernel.calls) != calls || len(backend.calls) != storageCalls {
		t.Fatalf("idempotent result=%+v operation=%q kernel=%d/%d storage=%d/%d", failure, operation, len(kernel.calls), calls, len(backend.calls), storageCalls)
	}
	for _, call := range backend.calls {
		if call == "offline-check" {
			t.Fatal("unsafe release checked or repaired the filesystem")
		}
	}
}
