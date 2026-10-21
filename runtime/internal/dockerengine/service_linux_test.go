//go:build linux

package dockerengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

const validInfo = `{"OSType":"linux","Driver":"vfs","DockerRootDir":"/home/rootless/.local/share/docker","SecurityOptions":["name=rootless"]}`

func TestProfileReadinessAndBoundedLogs(t *testing.T) {
	if err := validateInfo([]byte(validInfo)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"{}", "null", strings.Replace(validInfo, "vfs", "overlay2", 1), strings.Replace(validInfo, "rootless", "rootful", 1), strings.Replace(validInfo, "linux", "other", 1), validInfo + " {}"} {
		if validateInfo([]byte(data)) == nil {
			t.Fatal("unqualified daemon accepted")
		}
	}
	var log tailLog
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 10; j++ {
				_, _ = log.Write(bytes.Repeat([]byte("x"), logLimit+31))
			}
		})
	}
	wg.Wait()
	_, _ = log.Write([]byte("last"))
	if len(log.data) != logLimit || !log.truncated || !bytes.HasSuffix(log.data, []byte("last")) {
		t.Fatal("logging bound or tail lost")
	}
	if s, e := Launch(nil, nil, nil, ""); e == nil || s != nil {
		t.Fatal("missing inputs admitted")
	}
}

// This helper simulates the readiness protocol only, not Docker or a build.
func TestEngineReadinessWorker(t *testing.T) {
	socket := os.Getenv("MK_TEST_ENGINE_SOCKET")
	if socket == "" {
		t.Skip("readiness protocol fixture only")
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if os.Getenv("MK_TEST_ENGINE_EXIT") == "1" {
		return
	}
	if os.Getenv("MK_TEST_ENGINE_DETACHED") == "1" {
		c := exec.Command("/usr/bin/setsid", "/bin/sleep", "300")
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Process.Kill(); _ = c.Wait() }()
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_ping":
			fmt.Fprint(w, "OK")
		case "/info":
			if os.Getenv("MK_TEST_ENGINE_BAD_INFO") == "1" {
				fmt.Fprint(w, `{"Driver":"overlay2"}`)
			} else {
				fmt.Fprint(w, validInfo)
			}
		default:
			http.NotFound(w, r)
		}
	})
	fmt.Print(strings.Repeat("synthetic-daemon-log\n", 8192))
	if err := http.Serve(listener, handler); err != nil {
		t.Fatal(err)
	}
}
func installedScope(t *testing.T, suffix string) *guestscope.Scope {
	t.Helper()
	parent := os.Getenv("MK_TEST_GUEST_SCOPE_PARENT")
	if parent == "" {
		t.Skip("set MK_TEST_GUEST_SCOPE_PARENT for installed-kernel qualification")
	}
	s, e := guestscope.New(parent, "mk-engine-"+suffix+"-"+strconv.Itoa(os.Getpid()), guestscope.Limits{PIDs: 64, MemoryBytes: 128 << 20})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.Stop(ctx); e != nil {
			t.Error(e)
		}
	})
	return s
}
func worker(t *testing.T, dir string, extra ...string) *exec.Cmd {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	c := exec.Command(exe, "-test.run=^TestEngineReadinessWorker$")
	c.Env = append([]string{"MK_TEST_ENGINE_SOCKET=" + filepath.Join(dir, "docker.sock")}, extra...)
	c.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1000, Gid: 1000}}
	return c
}
func socketDir(t *testing.T) string {
	t.Helper()
	dir, e := os.MkdirTemp("/tmp", "mk-engine-ready-")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chown(dir, 1000, 1000); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
func TestInstalledEngineSupervision(t *testing.T) {
	s := installedScope(t, "ready")
	dir := socketDir(t)
	outside := exec.Command("/bin/sleep", "300")
	if e := outside.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = outside.Process.Kill(); _ = outside.Wait() }()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	service, e := Launch(ctx, s, worker(t, dir, "MK_TEST_ENGINE_DETACHED=1"), filepath.Join(dir, "docker.sock"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = service.Stop(context.Background()) })
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if e := service.Stop(canceled); e == nil {
		t.Fatal("canceled stop accepted")
	}
	if empty, e := s.Empty(); e != nil || empty {
		t.Fatal("canceled stop lost live ownership", e)
	}
	service.log.mu.Lock()
	bounded := service.log.truncated && len(service.log.data) <= logLimit
	service.log.mu.Unlock()
	if !bounded {
		t.Fatal("daemon logs not bounded")
	}
	if e = service.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = service.Stop(context.Background()); e != nil {
		t.Fatal("stop is not idempotent", e)
	}
	select {
	case <-service.UnexpectedExit():
		t.Fatal("requested stop published unexpected exit")
	default:
	}
	if e = outside.Process.Signal(syscall.Signal(0)); e != nil {
		t.Fatal("outside process affected", e)
	}
}
func TestInstalledEngineRefusesForeignPeer(t *testing.T) {
	s := installedScope(t, "foreign")
	dir := socketDir(t)
	foreign := worker(t, dir)
	foreign.Stdout = io.Discard
	if e := foreign.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = foreign.Process.Kill(); _ = foreign.Wait() }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, e := os.Stat(filepath.Join(dir, "docker.sock")); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("foreign fixture did not listen")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if conn, e := peerConnection(ctx, s, filepath.Join(dir, "docker.sock")); e == nil {
		conn.Close()
		t.Fatal("foreign daemon accepted")
	} else if e.Error() != "Docker socket peer is outside owned scope" {
		t.Fatal("did not reach cgroup peer refusal", e)
	}
	if e := foreign.Process.Signal(syscall.Signal(0)); e != nil {
		t.Fatal("foreign daemon was signaled")
	}
}
func TestInstalledEngineReadinessFailureCleansScope(t *testing.T) {
	for _, extra := range []string{"MK_TEST_ENGINE_BAD_INFO=1", "MK_TEST_ENGINE_EXIT=1"} {
		t.Run(extra, func(t *testing.T) {
			s := installedScope(t, "refuse")
			dir := socketDir(t)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			service, e := Launch(ctx, s, worker(t, dir, extra), filepath.Join(dir, "docker.sock"))
			if service == nil || e == nil {
				t.Fatal("failed readiness accepted")
			}
			if e := service.Stop(context.Background()); e != nil {
				t.Fatal("failed launch cleanup lost ownership", e)
			}
			if empty, e := s.Empty(); e != nil || !empty {
				t.Fatal("failed launch retained descendants", e)
			}
		})
	}
}

func TestInstalledEngineUnexpectedExit(t *testing.T) {
	s := installedScope(t, "exit")
	dir := socketDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	service, e := Launch(ctx, s, worker(t, dir), filepath.Join(dir, "docker.sock"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = service.Stop(context.Background()) }()
	if e = unix.PidfdSendSignal(service.pidfd, unix.SIGKILL, nil, 0); e != nil {
		t.Fatal(e)
	}
	select {
	case <-service.UnexpectedExit():
	case <-time.After(2 * time.Second):
		t.Fatal("unexpected engine death was not reported")
	}
	if e = service.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
}
