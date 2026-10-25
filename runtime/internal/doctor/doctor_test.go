package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const boot = "00ac7aff-a963-4e30-a085-6811e83f0e12"

func sandbox(id, state string) protocol.SandboxOwnership {
	return protocol.SandboxOwnership{SandboxID: id, Generation: strings.Repeat("a", 32), State: state, CPUs: []int{8, 9}, MemoryBytes: 3 << 30, ChildCID: 40, AgentPort: 1000}
}
func inputs(sandboxes []protocol.SandboxOwnership, ids []string) Inputs {
	return Inputs{Ownership: func(context.Context) (protocol.OwnershipReport, error) {
		return protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: 4, Sandboxes: sandboxes}, nil
	}, Kernel: func(context.Context) ([]string, error) { return ids, nil }, Boot: func() (string, error) { return boot, nil }}
}
func TestDiagnosticStates(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sandboxes    []protocol.SandboxOwnership
		ids          []string
		status, code string
	}{
		{"clean", []protocol.SandboxOwnership{}, []string{}, "consistent", ""},
		{"active", []protocol.SandboxOwnership{sandbox("box-a", "RUNNING")}, []string{"box-a"}, "consistent", ""},
		{"stale", []protocol.SandboxOwnership{sandbox("box-a", "RUNNING")}, []string{}, "degraded", "OWNED_INSTANCE_MISSING"},
		{"orphan", []protocol.SandboxOwnership{}, []string{"box-a"}, "degraded", "ORPHAN_CANDIDATE"},
		{"released", []protocol.SandboxOwnership{sandbox("box-a", "ABSENT")}, []string{"box-a"}, "degraded", "RELEASED_INSTANCE_PRESENT"},
		{"transition", []protocol.SandboxOwnership{sandbox("box-a", "STOPPING")}, []string{"box-a"}, "inconclusive", "TRANSITION_REQUIRES_RECHECK"},
		{"conflict", []protocol.SandboxOwnership{sandbox("box-a", "RUNNING"), sandbox("box-b", "RUNNING")}, []string{"box-a", "box-b"}, "degraded", "DUPLICATE_CHILD_CID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Inspect(context.Background(), inputs(tc.sandboxes, tc.ids), "test-revision")
			if r.Status != tc.status || r.KernelGenerationVerified || len(r.Unobserved) != 7 {
				t.Fatalf("%+v", r)
			}
			if tc.code != "" && (len(r.Findings) == 0 || r.Findings[0].Code != tc.code) {
				t.Fatalf("%+v", r)
			}
			if tc.name == "conflict" && (len(r.Findings) != 2 || r.Findings[1].Code != "OVERLAPPING_CPU_ASSIGNMENT") {
				t.Fatalf("%+v", r)
			}
			if len(tc.sandboxes) > 0 && tc.code != "" && tc.name != "orphan" && r.Findings[0].Generation != tc.sandboxes[0].Generation {
				t.Fatal("lost generation")
			}
		})
	}
}
func TestUnknownDriftAndPrivateErrors(t *testing.T) {
	for _, mode := range []string{"unavailable", "bad-report", "missing-kernel", "oversized", "malformed", "duplicate", "reordered", "boot-change", "sequence-change", "generation-change", "inventory-change", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			in := inputs([]protocol.SandboxOwnership{}, []string{})
			n := 0
			switch mode {
			case "unavailable":
				in.Ownership = func(context.Context) (protocol.OwnershipReport, error) {
					return protocol.OwnershipReport{}, errors.New("private-token-SECRET")
				}
			case "bad-report":
				in.Ownership = func(context.Context) (protocol.OwnershipReport, error) { return protocol.OwnershipReport{}, nil }
			case "missing-kernel":
				in.Kernel = func(context.Context) ([]string, error) { return nil, os.ErrNotExist }
			case "oversized":
				in.Kernel = func(context.Context) ([]string, error) { return make([]string, 257), nil }
			case "malformed":
				in.Kernel = func(context.Context) ([]string, error) { return []string{"../private-token-SECRET"}, nil }
			case "duplicate":
				in.Kernel = func(context.Context) ([]string, error) { return []string{"box-a", "box-a"}, nil }
			case "reordered":
				in.Kernel = func(context.Context) ([]string, error) { return []string{"box-b", "box-a"}, nil }
			case "boot-change":
				in.Boot = func() (string, error) {
					n++
					if n == 1 {
						return boot, nil
					}
					return "00000000-0000-0000-0000-000000000000", nil
				}
			case "sequence-change", "generation-change":
				in.Ownership = func(context.Context) (protocol.OwnershipReport, error) {
					n++
					r := protocol.OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: uint64(n), Sandboxes: []protocol.SandboxOwnership{sandbox("box-a", "RUNNING")}}
					if mode == "generation-change" {
						r.Sequence = 1
						if n == 2 {
							r.Sandboxes[0].Generation = strings.Repeat("b", 32)
						}
					}
					return r, nil
				}
			case "inventory-change":
				in.Kernel = func(context.Context) ([]string, error) {
					n++
					if n == 1 {
						return []string{}, nil
					}
					return []string{"box-a"}, nil
				}
			}
			ctx := context.Background()
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			r := Inspect(ctx, in, "test-revision")
			b, _ := json.Marshal(r)
			if r.Status != "inconclusive" || r.Ownership != nil || r.KernelInstances != nil || len(r.Findings) != 1 || strings.Contains(string(b), "SECRET") {
				t.Fatalf("%s", b)
			}
		})
	}
}
func TestKernelInventoryBoundedAndMissingIsNotEmpty(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	if _, err := KernelInventory(ctx, root); err == nil {
		t.Fatal("missing inventory accepted")
	}
	dir := filepath.Join(root, "instances")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ids, err := KernelInventory(ctx, root)
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("%v %v", ids, err)
	}
	if err = os.Mkdir(filepath.Join(dir, "box-a"), 0700); err != nil {
		t.Fatal(err)
	}
	ids, err = KernelInventory(ctx, root)
	if err != nil || len(ids) != 1 || ids[0] != "box-a" {
		t.Fatalf("%v %v", ids, err)
	}
	if err = os.Symlink("box-a", filepath.Join(dir, "box-b")); err != nil {
		t.Fatal(err)
	}
	if _, err = KernelInventory(ctx, root); err == nil {
		t.Fatal("alias accepted")
	}
	if err = os.Remove(filepath.Join(dir, "box-b")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		if _, err = os.MkdirTemp(dir, "box-"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = KernelInventory(ctx, root); err == nil {
		t.Fatal("oversize accepted")
	}
}
