//go:build linux

package agent

import (
	"debug/elf"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/transportguard"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

const maxCredentialSpec = 1 << 20
const credentialSeals = unix.F_SEAL_SEAL | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_WRITE

func protectOCIExecutor() error {
	if unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		return errors.New("OCI executor process protection failed")
	}
	value, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil || value != 0 {
		return errors.New("OCI executor process protection readback failed")
	}
	return nil
}

func validateStaticOCIExecutor() error {
	image, err := elf.Open("/proc/self/exe")
	if err != nil {
		return errors.New("trusted OCI executor ELF unavailable")
	}
	defer image.Close()
	for _, program := range image.Progs {
		if program.Type == elf.PT_INTERP {
			return errors.New("trusted OCI executor must be statically linked")
		}
	}
	if _, _, errno := syscall.AllThreadsSyscall(syscall.SYS_GETUID, 0, 0, 0); errno != 0 {
		return errors.New("trusted OCI executor must use pure Go")
	}
	return nil
}

func validateCredentialProcess(spec ProcessSpec) error {
	if err := validateProcessSpec(spec); err != nil {
		return err
	}
	if spec.User.UID == 0 || spec.User.UID == ^uint32(0) || spec.User.GID == ^uint32(0) {
		return errors.New("credential executor requires valid non-root IDs")
	}
	for _, group := range spec.User.AdditionalGids {
		if group == ^uint32(0) {
			return errors.New("credential executor group ID is invalid")
		}
	}
	for _, values := range spec.Capabilities {
		if len(values) > 0 {
			return errors.New("non-root credential executor refuses capability grants")
		}
	}
	return nil
}

// User-controlled environment/cwd are not interpreted with bootstrap identity.
// The static executor gets a fixed environment and a sealed anonymous spec FD.
func prepareCredentialSpec(spec ProcessSpec) (*os.File, error) {
	if err := validateCredentialProcess(spec); err != nil {
		return nil, err
	}
	data, err := json.Marshal(spec)
	if err != nil || len(data) > maxCredentialSpec {
		return nil, errors.New("credential executor spec exceeds bound")
	}
	descriptor, err := unix.MemfdCreate("mk-oci-process", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, errors.New("credential executor sealed spec unavailable")
	}
	file := os.NewFile(uintptr(descriptor), "OCI process spec")
	success := false
	defer func() {
		if !success {
			file.Close()
		}
	}()
	if err = file.Chmod(0600); err != nil {
		return nil, err
	}
	if _, err = file.Write(data); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err = unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, credentialSeals); err != nil {
		return nil, err
	}
	success = true
	return file, nil
}

func readCredentialSpec(file *os.File) (ProcessSpec, error) {
	var spec ProcessSpec
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || seals&credentialSeals != credentialSeals {
		return spec, errors.New("credential spec is not sealed")
	}
	var status unix.Stat_t
	if unix.Fstat(int(file.Fd()), &status) != nil || status.Mode&unix.S_IFMT != unix.S_IFREG || status.Mode&0777 != 0600 || status.Uid != uint32(os.Geteuid()) || status.Size <= 0 || status.Size > maxCredentialSpec {
		return spec, errors.New("credential spec metadata refused")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return spec, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxCredentialSpec+1))
	if err != nil || int64(len(data)) != status.Size {
		return spec, errors.New("credential spec read failed")
	}
	if err = protocol.StrictDecode(data, &spec); err != nil {
		return spec, errors.New("credential spec decode failed")
	}
	return spec, validateCredentialProcess(spec)
}

func credentialExecutorSupported() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return errors.New("credential execution requires the trusted root agent")
	}
	if err := validateStaticOCIExecutor(); err != nil {
		return err
	}
	// Read-only probe: cgo-linked programs cannot synchronize all Go threads.
	uid, _, errno := syscall.AllThreadsSyscall(syscall.SYS_GETUID, 0, 0, 0)
	if errno != 0 || uid != 0 {
		return errors.New("credential execution requires a pure Go agent")
	}
	return nil
}

// RunUnprivilegedOCIExec runs only in a fresh child of the trusted root agent.
// Root privileges are used to clear bounding bits before changing all IDs.
// No workload environment, cwd or executable is used until that transition.
func RunUnprivilegedOCIExec(descriptor string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if descriptor != "3" || os.Getuid() != 0 || os.Geteuid() != 0 {
		return errors.New("credential executor invocation refused")
	}
	if err := protectOCIExecutor(); err != nil {
		return err
	}
	if err := credentialExecutorSupported(); err != nil {
		return err
	}
	file := os.NewFile(3, "OCI process spec")
	spec, err := readCredentialSpec(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.New("credential executor spec refused")
	}
	for _, limit := range spec.Rlimits {
		if unix.Setrlimit(rlimitResources[limit.Type], &unix.Rlimit{Cur: limit.Soft, Max: limit.Hard}) != nil {
			return errors.New("credential executor rlimit failed")
		}
	}
	// NNP is set across every runtime thread before credential changes. A
	// sibling thread cannot retain an exec-gain path during the UID transition.
	for _, control := range []struct{ option, value uintptr }{{unix.PR_SET_NO_NEW_PRIVS, 1}, {unix.PR_SET_KEEPCAPS, 0}} {
		_, _, errno := syscall.AllThreadsSyscall6(syscall.SYS_PRCTL, control.option, control.value, 0, 0, 0, 0)
		if errno != 0 {
			return errors.New("credential executor thread policy failed")
		}
	}
	if err := transportguard.Install(); err != nil {
		return err
	}
	for capability := uint(0); capability <= unix.CAP_LAST_CAP; capability++ {
		if unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(capability), 0, 0, 0) != nil {
			return errors.New("credential executor bounding drop failed")
		}
	}
	groups := make([]int, len(spec.User.AdditionalGids))
	for i, gid := range spec.User.AdditionalGids {
		groups[i] = int(gid)
	}
	if syscall.Setgroups(groups) != nil || syscall.Setresgid(int(spec.User.GID), int(spec.User.GID), int(spec.User.GID)) != nil || syscall.Setresuid(int(spec.User.UID), int(spec.User.UID), int(spec.User.UID)) != nil {
		return errors.New("credential executor identity change failed")
	}
	// These syscall wrappers synchronize credentials across all runtime threads.
	uid, euid, suid := unix.Getresuid()
	gid, egid, sgid := unix.Getresgid()
	if uid != int(spec.User.UID) || euid != uid || suid != uid || gid != int(spec.User.GID) || egid != gid || sgid != gid {
		return errors.New("credential executor identity readback failed")
	}
	zero := [2]unix.CapUserData{}
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if unix.Capset(&header, &zero[0]) != nil || unix.Capget(&header, &zero[0]) != nil || zero != [2]unix.CapUserData{} {
		return errors.New("credential executor capability clear failed")
	}
	if unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0) != nil {
		return errors.New("credential executor ambient clear failed")
	}
	for capability := uint(0); capability <= unix.CAP_LAST_CAP; capability++ {
		value, e := unix.PrctlRetInt(unix.PR_CAPBSET_READ, uintptr(capability), 0, 0, 0)
		if e != nil || value != 0 {
			return errors.New("credential executor bounding readback failed")
		}
	}
	if err := protectOCIExecutor(); err != nil {
		return err
	}
	value, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || value != 1 {
		return errors.New("credential executor NNP readback failed")
	}
	if err = os.Chdir(spec.Cwd); err != nil {
		return errors.New("credential executor workload cwd refused")
	}
	if err := transportguard.CloseInheritedFrom(3); err != nil {
		return err
	}
	return unix.Exec(spec.Args[0], spec.Args, spec.Env)
}
