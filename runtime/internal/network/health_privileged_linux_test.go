//go:build linux

package network

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// All link/firewall mutations run in a fresh network namespace, never the host.
func TestNetworkHealthNativeFaultPrivileged(t *testing.T) {
	if os.Getenv("MK_G8_HEALTH_PRIVATE_CHILD") != "1" {
		if os.Getenv("MK_G7_PRIVILEGED_NETWORK_TEST") != "1" {
			t.Skip("requires explicit disposable-host network fixture opt-in")
		}
		if os.Geteuid() != 0 {
			t.Fatal("requires root")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNetworkHealthNativeFaultPrivileged$", "-test.v")
		cmd.Env = append(os.Environ(), "MK_G8_HEALTH_PRIVATE_CHILD=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
		out, err := cmd.CombinedOutput()
		t.Logf("isolated health fixture:\n%s", out)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	if os.Geteuid() != 0 {
		t.Fatal("requires root")
	}
	own, err := os.Stat("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	primary, err := os.Stat("/proc/1/ns/net")
	if err != nil || os.SameFile(own, primary) {
		t.Fatal("refuse native health fault in primary network namespace")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	runner := ExecRunner{Timeout: time.Second, MaxOutput: 1 << 20}
	run := func(args ...string) {
		t.Helper()
		if err := runner.Run(ctx, args[0], args[1:]...); err != nil {
			t.Fatal(err)
		}
	}
	run("/usr/sbin/ip", "link", "set", "lo", "up")
	run("/usr/sbin/ip", "link", "add", "ens4", "type", "dummy")
	run("/usr/sbin/ip", "link", "set", "ens4", "up")
	holder := exec.CommandContext(ctx, "/usr/bin/sleep", "45")
	holder.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill(); _ = holder.Wait() }()
	e := linuxEndpoint()
	e.NetNS = "/proc/" + strconv.Itoa(holder.Process.Pid) + "/ns/net"
	e.Owner = "cni"
	e.State = "READY"
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	e.PrimaryBootID = strings.TrimSpace(string(boot))
	backend := LinuxBackend{Runner: runner, Egress: "ens4"}
	if err := backend.Add(ctx, e); err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := backend.Delete(closeCtx, e); err != nil {
			t.Error(err)
		}
	}()
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(e); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(store.dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(store, backend, "172.31.0.0/24", 1400, DNS{}, e.PrimaryBootID)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: svc, AllowedUID: 0}
	defer server.Close()
	socket := filepath.Join(dir, "network.sock")
	done := make(chan error, 1)
	go func() { done <- server.Listen(ctx, socket) }()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Lstat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket unavailable")
		}
		time.Sleep(5 * time.Millisecond)
	}
	inspect := func(want string) {
		t.Helper()
		r, err := InspectHealth(ctx, Client{Path: socket})
		if err != nil || r.Validate() != nil || r.Status != want || len(r.Endpoints) != 1 || r.Endpoints[0].EndpointGeneration != e.Generation {
			t.Fatalf("health=%+v error=%v", r, err)
		}
	}
	inspect("checked")
	run("/usr/sbin/iptables", "-w", "-D", "INPUT", "-i", "mkv0123456789a", "-j", "DROP")
	inspect("degraded")
	if err := runner.Run(ctx, "/usr/sbin/iptables", "-w", "-C", "INPUT", "-i", "mkv0123456789a", "-j", "DROP"); err == nil {
		t.Fatal("health repaired missing guard")
	}
	run("/usr/sbin/iptables", "-w", "-I", "INPUT", "1", "-i", "mkv0123456789a", "-j", "DROP")
	inspect("checked")
	after, err := os.ReadFile(filepath.Join(store.dir, "state.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("health changed durable state")
	}
	server.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
	t.Log("NATIVE_HEALTH_RPC_CHECKED_MISSING_PRIMARY_GUARD_DEGRADED_WITHOUT_REPAIR_RESTORED_CHECKED_DURABLE_STATE_UNCHANGED")
}
