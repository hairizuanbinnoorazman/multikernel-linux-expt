package poolinspect

import (
	"strings"
	"testing"
)

func TestMemoryPoolCountsDescendantUnionAcrossChunks(t *testing.T) {
	input := `0000-ffff : System RAM
  1000-1fff : Multikernel Memory Pool
    1100-12ff : guest-private-name
      1100-11ff : nested allocation
    1300-13ff : other allocation
  3000-3fff : Multikernel Memory Pool
    3100-31ff : another allocation
`
	got, err := ParseMemory([]byte(input))
	if err != nil || got != (Memory{Chunks: 2, TotalBytes: 8192, AllocatedBytes: 1024, AvailableBytes: 7168, LargestFreeExtentBytes: 3584}) {
		t.Fatalf("memory=%+v err=%v", got, err)
	}
	empty, err := ParseMemory([]byte("0000-ffff : System RAM\n"))
	if err != nil || empty != (Memory{}) {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
}
func TestMemoryMapRefusesAmbiguityAndRedaction(t *testing.T) {
	cases := []string{
		"", "0000-ffff : System RAM", "0000-0000 : System RAM\n",
		"ffff-0000 : System RAM\n", "0-ffffffffffffffff : System RAM\n",
		"0000-ffff : System RAM\n    1000-1fff : Multikernel Memory Pool\n",
		"0000-ffff : System RAM\n  1000-1fff : Multikernel Memory Pool\n    2000-20ff : private-secret\n",
		"0000-ffff : System RAM\n  1000-1fff : Multikernel Memory Pool\n  1800-2fff : Multikernel Memory Pool\n",
		"0000-ffff : System RAM\n  1000-1fff : Multikernel Memory Pool private-secret\n",
		"0000-ffff : System RAM\n  1000-1fff : Multikernel Memory Pool\n    1000-1fff : Multikernel Memory Pool\n",
		"0000-ffff : System RAM\n\t1000-1fff : Multikernel Memory Pool\n",
		"0000-ffff : System RAM\n  0000-0000 : Multikernel Memory Pool\n",
		strings.Repeat("0000-ffff : System RAM\n", maxMemoryMapRanges+1),
		strings.Repeat("x", maxMemoryMapBytes+1),
	}
	for n, input := range cases {
		got, err := ParseMemory([]byte(input))
		if err != errMemoryMap || got != (Memory{}) {
			t.Fatalf("case %d accepted: %+v %v", n, got, err)
		}
	}
}
func TestMemoryPoolDoesNotCountAddressAliasAsDescendant(t *testing.T) {
	got, err := ParseMemory([]byte("0000-ffff : System RAM\n  1000-1fff : Multikernel Memory Pool\n1000-1fff : unrelated address alias\n"))
	if err != nil || got.AllocatedBytes != 0 || got.AvailableBytes != 4096 {
		t.Fatalf("%+v %v", got, err)
	}
}
func FuzzMemoryMap(f *testing.F) {
	f.Add([]byte("0000-ffff : System RAM\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := ParseMemory(data)
		if err == nil && (v.AllocatedBytes > v.TotalBytes || v.AvailableBytes != v.TotalBytes-v.AllocatedBytes) {
			t.Fatal("invalid accounting")
		}
	})
}
