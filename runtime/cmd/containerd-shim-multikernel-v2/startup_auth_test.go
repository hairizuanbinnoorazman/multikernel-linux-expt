//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/containerd/containerd/runtime/v2/shim"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
)

type fakeStartShimSocket struct{ closed bool }

func (*fakeStartShimSocket) File() (*os.File, error) { return nil, errors.New("not used") }

func (s *fakeStartShimSocket) Close() error {
	s.closed = true
	return nil
}

type fakeStartShimOwner struct {
	dial       func() (net.Conn, error)
	removed    bool
	closed     bool
	removeErr  error
	releaseErr error
}

func (o *fakeStartShimOwner) DialContext(context.Context) (net.Conn, error) { return o.dial() }

func (o *fakeStartShimOwner) Remove() error {
	o.removed = true
	return o.removeErr
}

func (o *fakeStartShimOwner) Close() error {
	o.closed = true
	return o.releaseErr
}

func shimAddressInUseError() error {
	return &net.OpError{Op: "listen", Err: &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}}
}

func TestAcquireStartShimSocketPreservesLiveOwnerAndRemovesOnlyCapturedStaleOwner(t *testing.T) {
	const address = "unix:///run/containerd/s/fake"
	t.Run("live", func(t *testing.T) {
		owner := &fakeStartShimOwner{dial: func() (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		}}
		creates := 0
		socket, existing, err := acquireStartShimSocket(context.Background(), address, func(string) (startShimSocket, error) {
			creates++
			return nil, shimAddressInUseError()
		}, func(path string) (startShimSocketOwner, error) {
			if path != "/run/containerd/s/fake" {
				t.Fatalf("captured path = %q", path)
			}
			return owner, nil
		})
		if err != nil || socket != nil || !existing || creates != 1 {
			t.Fatalf("acquire live socket = %v, %v, %v, creates %d", socket, existing, err, creates)
		}
		if owner.removed || !owner.closed {
			t.Fatalf("live owner removed=%v closed=%v", owner.removed, owner.closed)
		}
	})

	t.Run("stale", func(t *testing.T) {
		owner := &fakeStartShimOwner{dial: func() (net.Conn, error) {
			return nil, syscall.ECONNREFUSED
		}}
		created := &fakeStartShimSocket{}
		creates := 0
		socket, existing, err := acquireStartShimSocket(context.Background(), address, func(string) (startShimSocket, error) {
			creates++
			if creates == 1 {
				return nil, shimAddressInUseError()
			}
			return created, nil
		}, func(string) (startShimSocketOwner, error) { return owner, nil })
		if err != nil || socket != created || existing || creates != 2 {
			t.Fatalf("acquire stale socket = %v, %v, %v, creates %d", socket, existing, err, creates)
		}
		if !owner.removed {
			t.Fatal("captured stale owner was not removed")
		}
	})

	t.Run("replaced", func(t *testing.T) {
		owner := &fakeStartShimOwner{
			dial:      func() (net.Conn, error) { return nil, syscall.ECONNREFUSED },
			removeErr: errors.New("refusing to remove replaced Unix socket path"),
		}
		creates := 0
		socket, existing, err := acquireStartShimSocket(context.Background(), address, func(string) (startShimSocket, error) {
			creates++
			return nil, shimAddressInUseError()
		}, func(string) (startShimSocketOwner, error) { return owner, nil })
		if err == nil || socket != nil || existing || creates != 1 {
			t.Fatalf("acquire replaced socket = %v, %v, %v, creates %d", socket, existing, err, creates)
		}
		if !owner.removed || !owner.closed {
			t.Fatalf("replaced owner removed-attempt=%v closed=%v", owner.removed, owner.closed)
		}
	})

	t.Run("cancelled probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		owner := &fakeStartShimOwner{dial: func() (net.Conn, error) {
			cancel()
			return nil, syscall.ECONNREFUSED
		}}
		socket, existing, err := acquireStartShimSocket(ctx, address, func(string) (startShimSocket, error) {
			return nil, shimAddressInUseError()
		}, func(string) (startShimSocketOwner, error) { return owner, nil })
		if !errors.Is(err, context.Canceled) || socket != nil || existing {
			t.Fatalf("acquire cancelled socket = %v, %v, %v", socket, existing, err)
		}
		if owner.removed || !owner.closed {
			t.Fatalf("cancelled owner removed=%v closed=%v", owner.removed, owner.closed)
		}
	})
}

func TestPathnameShimSocketRejectsNonCanonicalAddresses(t *testing.T) {
	for _, address := range []string{"tcp://host/socket", "unix://relative", "unix:///run/../tmp/socket"} {
		if path, err := pathnameShimSocket(address); err == nil || path != "" {
			t.Fatalf("pathnameShimSocket(%q) = %q, %v", address, path, err)
		}
	}
}

func TestAcquireStartShimSocketWithRealLiveAndStalePaths(t *testing.T) {
	path := filepath.Join(privateTestDirectory(t), "shim.sock")
	address := "unix://" + path
	create := func(string) (startShimSocket, error) { return unixsocket.ListenExclusive(path, 0600) }
	capture := func(string) (startShimSocketOwner, error) { return unixsocket.Capture(path) }

	live, err := unixsocket.ListenExclusive(path, 0600)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	socket, existing, err := acquireStartShimSocket(context.Background(), address, create, capture)
	if err != nil || socket != nil || !existing {
		t.Fatalf("acquire real live socket = %v, %v, %v", socket, existing, err)
	}
	if err = live.Close(); err != nil {
		t.Fatal(err)
	}

	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = stale.Close(); err != nil {
		t.Fatal(err)
	}
	socket, existing, err = acquireStartShimSocket(context.Background(), address, create, capture)
	if err != nil || socket == nil || existing {
		t.Fatalf("acquire real stale socket = %v, %v, %v", socket, existing, err)
	}
	if err = socket.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExistingShimSupervisorProcess(t *testing.T) {
	if os.Getenv("MK_TEST_EXISTING_SHIM_SUPERVISOR") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestAuthenticateExistingShimBindsLaunchMetadataAndProcessIdentity(t *testing.T) {
	bundle := privateTestDirectory(t)
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	identity := directory.Identity()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, childInput, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(self, "-test.run=^TestExistingShimSupervisorProcess$", "--",
		"-namespace", "authenticated-ns", "-id", "authenticated-task", "-address", "/run/containerd/containerd.sock")
	command.Dir = directory.ProcPath()
	command.Env = append(os.Environ(), "MK_TEST_EXISTING_SHIM_SUPERVISOR=1")
	command.Stdin = input
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	if err = input.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = childInput.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	const address = "unix:///run/containerd/s/authenticated"
	if created, _, publishErr := directory.PublishExclusiveRegularIdentity("address", []byte(address), 0644); publishErr != nil || !created {
		t.Fatalf("publish address: created=%v error=%v", created, publishErr)
	}
	if created, _, publishErr := directory.PublishExclusiveRegularIdentity("shim.pid", []byte(strconv.Itoa(command.Process.Pid)), 0644); publishErr != nil || !created {
		t.Fatalf("publish PID: created=%v error=%v", created, publishErr)
	}
	s := &service{id: "authenticated-task", namespace: "authenticated-ns", bundleDirectory: directory,
		bundleIdentity: rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}}
	opts := shim.StartOpts{Address: "/run/containerd/containerd.sock"}
	deadline := time.Now().Add(time.Second)
	for {
		err = s.authenticateExistingShim(address, opts)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err != nil {
		t.Fatalf("authenticate exact supervisor: %v", err)
	}
	if err = s.authenticateExistingShim(address, shim.StartOpts{Address: "/run/containerd/other.sock"}); err == nil || !strings.Contains(err.Error(), "address invocation differs") {
		t.Fatalf("mismatched containerd address error = %v", err)
	}
	s.id = "other-task"
	if err = s.authenticateExistingShim(address, opts); err == nil || !strings.Contains(err.Error(), "id invocation differs") {
		t.Fatalf("mismatched task error = %v", err)
	}
}

func TestAuthenticateExistingShimRejectsWrongProcessOwnership(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		executable string
		wrongCWD   bool
		setpgid    bool
		want       string
	}{
		{name: "cwd", executable: self, wrongCWD: true, setpgid: true, want: "cwd differs"},
		{name: "process group", executable: self, setpgid: false, want: "process-group leader"},
		{name: "executable", executable: "/bin/sh", setpgid: true, want: "executable identity differs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			directory, openErr := safefile.OpenOwnedDirectory(bundle)
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer directory.Close()
			identity := directory.Identity()
			input, childInput, pipeErr := os.Pipe()
			if pipeErr != nil {
				t.Fatal(pipeErr)
			}
			arguments := []string{"-test.run=^TestExistingShimSupervisorProcess$", "--",
				"-namespace", "authenticated-ns", "-id", "authenticated-task", "-address", "/run/containerd/containerd.sock"}
			if test.executable == "/bin/sh" {
				arguments = append([]string{"-c", "cat", "--"}, arguments...)
			}
			command := exec.Command(test.executable, arguments...)
			command.Dir = directory.ProcPath()
			if test.wrongCWD {
				command.Dir = privateTestDirectory(t)
			}
			command.Env = append(os.Environ(), "MK_TEST_EXISTING_SHIM_SUPERVISOR=1")
			command.Stdin = input
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: test.setpgid}
			if startErr := command.Start(); startErr != nil {
				t.Fatal(startErr)
			}
			_ = input.Close()
			defer func() {
				_ = childInput.Close()
				_ = command.Process.Kill()
				_ = command.Wait()
			}()
			const address = "unix:///run/containerd/s/authenticated"
			if created, _, publishErr := directory.PublishExclusiveRegularIdentity("address", []byte(address), 0644); publishErr != nil || !created {
				t.Fatalf("publish address: created=%v error=%v", created, publishErr)
			}
			if created, _, publishErr := directory.PublishExclusiveRegularIdentity("shim.pid", []byte(strconv.Itoa(command.Process.Pid)), 0644); publishErr != nil || !created {
				t.Fatalf("publish PID: created=%v error=%v", created, publishErr)
			}
			s := &service{id: "authenticated-task", namespace: "authenticated-ns", bundleDirectory: directory,
				bundleIdentity: rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}}
			deadline := time.Now().Add(time.Second)
			for {
				err = s.authenticateExistingShim(address, shim.StartOpts{Address: "/run/containerd/containerd.sock"})
				if err != nil && strings.Contains(err.Error(), test.want) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("ownership rejection = %v, want %q", err, test.want)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
