package hostcheck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, release, online string, instances bool) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"proc/sys/kernel/osrelease": release, "sys/devices/system/cpu/online": online,
		"proc/meminfo": "MemTotal:       33554432 kB\n", "sys/kernel/security/lockdown": "[none] integrity confidentiality",
		"sys/kernel/kexec_loaded": "0", "run/google-guest-agent.status": "active",
		"run/secure-boot.status": "disabled", "run/serial-console.status": "available",
		"run/kerf-version": "kerf, version 0.2.0", "run/kerf-show": "No memory pool configured",
		"proc/kimage":             "MK_ID Type Start Address Segments Mode Cmdline\n----- ---- ------------- -------- ---- -------\n",
		"run/root-device":         "8:0",
		"run/kerf-dry-run.status": "ready",
		"run/kerf-dry-run.output": "pool transaction is feasible",
		"proc/net/route":          "Iface Destination Gateway Flags\nens4 00000000 00000000 0003\n",
		"boot/config-" + release:  "CONFIG_MULTIKERNEL=y\nCONFIG_KEXEC_CORE=y\nCONFIG_KEXEC_FILE=y\nCONFIG_MKTTY=y\n",
		"proc/cpuinfo":            "processor: 0\napicid: 0\nphysical id: 0\ncore id: 0\n\nprocessor: 1\napicid: 2\nphysical id: 0\ncore id: 1\n\nprocessor: 2\napicid: 4\nphysical id: 0\ncore id: 2\n\nprocessor: 3\napicid: 6\nphysical id: 0\ncore id: 3\n",
	}
	for p, v := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte(v), 0644)
	}
	os.MkdirAll(filepath.Join(root, "sys/fs/multikernel/instances"), 0755)
	os.MkdirAll(filepath.Join(root, "sys/devices/system/cpu/cpu0/node0"), 0755)
	diskTarget := filepath.Join(root, "sys/devices/pci0000:00/0000:00:01.0/0000:00:03.0/virtio0/block/sda")
	nicTarget := filepath.Join(root, "sys/devices/pci0000:00/0000:00:01.0/0000:00:04.0/virtio1/net/ens4")
	os.MkdirAll(diskTarget, 0755)
	os.MkdirAll(nicTarget, 0755)
	os.MkdirAll(filepath.Join(root, "sys/dev/block"), 0755)
	os.MkdirAll(filepath.Join(root, "sys/class/net/ens4"), 0755)
	os.Symlink(diskTarget, filepath.Join(root, "sys/dev/block/8:0"))
	os.Symlink(nicTarget, filepath.Join(root, "sys/class/net/ens4/device"))
	if instances {
		os.Mkdir(filepath.Join(root, "sys/fs/multikernel/instances/foreign"), 0755)
	}
	return root
}

func TestUnknownCriticalQualificationSignalsFailClosed(t *testing.T) {
	tests := []string{"run/kerf-version", "run/kerf-show", "run/kerf-dry-run.status", "run/secure-boot.status", "run/serial-console.status", "sys/kernel/security/lockdown", "sys/kernel/kexec_loaded", "run/google-guest-agent.status"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			root := fixture(t, "7.0.0-mk2-gce-lab", "0-3", false)
			if err := os.Remove(filepath.Join(root, name)); err != nil {
				t.Fatal(err)
			}
			r := Check(context.Background(), Options{Root: root, MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
			if r.Qualified {
				t.Fatalf("host qualified without %s", name)
			}
		})
	}
}

func TestIncompatibleCriticalQualificationSignalsFail(t *testing.T) {
	tests := map[string]string{
		"run/kerf-version":              "kerf, version 9.9.9",
		"run/kerf-dry-run.status":       "unavailable",
		"run/secure-boot.status":        "enabled",
		"run/serial-console.status":     "unavailable",
		"sys/kernel/security/lockdown":  "none [integrity] confidentiality",
		"run/google-guest-agent.status": "inactive",
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			root := fixture(t, "7.0.0-mk2-gce-lab", "0-3", false)
			if err := os.WriteFile(filepath.Join(root, name), []byte(value), 0644); err != nil {
				t.Fatal(err)
			}
			if report := Check(context.Background(), Options{Root: root, MinPrimaryCPUs: 4, MinPrimaryMemByte: 1}); report.Qualified {
				t.Fatalf("host qualified with %s=%q", name, value)
			}
		})
	}
}

func TestResourceInventoryAndStalePool(t *testing.T) {
	root := fixture(t, "7.0.0-mk2-gce-lab", "0-3", true)
	if err := os.WriteFile(filepath.Join(root, "sys/fs/multikernel/instances/foreign/status"), []byte("loaded\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sys/fs/multikernel/instances/foreign/devices/0000:00:04.0"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proc/kimage"), []byte("MK_ID Type\nforeign kernel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r := Check(context.Background(), Options{Root: root, MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if len(r.InstanceState) != 1 || r.InstanceState[0].Status != "loaded" || len(r.AssignedDevices) != 1 {
		t.Fatalf("resource inventory: %+v devices=%v", r.InstanceState, r.AssignedDevices)
	}
	if len(r.StaleResources) != 0 {
		t.Fatalf("matched resource reported stale: %v", r.StaleResources)
	}
	if err := os.WriteFile(filepath.Join(root, "run/kerf-show"), []byte("Pool: configured\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "sys/fs/multikernel/instances/foreign")); err != nil {
		t.Fatal(err)
	}
	r = Check(context.Background(), Options{Root: root, MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if len(r.StaleResources) == 0 || r.Qualified {
		t.Fatalf("stale pool not rejected: %+v", r)
	}
}

func TestQualifiedFixture(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-3", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if !r.Qualified {
		t.Fatalf("findings: %+v", r.Findings)
	}
}
func TestStockKernelFails(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "6.17.0-generic", "0-3", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if r.Qualified {
		t.Fatal("stock kernel qualified")
	}
}
func TestExistingInstanceFails(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-3", true), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if r.Qualified {
		t.Fatal("host with unknown instance qualified")
	}
}
func TestAPICPolicy(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-3", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if ValidateRequestedAPICs(r, []int{0}, 2) == nil {
		t.Fatal("APIC 0 accepted")
	}
	if ValidateRequestedAPICs(r, []int{2}, 4) == nil {
		t.Fatal("headroom violation accepted")
	}
	if ValidateRequestedAPICs(r, []int{2, 2}, 1) == nil {
		t.Fatal("duplicate APIC ID accepted")
	}
	r.CPUs[1].Online = false
	if ValidateRequestedAPICs(r, []int{2}, 1) == nil {
		t.Fatal("offline APIC ID accepted")
	}
}

func TestAPICPolicyRejectsSMTSplit(t *testing.T) {
	report := Report{CPUs: []CPU{
		{APIC: 0, Physical: 0, Core: 0, Online: true},
		{APIC: 1, Physical: 0, Core: 0, Online: true},
		{APIC: 2, Physical: 0, Core: 1, Online: true},
		{APIC: 3, Physical: 0, Core: 1, Online: true},
	}}
	if err := ValidateRequestedAPICs(report, []int{2}, 2); err == nil {
		t.Fatal("split SMT core accepted")
	}
	if err := ValidateRequestedAPICs(report, []int{2, 3}, 2); err != nil {
		t.Fatalf("whole SMT core rejected: %v", err)
	}
}

func TestProtectedDeviceTopologyAndRejection(t *testing.T) {
	report := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-3", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if len(report.ProtectedDevices) != 2 {
		t.Fatalf("protected devices = %+v", report.ProtectedDevices)
	}
	for _, function := range []string{"0000:00:01.0", "0000:00:03.0", "0000:00:04.0"} {
		if err := ValidateRequestedPCIFunctions(report, []string{function}); err == nil {
			t.Fatalf("protected PCI function %s accepted", function)
		}
	}
	if err := ValidateRequestedPCIFunctions(report, []string{"0000:00:05.0"}); err != nil {
		t.Fatalf("unprotected PCI function rejected: %v", err)
	}
}

func TestOfflineCPUsDoNotCountAsPrimaryHeadroom(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-2", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	if r.Qualified {
		t.Fatal("host with only three online CPUs qualified for four-CPU headroom")
	}
	if r.CPUs[3].Online {
		t.Fatal("offline logical CPU reported online")
	}
}

func TestDuplicateCPUAndAPICMappingsFail(t *testing.T) {
	tests := []struct {
		name    string
		cpuinfo string
	}{
		{"logical", "processor: 0\napicid: 0\n\nprocessor: 0\napicid: 2\n"},
		{"apic", "processor: 0\napicid: 0\n\nprocessor: 1\napicid: 0\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := fixture(t, "7.0.0-mk2-gce-lab", "0-1", false)
			if err := os.WriteFile(filepath.Join(root, "proc/cpuinfo"), []byte(test.cpuinfo), 0644); err != nil {
				t.Fatal(err)
			}
			r := Check(context.Background(), Options{Root: root, MinPrimaryCPUs: 1, MinPrimaryMemByte: 1})
			if r.Qualified {
				t.Fatalf("host with duplicate %s mapping qualified", test.name)
			}
		})
	}
}

func TestTopologyZeroValuesAreEncoded(t *testing.T) {
	r := Check(context.Background(), Options{Root: fixture(t, "7.0.0-mk2-gce-lab", "0-3", false), MinPrimaryCPUs: 4, MinPrimaryMemByte: 1})
	b, err := json.Marshal(r.CPUs[0])
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	if err := json.Unmarshal(b, &encoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"physical_id", "core_id", "numa_node"} {
		if value, ok := encoded[field]; !ok || value != float64(0) {
			t.Fatalf("%s zero value missing from %s", field, b)
		}
	}
}
