//go:build linux

package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// Explicit opt-in only. All firewall/link mutations occur in a fresh network
// namespace, with a second disposable namespace representing the endpoint.
func TestLinuxBackendPrimaryAccessPrivileged(t *testing.T) {
	if os.Getenv("MK_G7_PRIVATE_NETWORK_FIXTURE") == "1" {
		runPrimaryAccessFixture(t)
		return
	}
	if os.Getenv("MK_G7_PRIVILEGED_NETWORK_TEST") != "1" {
		t.Skip("requires explicit MK_G7_PRIVILEGED_NETWORK_TEST=1 on a disposable host")
	}
	if os.Geteuid() != 0 {
		t.Fatal("privileged network fixture requires root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLinuxBackendPrimaryAccessPrivileged$", "-test.v")
	cmd.Env = append(os.Environ(), "MK_G7_PRIVATE_NETWORK_FIXTURE=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	output, err := cmd.CombinedOutput()
	t.Logf("isolated fixture:\n%s", output)
	if err != nil {
		t.Fatalf("private network fixture: %v", err)
	}
}

func TestLinuxBackendPrimaryAccessProbe(t *testing.T) {
	address := os.Getenv("MK_G7_NETWORK_PROBE_ADDRESS")
	if address == "" {
		t.Skip("helper for isolated primary-access fixture")
	}
	connection, err := net.DialTimeout("tcp4", address, 300*time.Millisecond)
	if os.Getenv("MK_G7_NETWORK_PROBE_BLOCKED") == "1" {
		if connection != nil {
			connection.Close()
		}
		timeout, ok := err.(net.Error)
		if !ok || !timeout.Timeout() {
			t.Fatalf("primary TCP must time out under guard; got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unguarded positive-control connection: %v", err)
	}
	connection.Close()
}

func runPrimaryAccessFixture(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runner := ExecRunner{Timeout: 2 * time.Second}
	for _, args := range [][]string{
		{"link", "set", "lo", "up"},
		{"link", "add", "ens4", "type", "dummy"},
		{"address", "add", "192.0.2.1/24", "dev", "ens4"},
		{"link", "set", "ens4", "up"},
	} {
		if err := runner.Run(ctx, "/usr/sbin/ip", args...); err != nil {
			t.Fatal(err)
		}
	}
	holder := exec.CommandContext(ctx, "/usr/bin/sleep", "20")
	holder.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill(); _ = holder.Wait() }()
	endpoint := linuxEndpoint()
	endpoint.NetNS = "/proc/" + strconv.Itoa(holder.Process.Pid) + "/ns/net"
	backend := LinuxBackend{Runner: runner, Egress: "ens4"}
	defer func() {
		if err := backend.Delete(context.Background(), endpoint); err != nil {
			t.Errorf("endpoint cleanup: %v", err)
		}
	}()
	if err := backend.Add(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	if err := backend.Check(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "192.0.2.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	probe := func(blocked bool) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "/usr/bin/nsenter", "--net="+endpoint.NetNS, "--", os.Args[0], "-test.run=^TestLinuxBackendPrimaryAccessProbe$")
		cmd.Env = append(os.Environ(), "MK_G7_NETWORK_PROBE_ADDRESS="+listener.Addr().String(), fmt.Sprintf("MK_G7_NETWORK_PROBE_BLOCKED=%d", map[bool]int{false: 0, true: 1}[blocked]))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("probe blocked=%t: %v\n%s", blocked, err, output)
		}
	}
	probe(true)
	// Reproduce the missing-INPUT-policy route only inside this synthetic
	// namespace. A successful control proves the blocked listener was reachable.
	if err := runner.Run(ctx, "/usr/sbin/iptables", "-w", "-D", "INPUT", "-i", "mkv0123456789a", "-j", "DROP"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Check(ctx, endpoint); err == nil {
		t.Fatal("CHECK accepted a missing primary guard")
	}
	probe(false)
	if err := runner.Run(ctx, "/usr/sbin/iptables", "-w", "-I", "INPUT", "1", "-i", "mkv0123456789a", "-j", "DROP"); err != nil {
		t.Fatal(err)
	}
	probe(true)
	if err := backend.Delete(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	for _, guard := range []struct{ tool, chain string }{
		{"/usr/sbin/iptables", "INPUT"}, {"/usr/sbin/ip6tables", "INPUT"}, {"/usr/sbin/ip6tables", "FORWARD"},
	} {
		if err := runner.Run(ctx, guard.tool, "-w", "-C", guard.chain, "-i", "mkv0123456789a", "-j", "DROP"); err == nil {
			t.Fatalf("cleanup retained %s %s guard", guard.tool, guard.chain)
		}
	}
	if err := runner.Run(ctx, "/usr/sbin/ip", "link", "show", "mkv0123456789a"); err == nil {
		t.Fatal("cleanup retained endpoint link")
	}
	t.Log("primary access blocked; unguarded positive control passed; CHECK fails closed; guards and link reclaimed")
}
