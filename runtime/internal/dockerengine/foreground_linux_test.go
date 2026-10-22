//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
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
	// An image alias must fail before configuration or mounts are created.
	local := imageRoot + "/home/rootless/.local"
	backup := local + ".qualification-original"
	_, statErr := os.Lstat(local)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}
	if existed {
		if err := os.Rename(local, backup); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/tmp", local); err != nil {
		t.Fatal(err)
	}
	_, refusal := PrepareFilesystem()
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	if existed {
		if err := os.Rename(backup, local); err != nil {
			t.Fatal(err)
		}
	}
	if refusal == nil || refusal.Error() != "Docker data directory alias refused" {
		t.Fatal("image alias admitted", refusal)
	}
	if _, err := os.Stat(imageRoot + "/run/user"); !os.IsNotExist(err) {
		t.Fatal("preflight refusal changed runtime directory", err)
	}
	filesystem, err := PrepareFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{imageRoot + "/run/docker-daemon.json", imageRoot + "/sys/fs/cgroup/cgroup.subtree_control"} {
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CLOEXEC, 0)
		if fd >= 0 {
			unix.Close(fd)
		}
		if !errors.Is(err, unix.EROFS) {
			t.Fatal("inner configuration/cgroup view is writable", path, err)
		}
	}
	var outerState, innerState unix.Stat_t
	if unix.Stat(rootlessState, &outerState) != nil || unix.Stat(imageRoot+rootlessState, &innerState) != nil || outerState.Dev != innerState.Dev || outerState.Ino != innerState.Ino {
		t.Fatal("RootlessKit state is not shared")
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
	service, err := LaunchPrepared(ctx, scope, command, filepath.Join(root, "bundle/rootfs/run/docker.sock"), filesystem)
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
	// Cleanup must follow its captured mount even after an image-side rename.
	renamed := imageRoot + "/run/user-renamed"
	if err := os.Rename(imageRoot+"/run/user", renamed); err != nil {
		t.Fatal(err)
	}
	held, err := os.Open(renamed + "/1000/rootlesskit")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	busy, cancelBusy := context.WithTimeout(context.Background(), 10*time.Second)
	err = service.Stop(busy)
	cancelBusy()
	if !errors.Is(err, unix.EBUSY) {
		t.Fatal("normal unmount ignored a live directory reference", err)
	}
	if err := held.Close(); err != nil {
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
	if err := filesystem.Release(); err != nil {
		t.Fatal("filesystem release is not idempotent", err)
	}
	if _, err := os.Stat(releaseDirectory); !os.IsNotExist(err) {
		t.Fatal("owned state mount cleanup directory remains", err)
	}
	if !bounded {
		t.Fatal("daemon log bound exceeded")
	}
	t.Log("agent filesystem preparation, alias refusal, readonly config/cgroups, shared state, real rootless Docker API, foreground lifetime, bounded logs, mount rename/busy refusal/retry and owned cleanup passed")
}
