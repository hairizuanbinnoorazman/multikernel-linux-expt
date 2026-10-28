//go:build linux

package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// This forensic qualification deliberately avoids OpenStore: opening a legacy
// store can upgrade it. A historical ACTIVE lease must remain historical even
// when the operator independently proves a graceful native close.
func TestLiveRetainedClose0c50de9(t *testing.T) {
	if os.Getenv("MK_LIVE_RETAINED_CLOSE") != "0c50de9" {
		t.Skip("explicit exact-generation GCE qualification only")
	}
	const sid = "mk-fc7012dd2d64b03023855fe12ba09039-979d3d345f108048"
	const gen = "a49b785d3e4b944f8c33237120797712"
	const root = "/opt/mkruntime/g8-host-0c50de9/integration"
	host, err := os.Hostname()
	if err != nil || host != "mklinux-g7-storage-20261012" || os.Geteuid() != 0 {
		t.Fatal("exact root test host required")
	}
	check := func(path, expected string) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != expected {
			t.Fatal("parked generation precondition differs:", path)
		}
	}
	check("/proc/sys/kernel/random/boot_id", "3ae4f0d8-4b8f-4c68-a5d1-b43c90d878a6")
	check(filepath.Join("/sys/fs/multikernel/instances", sid, "status"), "loaded")
	check("/sys/module/mk_transport/refcnt", "0")
	check("/sys/module/mk_transport/srcversion", "0450DCCF7EDC67FAE2A6CAA")
	directory, err := safefile.OpenDirectory(filepath.Join(root, "storage-state"), false)
	if err != nil {
		t.Fatal("protected state directory unavailable")
	}
	defer directory.Close()
	data, found, err := directory.ReadPrivate("state.json", 16<<20)
	if err != nil || !found {
		t.Fatal("protected historical state unavailable")
	}
	var state diskState
	if protocol.StrictDecode(data, &state) != nil || state.Version != StateVersion || validateDiskState(state) != nil {
		t.Fatal("historical state invalid")
	}
	value, ok := state.Exports[exportKey(sid, gen)]
	if !ok || value.State != "ACTIVE" || value.ExportGeneration != "3be602943f90bdb16722e89abffbfbf2" ||
		value.PeerCID != 40 || value.FilesystemUUID != "8bd9f54f-728b-5c10-b747-4f285d75436e" ||
		value.SizeBytes != 2<<30 || value.ImageIdentity != (ImageIdentity{Device: 2064, Inode: 2098847}) {
		t.Fatal("exact historical export differs")
	}
	backend := &LinuxBackend{RuntimeDir: filepath.Join(root, "storage-runtime")}
	_, log := backend.paths(value)
	counters, err := parseCounters(log, value)
	if err != nil {
		t.Fatal("production strict native-close validation failed:", err)
	}
	// Numeric I/O counters are public; state, image paths and guest output are not.
	public, err := json.Marshal(struct {
		Scope                    string   `json:"scope"`
		Counters                 Counters `json:"counters"`
		Synced                   bool     `json:"synced"`
		HistoricalStateUnchanged bool     `json:"historical_state_unchanged"`
	}{"strict-native-close-only", counters, true, true})
	if err != nil {
		t.Fatal(err)
	}
	after, found, err := directory.ReadPrivate("state.json", 16<<20)
	if err != nil || !found || string(after) != string(data) {
		t.Fatal("historical state changed during observation")
	}
	t.Log("NATIVE_CLOSE_QUALIFIED", string(public))
}
