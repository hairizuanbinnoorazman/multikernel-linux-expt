//go:build linux

package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

var hostnameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`)

var supportedMaskedPaths = []string{
	"/proc/acpi", "/proc/asound", "/proc/kcore", "/proc/keys", "/proc/latency_stats",
	"/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug", "/sys/firmware",
	"/sys/devices/virtual/powercap", "/proc/scsi",
}
var supportedReadonlyPaths = []string{"/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"}
var readonlyBindOptions = []string{"bind", "nodev", "noexec", "nosuid", "ro"}

func validateReadonlyBindMounts(mounts []MountSpec) error {
	if len(mounts) > 8 {
		return errors.New("at most eight read-only bind inputs are supported")
	}
	seen := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if mount.Type != "bind" || mount.Source != mount.Destination || mount.Destination == "/" ||
			len(mount.Destination) > 4096 || strings.ContainsRune(mount.Destination, '\x00') ||
			!strings.HasPrefix(mount.Destination, "/") || filepath.Clean(mount.Destination) != mount.Destination {
			return errors.New("read-only bind input is not a sanitized materialized path")
		}
		if len(mount.Options) != len(readonlyBindOptions) {
			return errors.New("read-only bind input has unsupported options")
		}
		for index, option := range readonlyBindOptions {
			if mount.Options[index] != option {
				return errors.New("read-only bind input has unsupported options")
			}
		}
		for _, protected := range []string{"/dev", "/proc", "/run", "/sys"} {
			if mount.Destination == protected || strings.HasPrefix(mount.Destination, protected+"/") {
				return errors.New("read-only bind input overlaps a runtime-owned path")
			}
		}
		for _, prior := range seen {
			if mount.Destination == prior || strings.HasPrefix(mount.Destination, prior+"/") || strings.HasPrefix(prior, mount.Destination+"/") {
				return errors.New("read-only bind inputs overlap")
			}
		}
		seen = append(seen, mount.Destination)
	}
	return nil
}

func validatePolicyPaths(values, allowed []string, name string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if !slices.Contains(allowed, value) || seen[value] {
			return fmt.Errorf("%s contains an unsupported or duplicate path", name)
		}
		seen[value] = true
	}
	return nil
}

func validateRootPolicy(masked, readonly []string) error {
	if len(masked) > len(supportedMaskedPaths) || len(readonly) > len(supportedReadonlyPaths) {
		return errors.New("root path policy exceeds supported bounds")
	}
	if err := validatePolicyPaths(masked, supportedMaskedPaths, "maskedPaths"); err != nil {
		return err
	}
	return validatePolicyPaths(readonly, supportedReadonlyPaths, "readonlyPaths")
}

func policyTarget(root, path string) (string, os.FileInfo, error) {
	if !strings.HasPrefix(path, "/") || filepath.Clean(path) != path {
		return "", nil, errors.New("root policy path is not absolute and canonical")
	}
	target := filepath.Join(root, strings.TrimPrefix(path, "/"))
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("root policy target is unavailable or symlinked")
	}
	return target, info, nil
}

func readonlyBindTarget(root, destination string) (string, error) {
	target := filepath.Join(root, strings.TrimPrefix(destination, "/"))
	if err := secureDirectory(filepath.Dir(target)); err != nil {
		return "", err
	}
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return "", errors.New("materialized bind target must be a real directory or regular file")
	}
	return target, nil
}

func applyRootPolicy(config OCIConfig, root string) (retErr error) {
	if config.Hostname != "" && (len(config.Hostname) > 63 || !hostnameRE.MatchString(config.Hostname)) {
		return errors.New("hostname is malformed")
	}
	var mounted []string
	previousHostname, _ := os.Hostname()
	hostnameChanged := false
	defer func() {
		if retErr == nil {
			return
		}
		for index := len(mounted) - 1; index >= 0; index-- {
			_ = unix.Unmount(mounted[index], unix.MNT_DETACH)
		}
		if hostnameChanged {
			_ = unix.Sethostname([]byte(previousHostname))
		}
	}()
	if config.Hostname != "" {
		if err := unix.Sethostname([]byte(config.Hostname)); err != nil {
			return fmt.Errorf("set sandbox hostname: %w", err)
		}
		hostnameChanged = true
	}
	for _, mount := range config.Mounts {
		target, err := readonlyBindTarget(root, mount.Destination)
		if err != nil {
			return fmt.Errorf("read-only bind input %s: %w", mount.Destination, err)
		}
		if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind materialized input %s: %w", mount.Destination, err)
		}
		mounted = append(mounted, target)
		if err := unix.Mount("", target, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
			return fmt.Errorf("make materialized input %s read-only: %w", mount.Destination, err)
		}
	}
	if config.Linux != nil {
		for _, path := range config.Linux.MaskedPaths {
			target, info, err := policyTarget(root, path)
			if err != nil {
				return err
			}
			if info == nil {
				continue
			}
			if info.IsDir() {
				err = unix.Mount("tmpfs", target, "tmpfs", unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "size=0,mode=000")
			} else {
				err = unix.Mount("/dev/null", target, "", unix.MS_BIND, "")
				if err == nil {
					mounted = append(mounted, target)
					err = unix.Mount("", target, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "")
				}
			}
			if err != nil {
				return fmt.Errorf("mask %s: %w", path, err)
			}
			if info.IsDir() {
				mounted = append(mounted, target)
			}
		}
		for _, path := range config.Linux.ReadonlyPaths {
			target, info, err := policyTarget(root, path)
			if err != nil {
				return err
			}
			if info == nil {
				continue
			}
			if err = unix.Mount(target, target, "", unix.MS_BIND|unix.MS_REC, ""); err == nil {
				mounted = append(mounted, target)
				err = unix.Mount("", target, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, "")
			}
			if err != nil {
				return fmt.Errorf("make %s read-only: %w", path, err)
			}
		}
	}
	if config.Root.Readonly != nil && *config.Root.Readonly {
		if err := unix.Mount(root, root, "", unix.MS_BIND|unix.MS_REC, ""); err == nil {
			mounted = append(mounted, root)
			if err = unix.Mount("", root, "", unix.MS_BIND|unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
				return fmt.Errorf("make OCI root read-only: %w", err)
			}
		} else {
			return fmt.Errorf("bind OCI root for read-only policy: %w", err)
		}
	}
	return nil
}
