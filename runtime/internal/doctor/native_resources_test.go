package doctor

import (
	"context"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"strings"
	"testing"
)

func nativeFixture() protocol.NativeResourceReport {
	r := resourceInput()
	v := r.Storage.Exports[0]
	return protocol.NativeResourceReport{Version: 1, Source: "durable-resources-and-native-process-identity", Resources: r, Processes: []protocol.StorageProcessIdentity{{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, Status: "process-matched"}}}
}
func TestNativeDoctorRequiresRepeatedMatchedIdentity(t *testing.T) {
	for _, mode := range []string{"matched", "missing", "absent", "unavailable", "transition", "history", "unsupported", "private-error", "invalid", "drift", "owner-mismatch", "boot-change"} {
		t.Run(mode, func(t *testing.T) {
			n := nativeFixture()
			wantStatus, wantCode := "consistent", ""
			calls := 0
			boots := 0
			ids := []string{"box-a"}
			switch mode {
			case "missing":
				n.Processes[0].Status = "record-missing"
				wantStatus = "degraded"
				wantCode = "NATIVE_STORAGE_RECORD_MISSING"
			case "absent":
				n.Processes[0].Status = "process-absent"
				wantStatus = "degraded"
				wantCode = "NATIVE_STORAGE_PROCESS_ABSENT"
			case "unavailable":
				n.Processes[0].Status = "unavailable"
				wantStatus = "inconclusive"
				wantCode = "NATIVE_STORAGE_IDENTITY_UNAVAILABLE"
			case "transition":
				n.Resources.Storage.Exports[0].State = "QUIESCING"
				n.Processes[0].Status = "process-absent"
				wantStatus = "inconclusive"
			case "history":
				n.Resources.Storage.Exports[0].State = "RELEASED"
				n.Resources.Storage.Exports[0].OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("a", 64)
				n.Processes[0].Status = "not-probed-released"
				n.Resources.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				n.Resources.StorageBindings = []protocol.LifecycleStorageBinding{}
				ids = []string{}
			case "unsupported":
				wantStatus = "inconclusive"
				wantCode = "NATIVE_RESOURCE_IDENTITY_UNSUPPORTED"
			case "private-error", "invalid":
				wantStatus = "inconclusive"
				wantCode = "NATIVE_RESOURCE_IDENTITY_UNAVAILABLE"
			case "drift":
				wantStatus = "inconclusive"
				wantCode = "OBSERVATION_CHANGED"
			case "owner-mismatch":
				n.Resources.StorageBindings[0].DescriptorSHA256 = strings.Repeat("f", 64)
				n.Processes[0].Status = "unavailable"
				wantStatus = "degraded"
				wantCode = "STORAGE_DESCRIPTOR_MISMATCH"
			case "boot-change":
				wantStatus = "inconclusive"
				wantCode = "BOOT_CHANGED"
			}
			in := NativeInputs{Native: func(context.Context) (protocol.NativeResourceReport, error) {
				calls++
				if mode == "unsupported" {
					return protocol.NativeResourceReport{}, daemon.ErrNativeResourcesUnsupported
				}
				if mode == "private-error" {
					return protocol.NativeResourceReport{}, errors.New("PRIVATE_SECRET")
				}
				if mode == "invalid" {
					n.Processes[0].Generation = strings.Repeat("e", 32)
				}
				out := n
				out.Processes = append([]protocol.StorageProcessIdentity{}, n.Processes...)
				if mode == "drift" && calls == 2 {
					out.Processes[0].Status = "process-absent"
				}
				return out, nil
			}, Kernel: func(context.Context) ([]string, error) { return ids, nil }, Boot: func() (string, error) {
				boots++
				if mode == "boot-change" && boots == 2 {
					return "00000000-0000-4000-8000-000000000000", nil
				}
				return boot, nil
			}}
			r := InspectNativeResources(context.Background(), in, "revision")
			if r.Version != 3 || r.Status != wantStatus {
				t.Fatalf("result = %+v", r)
			}
			if wantCode != "" {
				found := false
				for _, f := range r.Findings {
					if f.Code == wantCode {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing %s: %+v", wantCode, r)
				}
			}
			if mode == "drift" || mode == "unsupported" || mode == "private-error" || mode == "invalid" || mode == "boot-change" {
				if r.NativeProcesses != nil || r.Resources != nil || r.Ownership != nil {
					t.Fatal("partial observation retained")
				}
			} else if r.NativeProcesses == nil {
				t.Fatal("native observations omitted")
			}
		})
	}
}
