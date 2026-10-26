//go:build linux

package network

import (
	"context"
	"strings"
	"testing"
)

type ingressSnapshotRunner struct {
	inspectingRunner
	tool, chain, prefix string
}

func (r *ingressSnapshotRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == r.tool && strings.Join(args, " ") == "-w -S "+r.chain {
		target := "DROP"
		if name == "iptables" && r.chain == "FORWARD" {
			target = "MK-0123456789ab"
		}
		return []byte("-P " + r.chain + " ACCEPT\n" + r.prefix + "-A " + r.chain + " -i mkv0123456789a -j " + target + "\n"), nil
	}
	return r.inspectingRunner.Output(ctx, name, args...)
}
func TestEndpointCheckRejectsIngressGuardShadowing(t *testing.T) {
	for _, tool := range []string{"iptables", "ip6tables"} {
		for _, chain := range []string{"INPUT", "FORWARD"} {
			for _, tc := range []struct {
				name, prefix string
				safe         bool
			}{
				{name: "first guard", safe: true},
				{name: "disjoint sibling", prefix: "-i mkvabcdef01234 -j ACCEPT", safe: true},
				{name: "disjoint egress", prefix: "-i ens4 -o mkv0123456789a -j ACCEPT", safe: true},
				{name: "broad accept", prefix: "-j ACCEPT"},
				{name: "broad return", prefix: "-j RETURN"},
				{name: "same ingress", prefix: "-i mkv0123456789a -j ACCEPT"},
				{name: "wildcard ingress", prefix: "-i mkv+ -j ACCEPT"},
				{name: "negated ingress", prefix: "! -i ens4 -j ACCEPT"},
				{name: "duplicate ingress", prefix: "-i ens4 -i mkv0123456789a -j ACCEPT"},
			} {
				t.Run(tool+"/"+chain+"/"+tc.name, func(t *testing.T) {
					prefix := ""
					if tc.prefix != "" {
						prefix = "-A " + chain + " " + tc.prefix + "\n"
					}
					r := &ingressSnapshotRunner{tool: tool, chain: chain, prefix: prefix}
					b := LinuxBackend{Runner: r, IP: "ip", IPTables: "iptables", IP6Tables: "ip6tables", Nsenter: "nsenter", Egress: "ens4"}
					err := b.Check(t.Context(), linuxEndpoint())
					if (err == nil) != tc.safe {
						t.Fatalf("safe=%v check=%v", tc.safe, err)
					}
				})
			}
		}
	}
}
