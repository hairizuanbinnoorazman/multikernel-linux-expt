package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOwnershipWireBudget(t *testing.T) {
	report := OwnershipReport{Version: 1, Source: "durable-lifecycle", Sequence: ^uint64(0), Sandboxes: []SandboxOwnership{}}
	for i := 0; i < MaxOwnershipRecords; i++ {
		sb := SandboxOwnership{SandboxID: fmt.Sprintf("work-%03d-", i) + strings.Repeat("x", 54), Generation: strings.Repeat("a", 32), State: "ALLOCATING", MemoryBytes: ^uint64(0), ChildCID: ^uint32(0), AgentPort: ^uint32(0)}
		for j := 0; j < MaxOwnershipCPUs/MaxOwnershipRecords; j++ {
			sb.CPUs = append(sb.CPUs, int(^uint(0)>>1)-16+j)
		}
		report.Sandboxes = append(report.Sandboxes, sb)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil || len(encoded) > 256<<10 {
		t.Fatalf("ownership exceeds reserved wire budget: %d bytes, %v", len(encoded), err)
	}
	report.Sandboxes[0].CPUs = append(report.Sandboxes[0].CPUs, int(^uint(0)>>1))
	if report.Validate() == nil {
		t.Fatal("aggregate CPU bound not enforced")
	}
}
