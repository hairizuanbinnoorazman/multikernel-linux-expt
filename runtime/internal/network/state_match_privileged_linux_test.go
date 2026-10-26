//go:build linux

package network

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Mutations occur only in fresh outer/endpoint network namespaces. No real
// primary interface, firewall, route or installed service is changed.
func TestLinuxBackendStateValuesPrivileged(t *testing.T) {
	if os.Getenv("MK_G8_STATE_VALUES_CHILD") != "1" {
		if os.Getenv("MK_G7_PRIVILEGED_NETWORK_TEST") != "1" {
			t.Skip("requires explicit disposable-host network fixture opt-in")
		}
		if os.Geteuid() != 0 {
			t.Fatal("requires root")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLinuxBackendStateValuesPrivileged$", "-test.v")
		cmd.Env = append(os.Environ(), "MK_G8_STATE_VALUES_CHILD=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
		out, err := cmd.CombinedOutput()
		t.Logf("private state-value controls:\n%s", out)
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 80*time.Second)
	defer cancel()
	runner := ExecRunner{Timeout: 2 * time.Second}
	run := func(name string, args ...string) {
		t.Helper()
		if err := runner.Run(ctx, name, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"link", "set", "lo", "up"}, {"link", "add", "ens4", "type", "dummy"}, {"link", "set", "ens4", "up"}} {
		run("/usr/sbin/ip", args...)
	}
	holder := exec.CommandContext(ctx, "/usr/bin/sleep", "80")
	holder.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Process.Kill(); _ = holder.Wait() }()
	endpoint := linuxEndpoint()
	endpoint.NetNS = "/proc/" + strconv.Itoa(holder.Process.Pid) + "/ns/net"
	backend := LinuxBackend{Runner: runner, Egress: "ens4"}
	defer func() {
		if err := backend.Delete(context.WithoutCancel(ctx), endpoint); err != nil {
			t.Error(err)
		}
	}()
	if err := backend.Add(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	if err := backend.Check(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	hostIf, _, guestNetwork, _, _, err := endpointTopology(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                     string
		namespace                bool
		mutate, inspect, restore []string
		oldExpected              string
	}{
		{"host MTU suffix", false, []string{"link", "set", hostIf, "mtu", "14000"}, []string{"-o", "link", "show", "dev", hostIf}, []string{"link", "set", hostIf, "mtu", "1400"}, "mtu 1400"},
		{"guest MTU suffix", true, []string{"link", "set", endpoint.IfName, "mtu", "14000"}, []string{"-o", "link", "show", "dev", endpoint.IfName}, []string{"link", "set", endpoint.IfName, "mtu", "1400"}, "mtu 1400"},
		{"host gateway suffix", false, []string{"route", "replace", guestNetwork, "via", "100.64.0.20", "dev", hostIf, "onlink"}, []string{"route", "show", guestNetwork}, []string{"route", "replace", guestNetwork, "via", "100.64.0.2", "dev", hostIf}, "via 100.64.0.2"},
		{"guest gateway suffix", true, []string{"route", "replace", "default", "via", "100.64.0.10", "dev", "mkhost0", "onlink"}, []string{"route", "show", "default"}, []string{"route", "replace", "default", "via", "100.64.0.1", "dev", "mkhost0"}, "via 100.64.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := "/usr/sbin/ip"
			mutate, inspect, restore := tc.mutate, tc.inspect, tc.restore
			if tc.namespace {
				name = "/usr/bin/nsenter"
				mutate = backend.ns(endpoint, append([]string{"/usr/sbin/ip"}, mutate...)...)
				inspect = backend.ns(endpoint, append([]string{"/usr/sbin/ip"}, inspect...)...)
				restore = backend.ns(endpoint, append([]string{"/usr/sbin/ip"}, restore...)...)
			}
			run(name, mutate...)
			defer run(name, restore...)
			out, err := runner.Output(ctx, name, inspect...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), tc.oldExpected) {
				t.Fatal("real state does not reproduce the old substring acceptance")
			}
			if err := backend.Check(ctx, endpoint); err == nil {
				t.Fatal("CHECK accepted real state lookalike")
			}
			run(name, restore...)
			if err := backend.Check(ctx, endpoint); err != nil {
				t.Fatalf("restored state rejected: %v", err)
			}
			t.Log("old substring accepts, corrected CHECK refuses, restored CHECK passes")
		})
	}
}
