//go:build linux

package dockerengine

import (
	"errors"
	"os"
	"runtime"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/transportguard"
	"golang.org/x/sys/unix"
)

// ExecBootstrap runs only in the newly cloned, owned service scope. It fences
// primary transports before the approved root bootstrap or mapping helpers run.
func ExecBootstrap() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return errors.New("Docker bootstrap requires trusted root identity")
	}
	if unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		return errors.New("Docker bootstrap protection failed")
	}
	flags, err := unix.FcntlInt(3, unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return errors.New("Docker bootstrap descriptor must be read-only")
	}
	var status unix.Stat_t
	if unix.Fstat(3, &status) != nil || status.Mode&unix.S_IFMT != unix.S_IFREG || status.Uid != 0 || status.Nlink != 1 || status.Mode&0022 != 0 || status.Size < 18 || status.Size > 1<<20 {
		return errors.New("Docker bootstrap descriptor refused")
	}
	prefix := make([]byte, len("#!/bin/busybox sh\n"))
	if n, err := unix.Pread(3, prefix, 0); err != nil || n != len(prefix) || string(prefix) != "#!/bin/busybox sh\n" {
		return errors.New("Docker bootstrap interpreter refused")
	}
	if err := transportguard.Install(); err != nil {
		return err
	}
	if err := transportguard.CloseInheritedFrom(4); err != nil {
		return err
	}
	return unix.Exec("/bin/busybox", []string{"/bin/busybox", "sh", "/proc/self/fd/3"}, []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/root", "LANG=C", "LC_ALL=C"})
}
