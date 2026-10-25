package storage

import (
	"encoding/json"
	"fmt"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStorageOwnershipProjectionNoBackendOrPersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"PREPARING", "ACTIVE", "QUIESCING", "RELEASED"} {
		v := validStoredExport(fmt.Sprintf("/PRIVATE_PATH_SECRET/root-%d.ext4", i))
		v.SandboxID = fmt.Sprintf("box-%d", i)
		v.SandboxGeneration = fmt.Sprintf("%032x", i+1)
		v.ExportGeneration = fmt.Sprintf("%032x", i+8)
		v.PeerCID = uint32(i + 40)
		v.Port = uint32(4000 + i)
		v.FilesystemUUID = fmt.Sprintf("11111111-2222-4333-8444-%012x", i)
		v.ImageID = "PRIVATE_IMAGE_SECRET"
		v.SHA256 = strings.Repeat("d", 64)
		for _, next := range []string{"PREPARING", "ACTIVE", "QUIESCING", "RELEASED"} {
			v.State = next
			if next == "RELEASED" {
				v.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("c", 64)
				v.ReleasedAt = v.UpdatedAt
				v.Counters.Writes = 17
			}
			if err = store.Put(v); err != nil {
				t.Fatal(err)
			}
			if next == state {
				break
			}
		}
	}
	before := store.List()
	bytesBefore, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, nil) // Any backend call would panic.
	svc.mu.Lock()
	defer svc.mu.Unlock() // Long operations cannot block inspection.
	report, failure := svc.Ownership()
	if failure != nil || report.Validate() != nil || len(report.Exports) != 4 {
		t.Fatalf("%+v %v", report, failure)
	}
	if report.Exports[3].Counters.Writes != 17 || report.Exports[0].PeerCID != 40 || report.Exports[0].ImageDevice != 1 {
		t.Fatalf("lost metadata: %+v", report)
	}
	out, _ := json.Marshal(report)
	for _, secret := range []string{"PRIVATE_PATH_SECRET", "PRIVATE_IMAGE_SECRET", strings.Repeat("d", 64), `"path"`, `"sha256"`, `"image_id"`} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("private input projected: %s", secret)
		}
	}
	bytesAfter, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil || string(bytesBefore) != string(bytesAfter) || !reflect.DeepEqual(before, store.List()) {
		t.Fatal("inspection mutated state")
	}
}
func TestStorageOwnershipBusyLimitAndMalformedStateRefuseWithoutPartialOutput(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(store, nil)
	store.mu.Lock()
	done := make(chan *protocol.Error, 1)
	go func() {
		r, f := svc.Ownership()
		if r.Exports != nil {
			t.Error("partial busy report")
		}
		done <- f
	}()
	select {
	case f := <-done:
		if f == nil || f.Code != "UNAVAILABLE" || !f.Retryable {
			t.Fatalf("busy result: %v", f)
		}
	case <-time.After(time.Second):
		store.mu.Unlock()
		t.Fatal("inspection waited for persistence lock")
	}
	store.mu.Unlock()
	for i := 0; i < 257; i++ {
		store.data.Exports[fmt.Sprint(i)] = Export{}
	}
	r, f := svc.Ownership()
	if f == nil || f.Code != "RESOURCE_EXHAUSTED" || r.Exports != nil {
		t.Fatal("limit returned partial or scanned malformed records")
	}
	store.data.Exports = map[string]Export{"PRIVATE_PATH_SECRET": validStoredExport("/PRIVATE_PATH_SECRET/root.ext4")}
	r, f = svc.Ownership()
	if f == nil || f.Code != "OPERATOR_ACTION" || r.Exports != nil || strings.Contains(f.Message, "PRIVATE_PATH_SECRET") {
		t.Fatal("malformed storage accepted or reflected")
	}
}
