// Package doctor provides inspection only. It never reconciles or repairs state.
package doctor

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"sort"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Inputs struct {
	Ownership func(context.Context) (protocol.OwnershipReport, error)
	Kernel    func(context.Context) ([]string, error)
	Boot      func() (string, error)
}
type Finding struct {
	Code       string `json:"code"`
	SandboxID  string `json:"sandbox_id,omitempty"`
	Generation string `json:"generation,omitempty"`
}
type Report struct {
	Version                  int                       `json:"version"`
	Scope                    string                    `json:"scope"`
	Status                   string                    `json:"status"`
	BootID                   string                    `json:"boot_id,omitempty"`
	BootStable               bool                      `json:"boot_stable"`
	Revision                 string                    `json:"revision"`
	Ownership                *protocol.OwnershipReport `json:"ownership,omitempty"`
	KernelInstances          []string                  `json:"kernel_instances,omitempty"`
	KernelGenerationVerified bool                      `json:"kernel_generation_verified"`
	Findings                 []Finding                 `json:"findings"`
	Unobserved               []string                  `json:"unobserved"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var bootPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Inspect brackets kernel inventories with two negotiated ownership reads and
// boot identities. This detects observed drift, not an atomic kernel snapshot
// or an ABA transition between samples. A generation exists only in durable
// ownership: matching names never proves the running kernel's incarnation.
func Inspect(ctx context.Context, in Inputs, revision string) Report {
	r := Report{Version: 1, Scope: "durable-ownership-and-kernel-presence", Status: "inconclusive", Revision: revision, Findings: []Finding{}, Unobserved: []string{"kernel-incarnation", "storage-exports", "network-endpoints", "pool-capacity", "service-readiness", "docker-engine", "containment"}}
	fail := func(code string) Report { r.Findings = []Finding{{Code: code}}; return r }
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	boot, err := in.Boot()
	if err != nil || !bootPattern.MatchString(boot) {
		return fail("BOOT_UNAVAILABLE")
	}
	// Failed observations still identify the boot at which inspection began.
	// Stability is confirmed only after the final independent boot read.
	r.BootID = boot
	before, err := in.Ownership(ctx)
	if err != nil || before.Validate() != nil {
		return fail(ownershipFailure(err))
	}
	first, err := in.Kernel(ctx)
	if err != nil || !validKernel(first) {
		return fail("KERNEL_INVENTORY_UNAVAILABLE")
	}
	after, err := in.Ownership(ctx)
	if err != nil || after.Validate() != nil {
		return fail(ownershipFailure(err))
	}
	second, err := in.Kernel(ctx)
	if err != nil || !validKernel(second) {
		return fail("KERNEL_INVENTORY_UNAVAILABLE")
	}
	end, err := in.Boot()
	if err != nil || end != boot {
		return fail("BOOT_CHANGED")
	}
	r.BootStable = true
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(first, second) {
		return fail("OBSERVATION_CHANGED")
	}
	r.BootID = boot
	r.Ownership = &after
	r.KernelInstances = second
	r.Status = "consistent"
	kernel := map[string]bool{}
	for _, id := range second {
		kernel[id] = true
	}
	owners := map[string]bool{}
	cpus := map[int]bool{}
	cids := map[uint32]bool{}
	add := func(code string, sb protocol.SandboxOwnership) {
		r.Findings = append(r.Findings, Finding{code, sb.SandboxID, sb.Generation})
		r.Status = "degraded"
	}
	for _, sb := range after.Sandboxes {
		owners[sb.SandboxID] = true
		if sb.State == "ABSENT" {
			if kernel[sb.SandboxID] {
				add("RELEASED_INSTANCE_PRESENT", sb)
			}
			continue
		}
		if cids[sb.ChildCID] {
			add("DUPLICATE_CHILD_CID", sb)
		}
		cids[sb.ChildCID] = true
		overlap := false
		for _, cpu := range sb.CPUs {
			if cpus[cpu] {
				overlap = true
			}
			cpus[cpu] = true
		}
		if overlap {
			add("OVERLAPPING_CPU_ASSIGNMENT", sb)
		}
		switch sb.State {
		case "ALLOCATING", "STOPPING", "RELEASING":
			r.Findings = append(r.Findings, Finding{"TRANSITION_REQUIRES_RECHECK", sb.SandboxID, sb.Generation})
			if r.Status == "consistent" {
				r.Status = "inconclusive"
			}
		default:
			if !kernel[sb.SandboxID] {
				add("OWNED_INSTANCE_MISSING", sb)
			}
		}
	}
	for _, id := range second {
		if !owners[id] {
			r.Findings = append(r.Findings, Finding{Code: "ORPHAN_CANDIDATE", SandboxID: id})
			r.Status = "degraded"
		}
	}
	return r
}

func ownershipFailure(err error) string {
	if errors.Is(err, daemon.ErrOwnershipUnsupported) {
		return "OWNERSHIP_UNSUPPORTED"
	}
	return "OWNERSHIP_UNAVAILABLE"
}

func validKernel(ids []string) bool {
	if ids == nil || len(ids) > protocol.MaxOwnershipRecords || !sort.StringsAreSorted(ids) {
		return false
	}
	previous := ""
	for _, id := range ids {
		if !idPattern.MatchString(id) || id == previous {
			return false
		}
		previous = id
	}
	return true
}
