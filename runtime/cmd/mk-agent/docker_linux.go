//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/dockerengine"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

// The production builder does not emit this opt-in yet. These paths are in the
// trusted outer root. Workloads may never select a bootstrap, executable, socket,
// resource policy or cgroup through OCI arguments, labels, environment or RPC.
func startDockerGuest(ctx context.Context) (*dockerengine.Service, error) {
	fd, err := unix.Open("/docker-bootstrap", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("trusted Docker bootstrap is unavailable")
	}
	bootstrap := os.NewFile(uintptr(fd), "Docker bootstrap")
	defer bootstrap.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != 0 || st.Nlink != 1 || st.Mode&0022 != 0 || st.Size < 18 || st.Size > 1<<20 {
		return nil, errors.New("trusted Docker bootstrap metadata refused")
	}
	header := make([]byte, len("#!/bin/busybox sh\n"))
	if _, err := io.ReadFull(bootstrap, header); err != nil || string(header) != "#!/bin/busybox sh\n" {
		return nil, errors.New("trusted Docker bootstrap interpreter refused")
	}
	if _, err := bootstrap.Seek(0, 0); err != nil {
		return nil, errors.New("trusted Docker bootstrap rewind failed")
	}
	if err := dockerengine.PrepareFilesystem(); err != nil {
		return nil, err
	}
	scope, err := guestscope.New("/sys/fs/cgroup", "mk-docker-engine", guestscope.Limits{PIDs: 512, MemoryBytes: 512 << 20})
	if err != nil {
		return nil, errors.New("Docker service scope creation failed")
	}
	command := exec.Command("/proc/self/exe", "__docker_bootstrap")
	command.ExtraFiles = []*os.File{bootstrap}
	command.Dir = "/"
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/root", "LANG=C", "LC_ALL=C"}
	return dockerengine.Launch(ctx, scope, command, "/bundle/rootfs/run/docker.sock")
}

// A failed service stop must prevent every filesystem quiescence operation.
func stopDockerBeforeStorage(stop func(context.Context) error, quiesce func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if stop != nil {
		if err := stop(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "MK_DOCKER_STOP_FAIL")
			return errors.New("Docker service has not quiesced")
		}
	}
	if quiesce != nil {
		err := quiesce()
		if err != nil {
			fmt.Fprintln(os.Stderr, "MK_DOCKER_STORAGE_QUIESCE_FAIL")
		}
		return err
	}
	return nil
}
