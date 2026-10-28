// Package poolinspect reads kernel pool metadata without allocating or repairing resources.
package poolinspect

import (
	"bytes"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxMemoryMapBytes = 1 << 20
const maxMemoryMapRanges = 8192
const poolName = "Multikernel Memory Pool"

var errMemoryMap = errors.New("kernel memory map is unavailable, obscured or invalid")
var memoryLine = regexp.MustCompile(`^( *)([0-9a-fA-F]{1,16})-([0-9a-fA-F]{1,16}) : ([ -~]{1,256})$`)

type Memory struct {
	Chunks         int    `json:"chunks"`
	TotalBytes     uint64 `json:"total_bytes"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}
type interval struct{ start, end uint64 }
type memoryNode struct {
	interval
	pool int
}

// ParseMemory counts the union of actual descendants of each canonical pool
// resource. Parent System RAM and unrelated address aliases are never counted
// as pool allocations. Physical addresses and resource names are not exported.
func ParseMemory(data []byte) (Memory, error) {
	fail := func() (Memory, error) { return Memory{}, errMemoryMap }
	if len(data) == 0 || len(data) > maxMemoryMapBytes || data[len(data)-1] != '\n' {
		return fail()
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > maxMemoryMapRanges {
		return fail()
	}
	var stack []memoryNode
	var chunks []interval
	var allocations [][]interval
	visibleRAM := false
	for _, line := range lines {
		match := memoryLine.FindSubmatch(line)
		if match == nil || len(match[1])%2 != 0 || len(match[1]) > 64 {
			return fail()
		}
		start, e1 := strconv.ParseUint(string(match[2]), 16, 64)
		end, e2 := strconv.ParseUint(string(match[3]), 16, 64)
		if e1 != nil || e2 != nil || end < start || end-start == ^uint64(0) {
			return fail()
		}
		depth := len(match[1]) / 2
		if depth > len(stack) {
			return fail()
		}
		stack = stack[:depth]
		pool := -1
		if depth > 0 {
			parent := stack[depth-1]
			if start < parent.start || end > parent.end {
				return fail()
			}
			pool = parent.pool
		}
		name := string(match[4])
		if strings.Contains(name, poolName) && name != poolName {
			return fail()
		}
		if name == "System RAM" && end > start {
			visibleRAM = true
		}
		region := interval{start, end}
		if name == poolName {
			if pool >= 0 || start == 0 && end == 0 {
				return fail()
			}
			pool = len(chunks)
			chunks = append(chunks, region)
			allocations = append(allocations, nil)
		} else if pool >= 0 {
			allocations[pool] = append(allocations[pool], region)
		}
		stack = append(stack, memoryNode{region, pool})
	}
	// Restricted /proc/iomem can silently replace every address with zero.
	// An empty pool is only meaningful when the map demonstrably exposes RAM.
	if !visibleRAM {
		return fail()
	}
	ordered := append([]interval(nil), chunks...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].start < ordered[j].start })
	for n := 1; n < len(ordered); n++ {
		if ordered[n].start <= ordered[n-1].end {
			return fail()
		}
	}
	result := Memory{Chunks: len(chunks)}
	add := func(total *uint64, n uint64) bool {
		if n > ^uint64(0)-*total {
			return false
		}
		*total += n
		return true
	}
	for n, chunk := range chunks {
		if !add(&result.TotalBytes, chunk.end-chunk.start+1) {
			return fail()
		}
		regions := allocations[n]
		sort.Slice(regions, func(i, j int) bool { return regions[i].start < regions[j].start })
		if len(regions) == 0 {
			continue
		}
		merged := regions[0]
		for _, region := range regions[1:] {
			if merged.end == ^uint64(0) || region.start <= merged.end+1 {
				if region.end > merged.end {
					merged.end = region.end
				}
			} else {
				if !add(&result.AllocatedBytes, merged.end-merged.start+1) {
					return fail()
				}
				merged = region
			}
		}
		if !add(&result.AllocatedBytes, merged.end-merged.start+1) {
			return fail()
		}
	}
	if result.AllocatedBytes > result.TotalBytes {
		return fail()
	}
	result.AvailableBytes = result.TotalBytes - result.AllocatedBytes
	return result, nil
}
