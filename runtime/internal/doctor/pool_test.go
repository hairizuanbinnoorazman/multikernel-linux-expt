package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/poolinspect"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
)

func poolFixture() (protocol.PoolOwnershipReport, poolinspect.Snapshot) {
	owner := protocol.PoolOwnershipReport{Version: 1, Source: "configured-pool-and-durable-lifecycle", Lifecycle: protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: 1, Sandboxes: []protocol.SandboxOwnership{{SandboxID: "box-a", Generation: strings.Repeat("a", 32), State: "CREATED", CPUs: []int{8, 9}, MemoryBytes: 3 << 30, ChildCID: 40, AgentPort: 7200}}}, ConfiguredCPUs: []int{8, 9, 10, 11}, ConfiguredMemoryBytes: 8 << 30, MemoryReserveBytes: 1 << 30, Initialized: true}
	kernel := poolinspect.Snapshot{CPUs: poolinspect.CPUs{Members: []uint64{8, 9, 10, 11}, Available: []uint64{10, 11}}, Memory: poolinspect.Memory{Chunks: 1, TotalBytes: 8 << 30, AllocatedBytes: 3 << 30, AvailableBytes: 5 << 30, LargestFreeExtentBytes: 5 << 30}}
	return owner, kernel
}
func poolInputs(owner protocol.PoolOwnershipReport, kernel poolinspect.Snapshot) PoolInputs {
	return PoolInputs{Ownership: func(context.Context) (protocol.PoolOwnershipReport, error) { return owner, nil }, Kernel: func(context.Context) (poolinspect.Snapshot, error) { return kernel, nil }, Boot: func() (string, error) { return boot, nil }}
}
func TestPoolDoctorCorrelatesCapacityReserveAndFragmentation(t *testing.T) {
	for _, mode := range []string{"allocated", "empty", "fragmented", "absent-history"} {
		t.Run(mode, func(t *testing.T) {
			owner, kernel := poolFixture()
			wantMemory := uint64(4 << 30)
			wantCPUs := []uint64{10, 11}
			switch mode {
			case "empty", "absent-history":
				if mode == "empty" {
					owner.Lifecycle.Sandboxes = []protocol.SandboxOwnership{}
				} else {
					owner.Lifecycle.Sandboxes[0].State = "ABSENT"
				}
				kernel.CPUs.Available = kernel.CPUs.Members
				kernel.Memory.AllocatedBytes = 0
				kernel.Memory.AvailableBytes = 8 << 30
				kernel.Memory.LargestFreeExtentBytes = 8 << 30
				wantMemory = 7 << 30
				wantCPUs = kernel.CPUs.Members
			case "fragmented":
				kernel.Memory.LargestFreeExtentBytes = 2 << 30
				wantMemory = 2 << 30
			}
			report := InspectPool(context.Background(), poolInputs(owner, kernel), "test-revision")
			if report.Status != "consistent" || !report.BootStable || report.AdmissionGuaranteed || report.MemoryCandidateUpperBoundBytes != wantMemory || !reflect.DeepEqual(report.CPUCandidates, wantCPUs) {
				t.Fatalf("%+v", report)
			}
		})
	}
}
func TestPoolDoctorDetectsOwnershipPolicyAndKernelDrift(t *testing.T) {
	cases := map[string]func(*protocol.PoolOwnershipReport, *poolinspect.Snapshot){
		"PRIMARY_CPU_ZERO_IN_CONFIGURED_POOL": func(o *protocol.PoolOwnershipReport, _ *poolinspect.Snapshot) {
			o.ConfiguredCPUs = append([]int{0}, o.ConfiguredCPUs...)
		},
		"PRIMARY_CPU_ZERO_IN_KERNEL_POOL": func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) {
			k.CPUs.Members = append([]uint64{0}, k.CPUs.Members...)
			k.CPUs.Available = append([]uint64{0}, k.CPUs.Available...)
		},
		"CLAIMED_CPU_REPORTED_FREE":            func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) { k.CPUs.Available = []uint64{8, 10, 11} },
		"CLAIMED_CPU_MISSING_FROM_KERNEL_POOL": func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) { k.CPUs.Members = []uint64{9, 10, 11} },
		"CPU_CLAIM_OUTSIDE_CONFIGURED_POLICY":  func(o *protocol.PoolOwnershipReport, _ *poolinspect.Snapshot) { o.ConfiguredCPUs = []int{9, 10, 11} },
		"UNATTRIBUTED_KERNEL_CPU":              func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) { k.CPUs.Available = []uint64{11} },
		"KERNEL_POOL_BELOW_CONFIGURED_MEMORY": func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) {
			k.Memory.TotalBytes = 7 << 30
			k.Memory.AvailableBytes = 4 << 30
			k.Memory.LargestFreeExtentBytes = 4 << 30
		},
		"CLAIMED_MEMORY_MISSING_FROM_KERNEL_POOL": func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) {
			k.Memory.AllocatedBytes = 2 << 30
			k.Memory.AvailableBytes = 6 << 30
		},
		"UNATTRIBUTED_KERNEL_MEMORY": func(_ *protocol.PoolOwnershipReport, k *poolinspect.Snapshot) {
			k.Memory.AllocatedBytes = 4 << 30
			k.Memory.AvailableBytes = 4 << 30
			k.Memory.LargestFreeExtentBytes = 4 << 30
		},
		"CONFIGURED_MEMORY_OVERCOMMITTED": func(o *protocol.PoolOwnershipReport, _ *poolinspect.Snapshot) { o.MemoryReserveBytes = 6 << 30 },
		"DAEMON_POOL_NOT_INITIALIZED":     func(o *protocol.PoolOwnershipReport, _ *poolinspect.Snapshot) { o.Initialized = false },
		"CPU_POOL_POLICY_UNRESTRICTED":    func(o *protocol.PoolOwnershipReport, _ *poolinspect.Snapshot) { o.ConfiguredCPUs = []int{} },
	}
	for code, change := range cases {
		t.Run(code, func(t *testing.T) {
			owner, kernel := poolFixture()
			change(&owner, &kernel)
			report := InspectPool(context.Background(), poolInputs(owner, kernel), "test")
			found := false
			for _, f := range report.Findings {
				found = found || f.Code == code
			}
			if report.Status != "degraded" || !found || report.MemoryCandidateUpperBoundBytes != 0 || len(report.CPUCandidates) != 0 {
				t.Fatalf("missing %s: %+v", code, report)
			}
		})
	}
}
func TestPoolDoctorRefusesPartialLegacyCanceledAndChangedSamples(t *testing.T) {
	for _, mode := range []string{"legacy", "kernel-unavailable", "free-list-unavailable", "invalid-kernel", "ownership-drift", "kernel-drift", "boot-drift", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			owner, kernel := poolFixture()
			in := poolInputs(owner, kernel)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := "OBSERVATION_CHANGED"
			switch mode {
			case "legacy":
				in.Ownership = func(context.Context) (protocol.PoolOwnershipReport, error) {
					return protocol.PoolOwnershipReport{}, daemon.ErrPoolOwnershipUnsupported
				}
				want = "POOL_OWNERSHIP_UNSUPPORTED"
			case "kernel-unavailable":
				in.Kernel = func(context.Context) (poolinspect.Snapshot, error) {
					return poolinspect.Snapshot{}, errors.New("synthetic-private-token")
				}
				want = "KERNEL_POOL_UNAVAILABLE"
			case "free-list-unavailable":
				in.Kernel = func(context.Context) (poolinspect.Snapshot, error) {
					return poolinspect.Snapshot{}, poolinspect.ErrCPUFreeListUnavailable
				}
				want = "KERNEL_CPU_FREE_LIST_UNAVAILABLE"
			case "invalid-kernel":
				kernel.Memory.AvailableBytes = 9 << 30
				in = poolInputs(owner, kernel)
				want = "KERNEL_POOL_UNAVAILABLE"
			case "ownership-drift":
				n := 0
				in.Ownership = func(context.Context) (protocol.PoolOwnershipReport, error) {
					n++
					copy := owner
					copy.Lifecycle.Sequence = uint64(n)
					return copy, nil
				}
			case "kernel-drift":
				n := 0
				in.Kernel = func(context.Context) (poolinspect.Snapshot, error) {
					n++
					copy := kernel
					copy.Memory.LargestFreeExtentBytes = uint64(n) << 30
					return copy, nil
				}
			case "boot-drift":
				n := 0
				in.Boot = func() (string, error) {
					n++
					if n == 1 {
						return boot, nil
					}
					return "different", nil
				}
				want = "BOOT_CHANGED"
			case "canceled":
				cancel()
				in.Ownership = func(context.Context) (protocol.PoolOwnershipReport, error) {
					t.Fatal("called after cancellation")
					return owner, nil
				}
				want = "DEADLINE_OR_CANCELED"
			}
			report := InspectPool(ctx, in, "test")
			if report.Status != "inconclusive" || report.Ownership != nil || report.Kernel != nil || len(report.Findings) != 1 || report.Findings[0].Code != want {
				t.Fatalf("%+v", report)
			}
			data, _ := json.Marshal(report)
			if strings.Contains(string(data), "synthetic-private-token") {
				t.Fatal("raw error escaped")
			}
		})
	}
}
