//go:build linux

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var procNetNS = regexp.MustCompile(`^/proc/([0-9]+)/ns/net$`)
var endpointGeneration = regexp.MustCompile(`^[0-9a-f]{32}$`)

type Runner interface {
	Run(context.Context, string, ...string) error
}

type InspectRunner interface {
	Runner
	Output(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

type commandError struct {
	command string
	err     error
	output  string
}

func (e *commandError) Error() string { return e.command + ": " + e.err.Error() + ": " + e.output }
func (e *commandError) Unwrap() error { return e.err }

func (ExecRunner) Run(ctx context.Context, name string, arguments ...string) error {
	_, err := (ExecRunner{}).Output(ctx, name, arguments...)
	return err
}

func (ExecRunner) Output(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, &commandError{command: name + " " + strings.Join(arguments, " "), err: err, output: strings.TrimSpace(string(output))}
	}
	return output, nil
}

type LinuxBackend struct {
	Runner   Runner
	IP       string
	IPTables string
	Nsenter  string
	Egress   string
}

func (b LinuxBackend) defaults() LinuxBackend {
	if b.Runner == nil {
		b.Runner = ExecRunner{}
	}
	if b.IP == "" {
		b.IP = "/usr/sbin/ip"
	}
	if b.IPTables == "" {
		b.IPTables = "/usr/sbin/iptables"
	}
	if b.Nsenter == "" {
		b.Nsenter = "/usr/bin/nsenter"
	}
	return b
}

func endpointTopology(endpoint Endpoint) (hostIf, chain, guestNetwork, transitHost, transitPeer string, err error) {
	if !endpointGeneration.MatchString(endpoint.Generation) {
		return "", "", "", "", "", errors.New("endpoint generation is not a 128-bit lowercase hexadecimal value")
	}
	hostIf = "mkv" + endpoint.Generation[:11]
	chain = "MK-" + endpoint.Generation[:12]
	ip, network, parseErr := net.ParseCIDR(endpoint.Address)
	if parseErr != nil || ip.To4() == nil {
		return "", "", "", "", "", errors.New("endpoint address is not IPv4 CIDR")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones != 30 || !ip.Equal(addIP(network.IP, 2)) {
		return "", "", "", "", "", errors.New("endpoint address must be the second usable address of an IPv4 /30")
	}
	if net.ParseIP(endpoint.Gateway) == nil || !net.ParseIP(endpoint.Gateway).Equal(addIP(network.IP, 1)) {
		return "", "", "", "", "", errors.New("endpoint gateway does not match its IPv4 /30")
	}
	guestNetwork = network.String()
	value := int(ip.To4()[2])<<8 | int(ip.To4()[3])
	slot := value / 4
	third := slot / 64
	fourth := (slot%64)*4 + 1
	if third > 255 {
		return "", "", "", "", "", errors.New("endpoint address exceeds the transit allocation range")
	}
	transitHost = "100.64." + strconv.Itoa(third) + "." + strconv.Itoa(fourth)
	transitPeer = "100.64." + strconv.Itoa(third) + "." + strconv.Itoa(fourth+1)
	return
}

func (b LinuxBackend) ns(endpoint Endpoint, arguments ...string) []string {
	return append([]string{"--net=" + endpoint.NetNS, "--"}, arguments...)
}

func netnsTarget(path string) (string, error) {
	if matches := procNetNS.FindStringSubmatch(path); matches != nil {
		return matches[1], nil
	}
	if filepath.Dir(path) == "/run/netns" || filepath.Dir(path) == "/var/run/netns" {
		name := filepath.Base(path)
		if identifier.MatchString(name) {
			return name, nil
		}
	}
	return "", errors.New("network namespace must be /proc/PID/ns/net or /run/netns/NAME")
}

func (b LinuxBackend) CreateNamespace(ctx context.Context, generation string) (string, error) {
	if !endpointGeneration.MatchString(generation) {
		return "", errors.New("invalid namespace generation")
	}
	b = b.defaults()
	name := "mk-" + generation[:12]
	if err := b.Runner.Run(ctx, b.IP, "netns", "add", name); err != nil {
		return "", err
	}
	return filepath.Join("/run/netns", name), nil
}

func (b LinuxBackend) DeleteNamespace(ctx context.Context, path string) error {
	b = b.defaults()
	target, err := netnsTarget(path)
	if err != nil || procNetNS.MatchString(path) {
		return errors.New("only a named managed namespace can be deleted")
	}
	if err = b.Runner.Run(ctx, b.IP, "netns", "delete", target); err != nil && !resourceAbsent(err) {
		return err
	}
	return nil
}

func (b LinuxBackend) Add(ctx context.Context, endpoint Endpoint) error {
	b = b.defaults()
	if b.Egress == "" {
		return errors.New("mknetd egress interface is required")
	}
	hostIf, chain, guestNetwork, transitHost, transitPeer, err := endpointTopology(endpoint)
	if err != nil {
		return err
	}
	target, err := netnsTarget(endpoint.NetNS)
	if err != nil {
		return err
	}
	peerIf := "mkhost0"
	commands := []struct {
		name string
		args []string
	}{
		{b.IP, []string{"link", "add", hostIf, "type", "veth", "peer", "name", peerIf}},
		{b.IP, []string{"link", "set", peerIf, "netns", target}},
		{b.IP, []string{"address", "add", transitHost + "/30", "dev", hostIf}},
		{b.IP, []string{"link", "set", hostIf, "mtu", strconv.Itoa(endpoint.MTU), "up"}},
		{b.Nsenter, b.ns(endpoint, b.IP, "link", "set", "lo", "up")},
		{b.Nsenter, b.ns(endpoint, b.IP, "address", "add", transitPeer+"/30", "dev", peerIf)},
		{b.Nsenter, b.ns(endpoint, b.IP, "link", "set", peerIf, "mtu", strconv.Itoa(endpoint.MTU), "up")},
		{b.Nsenter, b.ns(endpoint, b.IP, "tuntap", "add", "dev", endpoint.IfName, "mode", "tun")},
		{b.Nsenter, b.ns(endpoint, b.IP, "address", "add", endpoint.Gateway+"/30", "dev", endpoint.IfName)},
		{b.Nsenter, b.ns(endpoint, b.IP, "link", "set", endpoint.IfName, "mtu", strconv.Itoa(endpoint.MTU), "up")},
		{b.Nsenter, b.ns(endpoint, "/usr/sbin/sysctl", "-w", "net.ipv4.ip_forward=1")},
		{b.Nsenter, b.ns(endpoint, b.IP, "route", "replace", "default", "via", transitHost, "dev", peerIf)},
		{b.IP, []string{"route", "add", guestNetwork, "via", transitPeer, "dev", hostIf}},
		{b.IPTables, []string{"-w", "-N", chain}},
		{b.IPTables, []string{"-w", "-A", chain, "!", "-s", guestNetwork, "-j", "DROP"}},
		{b.IPTables, []string{"-w", "-A", chain, "-d", "169.254.169.254/32", "-p", "udp", "--dport", "53", "-j", "ACCEPT"}},
		{b.IPTables, []string{"-w", "-A", chain, "-d", "169.254.169.254/32", "-p", "tcp", "--dport", "53", "-j", "ACCEPT"}},
		{b.IPTables, []string{"-w", "-A", chain, "-d", "169.254.169.254/32", "-j", "REJECT"}},
		{b.IPTables, []string{"-w", "-A", chain, "-o", "mkv+", "-j", "DROP"}},
		{b.IPTables, []string{"-w", "-A", chain, "-o", b.Egress, "-j", "ACCEPT"}},
		{b.IPTables, []string{"-w", "-A", chain, "-j", "DROP"}},
		{b.IPTables, []string{"-w", "-I", "FORWARD", "1", "-i", hostIf, "-j", chain}},
		{b.IPTables, []string{"-w", "-I", "FORWARD", "1", "-i", b.Egress, "-o", hostIf, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{b.IPTables, []string{"-w", "-t", "nat", "-A", "POSTROUTING", "-s", guestNetwork, "-o", b.Egress, "-j", "MASQUERADE"}},
	}
	for _, command := range commands {
		if err = b.Runner.Run(ctx, command.name, command.args...); err != nil {
			_ = b.Delete(context.WithoutCancel(ctx), endpoint)
			return err
		}
	}
	return nil
}

func (b LinuxBackend) Check(ctx context.Context, endpoint Endpoint) error {
	b = b.defaults()
	hostIf, chain, guestNetwork, transitHost, transitPeer, err := endpointTopology(endpoint)
	if err != nil {
		return err
	}
	inspector, ok := b.Runner.(InspectRunner)
	if !ok {
		return errors.New("Linux backend runner cannot inspect endpoint state")
	}
	checks := []struct {
		name string
		args []string
		want []string
	}{
		{b.IP, []string{"-o", "link", "show", "dev", hostIf}, []string{"mtu " + strconv.Itoa(endpoint.MTU), "UP"}},
		{b.IP, []string{"route", "show", guestNetwork}, []string{"via " + transitPeer, "dev " + hostIf}},
		{b.Nsenter, b.ns(endpoint, b.IP, "-o", "address", "show", "dev", endpoint.IfName), []string{endpoint.Gateway + "/30"}},
		{b.Nsenter, b.ns(endpoint, b.IP, "-o", "link", "show", "dev", endpoint.IfName), []string{"mtu " + strconv.Itoa(endpoint.MTU), "UP"}},
		{b.Nsenter, b.ns(endpoint, "/usr/sbin/sysctl", "-n", "net.ipv4.ip_forward"), []string{"1"}},
		{b.Nsenter, b.ns(endpoint, b.IP, "route", "show", "default"), []string{"via " + transitHost, "dev mkhost0"}},
		{b.IPTables, []string{"-w", "-C", "FORWARD", "-i", hostIf, "-j", chain}, nil},
		{b.IPTables, []string{"-w", "-C", chain, "!", "-s", guestNetwork, "-j", "DROP"}, nil},
		{b.IPTables, []string{"-w", "-C", chain, "-d", "169.254.169.254/32", "-p", "udp", "--dport", "53", "-j", "ACCEPT"}, nil},
		{b.IPTables, []string{"-w", "-C", chain, "-d", "169.254.169.254/32", "-j", "REJECT"}, nil},
	}
	for _, check := range checks {
		output, outputErr := inspector.Output(ctx, check.name, check.args...)
		if outputErr != nil {
			return outputErr
		}
		for _, expected := range check.want {
			if !strings.Contains(string(output), expected) {
				return fmt.Errorf("endpoint check %s %s did not contain %q: %s", check.name, strings.Join(check.args, " "), expected, strings.TrimSpace(string(output)))
			}
		}
	}
	return nil
}

func (b LinuxBackend) Delete(ctx context.Context, endpoint Endpoint) error {
	b = b.defaults()
	hostIf, chain, guestNetwork, _, transitPeer, topologyErr := endpointTopology(endpoint)
	if topologyErr != nil {
		return topologyErr
	}
	commands := []struct {
		name string
		args []string
	}{
		{b.IPTables, []string{"-w", "-t", "nat", "-D", "POSTROUTING", "-s", guestNetwork, "-o", b.Egress, "-j", "MASQUERADE"}},
		{b.IPTables, []string{"-w", "-D", "FORWARD", "-i", b.Egress, "-o", hostIf, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{b.IPTables, []string{"-w", "-D", "FORWARD", "-i", hostIf, "-j", chain}},
		{b.IPTables, []string{"-w", "-F", chain}},
		{b.IPTables, []string{"-w", "-X", chain}},
		{b.IP, []string{"route", "del", guestNetwork, "via", transitPeer, "dev", hostIf}},
		{b.IP, []string{"link", "delete", hostIf}},
	}
	var failures []error
	for _, command := range commands {
		if err := b.Runner.Run(ctx, command.name, command.args...); err != nil && !resourceAbsent(err) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func resourceAbsent(err error) bool {
	var commandErr *commandError
	if !errors.As(err, &commandErr) {
		return false
	}
	for _, marker := range []string{
		"Bad rule (does a matching rule exist in that chain?)",
		"No chain/target/match by that name",
		"No such process",
		"Cannot find device",
		"does not exist",
	} {
		if strings.Contains(commandErr.output, marker) {
			return true
		}
	}
	return false
}
