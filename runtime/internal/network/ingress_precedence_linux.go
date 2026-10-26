//go:build linux

package network

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var literalIngress = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)

// ingressGuardFirst proves only sampled filter-table precedence for this
// endpoint. A preceding rule is allowed only when its ingress is provably
// disjoint. Unknown syntax is refused; this is not general iptables evaluation.
func ingressGuardFirst(data []byte, chain, hostIf, target string) bool {
	if len(data) > 1<<20 || len(data) == 0 || data[len(data)-1] != '\n' {
		return false
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) > 4096 || len(lines) < 2 || (lines[0] != "-P "+chain+" ACCEPT" && lines[0] != "-P "+chain+" DROP") {
		return false
	}
	guard := "-A " + chain + " -i " + hostIf + " -j " + target
	for _, line := range lines[1:] {
		if line == guard {
			return true
		}
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[0] != "-A" || fields[1] != chain || fields[2] != "-i" || !literalIngress.MatchString(fields[3]) || fields[3] == hostIf {
			return false
		}
		// No duplicate interface match, negation or quoting can override the
		// explicit positive ingress constraint. Unrecognized cases fail closed.
		for _, field := range fields[4:] {
			if field == "-i" || field == "--in-interface" || field == "!" || strings.ContainsAny(field, "\"'\\") {
				return false
			}
		}
	}
	return false
}

func (b LinuxBackend) checkIngressPrecedence(ctx context.Context, inspector InspectRunner, hostIf, privateChain string) error {
	for _, check := range []struct{ tool, chain, target string }{
		{b.IPTables, "INPUT", "DROP"}, {b.IPTables, "FORWARD", privateChain},
		{b.IP6Tables, "INPUT", "DROP"}, {b.IP6Tables, "FORWARD", "DROP"},
	} {
		output, err := inspector.Output(ctx, check.tool, "-w", "-S", check.chain)
		if err != nil {
			return err
		}
		if !ingressGuardFirst(output, check.chain, hostIf, check.target) {
			return errors.New("endpoint ingress guard precedence is not established")
		}
	}
	return nil
}
