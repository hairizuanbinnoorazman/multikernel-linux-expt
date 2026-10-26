//go:build linux

package network

import (
	"context"
	"strings"
	"testing"
)

const canonicalEndpointChain = `-N MK-0123456789ab
-A MK-0123456789ab ! -s 172.31.0.2/32 -j DROP
-A MK-0123456789ab -d 169.254.169.254/32 -p udp -m udp --dport 53 -j ACCEPT
-A MK-0123456789ab -d 169.254.169.254/32 -p tcp -m tcp --dport 53 -j ACCEPT
-A MK-0123456789ab -d 169.254.169.254/32 -j REJECT --reject-with icmp-port-unreachable
-A MK-0123456789ab -o mkv+ -j DROP
-A MK-0123456789ab -o ens4 -j ACCEPT
-A MK-0123456789ab -j DROP
`

type chainSnapshotRunner struct {
	inspectingRunner
	snapshot string
}

func (r *chainSnapshotRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if strings.Join(args, " ") == "-w -S MK-0123456789ab" {
		return []byte(r.snapshot), nil
	}
	return r.inspectingRunner.Output(ctx, name, args...)
}

func TestEndpointCheckRejectsPolicyBypassDespiteRulePresence(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(canonicalEndpointChain), "\n")
	swapped := append([]string(nil), lines...)
	swapped[4], swapped[6] = swapped[6], swapped[4]
	for name, snapshot := range map[string]string{
		"canonical":            canonicalEndpointChain,
		"early accept":         strings.Replace(canonicalEndpointChain, lines[1], "-A MK-0123456789ab -j ACCEPT\n"+lines[1], 1),
		"late metadata reject": strings.Join(swapped, "\n") + "\n",
		"duplicate":            canonicalEndpointChain + lines[6] + "\n",
		"return":               strings.Replace(canonicalEndpointChain, lines[7], "-A MK-0123456789ab -j RETURN", 1),
		"different source":     strings.ReplaceAll(canonicalEndpointChain, "172.31.0.2/32", "172.31.0.3/32"),
		"empty":                "",
		"truncated":            strings.TrimSuffix(canonicalEndpointChain, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			r := &chainSnapshotRunner{snapshot: snapshot}
			b := LinuxBackend{Runner: r, IP: "ip", IPTables: "iptables", IP6Tables: "ip6tables", Nsenter: "nsenter", Egress: "ens4"}
			err := b.Check(t.Context(), linuxEndpoint())
			if (err == nil) != (name == "canonical") {
				t.Fatalf("policy check accepted=%v: %v", err == nil, err)
			}
			for _, call := range r.calls {
				if strings.Contains(call, " -A ") || strings.Contains(call, " -I ") || strings.Contains(call, " -D ") {
					t.Fatal("inspection mutated firewall")
				}
			}
		})
	}
}
