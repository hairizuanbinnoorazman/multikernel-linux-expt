//go:build linux

package dockerengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestBootstrapExecHelper(t *testing.T) {
	if os.Getenv("MK_BOOTSTRAP_EXEC_HELPER") != "1" {
		return
	}
	if os.Getenv("MK_BOOTSTRAP_DENY_SECCOMP") == "1" {
		if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		code := []unix.SockFilter{{Code: 0x20, K: 0}, {Code: 0x15, K: unix.SYS_SECCOMP, Jf: 1}, {Code: 0x06, K: uint32(unix.SECCOMP_RET_ERRNO) | uint32(unix.EPERM)}, {Code: 0x06, K: unix.SECCOMP_RET_ALLOW}}
		filter := unix.SockFprog{Len: uint16(len(code)), Filter: &code[0]}
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&filter)), 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := ExecBootstrap(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	os.Exit(125)
}
func TestBootstrapObservedHelper(t *testing.T) {
	if os.Getenv("MK_BOOTSTRAP_OBSERVE") != "1" {
		return
	}
	value, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || value != 0 {
		t.Fatal("bootstrap mapping-helper exec gains were disabled prematurely")
	}
	var status unix.Stat_t
	forbidden, _ := strconv.ParseUint(os.Getenv("MK_BOOTSTRAP_FORBIDDEN_INODE"), 10, 64)
	if err := unix.Fstat(4, &status); err == nil && status.Ino == forbidden {
		t.Fatal("inherited transport descriptor survived bootstrap")
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
	if fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0); !errors.Is(err, unix.EPERM) {
		if fd >= 0 {
			unix.Close(fd)
		}
		t.Fatal("bootstrap transport access survived", err)
	}
	if _, _, err := unix.Syscall(unix.SYS_IO_URING_SETUP, 0, 0, 0); err != unix.EPERM {
		t.Fatal("bootstrap io_uring survived", err)
	}
}
func TestInstalledBootstrapTransportBoundary(t *testing.T) {
	if os.Getenv("MK_TEST_BOOTSTRAP_GUARD") != "1" {
		t.Skip("explicit installed root qualification")
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Fatal("root qualification required")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	for _, deny := range []bool{false, true} {
		file, err := os.CreateTemp(t.TempDir(), "bootstrap")
		if err != nil {
			t.Fatal(err)
		}
		sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			file.Close()
			t.Fatal(err)
		}
		inherited := os.NewFile(uintptr(sockets[0]), "untrusted-inherited-socket")
		var status unix.Stat_t
		if err := unix.Fstat(sockets[0], &status); err != nil {
			t.Fatal(err)
		}
		body := "#!/bin/busybox sh\nexport MK_BOOTSTRAP_OBSERVE=1\nexport MK_BOOTSTRAP_FORBIDDEN_INODE=" + strconv.FormatUint(status.Ino, 10) + "\nexec " + quote(exe) + " '-test.run=^TestBootstrapObservedHelper$'\n"
		if _, err = file.WriteString(body); err != nil {
			t.Fatal(err)
		}
		if _, err = file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		path := file.Name()
		file.Close()
		file, err = os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(exe, "-test.run=^TestBootstrapExecHelper$")
		command.Env = []string{"MK_BOOTSTRAP_EXEC_HELPER=1"}
		if deny {
			command.Env = append(command.Env, "MK_BOOTSTRAP_DENY_SECCOMP=1")
		}
		command.ExtraFiles = []*os.File{file, inherited}
		output, err := command.CombinedOutput()
		file.Close()
		inherited.Close()
		unix.Close(sockets[1])
		if deny {
			if err == nil || !strings.Contains(string(output), "transport guard synchronization failed") || strings.Contains(string(output), "PASS") {
				t.Fatal("failed guard did not stop bootstrap", err, string(output))
			}
		} else if err != nil {
			t.Fatal("privileged bootstrap boundary", err, string(output))
		}
	}
}
