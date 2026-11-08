//go:build linux

// mk-shimctl observes a single owned child engine over its existing shim socket.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func observe(ctx context.Context, socket string, owner inspection.Request) (inspection.Report, error) {
	if owner.Validate() != nil {
		return inspection.Report{}, errors.New("invalid inspection identity")
	}
	socket = strings.TrimPrefix(socket, "unix://")
	path, err := unixsocket.Capture(socket)
	if err != nil {
		return inspection.Report{}, err
	}
	defer path.Close()
	conn, err := path.DialContext(ctx)
	if err != nil {
		return inspection.Report{}, err
	}
	defer conn.Close()
	stop := protocol.CloseOnContext(ctx, conn)
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return inspection.Report{}, err
		}
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return inspection.Report{}, errors.New("inspection socket is not Unix")
	}
	raw, err := u.SyscallConn()
	if err != nil {
		return inspection.Report{}, err
	}
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil || cred.Uid != uint32(os.Geteuid()) {
			peerErr = errors.New("inspection peer owner refused")
		}
	}); err != nil {
		return inspection.Report{}, err
	}
	if peerErr != nil {
		return inspection.Report{}, peerErr
	}
	client := ttrpc.NewClient(conn)
	defer client.Close()
	return inspection.Inspect(ctx, client, owner)
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
