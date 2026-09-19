package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
)

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-agent", os.Args[1:]) {
		return
	}
	if len(os.Args) >= 4 && os.Args[1] == "__oci_exec" {
		if err := agent.RunOCIExec(os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, "OCI executor:", err)
			os.Exit(126)
		}
		return
	}
	var id, gen, token, unixSocket string
	var port uint
	var noChroot, mediatedRoot bool
	flag.StringVar(&id, "sandbox-id", "", "sandbox ID")
	flag.StringVar(&gen, "generation", "", "sandbox generation")
	flag.StringVar(&token, "token-hex", "", "256-bit authentication token")
	flag.UintVar(&port, "port", 0, "AF_VSOCK port")
	flag.BoolVar(&noChroot, "test-no-chroot", false, "disable chroot for explicit local tests")
	flag.BoolVar(&mediatedRoot, "mediated-root", false, "remount the mediated ext4 root read-only before shutdown")
	flag.StringVar(&unixSocket, "unix-socket", "", "child-local Unix control socket")
	flag.Parse()
	key, e := hex.DecodeString(token)
	if e != nil || len(key) != 32 || id == "" || gen == "" || port < 1024 {
		fmt.Fprintln(os.Stderr, "sandbox identity, port, and 32-byte token are required")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := &agent.Server{Manager: agent.NewManager(noChroot), SandboxID: id, Generation: gen, Bundle: "/bundle", Endpoint: uint32(port), Token: key}
	shutdown := linuxShutdownPlatform()
	if mediatedRoot {
		s.BeforeShutdown = func() error { return quiesceMediatedRoot(shutdown) }
	}
	if unixSocket == "" {
		fmt.Fprintln(os.Stderr, "--unix-socket is required; direct Go AF_VSOCK is prohibited")
		os.Exit(2)
	}
	listener, listenErr := unixsocket.Listen(unixSocket, 0600)
	if listenErr != nil {
		fmt.Fprintln(os.Stderr, listenErr)
		os.Exit(1)
	}
	e = s.Serve(ctx, listener)
	e = errors.Join(e, listener.Close())
	if errors.Is(e, agent.ErrShutdownRequested) {
		if e = finishGuestShutdown(mediatedRoot, shutdown, os.Stdout); e != nil {
			fmt.Fprintln(os.Stderr, "shutdown:", e)
			os.Exit(1)
		}
		return
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
