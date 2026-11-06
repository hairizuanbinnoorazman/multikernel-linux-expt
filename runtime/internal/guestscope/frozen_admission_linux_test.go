//go:build linux

package guestscope

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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

func TestFrozenScopeAdmissionChild(t *testing.T) {
	if os.Getenv("MK_TEST_FROZEN_SCOPE_CHILD") != "1" {
		t.Skip("internal fixed fixture process")
	}
	fmt.Println("FROZEN_SCOPE_CHILD_READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

// This opt-in test creates only fresh leaves below a dedicated delegated test
// parent. It never freezes, adopts or repairs any existing cgroup. The harness
// must keep every test group below its own systemd control group for a bounded
// failure backstop, and must verify that parent is empty after this binary exits.
func TestInstalledScopeStopQueueDuringFrozenAdmission(t *testing.T) {
	testInstalledScopeQueueDuringFrozenAdmission(t, false)
}

func TestInstalledScopePeerQueueDuringFrozenAdmission(t *testing.T) {
	testInstalledScopeQueueDuringFrozenAdmission(t, true)
}

func testInstalledScopeQueueDuringFrozenAdmission(t *testing.T, peer bool) {
	parent := os.Getenv("MK_TEST_GUEST_SCOPE_PARENT")
	if parent == "" || os.Getenv("MK_TEST_FROZEN_SCOPE_ADMISSION") != "1" {
		t.Skip("explicit fresh delegated cgroup parent and frozen-admission opt-in required")
	}
	if os.Geteuid() != 0 {
		t.Fatal("frozen-admission fixture requires trusted root")
	}
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			name := "mk-frozen-" + strconv.Itoa(os.Getpid()) + "-" + mode
			scope, err := New(parent, name, Limits{PIDs: 64, MemoryBytes: 64 << 20})
			if err != nil {
				t.Fatal("fresh scope admission refused", err)
			}
			witness, err := unix.FcntlInt(uintptr(scope.directory), unix.F_DUPFD_CLOEXEC, 3)
			if err != nil {
				_ = scope.Stop(context.Background())
				t.Fatal(err)
			}
			control := func(file, value string) error {
				var st unix.Stat_t
				if witness < 0 || unix.Fstat(witness, &st) != nil || uint64(st.Dev) != scope.device || st.Ino != scope.inode {
					return ErrIdentity
				}
				return writeControl(witness, file, value)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestFrozenScopeAdmissionChild$")
			command.Env = append(os.Environ(), "MK_TEST_FROZEN_SCOPE_CHILD=1", "GORACE=atexit_sleep_ms=0")
			pidfd := -1
			command.SysProcAttr = &syscall.SysProcAttr{PidFD: &pidfd}
			var input io.WriteCloser
			var output io.ReadCloser
			command.Stderr = io.Discard
			startDone := make(chan struct{})
			var startErr error
			startCalled := false
			waitDone := make(chan struct{})
			var waitErr error
			waitCalled := false
			startWait := func() {
				if !waitCalled {
					waitCalled = true
					go func() { waitErr = command.Wait(); close(waitDone) }()
				}
			}
			t.Cleanup(func() {
				// Thaw only the held, identity-checked fresh group, before asking the
				// production Stop to acquire the admission lock. Kill is an emergency
				// test-only backstop through that same held group descriptor, never a PID.
				if witness >= 0 {
					_ = control("cgroup.freeze", "0")
				}
				if startCalled {
					select {
					case <-startDone:
					case <-time.After(2 * time.Second):
						if witness >= 0 {
							_ = control("cgroup.kill", "1")
						}
						select {
						case <-startDone:
						case <-time.After(time.Second):
							t.Error("fresh admission retained after thaw/owned-group kill")
							return
						}
					}
					if startErr == nil {
						startWait()
					}
				}
				if witness >= 0 {
					_ = unix.Close(witness)
					witness = -1
				}
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := scope.Stop(cleanup); err != nil {
					t.Error("fresh scope cleanup failed", err)
				}
				if input != nil {
					input.Close()
				}
				if output != nil {
					output.Close()
				}
				if waitCalled {
					select {
					case <-waitDone:
					case <-time.After(time.Second):
						t.Error("fresh launcher was not reaped")
					}
				}
				if pidfd >= 0 {
					_ = unix.Close(pidfd)
					pidfd = -1
				}
			})
			input, err = command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err = command.StdoutPipe()
			if err != nil {
				input.Close()
				t.Fatal(err)
			}
			if err = control("cgroup.freeze", "1"); err != nil {
				t.Fatal("fresh scope freeze refused", err)
			}
			startCalled = true
			go func() { startErr = scope.Start(command); close(startDone) }()
			var frozenPID string
			until := time.Now().Add(3 * time.Second)
			for {
				select {
				case <-startDone:
					t.Fatal("admission did not block inside fresh frozen group", startErr)
				default:
				}
				events, e := readControl(witness, "cgroup.events")
				members, m := readControl(witness, "cgroup.procs")
				if e == nil && m == nil && strings.Contains("\n"+events+"\n", "\nfrozen 1\n") {
					ids := strings.Fields(members)
					if len(ids) == 1 {
						if pid, e := strconv.Atoi(ids[0]); e == nil && pid > 1 {
							frozenPID = ids[0]
							break
						}
					}
				}
				if time.Now().After(until) {
					t.Fatal("fresh child was not observed frozen during admission")
				}
				time.Sleep(time.Millisecond)
			}
			t.Log("FROZEN_ADMISSION_OWNER_AND_MEMBER_OBSERVED")
			var ctx context.Context
			var cancel context.CancelFunc
			expected := context.Canceled
			if mode == "cancel" {
				ctx, cancel = context.WithCancel(t.Context())
			} else {
				ctx, cancel = context.WithTimeout(t.Context(), 30*time.Millisecond)
				expected = context.DeadlineExceeded
			}
			defer cancel()
			returned := make(chan error, 1)
			entered := make(chan struct{})
			go func() {
				close(entered)
				if peer {
					// Invalid peer arguments must remain unobserved while queued.
					inside, err := scope.ContainsLivePeerContext(ctx, 1, -1)
					if inside {
						err = errors.New("canceled peer check claimed membership")
					}
					returned <- err
				} else {
					returned <- scope.Stop(ctx)
				}
			}()
			<-entered
			if mode == "cancel" {
				select {
				case <-returned:
					t.Fatal("caller returned before cancellation")
				case <-time.After(20 * time.Millisecond):
				}
				cancel()
			}
			var stopErr error
			early := false
			select {
			case stopErr = <-returned:
				early = true
			case <-time.After(200 * time.Millisecond):
			}
			select {
			case <-startDone:
				t.Fatal("canceled caller disturbed active frozen admission")
			default:
			}
			events, e := readControl(witness, "cgroup.events")
			members, m := readControl(witness, "cgroup.procs")
			if e != nil || m != nil || strings.TrimSpace(members) != frozenPID || !strings.Contains("\n"+events+"\n", "\nfrozen 1\n") || scope.closing || scope.closed {
				t.Fatal("canceled caller changed frozen member or cleanup authority")
			}
			if early {
				t.Log("QUEUED_CONTEXT_RETURNED_WITH_ADMISSION_STILL_FROZEN")
			}
			if err = control("cgroup.freeze", "0"); err != nil {
				t.Fatal("owned-group thaw failed", err)
			}
			select {
			case <-startDone:
			case <-time.After(2 * time.Second):
				t.Fatal("admission did not resume after owned-group thaw")
			}
			if startErr != nil || pidfd < 0 {
				t.Fatal("thawed clone admission or clone-bound pidfd failed", startErr)
			}
			startWait()
			if !early {
				select {
				case stopErr = <-returned:
				case <-time.After(time.Second):
					t.Fatal("queued caller did not return after admission resumed")
				}
			}
			if !errors.Is(stopErr, expected) {
				t.Fatal("queued caller returned wrong result", stopErr)
			}
			ready := make(chan bool, 1)
			go func() {
				line, e := bufio.NewReader(output).ReadString('\n')
				ready <- e == nil && line == "FROZEN_SCOPE_CHILD_READY\n"
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("thawed fixed child did not initialize")
				}
			case <-time.After(time.Second):
				t.Fatal("thawed child readiness deadline")
			}
			if inside, e := scope.ContainsLivePeerContext(t.Context(), command.Process.Pid, pidfd); e != nil || !inside {
				t.Fatal("thawed child lost exact live membership", inside, e)
			}
			outsideFD := -1
			outside := exec.Command("/bin/sleep", "300")
			outside.SysProcAttr = &syscall.SysProcAttr{PidFD: &outsideFD}
			if err = outside.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if outsideFD >= 0 {
					_ = unix.PidfdSendSignal(outsideFD, unix.SIGKILL, nil, 0)
				} else {
					_ = outside.Process.Kill()
				}
				_ = outside.Wait()
				if outsideFD >= 0 {
					_ = unix.Close(outsideFD)
				}
			}()
			if outsideFD < 0 {
				t.Fatal("outside-process clone-bound pidfd unavailable")
			}
			if inside, e := scope.ContainsLivePeer(outside.Process.Pid, outsideFD); e != nil || inside {
				t.Fatal("outside process accepted as member", inside, e)
			}
			// Release the witness before normal production removal, then rely solely
			// on Scope.Stop's population and identity checks for its owned leaf.
			_ = unix.Close(witness)
			witness = -1
			cleanup, finish := context.WithTimeout(t.Context(), 5*time.Second)
			err = scope.Stop(cleanup)
			finish()
			if err != nil {
				t.Fatal("normal owned scope release failed", err)
			}
			select {
			case <-waitDone:
			case <-time.After(time.Second):
				t.Fatal("normal scope stop did not reap fixed child")
			}
			var exit *exec.ExitError
			if !errors.As(waitErr, &exit) {
				t.Fatal("fixed child was not killed by owned scope release", waitErr)
			}
			if _, err = os.Lstat(filepath.Join(parent, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("released scope leaf remains", err)
			}
			eventsFD := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
			if n, e := unix.Poll(eventsFD, 0); e != nil || n != 1 || eventsFD[0].Revents&unix.POLLIN == 0 {
				t.Fatal("clone-bound child pidfd is not terminal", n, e)
			}
			if err = unix.PidfdSendSignal(outsideFD, 0, nil, 0); err != nil {
				t.Fatal("release affected outside process", err)
			}
			if err = scope.Stop(context.Background()); err != nil {
				t.Fatal("completed owned release lost idempotence", err)
			}
			t.Log("OWNED_SCOPE_REMOVED_CHILD_REAPED_OUTSIDE_PROCESS_ALIVE")
			if !early {
				t.Error("queued caller did not return before frozen admission resumed")
			}
		})
	}
}
