//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"golang.org/x/sys/unix"
)

// Real UID-1000 sockets and owned cgroups; a synthetic Docker API, not Docker.
func TestInstalledEngineObservationAuthenticatedLifecycle(t *testing.T) {
	scope := installedScope(t, "observe")
	dir := socketDir(t)
	control := filepath.Join(dir, "health-control")
	service, err := Launch(t.Context(), scope, worker(t, dir, "MK_TEST_ENGINE_HEALTH_CONTROL="+control), filepath.Join(dir, "docker.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	socket := filepath.Join(t.TempDir(), "agent.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &agent.Server{Manager: agent.NewManager(true), SandboxID: "box", Generation: "generation", Endpoint: 7001, Token: []byte("synthetic-observation-token"), ObserveDockerEngine: service.Observe}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	client, err := agent.DialContext(ctx, socket, server.SandboxID, server.Generation, server.Endpoint, server.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	check := func(want string) {
		t.Helper()
		r, err := client.InspectDockerEngine(ctx)
		if err != nil || r.Status != want {
			t.Fatalf("observation want %s got %+v %v", want, r, err)
		}
	}
	check("responding")
	if err := os.WriteFile(control, []byte("unavailable"), 0644); err != nil {
		t.Fatal(err)
	}
	check("unavailable")
	if err := os.Remove(control); err != nil {
		t.Fatal(err)
	}
	check("responding")
	// A second UID-1000 service outside the supervisor's scope cannot borrow
	// readiness by replacing its pathname, even while the launcher stays alive.
	original := filepath.Join(dir, "original.sock")
	if err := os.Rename(filepath.Join(dir, "docker.sock"), original); err != nil {
		t.Fatal(err)
	}
	foreign := worker(t, dir)
	foreign.Stdout = io.Discard
	foreign.Stderr = io.Discard
	if err := foreign.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := false
	defer func() {
		if !reaped {
			_ = foreign.Process.Kill()
			_ = foreign.Wait()
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "docker.sock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("foreign socket unavailable")
		}
		time.Sleep(time.Millisecond)
	}
	check("unavailable")
	if err := foreign.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("foreign service affected", err)
	}
	if err := foreign.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = foreign.Wait()
	reaped = true
	if err := os.Remove(filepath.Join(dir, "docker.sock")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Rename(original, filepath.Join(dir, "docker.sock")); err != nil {
		t.Fatal(err)
	}
	check("responding")
	stop, cancelStop := context.WithTimeout(ctx, 5*time.Second)
	defer cancelStop()
	if err := service.Stop(stop); err != nil {
		t.Fatal(err)
	}
	check("stopped")
	if empty, err := scope.Empty(); err != nil || !empty {
		t.Fatal("scope not released", err)
	}
	if err := client.CallContext(ctx, "Quiesce", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.InspectDockerEngine(ctx); err == nil {
		t.Fatal("quiesced observation accepted")
	}
	client.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("agent fixture did not stop")
	}
	t.Log("OWNED_API_DEGRADATION_RECOVERY_FOREIGN_PEER_REFUSAL_STOP_AND_AUTHENTICATED_QUIESCENCE_PASS")
}

func TestInstalledEngineObservationUnexpectedExit(t *testing.T) {
	scope := installedScope(t, "observe-exit")
	dir := socketDir(t)
	service, err := Launch(t.Context(), scope, worker(t, dir), filepath.Join(dir, "docker.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := service.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := unix.PidfdSendSignal(service.pidfd, unix.SIGKILL, nil, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-service.done:
	case <-time.After(3 * time.Second):
		t.Fatal("launcher did not exit")
	}
	r, err := service.Observe(t.Context())
	if err != nil || r.Status != "exited" {
		t.Fatal("exited launcher accepted", r, err)
	}
	if err := service.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Log("HELD_LAUNCHER_EXIT_OBSERVED_AND_OWNED_SCOPE_RELEASED_PASS")
}

// A launcher can be dead while an owned descendant still serves valid API
// responses and log reaping has not completed. Liveness must use the pidfd.
func TestInstalledEngineObservationDeadLauncherLiveAPI(t *testing.T) {
	scope := installedScope(t, "observe-dead-live")
	dir := socketDir(t)
	engine := worker(t, dir)
	engine.Stdout = io.Discard
	engine.Stderr = io.Discard
	if err := scope.Start(engine); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = scope.Stop(context.Background()); _ = engine.Wait() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := probe(t.Context(), scope, filepath.Join(dir, "docker.sock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned API did not start")
		}
		time.Sleep(time.Millisecond)
	}
	fd := -1
	launcher := exec.Command("/bin/true")
	launcher.SysProcAttr = &syscall.SysProcAttr{PidFD: &fd}
	if err := scope.Start(launcher); err != nil {
		t.Fatal(err)
	}
	defer launcher.Wait()
	if fd < 0 {
		t.Fatal("launcher pidfd missing")
	}
	defer unix.Close(fd)
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if n, err := unix.Poll(fds, 3000); err != nil || n != 1 {
		t.Fatal("held launcher did not exit", err)
	}
	service := &Service{scope: scope, started: true, pidfd: fd, done: make(chan struct{}), socket: filepath.Join(dir, "docker.sock")}
	r, err := service.Observe(t.Context())
	if err != nil || r.Status != "exited" {
		t.Fatal("live API masked dead launcher", r, err)
	}
	if err := probe(t.Context(), scope, filepath.Join(dir, "docker.sock")); err != nil {
		t.Fatal("API did not remain live", err)
	}
	t.Log("LIVE_OWNED_API_CANNOT_MASK_HELD_LAUNCHER_DEATH_BEFORE_LOG_REAP_PASS")
}
