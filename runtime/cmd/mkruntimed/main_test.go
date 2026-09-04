package main

import (
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostcheck"
)

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

func TestValidatePoolReport(t *testing.T) {
	report := hostcheck.Report{Qualified: true, MemoryBytes: 32 << 30, CPUs: []hostcheck.CPU{
		{APIC: 0, Physical: 0, Core: 0, Online: true},
		{APIC: 2, Physical: 0, Core: 1, Online: true},
		{APIC: 4, Physical: 0, Core: 2, Online: true},
		{APIC: 6, Physical: 0, Core: 3, Online: true},
	}}
	if err := validatePoolReport(report, []int{4}, []int{0}, 2, 8<<30, 8<<30); err != nil {
		t.Fatalf("valid pool rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		ids    []int
		memory uint64
	}{
		{"apic-zero", []int{0}, 8 << 30},
		{"offline", []int{9}, 8 << 30},
		{"cpu-headroom", []int{2, 4, 6}, 8 << 30},
		{"memory-headroom", []int{4}, 25 << 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePoolReport(report, test.ids, []int{0}, 2, test.memory, 8<<30); err == nil {
				t.Fatal("unsafe pool accepted")
			}
		})
	}
	report.Qualified = false
	if err := validatePoolReport(report, []int{4}, []int{0}, 2, 8<<30, 8<<30); err == nil {
		t.Fatal("unqualified host accepted")
	}
}
