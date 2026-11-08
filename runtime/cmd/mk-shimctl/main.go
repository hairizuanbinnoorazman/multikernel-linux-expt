//go:build linux

// mk-shimctl observes a single owned child engine over its existing shim socket.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
)

func observe(ctx context.Context, socket string, owner inspection.Request) (inspection.Report, error) {
	return inspection.ObserveSocket(ctx, socket, owner)
}

func run(ctx context.Context, args []string, stdout io.Writer) int {
	flags := flag.NewFlagSet("mk-shimctl", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var socket string
	owner := inspection.Request{Version: 1}
	flags.StringVar(&socket, "socket", "", "existing protected shim socket")
	flags.StringVar(&owner.Namespace, "namespace", "", "containerd namespace")
	flags.StringVar(&owner.ContainerID, "container-id", "", "owned task identity")
	flags.StringVar(&owner.SandboxID, "sandbox-id", "", "expected durable sandbox identity")
	flags.StringVar(&owner.Generation, "generation", "", "expected durable generation")
	if flags.Parse(args) != nil || flags.NArg() != 0 || socket == "" || owner.Validate() != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	report, err := observe(ctx, socket, owner)
	if err != nil {
		return 1
	}
	if json.NewEncoder(stdout).Encode(report) != nil || report.Observation.Status != "responding" {
		return 1
	}
	return 0
}
func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-shimctl", os.Args[1:]) {
		return
	}
	code := run(context.Background(), os.Args[1:], os.Stdout)
	if code != 0 {
		fmt.Fprintln(os.Stderr, "Docker engine inspection unavailable")
	}
	os.Exit(code)
}
