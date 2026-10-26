//go:build linux

package network

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt in only inside an ephemeral network namespace. The outer test runner
// creates it with unshare --net; exit destroys every rule created here.
func TestOrderedFirewallInPrivateNetworkNamespace(t *testing.T) {
	if os.Getenv("MK_TEST_FIREWALL_NAMESPACE") != "1" {
		t.Skip("requires an explicitly isolated network namespace")
	}
	if os.Geteuid() != 0 {
		t.Fatal("private firewall fixture requires root")
	}
	own, err := os.Stat("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Stat("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(own, host) {
		t.Fatal("refuse firewall fixture in the primary network namespace")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	r := ExecRunner{Timeout: time.Second, MaxOutput: 16384}
	tool := "/usr/sbin/iptables"
	chain := "MK-0123456789ab"
	run := func(args ...string) {
		t.Helper()
		if err := r.Run(ctx, tool, append([]string{"-w"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	run("-N", chain)
	rows := [][]string{
		{"!", "-s", "172.31.0.2/32", "-j", "DROP"},
		{"-d", "169.254.169.254/32", "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		{"-d", "169.254.169.254/32", "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		{"-d", "169.254.169.254/32", "-j", "REJECT"},
		{"-o", "mkv+", "-j", "DROP"}, {"-o", "ens4", "-j", "ACCEPT"}, {"-j", "DROP"},
	}
	for _, row := range rows {
		run(append([]string{"-A", chain}, row...)...)
	}
	b := LinuxBackend{IPTables: tool, Egress: "ens4"}
	if err := b.checkEndpointChain(ctx, r, chain, "172.31.0.2/32"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.Output(ctx, tool, "-w", "-S", chain)
	if err != nil {
		t.Fatal(err)
	}
	if string(snapshot) != canonicalEndpointChain {
		t.Fatal("real iptables canonical output differs")
	}
	for _, target := range []string{"ACCEPT", "RETURN"} {
		run("-I", chain, "1", "-j", target)
		for _, row := range rows {
			run(append([]string{"-C", chain}, row...)...)
		}
		if err := b.checkEndpointChain(ctx, r, chain, "172.31.0.2/32"); err == nil || !strings.Contains(err.Error(), "ordered policy") {
			t.Fatalf("%s bypass not refused: %v", target, err)
		}
		t.Log(target + " bypass retains every required rule but ordered check refuses")
		run("-D", chain, "1")
		if err := b.checkEndpointChain(ctx, r, chain, "172.31.0.2/32"); err != nil {
			t.Fatal(err)
		}
	}
	b.IP6Tables = "/usr/sbin/ip6tables"
	checks := []struct{ tool, chain, target string }{
		{tool, "INPUT", "DROP"}, {tool, "FORWARD", chain},
		{b.IP6Tables, "INPUT", "DROP"}, {b.IP6Tables, "FORWARD", "DROP"},
	}
	command := func(tool string, args ...string) {
		t.Helper()
		if err := r.Run(ctx, tool, append([]string{"-w"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range checks {
		command(c.tool, "-A", c.chain, "-i", "mkv0123456789a", "-j", c.target)
	}
	if err := b.checkIngressPrecedence(ctx, r, "mkv0123456789a", chain); err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		for _, target := range []string{"ACCEPT", "RETURN"} {
			command(c.tool, "-I", c.chain, "1", "-j", target)
			command(c.tool, "-C", c.chain, "-i", "mkv0123456789a", "-j", c.target)
			if err := b.checkIngressPrecedence(ctx, r, "mkv0123456789a", chain); err == nil {
				t.Fatal("shadowed ingress guard accepted")
			}
			command(c.tool, "-D", c.chain, "1")
			if err := b.checkIngressPrecedence(ctx, r, "mkv0123456789a", chain); err != nil {
				t.Fatal(err)
			}
			t.Log(c.tool + " " + c.chain + " " + target + " shadow refused despite guard presence; restored")
		}
	}
	t.Log("IPV4_IPV6_INPUT_FORWARD_SHADOW_REFUSAL_AND_RESTORATION_PASS")
	t.Log("PRIVATE_NAMESPACE_REAL_IPTABLES_ORDERED_POLICY_AND_BYPASS_REFUSAL_PASS")
}
