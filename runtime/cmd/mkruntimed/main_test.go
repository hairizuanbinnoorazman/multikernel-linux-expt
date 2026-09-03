package main

import "testing"

func TestMemoryBytes(t *testing.T) {
	tests := map[string]uint64{
		"12GB":  12_000_000_000,
		"4GiB":  4 << 30,
		"1024":  1024,
		"512MB": 512_000_000,
	}
	for input, want := range tests {
		got, err := memoryBytes(input)
		if err != nil || got != want {
			t.Errorf("memoryBytes(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "1.5GB", "GB", "18446744073709551615TB"} {
		if _, err := memoryBytes(input); err == nil {
			t.Errorf("memoryBytes(%q) unexpectedly succeeded", input)
		}
	}
}
