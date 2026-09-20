package hostconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Config struct {
	SchemaVersion           int    `json:"schema_version"`
	StateDirectory          string `json:"state_directory"`
	SocketPath              string `json:"socket_path"`
	KerfExecutable          string `json:"kerf_executable"`
	MultikernelSysfsRoot    string `json:"multikernel_sysfs_root"`
	KernelManifestDirectory string `json:"kernel_manifest_directory"`
	MinPrimaryCPUs          int    `json:"min_primary_cpus"`
	MinPrimaryMemoryBytes   uint64 `json:"min_primary_memory_bytes"`
	ForbiddenAPICIDs        []int  `json:"forbidden_apic_ids"`
	BackendTimeoutSeconds   int    `json:"backend_timeout_seconds"`
	MaxFrameSizeBytes       int    `json:"max_frame_size_bytes"`
}

func (c Config) BackendTimeout() time.Duration {
	return time.Duration(c.BackendTimeoutSeconds) * time.Second
}

func Load(path string) (Config, error) {
	return load(path, 0, "/")
}

func load(path string, requiredUID uint32, boundary string) (Config, error) {
	var config Config
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return config, errors.New("host configuration path must be canonical and absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return config, err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return config, errors.New("host configuration must be a regular file or managed symlink")
	}
	if err = requireOwner(info, requiredUID); err != nil {
		return config, err
	}
	if err = safeParents(filepath.Dir(path), requiredUID, boundary); err != nil {
		return config, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return config, err
	}
	if !filepath.IsAbs(resolved) || filepath.Clean(resolved) != resolved {
		return config, errors.New("resolved host configuration path is unsafe")
	}
	if err = safeParents(filepath.Dir(resolved), requiredUID, boundary); err != nil {
		return config, err
	}
	raw, err := readStableConfig(resolved, requiredUID)
	if err != nil {
		return config, err
	}
	if err = protocol.StrictDecode(raw, &config); err != nil {
		return config, err
	}
	if err = config.validate(requiredUID, boundary); err != nil {
		return config, err
	}
	return config, nil
}

func sameFileState(first, second os.FileInfo) bool {
	left, leftOK := first.Sys().(*syscall.Stat_t)
	right, rightOK := second.Sys().(*syscall.Stat_t)
	return leftOK && rightOK && left.Dev == right.Dev && left.Ino == right.Ino &&
		left.Size == right.Size && left.Mtim == right.Mtim && left.Ctim == right.Ctim
}

func readStableConfig(path string, requiredUID uint32) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0137 != 0 || before.Size() < 1 || before.Size() > 1<<20 {
		return nil, errors.New("host configuration target must be a bounded regular file with mode 0640 or stricter")
	}
	if err = requireOwner(before, requiredUID); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !sameFileState(before, opened) {
		return nil, errors.Join(errors.New("host configuration identity changed while opening"), err)
	}
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.Join(errors.New("host configuration read failed or exceeded its bound"), err)
	}
	after, err := file.Stat()
	if err != nil || !sameFileState(opened, after) {
		return nil, errors.Join(errors.New("host configuration identity changed while reading"), err)
	}
	named, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, named) || !sameFileState(opened, named) {
		return nil, errors.Join(errors.New("host configuration target changed while reading"), err)
	}
	return raw, nil
}

func (c Config) validate(requiredUID uint32, boundary string) error {
	if c.SchemaVersion != 1 || c.MinPrimaryCPUs < 1 || c.MinPrimaryMemoryBytes < 512<<20 ||
		c.BackendTimeoutSeconds < 1 || c.BackendTimeoutSeconds > 3600 ||
		c.MaxFrameSizeBytes < 4096 || c.MaxFrameSizeBytes > 16<<20 {
		return errors.New("host configuration value is outside the v1 bounds")
	}
	for _, value := range []string{c.StateDirectory, c.SocketPath, c.KerfExecutable, c.MultikernelSysfsRoot, c.KernelManifestDirectory} {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("unsafe or relative configured path %q", value)
		}
	}
	seen, hasZero := map[int]bool{}, false
	for _, id := range c.ForbiddenAPICIDs {
		if id < 0 || seen[id] {
			return errors.New("forbidden APIC IDs must be unique non-negative integers")
		}
		seen[id], hasZero = true, hasZero || id == 0
	}
	if !hasZero {
		return errors.New("forbidden APIC IDs must include 0")
	}
	for _, parent := range []string{filepath.Dir(c.StateDirectory), filepath.Dir(c.SocketPath), filepath.Dir(c.KernelManifestDirectory)} {
		if err := safeParents(parent, requiredUID, boundary); err != nil {
			return err
		}
	}
	for _, target := range []string{c.KerfExecutable, c.MultikernelSysfsRoot, c.KernelManifestDirectory} {
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		if err = requireOwner(info, requiredUID); err != nil {
			return err
		}
		if info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("configured target %s is group/world writable", target)
		}
	}
	return nil
}

func requireOwner(info os.FileInfo, uid uint32) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uid {
		return fmt.Errorf("%s is not owned by required uid %d", info.Name(), uid)
	}
	return nil
}

func safeParents(start string, uid uint32, boundary string) error {
	boundary = filepath.Clean(boundary)
	for current := filepath.Clean(start); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("unsafe writable parent directory %s", current)
		}
		if err = requireOwner(info, uid); err != nil {
			return err
		}
		if current == boundary {
			return nil
		}
		parent := filepath.Dir(current)
		if parent == current || !within(parent, boundary) {
			return fmt.Errorf("path %s escapes validation boundary %s", start, boundary)
		}
	}
}

func within(path, boundary string) bool {
	relative, err := filepath.Rel(boundary, path)
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && (relative == "." || len(relative) < 3 || relative[:3] != "../")
}
