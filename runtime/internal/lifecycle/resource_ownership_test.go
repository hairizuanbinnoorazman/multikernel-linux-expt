package lifecycle

import (
	"encoding/json"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCombinedResourceProjectionBindsBothStoresWithoutBackend(t *testing.T) {
	life, e := state.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer life.Close()
	store, e := storage.OpenStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	svc := New(life, nil, Artifacts{})
	svc.SetStorage(storage.NewService(store, nil))
	sb := ownershipFixture("box")
	sb.Config.Storage = &protocol.StorageConfig{Path: "/PRIVATE_PATH/root.ext4", ImageID: "PRIVATE_IMAGE", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("d", 64)}
	sb.Storage = &protocol.StorageStatus{ExportGeneration: strings.Repeat("b", 32), State: "ACTIVE"}
	if e = life.SetSandbox(sb); e != nil {
		t.Fatal(e)
	}
	cfg := sb.Config.Storage
	now := time.Now().UTC()
	v := storage.Export{SandboxID: sb.ID, SandboxGeneration: sb.Generation, ExportGeneration: sb.Storage.ExportGeneration, PeerCID: sb.Config.ChildCID, State: "PREPARING", PreparedImage: storage.PreparedImage{Path: cfg.Path, ImageID: cfg.ImageID, FilesystemUUID: cfg.FilesystemUUID, SizeBytes: cfg.SizeBytes, QuotaBytes: cfg.QuotaBytes, InodeLimit: cfg.InodeLimit, Port: cfg.Port, SHA256: cfg.SHA256}, ImageIdentity: storage.ImageIdentity{Device: 1, Inode: 2}, CreatedAt: now, UpdatedAt: now}
	if e = store.Put(v); e != nil {
		t.Fatal(e)
	}
	v.State = "ACTIVE"
	if e = store.Put(v); e != nil {
		t.Fatal(e)
	}
	beforeLife, beforeStorage := life.Snapshot(), store.List()
	svc.global.Lock()
	defer svc.global.Unlock() // No mutation/global lock dependency.
	r, f := svc.ResourceOwnership()
	if f != nil || r.Validate() != nil {
		t.Fatalf("%+v %v", r, f)
	}
	if len(r.StorageBindings) != 1 || r.StorageBindings[0].DescriptorSHA256 != r.StorageDescriptors[0].DescriptorSHA256 || r.StorageBindings[0].ExportGeneration != v.ExportGeneration {
		t.Fatal("stores were not bound to identical descriptors")
	}
	raw, _ := json.Marshal(r)
	for _, secret := range []string{"PRIVATE_PATH", "PRIVATE_IMAGE", "synthetic-private-secret", strings.Repeat("d", 64)} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private input exposed")
		}
	}
	if !reflect.DeepEqual(beforeLife, life.Snapshot()) || !reflect.DeepEqual(beforeStorage, store.List()) {
		t.Fatal("projection mutated a store")
	}
	svc.SetStorage(nil)
	r, f = svc.ResourceOwnership()
	if f == nil || f.Code != "UNSUPPORTED" || r.Storage.Exports != nil {
		t.Fatal("unsupported service returned partial success")
	}
}
