package poolinspect

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type testProp struct {
	name string
	data []byte
}

func cpuCells(values ...uint64) []byte {
	var b bytes.Buffer
	for _, v := range values {
		_ = binary.Write(&b, binary.BigEndian, v)
	}
	return b.Bytes()
}
func cpuTree(properties []testProp) []byte {
	var body, names bytes.Buffer
	word := func(v uint32) { _ = binary.Write(&body, binary.BigEndian, v) }
	pad := func() {
		for body.Len()%4 != 0 {
			body.WriteByte(0)
		}
	}
	node := func(name string) { word(1); body.WriteString(name); body.WriteByte(0); pad() }
	prop := func(name string, data []byte) {
		offset := names.Len()
		names.WriteString(name)
		names.WriteByte(0)
		word(3)
		word(uint32(len(data)))
		word(uint32(offset))
		body.Write(data)
		pad()
	}
	node("")
	prop("compatible", []byte("multikernel-v1\x00"))
	prop("id", []byte{0, 0, 0, 0})
	node("resources")
	for _, p := range properties {
		prop(p.name, p.data)
	}
	word(2)
	word(2)
	word(9)
	result := make([]byte, 56)
	result = append(result, body.Bytes()...)
	result = append(result, names.Bytes()...)
	values := []uint32{0xd00dfeed, uint32(len(result)), 56, uint32(56 + body.Len()), 40, 17, 16, 0, uint32(names.Len()), uint32(body.Len())}
	for n, v := range values {
		binary.BigEndian.PutUint32(result[n*4:], v)
	}
	return result
}
func TestCPUFreeListRequiresCurrentExplicitKernelABI(t *testing.T) {
	data := cpuTree([]testProp{{"cpus", cpuCells(15, 8, 9)}, {"cpus-available", cpuCells(15, 9)}})
	got, err := ParseCPUs(data)
	if err != nil || len(got.Members) != 3 || got.Members[0] != 8 || got.Available[0] != 9 || got.Available[1] != 15 {
		t.Fatalf("%+v %v", got, err)
	}
	idle := cpuTree([]testProp{{"cpus", cpuCells(0)}})
	if _, err = ParseCPUs(idle); err != ErrCPUFreeListUnavailable {
		t.Fatalf("idle CPU 0 treated as free pool capacity: %v", err)
	}
	empty := cpuTree([]testProp{{"cpus", []byte{}}, {"cpus-available", []byte{}}})
	if got, err = ParseCPUs(empty); err != nil || got.Members == nil || got.Available == nil {
		t.Fatalf("explicit empty free list: %+v %v", got, err)
	}
}
func TestCPUTreeRefusesMalformedOrAmbiguousMetadata(t *testing.T) {
	for _, props := range [][]testProp{
		{{"cpus", cpuCells(8, 8)}, {"cpus-available", cpuCells(8)}},
		{{"cpus", cpuCells(8)}, {"cpus-available", cpuCells(9)}},
		{{"cpus", cpuCells(8)}, {"cpus-available", cpuCells(8, 8)}},
		{{"cpus", []byte{0, 0, 0, 8}}, {"cpus-available", cpuCells(8)}},
		{{"cpus", cpuCells(8)}, {"cpus", cpuCells(8)}, {"cpus-available", cpuCells(8)}},
		{{"cpus-available", cpuCells(8)}},
	} {
		if _, err := ParseCPUs(cpuTree(props)); err != errCPU {
			t.Fatalf("malformed properties accepted: %v", err)
		}
	}
	good := cpuTree([]testProp{{"cpus", cpuCells(8)}, {"cpus-available", cpuCells(8)}})
	for end := 0; end < len(good); end++ {
		if _, err := ParseCPUs(good[:end]); err == nil {
			t.Fatalf("truncation accepted at %d", end)
		}
	}
	for _, offset := range []int{0, 4, 8, 12, 16, 20, 24, 32, 36} {
		bad := bytes.Clone(good)
		binary.BigEndian.PutUint32(bad[offset:], ^uint32(0))
		if _, err := ParseCPUs(bad); err == nil {
			t.Fatalf("corrupt header at %d accepted", offset)
		}
	}
	// A child instance DT cannot supply host pool capacity.
	bad := bytes.Clone(good)
	id := bytes.Index(bad, []byte{0, 0, 0, 3, 0, 0, 0, 4})
	if id < 0 {
		t.Fatal("fixture missing id")
	}
	bad[id+15] = 40
	if _, err := ParseCPUs(bad); err == nil {
		t.Fatal("non-root tree accepted")
	}
}
func FuzzCPUFreeList(f *testing.F) {
	f.Add(cpuTree([]testProp{{"cpus", cpuCells(8, 15)}, {"cpus-available", cpuCells(15)}}))
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := ParseCPUs(data)
		if err != nil {
			return
		}
		seen := map[uint64]bool{}
		for _, cpu := range v.Members {
			seen[cpu] = true
		}
		for _, cpu := range v.Available {
			if !seen[cpu] {
				t.Fatal("free CPU not in pool")
			}
		}
	})
}
