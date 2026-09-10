//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

type configuration struct {
	socket, stateDir, subnet, egress string
	mtu, allowedUID                  int
	dns                              network.DNS
}

func parseConfiguration(arguments []string) (configuration, error) {
	var value configuration
	set := flag.NewFlagSet("mknetd", flag.ContinueOnError)
	set.StringVar(&value.socket, "socket", "/run/mknetd.sock", "authenticated Unix RPC socket")
	set.StringVar(&value.stateDir, "state-dir", "/var/lib/mknetd", "durable endpoint state directory")
	set.StringVar(&value.subnet, "subnet", "172.31.0.0/24", "guest IPv4 allocation subnet")
	set.StringVar(&value.egress, "egress", "", "primary egress interface (required)")
	set.IntVar(&value.mtu, "mtu", 1400, "negotiated endpoint MTU")
	set.IntVar(&value.allowedUID, "allowed-uid", 0, "UID authorized to issue CNI requests")
	var nameservers string
	set.StringVar(&nameservers, "dns", "", "comma-separated guest DNS servers")
	if err := set.Parse(arguments); err != nil {
		return value, err
	}
	if set.NArg() != 0 || !filepath.IsAbs(value.socket) || !filepath.IsAbs(value.stateDir) || value.egress == "" || value.allowedUID < 0 {
		return value, errors.New("absolute socket/state-dir, explicit egress, and non-negative allowed-uid are required")
	}
	if len(value.egress) > 15 || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`).MatchString(value.egress) {
		return value, errors.New("egress must be a valid Linux interface name")
	}
	if nameservers != "" {
		for _, server := range strings.Split(nameservers, ",") {
			server = strings.TrimSpace(server)
			if server == "" {
				return value, errors.New("DNS list contains an empty server")
			}
			value.dns.Nameservers = append(value.dns.Nameservers, server)
		}
	}
	return value, nil
}

func run(ctx context.Context, value configuration) error {
	if _, err := (network.ExecRunner{}).Output(ctx, "/usr/sbin/ip", "link", "show", "dev", value.egress); err != nil {
		return fmt.Errorf("validate egress %q: %w", value.egress, err)
	}
	store, err := network.OpenStore(value.stateDir)
	if err != nil {
		return err
	}
	backend := network.LinuxBackend{Egress: value.egress}
	service, err := network.NewService(store, backend, value.subnet, value.mtu, value.dns)
	if err != nil {
		return err
	}
	if err = service.Reconcile(ctx); err != nil {
		return fmt.Errorf("refuse inconsistent durable network state: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(value.socket), 0755); err != nil {
		return err
	}
	server := &network.Server{Service: service, AllowedUID: uint32(value.allowedUID)}
	defer server.Close()
	return server.Listen(ctx, value.socket)
}

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mknetd", os.Args[1:]) {
		return
	}
	value, err := parseConfiguration(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if err = run(ctx, value); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
