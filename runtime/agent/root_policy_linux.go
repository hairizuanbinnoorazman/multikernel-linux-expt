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
