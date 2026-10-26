//go:build linux

package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

// checkEndpointChain verifies the entire ordered private chain. Individual -C
// probes cannot detect a bypass before otherwise-present deny rules. This is a
// sampled read, not an atomic firewall attestation or a host-global policy check.
func (b LinuxBackend) checkEndpointChain(ctx context.Context, inspector InspectRunner, chain, source string) error {
	output, err := inspector.Output(ctx, b.IPTables, "-w", "-S", chain)
	if err != nil {
		return err
	}
	expected := fmt.Sprintf(`-N %[1]s
-A %[1]s ! -s %[2]s -j DROP
-A %[1]s -d 169.254.169.254/32 -p udp -m udp --dport 53 -j ACCEPT
-A %[1]s -d 169.254.169.254/32 -p tcp -m tcp --dport 53 -j ACCEPT
-A %[1]s -d 169.254.169.254/32 -j REJECT --reject-with icmp-port-unreachable
-A %[1]s -o mkv+ -j DROP
-A %[1]s -o %[3]s -j ACCEPT
-A %[1]s -j DROP
`, chain, source, b.Egress)
	if !bytes.Equal(output, []byte(expected)) {
		return errors.New("endpoint firewall chain differs from the required ordered policy")
	}
	return nil
}
