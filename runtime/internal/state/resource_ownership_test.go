package state

import (
	"encoding/json"
	"fmt"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
	"time"
)

func resourceSandbox() protocol.Sandbox {
	return protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32), State: "RUNNING", Config: protocol.SandboxConfig{ID: "box", CPUs: []int{9, 8}, MemoryBytes: 3 << 30, ChildCID: 40, AgentPort: 7200, Bundle: "/PRIVATE_BUNDLE", Labels: map[string]string{"secret": "PRIVATE_LABEL"}, Storage: &protocol.StorageConfig{Path: "/PRIVATE_PATH/root.ext4", ImageID: "PRIVATE_IMAGE", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("d", 64)}}, Storage: &protocol.StorageStatus{ExportGeneration: strings.Repeat("b", 32), State: "ACTIVE"}}
}
func TestResourceLifecycleProjectionPrivateBoundAndReadOnly(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sb := resourceSandbox()
	if err = store.SetSandbox(sb); err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	r, f := store.ResourceOwnershipSnapshot()
	if f != nil || r.ValidateLifecycle() != nil || len(r.StorageBindings) != 1 {
		t.Fatalf("%+v %v", r, f)
	}
	if !r.StorageBindings[0].Required || r.StorageBindings[0].DescriptorSHA256 != protocol.StorageDescriptorSHA256(*sb.Config.Storage) || r.StorageBindings[0].ExportGeneration != sb.Storage.ExportGeneration {
		t.Fatal("lost binding")
	}
	out, _ := json.Marshal(r)
	for _, secret := range []string{"PRIVATE_BUNDLE", "PRIVATE_PATH", "PRIVATE_IMAGE", "PRIVATE_LABEL", strings.Repeat("d", 64)} {
		if strings.Contains(string(out), secret) {
			t.Fatal("private input projected")
		}
	}
	if !reflect.DeepEqual(before, store.Snapshot()) {
		t.Fatal("inspection mutated state")
	}
	sb.Config.Storage = nil
	sb.Storage = nil
	if err = store.SetSandbox(sb); err != nil {
		t.Fatal(err)
	}
	r, f = store.ResourceOwnershipSnapshot()
	if f != nil || r.ValidateLifecycle() != nil || r.StorageBindings[0].Required || r.StorageBindings[0].DescriptorSHA256 != "" {
		t.Fatal("ordinary sandbox made storage-required")
	}
	store.mu.Lock()
	done := make(chan *protocol.Error, 1)
	go func() { _, f := store.ResourceOwnershipSnapshot(); done <- f }()
	select {
	case f := <-done:
		if f == nil || f.Code != "UNAVAILABLE" {
			store.mu.Unlock()
			t.Fatal("busy snapshot accepted")
		}
	case <-time.After(time.Second):
		store.mu.Unlock()
		t.Fatal("snapshot blocked on persistence")
	}
	store.mu.Unlock()
}
func TestResourceLifecycleProjectionRefusesMalformedOrOversize(t *testing.T) {
	for _, mode := range []string{"limit", "cpus", "key", "binding"} {
		t.Run(mode, func(t *testing.T) {
			store, e := Open(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			sb := resourceSandbox()
			want := "OPERATOR_ACTION"
			switch mode {
			case "limit":
				want = "RESOURCE_EXHAUSTED"
				for i := 0; i < 257; i++ {
					store.data.Sandboxes[fmt.Sprint(i)] = protocol.Sandbox{}
				}
			case "cpus":
				want = "RESOURCE_EXHAUSTED"
				sb.Config.CPUs = make([]int, 4097)
				store.data.Sandboxes[sb.ID] = sb
			case "key":
				store.data.Sandboxes["PRIVATE_SECRET"] = sb
			case "binding":
				sb.Config.Storage = nil
				store.data.Sandboxes[sb.ID] = sb
			}
			r, f := store.ResourceOwnershipSnapshot()
			if f == nil || f.Code != want || r.Lifecycle.Sandboxes != nil || strings.Contains(f.Message, "PRIVATE_SECRET") {
				t.Fatal("partial or reflected failure")
			}
		})
	}
}
