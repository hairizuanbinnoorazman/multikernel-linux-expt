package poolinspect

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sort"
	"strings"
)

var errCPU = errors.New("kernel CPU pool tree is invalid or unsupported")
var ErrCPUFreeListUnavailable = errors.New("kernel CPU pool free list is unavailable")

type CPUs struct {
	Members   []uint64 `json:"members"`
	Available []uint64 `json:"available"`
}
type dtNode struct {
	name     string
	props    map[string]bool
	children map[string]bool
	hasChild bool
}

// ParseCPUs reads only the current 64-bit CPU-cell ABI. In particular, the
// root instance's CPU 0 on an uninitialized pool is not child pool capacity.
// Missing cpus-available is inconclusive, never a fallback to all members free.
func ParseCPUs(data []byte) (CPUs, error) {
	fail := func() (CPUs, error) { return CPUs{}, errCPU }
	if len(data) < 56 || len(data) > 1<<20 {
		return fail()
	}
	word := func(offset int) uint32 { return binary.BigEndian.Uint32(data[offset : offset+4]) }
	if word(0) != 0xd00dfeed || uint64(word(4)) != uint64(len(data)) || word(20) != 17 || word(24) != 16 {
		return fail()
	}
	offStruct, offStrings, offReserve := uint64(word(8)), uint64(word(12)), uint64(word(16))
	structEnd, stringsEnd := offStruct+uint64(word(36)), offStrings+uint64(word(32))
	if offStruct < 40 || offStrings < 40 || offReserve < 40 || offStruct%4 != 0 || offReserve%8 != 0 || structEnd > uint64(len(data)) || stringsEnd > uint64(len(data)) || offStruct < stringsEnd && offStrings < structEnd {
		return fail()
	}
	// Validate and bound the reservation map independently of both other blocks.
	reserveEnd := offReserve
	for n := 0; ; n++ {
		if n > 64 || reserveEnd+16 > uint64(len(data)) || reserveEnd < structEnd && offStruct < reserveEnd+16 || reserveEnd < stringsEnd && offStrings < reserveEnd+16 {
			return fail()
		}
		address, size := binary.BigEndian.Uint64(data[reserveEnd:]), binary.BigEndian.Uint64(data[reserveEnd+8:])
		reserveEnd += 16
		if address == 0 && size == 0 {
			break
		}
		if size == 0 || address > ^uint64(0)-(size-1) {
			return fail()
		}
	}
	structure, names := data[offStruct:structEnd], data[offStrings:stringsEnd]
	var stack []dtNode
	var members, available []uint64
	rootSeen, rootClosed, resources, compatible, rootID, freeSeen := false, false, false, false, false, false
	props, nodes := 0, 0
	readName := func(raw []byte) (string, int, bool) {
		n := bytes.IndexByte(raw, 0)
		if n < 0 || n > 256 {
			return "", 0, false
		}
		value := string(raw[:n])
		for _, v := range value {
			if v < 33 || v > 126 || v == '/' {
				return "", 0, false
			}
		}
		return value, n + 1, true
	}
	cells := func(raw []byte) ([]uint64, bool) {
		if len(raw)%8 != 0 || len(raw) > 4096*8 {
			return nil, false
		}
		values := make([]uint64, 0, len(raw)/8)
		seen := map[uint64]bool{}
		for len(raw) > 0 {
			v := binary.BigEndian.Uint64(raw)
			if seen[v] {
				return nil, false
			}
			seen[v] = true
			values = append(values, v)
			raw = raw[8:]
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		return values, true
	}
	for pos := 0; pos < len(structure); {
		if pos+4 > len(structure) {
			return fail()
		}
		token := binary.BigEndian.Uint32(structure[pos:])
		pos += 4
		switch token {
		case 1:
			if rootClosed || len(stack) >= 32 || nodes >= 4096 {
				return fail()
			}
			nodes++
			name, n, ok := readName(structure[pos:])
			if !ok {
				return fail()
			}
			padded := (n + 3) &^ 3
			if pos+padded > len(structure) || !allZero(structure[pos+n:pos+padded]) {
				return fail()
			}
			pos += padded
			if len(stack) == 0 {
				if rootSeen {
					return fail()
				}
				rootSeen = true
			} else {
				parent := &stack[len(stack)-1]
				if name == "" || parent.children[name] {
					return fail()
				}
				parent.children[name] = true
				parent.hasChild = true
			}
			if len(stack) == 1 && name == "resources" {
				resources = true
			}
			stack = append(stack, dtNode{name: name, props: map[string]bool{}, children: map[string]bool{}})
		case 2:
			if len(stack) == 0 {
				return fail()
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				rootClosed = true
			}
		case 3:
			if len(stack) == 0 || stack[len(stack)-1].hasChild || pos+8 > len(structure) || props >= 8192 {
				return fail()
			}
			props++
			size, offset := uint64(binary.BigEndian.Uint32(structure[pos:])), uint64(binary.BigEndian.Uint32(structure[pos+4:]))
			pos += 8
			padded := (size + 3) &^ uint64(3)
			if uint64(pos)+padded > uint64(len(structure)) || offset >= uint64(len(names)) || offset > 0 && names[offset-1] != 0 {
				return fail()
			}
			name, _, ok := readName(names[offset:])
			if !ok || name == "" || stack[len(stack)-1].props[name] {
				return fail()
			}
			stack[len(stack)-1].props[name] = true
			raw := structure[pos : uint64(pos)+size]
			if !allZero(structure[uint64(pos)+size : uint64(pos)+padded]) {
				return fail()
			}
			pos += int(padded)
			if len(stack) == 1 {
				switch name {
				case "compatible":
					compatible = bytes.Equal(raw, []byte("multikernel-v1\x00"))
				case "id":
					rootID = len(raw) == 4 && binary.BigEndian.Uint32(raw) == 0
				}
			}
			if len(stack) == 2 && stack[1].name == "resources" {
				switch name {
				case "cpus":
					members, ok = cells(raw)
					if !ok {
						return fail()
					}
				case "cpus-available":
					available, ok = cells(raw)
					if !ok {
						return fail()
					}
					freeSeen = true
				}
			}
		case 4: // FDT_NOP
		case 9:
			if !rootSeen || !rootClosed || len(stack) != 0 || pos != len(structure) || !resources || !compatible || !rootID || members == nil {
				return fail()
			}
			if !freeSeen {
				return CPUs{}, ErrCPUFreeListUnavailable
			}
			pool := map[uint64]bool{}
			for _, v := range members {
				pool[v] = true
			}
			for _, v := range available {
				if !pool[v] {
					return fail()
				}
			}
			return CPUs{Members: members, Available: available}, nil
		default:
			return fail()
		}
	}
	return fail()
}
func allZero(data []byte) bool { return strings.Trim(string(data), "\x00") == "" }
