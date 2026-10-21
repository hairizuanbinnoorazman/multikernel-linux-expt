//go:build linux

package agent

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCredentialSpecIsSealedAndRejectsPrivilegeGrants(t *testing.T) {
	spec := ProcessSpec{User: User{UID: 1000, GID: 1000, AdditionalGids: []uint32{1000}}, Args: []string{"/bin/true"}, Env: []string{"LD_PRELOAD=/user-selected"}, Cwd: "/work", Capabilities: map[string][]string{}}
	file, err := prepareCredentialSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.Write([]byte("modified")); !errors.Is(err, unix.EPERM) {
		t.Fatal("sealed spec can be changed", err)
	}
	decoded, err := readCredentialSpec(file)
	if err != nil || decoded.User.UID != 1000 || decoded.Env[0] != spec.Env[0] {
		t.Fatal(decoded, err)
	}
	normal, err := os.CreateTemp(t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	defer normal.Close()
	if _, err := readCredentialSpec(normal); err == nil {
		t.Fatal("unsealed spec admitted")
	}
	for _, uid := range []uint32{0, ^uint32(0)} {
		bad := spec
		bad.User.UID = uid
		if f, e := prepareCredentialSpec(bad); e == nil {
			f.Close()
			t.Fatal("invalid UID admitted")
		}
	}
	bad := spec
	bad.Capabilities = map[string][]string{"bounding": {"CAP_CHOWN"}}
	if f, e := prepareCredentialSpec(bad); e == nil {
		f.Close()
		t.Fatal("non-root capability grant admitted")
	}
}

func TestCredentialReexecHelper(t *testing.T) {
	if os.Getenv("MK_CREDENTIAL_EXEC_HELPER") != "1" {
		return
	}
	if err := RunUnprivilegedOCIExec("3"); err != nil {
		os.Stderr.WriteString("credential fixture refused: " + err.Error() + "\n")
		os.Exit(126)
	}
	os.Exit(125)
}
func TestCredentialObservedHelper(t *testing.T) {
	if os.Getenv("MK_CREDENTIAL_OBSERVED") != "1" {
		return
	}
	data, e := os.ReadFile("/proc/self/status")
	if e != nil {
		t.Fatal(e)
	}
	status := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if name, value, ok := strings.Cut(line, ":"); ok {
			status[name] = strings.TrimSpace(value)
		}
	}
	if status["Uid"] != "1000\t1000\t1000\t1000" || status["Gid"] != "1000\t1000\t1000\t1000" || status["NoNewPrivs"] != "1" {
		t.Fatal("wrong credential/NNP transition")
	}
	for _, name := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		value, e := strconv.ParseUint(status[name], 16, 64)
		if e != nil || value != 0 {
			t.Fatal("capability set retained", name)
		}
	}
	cwd, e := os.Getwd()
	if e != nil || cwd != os.Getenv("MK_CREDENTIAL_CWD") {
		t.Fatal("workload cwd was not applied after dropping identity")
	}
	if os.Getenv("LD_PRELOAD") != "/synthetic-workload-preload" {
		t.Fatal("workload environment was not forwarded")
	}
	for _, name := range []string{"MK_CREDENTIAL_EXEC_HELPER", "PATH", "LANG"} {
		if os.Getenv(name) != "" {
			t.Fatal("bootstrap environment leaked", name)
		}
	}
	entries, e := os.ReadDir("/proc/self/fd")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		target, _ := os.Readlink("/proc/self/fd/" + entry.Name())
		if strings.Contains(target, "memfd:mk-oci-process") {
			t.Fatal("sealed spec descriptor leaked into workload")
		}
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("ordinary Unix socket refused", err)
	}
	unix.Close(fd)
	if fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0); !errors.Is(err, unix.EPERM) {
		if fd >= 0 {
			unix.Close(fd)
		}
		t.Fatal("workload native transport was not denied", err)
	}
	for _, call := range []uintptr{unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER} {
		if _, _, err := unix.Syscall(call, 0, 0, 0); err != unix.EPERM {
			t.Fatal("workload io_uring was not denied", call, err)
		}
	}
	marker := os.Getenv("MK_CREDENTIAL_MARKER")
	body, _ := json.Marshal(map[string]bool{"uid_gid_1000": true, "five_capability_sets_zero": true, "nnp": true, "workload_env_and_cwd": true, "spec_fd_closed": true, "native_transport_denied": true})
	if e = os.WriteFile(marker, body, 0600); e != nil {
		t.Fatal(e)
	}
}

// Explicit root-only installed-kernel proof, using a traversable static test
// binary. No image or child kernel is launched and no process is adopted.
func TestInstalledUnprivilegedCredentialTransition(t *testing.T) {
	if os.Getenv("MK_TEST_CREDENTIAL_EXEC") != "1" {
		t.Skip("set MK_TEST_CREDENTIAL_EXEC=1 on qualification VM")
	}
	if err := credentialExecutorSupported(); err != nil {
		t.Fatal(err)
	}
	directory, e := os.MkdirTemp("/tmp", "mk-credential-proof-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(directory)
	if e = os.Chmod(directory, 0755); e != nil {
		t.Fatal(e)
	}
	output := filepath.Join(directory, "output")
	if e = os.Mkdir(output, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Chown(output, 1000, 1000); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(output, "result.json")
	for _, cwd := range []string{output, "/proc/" + strconv.Itoa(os.Getpid()) + "/root"} {
		spec := ProcessSpec{User: User{UID: 1000, GID: 1000, AdditionalGids: []uint32{1000}}, Args: []string{exe, "-test.run=^TestCredentialObservedHelper$"}, Cwd: cwd, Capabilities: map[string][]string{}, Env: []string{"MK_CREDENTIAL_OBSERVED=1", "MK_CREDENTIAL_MARKER=" + marker, "MK_CREDENTIAL_CWD=" + output, "LD_PRELOAD=/synthetic-workload-preload"}}
		file, e := prepareCredentialSpec(spec)
		if e != nil {
			t.Fatal(e)
		}
		command := exec.Command(exe, "-test.run=^TestCredentialReexecHelper$")
		command.Dir = "/"
		command.Env = []string{"MK_CREDENTIAL_EXEC_HELPER=1"}
		command.ExtraFiles = []*os.File{file}
		data, err := command.CombinedOutput()
		file.Close()
		if cwd == output {
			if err != nil {
				t.Fatalf("credential transition: %v %s", err, data)
			}
			info, e := os.Stat(marker)
			if e != nil || info.Size() == 0 {
				t.Fatal("no credential observation", e)
			}
			os.Remove(marker)
		} else {
			if err == nil || !strings.Contains(string(data), "workload cwd refused") {
				t.Fatal("privileged procfs cwd was not refused", err, string(data))
			}
			if _, e := os.Stat(marker); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("refused workload ran")
			}
		}
	}
}

func init() {
	if len(os.Args) >= 4 && os.Args[1] == "__oci_exec" {
		if err := RunOCIExec(os.Args[2], os.Args[3:]); err != nil {
			os.Stderr.WriteString("constraint fixture refused: " + err.Error() + "\n")
			os.Exit(126)
		}
		os.Exit(125)
	}
	// Match the production agent's dispatch in the static installed test binary.
	if len(os.Args) == 3 && os.Args[1] == "__oci_exec_fd" {
		if err := RunUnprivilegedOCIExec(os.Args[2]); err != nil {
			os.Stderr.WriteString("credential fixture refused: " + err.Error() + "\n")
			os.Exit(126)
		}
		os.Exit(125)
	}
}

func TestInstalledManagerCredentialProjection(t *testing.T) {
	if os.Getenv("MK_TEST_CREDENTIAL_EXEC") != "1" {
		t.Skip("installed static root qualification only")
	}
	if err := credentialExecutorSupported(); err != nil {
		t.Fatal(err)
	}
	namespace, e := os.Readlink("/proc/self/ns/mnt")
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestCredentialManagerNamespaceHelper$", "-test.timeout=10s")
	command.Env = []string{"MK_CREDENTIAL_MANAGER_HELPER=1", "MK_CREDENTIAL_PARENT_MNT=" + namespace}
	command.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWNS}
	if output, e := command.CombinedOutput(); e != nil {
		t.Fatalf("manager projection: %v %s", e, output)
	}
}
func TestCredentialManagerNamespaceHelper(t *testing.T) {
	if os.Getenv("MK_CREDENTIAL_MANAGER_HELPER") != "1" {
		return
	}
	namespace, e := os.Readlink("/proc/self/ns/mnt")
	if e != nil || namespace == os.Getenv("MK_CREDENTIAL_PARENT_MNT") {
		t.Fatal("mount namespace was not isolated")
	}
	if e = unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); e != nil {
		t.Fatal(e)
	}
	directory, e := os.MkdirTemp("/tmp", "mk-credential-manager-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(directory)
	root := filepath.Join(directory, "rootfs")
	if e = os.Mkdir(root, 0755); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(root, 0755); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"proc", "output"} {
		if e = os.Mkdir(filepath.Join(root, name), 0755); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.Chown(filepath.Join(root, "output"), 1000, 1000); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(os.Args[0])
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "probe"), data, 0755); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(filepath.Join(root, "probe"), 0755); e != nil {
		t.Fatal(e)
	}
	if e = unix.Mount("/proc", filepath.Join(root, "proc"), "", unix.MS_BIND|unix.MS_REC, ""); e != nil {
		t.Fatal(e)
	}
	defer unix.Unmount(filepath.Join(root, "proc"), unix.MNT_DETACH)
	spec := ProcessSpec{User: User{UID: 1000, GID: 1000, AdditionalGids: []uint32{1000}}, Args: []string{"/probe", "-test.run=^TestCredentialObservedHelper$"}, Cwd: "/output", Capabilities: map[string][]string{}, Env: []string{"MK_CREDENTIAL_OBSERVED=1", "MK_CREDENTIAL_MARKER=/output/result.json", "MK_CREDENTIAL_CWD=/output", "LD_PRELOAD=/synthetic-workload-preload"}}
	value := true
	spec.NoNewPrivileges = &value
	config := OCIConfig{OCIVersion: "1.1.0", Process: spec, Root: Root{Path: "rootfs"}}
	encoded, _ := json.Marshal(config)
	if e = os.WriteFile(filepath.Join(directory, "config.json"), encoded, 0600); e != nil {
		t.Fatal(e)
	}
	manager := NewDockerGuestManager()
	if e = manager.Create("init", directory); e != nil {
		t.Fatal(e)
	}
	if e = manager.Start("init"); e != nil {
		t.Fatal(e)
	}
	state, e := manager.Wait("init")
	if e != nil || state.ExitCode != 0 {
		t.Fatal("projected workload failed", state, e)
	}
	if _, e = os.Stat(filepath.Join(root, "output/result.json")); e != nil {
		t.Fatal(e)
	}
	if e = manager.Delete("init"); e != nil {
		t.Fatal(e)
	}
	// Restricted UID 0 also resolves its cwd after applying capability/NNP
	// restrictions. A privileged procfs root may not become an escape cwd.
	config.Process.User = User{UID: 0, GID: 0}
	config.Process.Cwd = "/proc/" + strconv.Itoa(os.Getpid()) + "/root"
	encoded, _ = json.Marshal(config)
	if e = os.WriteFile(filepath.Join(directory, "config.json"), encoded, 0600); e != nil {
		t.Fatal(e)
	}
	ordinary := NewManager(false)
	if e = ordinary.Create("root-cwd", directory); e != nil {
		t.Fatal(e)
	}
	if e = ordinary.Start("root-cwd"); e != nil {
		t.Fatal(e)
	}
	state, e = ordinary.Wait("root-cwd")
	if e != nil || state.ExitCode != 126 || !strings.Contains(state.Stderr, "workload cwd refused") {
		t.Fatal("privileged root cwd admitted", state, e)
	}
}
