package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var id, gen, token, unixSocket string
	var port uint
	var noChroot bool
	flag.StringVar(&id, "sandbox-id", "", "sandbox ID")
	flag.StringVar(&gen, "generation", "", "sandbox generation")
	flag.StringVar(&token, "token-hex", "", "256-bit authentication token")
	flag.UintVar(&port, "port", 0, "AF_VSOCK port")
	flag.BoolVar(&noChroot, "test-no-chroot", false, "disable chroot for explicit local tests")
	flag.StringVar(&unixSocket, "unix-socket", "", "child-local Unix control socket")
	flag.Parse()
	key, e := hex.DecodeString(token)
	if e != nil || len(key) != 32 || id == "" || gen == "" || port < 1024 {
		fmt.Fprintln(os.Stderr, "sandbox identity, port, and 32-byte token are required")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := &agent.Server{Manager: agent.NewManager(noChroot), SandboxID: id, Generation: gen, Endpoint: uint32(port), Token: key}
	if unixSocket == "" {
		fmt.Fprintln(os.Stderr, "--unix-socket is required; direct Go AF_VSOCK is prohibited")
		os.Exit(2)
	}
	os.Remove(unixSocket)
	listener, listenErr := net.Listen("unix", unixSocket)
	if listenErr != nil {
		fmt.Fprintln(os.Stderr, listenErr)
		os.Exit(1)
	}
	defer listener.Close()
	conn, e := listener.Accept()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer conn.Close()
	if e = s.ServeConn(ctx, conn); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
