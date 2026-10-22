//go:build linux

package dockerengine

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

const imageRoot = "/bundle/rootfs"
const rootlessState = "/run/user/1000/rootlesskit"
const daemonConfiguration = `{"rootless":true,"storage-driver":"vfs","features":{"containerd-snapshotter":false},"iptables":false,"ip6tables":false,"ip-forward":false,"ip-masq":false,"hosts":["unix:///var/run/docker.sock"],"data-root":"/home/rootless/.local/share/docker","exec-root":"/run/user/1000/docker","pidfile":"/run/user/1000/docker.pid"}`

// directoryIdentity refuses aliases in every component, including image-owned
// ancestors. Before client admission only trusted boot processes are running;
// their outer root and approved image cannot be replaced by another writer.
func directoryIdentity(path string) (unix.Stat_t, error) {
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return unix.Stat_t{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	err = unix.Fstat(fd, &st)
	return st, err
}

func privateDataDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if _, err := directoryIdentity(path); err != nil {
		return err
	}
	if err := os.Chown(path, 1000, 1000); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func readonlyBind(source, target string) error {
	if err := unix.Mount(source, target, "", unix.MS_BIND, ""); err != nil {
		return err
	}
	return unix.Mount("", target, "", unix.MS_REMOUNT|unix.MS_BIND|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "")
}

// PrepareFilesystem runs once in the trusted guest root before any client can
// be admitted. Generic init has already mounted curated dev/proc/sys and a fresh
// inner /run tmpfs. The builder must install independent outer helpers/accounts
// and approved modules. Failure does not serve clients or permit in-place retry;
// the runtime retains ownership of the failed child for teardown.
func PrepareFilesystem() (*PreparedFilesystem, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, errors.New("Docker preparation requires trusted root identity")
	}
	for _, path := range []string{imageRoot, imageRoot + "/dev", imageRoot + "/proc", imageRoot + "/sys", imageRoot + "/sys/fs/cgroup", imageRoot + "/run", imageRoot + "/home", imageRoot + "/home/rootless", "/sys/fs/cgroup", rootlessState} {
		if _, err := directoryIdentity(path); err != nil {
			return nil, errors.New("Docker preparation directory refused")
		}
	}
	for _, path := range []string{imageRoot + "/home/rootless/.local", imageRoot + "/home/rootless/.local/tmp"} {
		if _, err := directoryIdentity(path); err != nil && !errors.Is(err, unix.ENOENT) {
			return nil, errors.New("Docker data directory alias refused")
		}
	}
	state, err := directoryIdentity(rootlessState)
	if err != nil || state.Uid != 1000 || state.Gid != 1000 || state.Mode&0777 != 0700 {
		return nil, errors.New("Docker outer state identity refused")
	}
	stateDir, err := os.Open(rootlessState)
	if err != nil {
		return nil, errors.New("Docker outer state unavailable")
	}
	entries, readErr := stateDir.Readdirnames(1)
	closeErr := stateDir.Close()
	if len(entries) != 0 || readErr != io.EOF || closeErr != nil {
		return nil, errors.New("Docker outer state must be fresh")
	}
	run := imageRoot + "/run"
	status, err := directoryIdentity(run)
	var fs unix.Statfs_t
	if err != nil || status.Uid != 0 || status.Mode&0022 != 0 || unix.Statfs(run, &fs) != nil || fs.Type != unix.TMPFS_MAGIC {
		return nil, errors.New("Docker inner runtime must be a fresh protected tmpfs")
	}
	// Enable only missing controllers; the trusted root cgroup is the parent of
	// the non-delegated bounded service scope. Existing enablement is retained.
	controls, err := os.ReadFile("/sys/fs/cgroup/cgroup.subtree_control")
	if err != nil {
		return nil, errors.New("Docker service controller inventory failed")
	}
	enabled := strings.Fields(string(controls))
	missing := []string{}
	for _, name := range []string{"memory", "pids"} {
		if !slices.Contains(enabled, name) {
			missing = append(missing, "+"+name)
		}
	}
	if len(missing) > 0 {
		if err := os.WriteFile("/sys/fs/cgroup/cgroup.subtree_control", []byte(strings.Join(missing, " ")), 0200); err != nil {
			return nil, errors.New("Docker service controller enablement failed")
		}
	}
	controls, err = os.ReadFile("/sys/fs/cgroup/cgroup.subtree_control")
	if err != nil {
		return nil, errors.New("Docker service controller readback failed")
	}
	for _, name := range []string{"memory", "pids"} {
		if !slices.Contains(strings.Fields(string(controls)), name) {
			return nil, errors.New("Docker service controller readback failed")
		}
	}
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return nil, errors.New("Docker private mount preparation failed")
	}
	if err := unix.Mount(imageRoot, imageRoot, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return nil, errors.New("Docker image mount preparation failed")
	}
	if err := readonlyBind("/sys/fs/cgroup", imageRoot+"/sys/fs/cgroup"); err != nil {
		return nil, errors.New("Docker cgroup view preparation failed")
	}
	if err := os.Mkdir(run+"/user", 0755); err != nil {
		return nil, errors.New("Docker inner runtime is not fresh")
	}
	for _, path := range []string{run + "/user/1000", imageRoot + rootlessState, imageRoot + "/home/rootless/.local", imageRoot + "/home/rootless/.local/tmp"} {
		if err := privateDataDirectory(path); err != nil {
			return nil, errors.New("Docker private data preparation failed")
		}
	}
	// RootlessKit removes its state directory on exit. A source mountpoint
	// keeps that inode linked until both owned binds can be released normally.
	if err := unix.Mount(rootlessState, rootlessState, "", unix.MS_BIND, ""); err != nil {
		return nil, errors.New("Docker source state mount preparation failed")
	}
	if err := unix.Mount(rootlessState, imageRoot+rootlessState, "", unix.MS_BIND, ""); err != nil {
		return nil, errors.New("Docker state sharing failed")
	}
	config := filepath.Join(run, "docker-daemon.json")
	fd, err := unix.Open(config, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, errors.New("Docker configuration destination refused")
	}
	file := os.NewFile(uintptr(fd), "Docker daemon configuration")
	_, writeErr := io.WriteString(file, daemonConfiguration+"\n")
	modeErr := file.Chmod(0444)
	closeErr = file.Close()
	if writeErr != nil || modeErr != nil || closeErr != nil {
		return nil, errors.New("Docker configuration creation failed")
	}
	if err := readonlyBind(config, config); err != nil {
		return nil, errors.New("Docker configuration sealing failed")
	}
	if err := os.Chown(run, 1000, 1000); err != nil {
		return nil, errors.New("Docker socket directory ownership failed")
	}
	return captureStateMount()
}
