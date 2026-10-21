//go:build linux

package guestscope

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestScopePolicyAndEvents(t *testing.T) {
	var zero Scope
	if e := zero.Stop(context.Background()); !errors.Is(e, ErrIdentity) {
		t.Fatal("zero scope acquired authority")
	}
	if e := zero.Start(exec.Command("/bin/true")); !errors.Is(e, ErrIdentity) {
		t.Fatal("zero scope admitted a child")
	}
	for _, name := range []string{"", "..", "engine/other", "engine\nsecret"} {
		if s, e := New("/sys/fs/cgroup", name, Limits{PIDs: 32, MemoryBytes: 32 << 20}); e == nil || s != nil {
			t.Fatal("invalid name admitted")
		}
	}
	for _, limits := range []Limits{{}, {PIDs: 4097, MemoryBytes: 32 << 20}, {PIDs: 1, MemoryBytes: 17}} {
		if s, e := New("/sys/fs/cgroup", "engine", limits); e == nil || s != nil {
			t.Fatal("invalid resource policy admitted")
		}
	}
	for _, value := range []string{"", "populated 2", "populated 0\npopulated 1", "populated"} {
		if _, e := populated(value); e == nil {
			t.Fatal("ambiguous population admitted")
		}
	}
	if active, e := populated("populated 1\nfrozen 0"); e != nil || !active {
		t.Fatal("valid event refused")
	}
	if active, e := populated("populated 0\nfrozen 0"); e != nil || active {
		t.Fatal("empty event refused")
	}
	if s, e := New(t.TempDir(), "engine", Limits{PIDs: 32, MemoryBytes: 32 << 20}); e == nil || s != nil {
		t.Fatal("ordinary filesystem accepted as cgroup")
	}
}

// Explicit opt-in on a qualified disposable host. Every process started here
// belongs to a newly created group; there is no adoption or broad repair path.
func TestInstalledScopeContainsAndStopsSessions(t *testing.T) {
	parent := os.Getenv("MK_TEST_GUEST_SCOPE_PARENT")
	if parent == "" {
		t.Skip("set MK_TEST_GUEST_SCOPE_PARENT for cgroup-v2 qualification")
	}
	name := "mk-docker-proof-" + strconv.Itoa(os.Getpid())
	limits := Limits{PIDs: 64, MemoryBytes: 64 << 20}
	s, e := New(parent, name, limits)
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
	if duplicate, e := New(parent, name, limits); e == nil || duplicate != nil {
		t.Fatal("existing group adopted")
	}
	// Root shell and detached child use different sessions. Killing just the
	// launcher's process group would leave the detached sleeper behind.
	ready := filepath.Join(t.TempDir(), "ready")
	outside := exec.Command("/bin/sleep", "300")
	if e = outside.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = outside.Process.Kill(); _ = outside.Wait() }()
	command := exec.Command("/bin/sh", "-c", `/usr/bin/setsid /bin/sh -c 'echo $$ > "$1.tmp"; /bin/mv "$1.tmp" "$1"; exec /bin/sleep 300' detached "$1" & wait`, "scope-proof", ready)
	if e = s.Start(command); e != nil {
		t.Fatal(e)
	}
	waited := false
	defer func() {
		if !waited {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.Stop(ctx)
			_ = command.Wait()
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	var detached int
	for {
		if data, e := os.ReadFile(ready); e == nil {
			detached, e = strconv.Atoi(strings.TrimSpace(string(data)))
			if e != nil {
				t.Fatal(e)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scope child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if sid, e := unix.Getsid(detached); e != nil || sid != detached {
		t.Fatal("detached session was not established")
	}
	if members, e := readControl(s.directory, "cgroup.procs"); e != nil || !strings.Contains("\n"+members+"\n", "\n"+strconv.Itoa(detached)+"\n") {
		t.Fatal("detached child escaped scope")
	}
	if empty, e := s.Empty(); e != nil || empty {
		t.Fatalf("live scope missing: %v", e)
	}
	// Even an already-canceled request must not turn into a completed release.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if e = s.Stop(canceled); !errors.Is(e, context.Canceled) {
		t.Fatalf("canceled stop = %v", e)
	}
	if empty, e := s.Empty(); e != nil || empty {
		t.Fatal("canceled stop killed or lost the group")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e = s.Stop(ctx); e != nil {
		t.Fatal(e)
	}
	_ = command.Wait()
	waited = true
	if _, e = os.Lstat(filepath.Join(parent, name)); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("owned group was not removed: %v", e)
	}
	if e = s.Stop(context.Background()); e != nil {
		t.Fatal("completed release is not idempotent")
	}
	if e = s.Start(exec.Command("/bin/true")); !errors.Is(e, ErrClosed) {
		t.Fatal("start after removal admitted")
	}
	// A genuinely separate process outside the owned group remains alive.
	if e = outside.Process.Signal(syscall.Signal(0)); e != nil {
		t.Fatal("process outside scope was affected")
	}
}

func TestScopeMemoryWorker(t *testing.T) {
	if os.Getenv("MK_SCOPE_MEMORY_WORKER") != "1" {
		t.Skip("internal memory pressure worker")
	}
	buffer, err := unix.Mmap(-1, 0, 128<<20, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Munmap(buffer)
	for i := 0; i < len(buffer); i += 4096 {
		buffer[i] = 1
	}
	t.Fatal("worker exceeded its required cgroup memory limit")
}

func TestInstalledScopeMemoryPressure(t *testing.T) {
	parent := os.Getenv("MK_TEST_GUEST_SCOPE_PARENT")
	if parent == "" {
		t.Skip("set MK_TEST_GUEST_SCOPE_PARENT for memory pressure qualification")
	}
	s, e := New(parent, "mk-docker-memory-"+strconv.Itoa(os.Getpid()), Limits{PIDs: 64, MemoryBytes: 64 << 20})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.Stop(ctx); e != nil {
			t.Error(e)
		}
	}()
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	worker := exec.CommandContext(ctx, executable, "-test.run", "^TestScopeMemoryWorker$")
	worker.Env = append(os.Environ(), "MK_SCOPE_MEMORY_WORKER=1")
	if e = s.Start(worker); e != nil {
		t.Fatal(e)
	}
	if e = worker.Wait(); e == nil || ctx.Err() != nil {
		t.Fatalf("memory worker was not bounded by cgroup: %v", e)
	}
	events, e := readControl(s.directory, "memory.events")
	if e != nil {
		t.Fatal(e)
	}
	var killed uint64
	for _, line := range strings.Split(events, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "oom_kill" {
			killed, e = strconv.ParseUint(fields[1], 10, 64)
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if killed == 0 {
		t.Fatal("memory worker failed without a cgroup OOM kill")
	}
}
