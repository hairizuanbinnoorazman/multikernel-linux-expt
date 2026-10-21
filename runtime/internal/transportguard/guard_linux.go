//go:build linux

// Package transportguard excludes native primary transports from untrusted
// guest processes. It does not restrict the trusted agent/relay/NBD processes.
package transportguard

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Install synchronizes an irreversible seccomp filter across the entire process.
// Call before untrusted code; descendants inherit it across fork/exec/userns.
// NNP or CAP_SYS_ADMIN is required. The privileged Docker bootstrap deliberately
// uses CAP_SYS_ADMIN, preserving setuid mapping helpers until namespace setup.
func Install() error {
	if runtime.GOARCH != "amd64" {
		return errors.New("transport guard requires audited amd64 ABI")
	}
	program := instructions()
	descriptor := unix.SockFprog{Len: uint16(len(program)), Filter: &program[0]}
	result, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&descriptor)))
	runtime.KeepAlive(program)
	if errno != 0 || result != 0 {
		return errors.New("guest transport guard synchronization failed")
	}
	return nil
}

func instructions() []unix.SockFilter {
	load := func(offset uint32) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: offset}
	}
	equal := func(value uint32, yes, no uint8) unix.SockFilter {
		return unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: value, Jt: yes, Jf: no}
	}
	ret := func(value uint32) unix.SockFilter { return unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: value} }
	deny := uint32(unix.SECCOMP_RET_ERRNO) | uint32(unix.EPERM)
	return []unix.SockFilter{
		load(4), equal(unix.AUDIT_ARCH_X86_64, 1, 0), ret(unix.SECCOMP_RET_KILL_PROCESS),
		load(0), {Code: unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K, K: 0x40000000, Jt: 0, Jf: 1}, ret(unix.SECCOMP_RET_KILL_PROCESS),
		// io_uring can create sockets without the socket syscall filter seeing them.
		equal(unix.SYS_IO_URING_SETUP, 0, 1), ret(deny),
		equal(unix.SYS_IO_URING_ENTER, 0, 1), ret(deny),
		equal(unix.SYS_IO_URING_REGISTER, 0, 1), ret(deny),
		equal(unix.SYS_SOCKET, 1, 0), equal(unix.SYS_SOCKETPAIR, 0, 3),
		load(16), equal(unix.AF_VSOCK, 0, 1), ret(deny),
		ret(unix.SECCOMP_RET_ALLOW),
	}
}

// CloseInheritedFrom closes every non-stdio descriptor, preserving only the
// bootstrap script at fd3 when first is 4. Call on an OS-thread-locked executor
// immediately before exec. Unsharing the descriptor table prevents concurrent
// Go runtime opens from repopulating the table inherited by untrusted code.
func CloseInheritedFrom(first uint) error {
	if first != 3 && first != 4 {
		return errors.New("invalid guest descriptor boundary")
	}
	if err := unix.CloseRange(first, ^uint(0), unix.CLOSE_RANGE_UNSHARE); err != nil {
		return errors.New("guest inherited descriptor close failed")
	}
	return nil
}
