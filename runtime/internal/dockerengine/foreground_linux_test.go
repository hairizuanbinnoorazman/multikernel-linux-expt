//go:build linux

package dockerengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
)

// The installed harness supplies a private mount namespace and an independently
// staged outer root containing the pinned real image. No host Docker socket is
// passed. This tests the real foreground chain, not a readiness protocol stub.
func TestInstalledRealForegroundBootstrap(t *testing.T) {
	root := os.Getenv("MK_TEST_DOCKER_FOREGROUND_ROOT")
	if root == "" {
		t.Skip("explicit installed real-image qualification")
	}
	parent := os.Getenv("MK_TEST_GUEST_SCOPE_PARENT")
	if parent == "" || os.Geteuid() != 0 {
		t.Fatal("root and protected scope parent required")
	}
	name := "mk-engine-foreground-" + strconv.Itoa(os.Getpid())
	scope, err := guestscope.New(parent, name, guestscope.Limits{PIDs: 512, MemoryBytes: 512 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := scope.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	script, err := os.Open(filepath.Join(root, "docker-bootstrap"))
	if err != nil {
		t.Fatal(err)
	}
	defer script.Close()
	command := exec.Command("/bin/busybox", "chroot", root, "/mk-agent", "__docker_bootstrap")
	command.ExtraFiles = []*os.File{script}
	command.Dir = "/"
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/root", "LANG=C", "LC_ALL=C"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	service, err := Launch(ctx, scope, command, filepath.Join(root, "bundle/rootfs/run/docker.sock"))
	if service != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := service.Stop(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		if service != nil {
			service.log.mu.Lock()
			t.Logf("dedicated daemon log: %s", service.log.data)
			service.log.mu.Unlock()
		}
		t.Fatal(err)
	}
	// Socket creation must not terminate the launcher. Readiness is established
	// by Launch's identity-bound /_ping and /info checks, then checked again.
	select {
	case <-service.UnexpectedExit():
		t.Fatal("launcher exited after API readiness")
	case <-time.After(time.Second):
	}
	if err := probe(ctx, scope, filepath.Join(root, "bundle/rootfs/run/docker.sock")); err != nil {
		t.Fatal(err)
	}
	stop, cancelStop := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStop()
	if err := service.Stop(stop); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(parent, name)); !os.IsNotExist(err) {
		t.Fatal("owned service cgroup remains", err)
	}
	select {
	case <-service.UnexpectedExit():
		t.Fatal("requested stop reported unexpected exit")
	default:
	}
	service.log.mu.Lock()
	bounded := len(service.log.data) <= logLimit
	service.log.mu.Unlock()
	if !bounded {
		t.Fatal("daemon log bound exceeded")
	}
	t.Log("real rootless Docker API, foreground lifetime, bounded logs and owned cgroup removal passed")
}
