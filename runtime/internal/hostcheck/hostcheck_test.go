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
		"boot/config-" + release: "CONFIG_MULTIKERNEL=y\nCONFIG_KEXEC_CORE=y\nCONFIG_KEXEC_FILE=y\nCONFIG_MKTTY=y\n",
		"proc/cpuinfo":           "processor: 0\napicid: 0\nphysical id: 0\ncore id: 0\n\nprocessor: 1\napicid: 2\nphysical id: 0\ncore id: 1\n\nprocessor: 2\napicid: 4\nphysical id: 0\ncore id: 2\n\nprocessor: 3\napicid: 6\nphysical id: 0\ncore id: 3\n",
	}
	for p, v := range files {
		full := filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(full), 0755)
		os.WriteFile(full, []byte(v), 0644)
	}
	os.MkdirAll(filepath.Join(root, "sys/fs/multikernel/instances"), 0755)
	os.MkdirAll(filepath.Join(root, "sys/devices/system/cpu/cpu0/node0"), 0755)
	if instances {
		os.Mkdir(filepath.Join(root, "sys/fs/multikernel/instances/foreign"), 0755)
	}
	return root
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
