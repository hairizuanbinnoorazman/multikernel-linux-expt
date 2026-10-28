//go:build linux

package main

import (
	"context"
	"encoding/json"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/hostconfig"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/poolinspect"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Allocates only a CREATED, unloaded reservation through the production
// lifecycle. It never loads/starts a child, invokes Docker or swaps a module.
// Failed mutation keeps private state/resources for inspection, never retries.
func TestLivePoolCapacityGCE(t *testing.T) {
	if os.Getenv("MK_LIVE_POOL_CAPACITY") != "0f3e3a52" {
		t.Skip("explicit pinned GCE allocation qualification only")
	}
	host, _ := os.Hostname()
	if os.Geteuid() != 0 || host != "mklinux-g7-storage-20261012" {
		t.Fatal("exact root GCE host required")
	}
	check := func(path, expected string) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != expected {
			t.Fatal("host precondition differs:", path)
		}
	}
	check("/proc/sys/kernel/random/boot_id", "0f3e3a52-fe01-4c15-a907-237d67cd19b2")
	check("/sys/module/mk_transport/srcversion", "65C55BB38C669FF94A4B772")
	check("/sys/module/mk_transport/refcnt", "0")
	check("/sys/devices/system/cpu/online", "0-15")
	root := os.Getenv("MK_POOL_CAPACITY_ROOT")
	if !regexp.MustCompile(`^/opt/mkruntime/g8-pool-capacity-[a-f0-9]{7,40}$`).MatchString(root) {
		t.Fatal("protected work generation required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	ids, err := doctor.KernelInventory(ctx, "/sys/fs/multikernel")
	if err != nil || len(ids) != 0 {
		t.Fatal("kernel inventory is not idle")
	}
	if _, err = poolinspect.Read(ctx, "/sys/fs/multikernel"); err != poolinspect.ErrCPUFreeListUnavailable {
		t.Fatal("idle free-list precondition differs")
	}
	private := filepath.Join(root, "private")
	if err = os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(filepath.Join(private, "state"))
	if err != nil {
		t.Fatal("private store unavailable")
	}
	defer store.Close()
	backend := &kerf.CLI{Path: "/opt/mkruntime/kerf-venv/bin/kerf", Sysfs: "/sys/fs/multikernel", PoolCPUs: []int{8, 9, 10, 11, 12, 13, 14, 15}, PoolMemory: "8GB", Timeout: 30 * time.Second}
	svc := lifecycle.New(store, backend, lifecycle.Artifacts{})
	svc.SetPoolCPUs(backend.PoolCPUs)
	svc.SetPoolMemory(8<<30, 1<<30)
	sock := filepath.Join(private, "daemon.sock")
	server := &daemon.Server{Service: svc, AllowedUID: 0}
	served := make(chan error, 1)
	go func() { served <- server.Listen(ctx, sock) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err = daemon.InspectPoolOwnership(ctx, daemon.Client{Path: sock, Timeout: time.Second})
		if err == nil {
			break
		}
		select {
		case <-served:
			t.Fatal("private daemon listener failed")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("private daemon did not become available")
		}
		time.Sleep(20 * time.Millisecond)
	}
	config, err := hostconfig.Load("/etc/mkruntime/config.json")
	if err != nil {
		t.Fatal("primary configuration unavailable")
	}
	config.SocketPath = sock
	config.StateDirectory = filepath.Join(private, "state")
	configData, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(private, "host.json")
	if err = os.WriteFile(configPath, configData, 0600); err != nil {
		t.Fatal(err)
	}
	publish := func(name string, v any) {
		t.Helper()
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(append(data, '\n'))
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal("public metadata publication failed")
		}
	}
	observe := func(name, status, code string, wantBytes uint64, wantCPUs int) {
		t.Helper()
		command := exec.CommandContext(ctx, filepath.Join(root, "bin/mkruntimed"), "--config="+configPath, "--doctor-pool")
		var stderr strings.Builder
		command.Stderr = &stderr
		out, runErr := command.Output()
		if len(out) > 1<<20 || stderr.Len() != 0 {
			t.Fatal("CLI output bound/redaction differs")
		}
		var report doctor.PoolReport
		if protocol.StrictDecode(out, &report) != nil || report.Status != status || report.AdmissionGuaranteed {
			t.Fatal("CLI report differs")
		}
		if status == "consistent" {
			if runErr != nil || report.MemoryCandidateUpperBoundBytes != wantBytes || len(report.CPUCandidates) != wantCPUs {
				t.Fatal("capacity observation differs")
			}
		} else if runErr == nil || len(report.Findings) != 1 || report.Findings[0].Code != code || report.Kernel != nil || report.Ownership != nil {
			t.Fatal("inconclusive CLI observation differs")
		}
		publish(name, report)
	}
	observe("before.json", "inconclusive", "KERNEL_CPU_FREE_LIST_UNAVAILABLE", 0, 0)
	bundle := filepath.Join(private, "bundle")
	if err = os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	var identity unix.Stat_t
	if unix.Stat(bundle, &identity) != nil {
		t.Fatal("bundle identity unavailable")
	}
	// Exclusive durable mutation intent precedes any kernel/pool mutation.
	publish("mutation-requested.json", map[string]any{"boot_id": "0f3e3a52-fe01-4c15-a907-237d67cd19b2", "action": "production-create-unloaded-reservation", "child_start_performed": false})
	directory, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	err = directory.Sync()
	directory.Close()
	if err != nil {
		t.Fatal("intent directory sync failed")
	}
	cfg := protocol.SandboxConfig{SchemaVersion: 1, ID: "g8-pool-capacity-20261028", CPUs: []int{8, 9, 10, 11}, MemoryBytes: 3 << 30, KernelManifest: "gce-mk2", Bundle: bundle, BundleIdentity: protocol.DirectoryIdentity{Device: uint64(identity.Dev), Inode: identity.Ino, UID: 0}, AgentPort: 7599, ChildCID: 50}
	created, failure := svc.Create(ctx, cfg, "g8-capacity-created-only")
	if failure != nil {
		t.Fatal("reservation create failed; retain exact private state")
	}
	statusData, statusErr := os.ReadFile("/sys/fs/multikernel/instances/" + cfg.ID + "/status")
	status := strings.TrimSpace(string(statusData))
	if statusErr != nil || (status != "created" && status != "ready") {
		t.Fatal("created reservation is not in a production-mapped CREATED kernel state")
	}
	observe("allocated.json", "consistent", "", 4<<30, 4)
	if _, failure = svc.Delete(ctx, cfg.ID, created.Sandbox.Generation, "g8-capacity-delete-created"); failure != nil {
		t.Fatal("exact created-reservation delete failed; retain private state")
	}
	observe("returned-to-pool.json", "consistent", "", 7<<30, 8)
	if svc.ReleaseIdlePool(ctx) != nil {
		t.Fatal("idle pool release failed; retain state")
	}
	observe("after.json", "inconclusive", "KERNEL_CPU_FREE_LIST_UNAVAILABLE", 0, 0)
	ids, err = doctor.KernelInventory(ctx, "/sys/fs/multikernel")
	if err != nil || len(ids) != 0 {
		t.Fatal("kernel reservation remains")
	}
	check("/sys/devices/system/cpu/online", "0-15")
	check("/sys/module/mk_transport/refcnt", "0")
	cancel()
	select {
	case err = <-served:
		if err != nil {
			t.Fatal("private listener cleanup failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("private listener did not stop")
	}
	publish("result.json", map[string]any{"created_reservation_deleted": true, "pool_released": true, "child_load_or_start_performed": false, "docker_workload_tested": false, "full_g7_g8_closed": false})
	t.Log("REAL_CREATED_RESERVATION_POOL_CAPACITY_RPCS_AND_CLI_PASS_NO_CHILD_BOOT_DOCKER_OR_MODULE_CHANGE")
}
