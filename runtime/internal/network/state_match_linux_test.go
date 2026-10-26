//go:build linux

package network

import (
	"context"
	"strings"
	"testing"
)

type stateSnapshotRunner struct {
	inspectingRunner
	match, replacement string
}

func (r *stateSnapshotRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	output, err := r.inspectingRunner.Output(ctx, name, args...)
	if err == nil && strings.Contains(strings.Join(args, " "), r.match) {
		return []byte(r.replacement), nil
	}
	return output, err
}

func TestEndpointCheckRejectsStateSubstringLookalikes(t *testing.T) {
	for _, tc := range []struct{ name, match, output string }{
		{"host mtu suffix", "-o link show dev mkv", "2: mkv0123456789a: <UP> mtu 14000 state UP\n"},
		{"host lower up only", "-o link show dev mkv", "2: mkv0123456789a: <LOWER_UP> mtu 1400 state DOWN\n"},
		{"guest mtu suffix", "-o link show dev eth0", "3: eth0: <UP> mtu 14000 state UP\n"},
		{"guest lower up only", "-o link show dev eth0", "3: eth0: <LOWER_UP> mtu 1400 state DOWN\n"},
		{"route gateway suffix", "route show 172.31.0.0/30", "172.31.0.0/30 via 100.64.0.20 dev mkv0123456789a\n"},
		{"route device suffix", "route show 172.31.0.0/30", "172.31.0.0/30 via 100.64.0.2 dev mkv0123456789ab\n"},
		{"guest address suffix", "-o address show dev eth0", "inet 172.31.0.1/300 scope global eth0\n"},
		{"forwarding suffix", "sysctl -n", "11\n"},
		{"default gateway suffix", "route show default", "default via 100.64.0.10 dev mkhost0\n"},
		{"default device suffix", "route show default", "default via 100.64.0.1 dev mkhost01\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &stateSnapshotRunner{match: tc.match, replacement: tc.output}
			b := LinuxBackend{Runner: r, IP: "ip", IPTables: "iptables", IP6Tables: "ip6tables", Nsenter: "nsenter", Egress: "ens4"}
			if err := b.Check(t.Context(), linuxEndpoint()); err == nil {
				t.Fatal("CHECK accepted a state substring lookalike")
			}
		})
	}
}
