//go:build linux

package transportguard

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func checkSockets(t *testing.T) {
	t.Helper()
	for _, domain := range []int{unix.AF_UNIX, unix.AF_INET} {
		fd, err := unix.Socket(domain, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			t.Fatal("ordinary socket blocked", domain, err)
		}
		unix.Close(fd)
	}
	if fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0); !errors.Is(err, unix.EPERM) {
		if fd >= 0 {
			unix.Close(fd)
		}
		t.Fatal("vsock socket not denied", err)
	}
	if fds, err := unix.Socketpair(unix.AF_VSOCK, unix.SOCK_STREAM, 0); !errors.Is(err, unix.EPERM) {
		if err == nil {
			unix.Close(fds[0])
			unix.Close(fds[1])
		}
		t.Fatal("vsock socketpair not denied", err)
	}
	for _, nr := range []uintptr{unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER} {
		_, _, errno := unix.Syscall6(nr, 0, 0, 0, 0, 0, 0)
		if errno != unix.EPERM {
			t.Fatal("io_uring not denied", nr, errno)
		}
	}
}
func TestTransportGuardExecHelper(t *testing.T) {
	mode := os.Getenv("MK_GUARD_HELPER")
	if mode == "" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch mode {
	case "install", "inherit", "x32":
		if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := Install(); err != nil {
			t.Fatal(err)
		}
		checkSockets(t)
		if mode == "inherit" {
			if err := unix.Exec(os.Args[0], []string{os.Args[0], "-test.run=^TestTransportGuardExecHelper$"}, []string{"MK_GUARD_HELPER=observe"}); err != nil {
				t.Fatal(err)
			}
		}
		if mode == "x32" {
			unix.RawSyscall(0x40000000|unix.SYS_GETUID, 0, 0, 0)
			t.Fatal("x32 ABI survived")
		}
	case "close-workload", "close-bootstrap":
		first := uint(3)
		if mode == "close-bootstrap" {
			first = 4
		}
		if err := CloseInheritedFrom(first); err != nil {
			t.Fatal(err)
		}
		var status unix.Stat_t
		if err := unix.Fstat(4, &status); !errors.Is(err, unix.EBADF) {
			t.Fatal("inherited socket descriptor survived", err)
		}
		err := unix.Fstat(3, &status)
		if first == 3 && !errors.Is(err, unix.EBADF) {
			t.Fatal("workload fd3 survived", err)
		}
		if first == 4 && err != nil {
			t.Fatal("bootstrap script descriptor was closed", err)
		}
	case "install-failure":
		if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		code := []unix.SockFilter{{Code: 0x20, K: 0}, {Code: 0x15, K: unix.SYS_SECCOMP, Jf: 1}, {Code: 0x06, K: uint32(unix.SECCOMP_RET_ERRNO) | uint32(unix.EPERM)}, {Code: 0x06, K: unix.SECCOMP_RET_ALLOW}}
		filter := unix.SockFprog{Len: uint16(len(code)), Filter: &code[0]}
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&filter)), 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := Install(); err == nil {
			t.Fatal("failed seccomp installation admitted")
		}
	case "observe":
		checkSockets(t)
	default:
		t.Fatal("unknown fixture mode")
	}
}
func TestTransportGuardEnforcesAndSurvivesExec(t *testing.T) {
	for _, mode := range []string{"install", "inherit", "x32"} {
		command := exec.Command(os.Args[0], "-test.run=^TestTransportGuardExecHelper$")
		command.Env = append(os.Environ(), "MK_GUARD_HELPER="+mode)
		data, err := command.CombinedOutput()
		if mode == "x32" {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal("unsupported ABI not killed", err, string(data))
			}
			state := exit.Sys().(syscall.WaitStatus)
			if !state.Signaled() || state.Signal() != unix.SIGSYS {
				t.Fatal("wrong unsupported ABI outcome", state, string(data))
			}
		} else if err != nil {
			t.Fatal(mode, err, string(data))
		}
	}
}
func TestTransportGuardFailureAndDescriptorBoundary(t *testing.T) {
	for _, mode := range []string{"install-failure", "close-workload", "close-bootstrap"} {
		command := exec.Command(os.Args[0], "-test.run=^TestTransportGuardExecHelper$")
		command.Env = append(os.Environ(), "MK_GUARD_HELPER="+mode)
		file, err := os.CreateTemp(t.TempDir(), "bootstrap")
		if err != nil {
			t.Fatal(err)
		}
		sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			file.Close()
			t.Fatal(err)
		}
		inherited := os.NewFile(uintptr(sockets[0]), "inherited socket")
		command.ExtraFiles = []*os.File{file, inherited}
		output, err := command.CombinedOutput()
		file.Close()
		inherited.Close()
		unix.Close(sockets[1])
		if err != nil {
			t.Fatal(mode, err, string(output))
		}
	}
}
