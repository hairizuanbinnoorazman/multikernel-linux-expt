package main

import (
	"flag"
	"os"
	"os/exec"
	"strings"
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

func TestRecoverableIdlePool(t *testing.T) {
	exact := hostcheck.Report{PoolConfigured: true, StaleResources: []string{"configured Kerf pool has no matching instance"}}
	if !recoverableIdlePool(exact) {
		t.Fatal("exact idle daemon-owned pool was not recoverable")
	}
	for name, report := range map[string]hostcheck.Report{
		"instance": {PoolConfigured: true, Instances: []string{"box-a"}, StaleResources: exact.StaleResources},
		"extra-stale": {PoolConfigured: true, StaleResources: []string{
			"configured Kerf pool has no matching instance", "unmatched /proc/kimage entry box-a",
		}},
		"no-pool": {},
	} {
		t.Run(name, func(t *testing.T) {
			if recoverableIdlePool(report) {
				t.Fatal("unsafe idle-pool recovery accepted")
			}
		})
	}
}

// Run actual argument dispatch in a subprocess: these refusals must precede
// configuration loading, daemon startup, or any mutation of runtime state.
func TestUnifiedInspectionModeCLI(t *testing.T) {
	if os.Getenv("MK_TEST_UNIFIED_MODE_DISPATCH") == "1" {
		flag.CommandLine = flag.NewFlagSet("mkruntimed", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"mkruntimed"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		os.Exit(0)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"bundle-empty", []string{"--diagnostic-bundle="}, "diagnostic-bundle requires a canonical absolute JSON file destination"},
		{"bundle-relative", []string{"--diagnostic-bundle=local.json"}, "diagnostic-bundle requires a canonical absolute JSON file destination"},
		{"bundle-noncanonical", []string{"--diagnostic-bundle=/tmp/../tmp/local.json"}, "diagnostic-bundle requires a canonical absolute JSON file destination"},
		{"bundle-conflict", []string{"--diagnostic-bundle=/tmp/local.json", "--doctor-all"}, "select one inspection mode"},
		{"bundle-format", []string{"--diagnostic-bundle=/tmp/local.json", "--doctor-format=7"}, "doctor-format requires --doctor-all"},
		{"bundle-relative-network", []string{"--diagnostic-bundle=/tmp/local.json", "--network-socket=relative.sock"}, "network inspection requires an absolute network socket"},
		{"format-without-mode", []string{"--doctor-format=7"}, "doctor-format requires --doctor-all"},
		{"explicit-default-without-mode", []string{"--doctor-format=5"}, "doctor-format requires --doctor-all"},
		{"format-with-alias", []string{"--doctor-all-pool", "--doctor-format=6"}, "doctor-format requires --doctor-all"},
		{"format-with-components", []string{"--inspect-components", "--doctor-format=7"}, "doctor-format requires --doctor-all"},
		{"format-unknown", []string{"--doctor-all", "--doctor-format=8"}, "doctor-format must be 5, 6 or 7"},
		{"format-negative", []string{"--doctor-all", "--doctor-format=-1"}, "doctor-format must be 5, 6 or 7"},
		{"format-relative-network", []string{"--doctor-all", "--doctor-format=7", "--network-socket=relative.sock"}, "network inspection requires an absolute network socket"},
		{"journal-conflict", []string{"--inspect-journal", "--inspect-ownership"}, "select one inspection mode"},
		{"journal-format-conflict", []string{"--inspect-journal", "--doctor-format=7"}, "doctor-format requires --doctor-all"},
		{"component-conflict", []string{"--inspect-components", "--doctor-all-pool"}, "select one inspection mode"},
		{"component-relative-network", []string{"--inspect-components", "--network-socket=relative.sock"}, "network inspection requires an absolute network socket"},
		{"combined-conflict", []string{"--doctor-all-pool", "--doctor-all"}, "select one inspection mode"},
		{"pool-conflict", []string{"--doctor-all-pool", "--doctor-pool"}, "select one inspection mode"},
		{"relative-network", []string{"--doctor-all-pool", "--network-socket=relative.sock"}, "network inspection requires an absolute network socket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], append([]string{"-test.run=^TestUnifiedInspectionModeCLI$", "--", "--config=/nonexistent/unified-test-config.json"}, tc.args...)...)
			cmd.Env = append(os.Environ(), "MK_TEST_UNIFIED_MODE_DISPATCH=1")
			out, err := cmd.CombinedOutput()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 2 || strings.TrimSpace(string(out)) != tc.want {
				t.Fatalf("status=%v output=%s", err, out)
			}
		})
	}
}
