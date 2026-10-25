package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"strings"
	"testing"
)

func resourceInput() protocol.ResourceOwnershipReport {
	owner := sandbox("box-a", "RUNNING")
	gen := strings.Repeat("b", 32)
	hash := strings.Repeat("c", 64)
	return protocol.ResourceOwnershipReport{Version: 1, Source: "durable-lifecycle-storage", Lifecycle: protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: 4, Sandboxes: []protocol.SandboxOwnership{owner}}, StorageBindings: []protocol.LifecycleStorageBinding{{SandboxID: owner.SandboxID, Generation: owner.Generation, Required: true, ExportGeneration: gen, DescriptorSHA256: hash}}, Storage: protocol.StorageOwnershipReport{Version: 1, Source: "durable-storage", Exports: []protocol.StorageExportOwnership{{SandboxID: owner.SandboxID, SandboxGeneration: owner.Generation, ExportGeneration: gen, PeerCID: owner.ChildCID, State: "ACTIVE", FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, ImageDevice: 1, ImageInode: 2}}}, StorageDescriptors: []protocol.StorageDescriptorBinding{{SandboxID: owner.SandboxID, Generation: owner.Generation, ExportGeneration: gen, DescriptorSHA256: hash}}}
}
func resourceInputs(record protocol.ResourceOwnershipReport, ids []string) ResourceInputs {
	return ResourceInputs{Resources: func(context.Context) (protocol.ResourceOwnershipReport, error) { return record, nil }, Kernel: func(context.Context) ([]string, error) { return ids, nil }, Boot: func() (string, error) { return boot, nil }}
}
func TestResourceDoctorCorrelationStatesAndHistory(t *testing.T) {
	for _, mode := range []string{"matching", "ordinary", "history", "missing", "orphan", "generation", "cid", "descriptor", "export-generation", "binding-missing", "unexpected", "released-owned", "released-owner", "preparing", "quiescing", "allocating"} {
		t.Run(mode, func(t *testing.T) {
			r := resourceInput()
			ids := []string{"box-a"}
			wantStatus, wantCode := "degraded", ""
			lease := &r.Storage.Exports[0]
			switch mode {
			case "matching":
				wantStatus = "consistent"
			case "ordinary":
				r.Storage.Exports = []protocol.StorageExportOwnership{}
				r.StorageDescriptors = []protocol.StorageDescriptorBinding{}
				r.StorageBindings[0].Required = false
				r.StorageBindings[0].ExportGeneration = ""
				r.StorageBindings[0].DescriptorSHA256 = ""
				wantStatus = "consistent"
			case "history":
				lease.State = "RELEASED"
				lease.PeerCID = 0
				lease.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("d", 64)
				r.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				r.StorageBindings = []protocol.LifecycleStorageBinding{}
				ids = []string{}
				wantStatus = "consistent"
			case "missing":
				r.Storage.Exports = []protocol.StorageExportOwnership{}
				r.StorageDescriptors = []protocol.StorageDescriptorBinding{}
				wantCode = "STORAGE_LEASE_MISSING"
			case "orphan":
				r.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				r.StorageBindings = []protocol.LifecycleStorageBinding{}
				ids = []string{}
				wantCode = "STORAGE_ORPHAN_CANDIDATE"
			case "generation":
				lease.SandboxGeneration = strings.Repeat("f", 32)
				r.StorageDescriptors[0].Generation = lease.SandboxGeneration
				wantCode = "STORAGE_GENERATION_MISMATCH"
			case "cid":
				lease.PeerCID++
				wantCode = "STORAGE_PEER_MISMATCH"
			case "descriptor":
				r.StorageDescriptors[0].DescriptorSHA256 = strings.Repeat("e", 64)
				wantCode = "STORAGE_DESCRIPTOR_MISMATCH"
			case "export-generation":
				r.StorageBindings[0].ExportGeneration = strings.Repeat("f", 32)
				wantCode = "STORAGE_EXPORT_GENERATION_MISMATCH"
			case "binding-missing":
				r.StorageBindings[0].ExportGeneration = ""
				wantCode = "STORAGE_BINDING_MISSING"
			case "unexpected":
				r.StorageBindings[0].Required = false
				r.StorageBindings[0].ExportGeneration = ""
				r.StorageBindings[0].DescriptorSHA256 = ""
				wantCode = "STORAGE_UNEXPECTED_LEASE"
			case "released-owned":
				lease.State = "RELEASED"
				lease.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("d", 64)
				wantCode = "STORAGE_RELEASED_WHILE_OWNED"
			case "released-owner":
				r.Lifecycle.Sandboxes[0].State = "ABSENT"
				ids = []string{}
				wantCode = "STORAGE_AFTER_OWNER_RELEASE"
			case "preparing", "quiescing":
				lease.State = strings.ToUpper(mode)
				wantStatus = "inconclusive"
				wantCode = "STORAGE_TRANSITION_REQUIRES_RECHECK"
			case "allocating":
				r.Lifecycle.Sandboxes[0].State = "ALLOCATING"
				r.StorageBindings[0].ExportGeneration = ""
				r.Storage.Exports = []protocol.StorageExportOwnership{}
				r.StorageDescriptors = []protocol.StorageDescriptorBinding{}
				ids = []string{}
				wantStatus = "inconclusive"
				wantCode = "TRANSITION_REQUIRES_RECHECK"
			}
			if r.Validate() != nil {
				t.Fatal("fixture malformed")
			}
			out := InspectResources(context.Background(), resourceInputs(r, ids), "test-revision")
			if out.Version != 2 || out.Status != wantStatus || out.Resources == nil || out.KernelGenerationVerified || out.Unobserved[1] != "native-storage-health" {
				t.Fatalf("%+v", out)
			}
			if wantCode != "" && (len(out.Findings) == 0 || out.Findings[0].Code != wantCode) {
				t.Fatalf("%+v", out)
			}
			if mode == "generation" && out.Findings[0].ExpectedGeneration != r.Lifecycle.Sandboxes[0].Generation {
				t.Fatal("missing expected generation")
			}
		})
	}
}
func TestResourceDoctorUnavailableDriftAndNoPartialFallback(t *testing.T) {
	for _, mode := range []string{"unsupported", "unavailable", "malformed", "storage-drift", "binding-drift", "kernel", "boot"} {
		t.Run(mode, func(t *testing.T) {
			r := resourceInput()
			in := resourceInputs(r, []string{"box-a"})
			calls := 0
			switch mode {
			case "unsupported", "unavailable", "malformed":
				in.Resources = func(context.Context) (protocol.ResourceOwnershipReport, error) {
					if mode == "malformed" {
						return protocol.ResourceOwnershipReport{}, nil
					}
					if mode == "unsupported" {
						return protocol.ResourceOwnershipReport{}, errors.Join(daemon.ErrResourceOwnershipUnsupported, errors.New("PRIVATE_SECRET"))
					}
					return protocol.ResourceOwnershipReport{}, errors.New("PRIVATE_SECRET")
				}
			case "storage-drift", "binding-drift":
				in.Resources = func(context.Context) (protocol.ResourceOwnershipReport, error) {
					calls++
					r := resourceInput()
					if calls == 2 {
						if mode == "storage-drift" {
							r.Storage.Exports[0].ImageInode++
						} else {
							r.StorageBindings[0].DescriptorSHA256 = strings.Repeat("e", 64)
						}
					}
					return r, nil
				}
			case "kernel":
				in.Kernel = func(context.Context) ([]string, error) { return nil, errors.New("PRIVATE_SECRET") }
			case "boot":
				in.Boot = func() (string, error) {
					calls++
					if calls == 1 {
						return boot, nil
					}
					return "changed", nil
				}
			}
			out := InspectResources(context.Background(), in, "test-revision")
			raw, _ := json.Marshal(out)
			if out.Status != "inconclusive" || out.Ownership != nil || out.Resources != nil || out.KernelInstances != nil || strings.Contains(string(raw), "PRIVATE_SECRET") {
				t.Fatalf("%s", raw)
			}
			if mode == "unsupported" && out.Findings[0].Code != "RESOURCE_OWNERSHIP_UNSUPPORTED" {
				t.Fatalf("%+v", out)
			}
		})
	}
}
