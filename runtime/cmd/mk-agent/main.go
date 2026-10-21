package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/dockerengine"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"golang.org/x/sys/unix"
)

func main() {
	if buildinfo.PrintRequested(os.Stdout, "mk-agent", os.Args[1:]) {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "__docker_bootstrap" {
		if err := dockerengine.ExecBootstrap(); err != nil {
			fmt.Fprintln(os.Stderr, "Docker bootstrap:", err)
			os.Exit(126)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "__oci_exec_fd" {
		if err := agent.RunUnprivilegedOCIExec(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OCI credential executor:", err)
			os.Exit(126)
		}
		return
	}
	if len(os.Args) >= 4 && os.Args[1] == "__oci_exec" {
		if err := agent.RunOCIExec(os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, "OCI executor:", err)
			os.Exit(126)
		}
		return
	}
	var id, gen, tokenFile, unixSocket string
	var port uint
	var noChroot, mediatedRoot, dockerGuest bool
	flag.StringVar(&id, "sandbox-id", "", "sandbox ID")
	flag.StringVar(&gen, "generation", "", "sandbox generation")
	flag.StringVar(&tokenFile, "token-file", "", "private one-shot authentication credential file")
	flag.UintVar(&port, "port", 0, "AF_VSOCK port")
	flag.BoolVar(&noChroot, "test-no-chroot", false, "disable chroot for explicit local tests")
	flag.BoolVar(&dockerGuest, "docker-guest", false, "supervise the fixed approved outer-root Docker bootstrap")
	flag.BoolVar(&mediatedRoot, "mediated-root", false, "remount the mediated ext4 root read-only before shutdown")
	flag.StringVar(&unixSocket, "unix-socket", "", "child-local Unix control socket")
	flag.Parse()
	if id == "" || gen == "" || port < 1024 || tokenFile == "" {
		fmt.Fprintln(os.Stderr, "sandbox identity, port, and private credential file are required")
		os.Exit(2)
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		fmt.Fprintln(os.Stderr, "agent process protection failed")
		os.Exit(1)
	}
	key, e := consumeAgentToken(tokenFile)
	if e != nil {
		fmt.Fprintln(os.Stderr, "private agent credential validation failed")
		os.Exit(2)
	}
	if unixSocket == "" {
		fmt.Fprintln(os.Stderr, "--unix-socket is required; direct Go AF_VSOCK is prohibited")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := &agent.Server{Manager: agent.NewManager(noChroot), SandboxID: id, Generation: gen, Bundle: "/bundle", Endpoint: uint32(port), Token: key}
	shutdown := linuxShutdownPlatform()
	var stopDocker func() error
	var storageQuiesce func() error
	if mediatedRoot {
		storageQuiesce = func() error { return quiesceMediatedRoot(shutdown, os.Stdout) }
	}
	if dockerGuest {
		if noChroot {
			fmt.Fprintln(os.Stderr, "Docker service requires the trusted guest root")
			os.Exit(2)
		}
		s.Manager = agent.NewDockerGuestManager()
		service, startErr := startDockerGuest(ctx)
		if startErr != nil {
			if service != nil {
				_ = stopDockerBeforeStorage(service.Stop, nil)
			}
			fmt.Fprintln(os.Stderr, "Docker guest startup failed")
			os.Exit(1)
		}
		stopDocker = func() error { return stopDockerBeforeStorage(service.Stop, nil) }
		s.BeforeShutdown = func() error { return stopDockerBeforeStorage(service.Stop, storageQuiesce) }
		go func() {
			select {
			case <-service.UnexpectedExit():
				cancel()
			case <-ctx.Done():
			}
		}()
		defer func() {
			if err := stopDockerBeforeStorage(service.Stop, nil); err != nil {
				fmt.Fprintln(os.Stderr, "Docker guest cleanup incomplete")
			}
		}()
	} else {
		s.BeforeShutdown = storageQuiesce
	}
	listener, listenErr := unixsocket.Listen(unixSocket, 0600)
	if listenErr != nil {
		if stopDocker != nil {
			_ = stopDocker()
		}
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
		if stopDocker != nil {
			e = errors.Join(e, stopDocker())
		}
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
