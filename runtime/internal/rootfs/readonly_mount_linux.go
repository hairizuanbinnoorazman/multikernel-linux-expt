package rootfs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd/mount"
	"golang.org/x/sys/unix"
)

// Clone a recursive mount view rather than changing the caller's mount. The
// detached descriptor owns the clone; overlay/bind mounts retain their own
// references, and closing our descriptors leaves no staging mountpoints.
func readonlyRootfsClone(ctx context.Context, path string) (*os.File, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	fd, err := unix.OpenTree(unix.AT_FDCWD, path, unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC|unix.AT_RECURSIVE)
	if err != nil {
		return nil, "", fmt.Errorf("clone rootfs mount: %w", err)
	}
	file := os.NewFile(uintptr(fd), "readonly-rootfs-view")
	if err := ctx.Err(); err != nil {
		file.Close()
		return nil, "", err
	}
	attributes := &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NODEV | unix.MOUNT_ATTR_NOSUID | unix.MOUNT_ATTR_NOEXEC | unix.MOUNT_ATTR_NOATIME, Attr_clr: unix.MOUNT_ATTR__ATIME, Propagation: unix.MS_PRIVATE}
	if err := unix.MountSetattr(fd, "", unix.AT_EMPTY_PATH|unix.AT_RECURSIVE, attributes); err != nil {
		file.Close()
		return nil, "", fmt.Errorf("seal recursive rootfs mount: %w", err)
	}
	if err := ctx.Err(); err != nil {
		file.Close()
		return nil, "", err
	}
	return file, fmt.Sprintf("/proc/self/fd/%d", fd), nil
}

func sealRootfsMountInputs(ctx context.Context, input []mount.Mount) ([]mount.Mount, []*os.File, error) {
	result := make([]mount.Mount, len(input))
	var files []*os.File
	seal := func(path string) (string, error) {
		file, anchored, err := readonlyRootfsClone(ctx, path)
		if err != nil {
			return "", err
		}
		files = append(files, file)
		return anchored, nil
	}
	for index, item := range input {
		if err := ctx.Err(); err != nil {
			return nil, files, err
		}
		item.Options = append([]string(nil), item.Options...)
		var err error
		if item.Type == "bind" || item.Type == "none" {
			item.Source, err = seal(item.Source)
			if err != nil {
				return nil, files, err
			}
			// The clone already has recursive readonly/security/atime flags.
			// Avoid containerd's bind-remount step: our target descriptor pins
			// the original directory beneath the newly attached mount, so a
			// remount through that descriptor would address the old mount.
			options := item.Options[:0]
			for _, option := range item.Options {
				if option != "ro" {
					options = append(options, option)
				}
			}
			item.Options = options
		}
		for optionIndex, option := range item.Options {
			if item.Type != "overlay" || !strings.HasPrefix(option, "lowerdir=") {
				continue
			}
			paths := strings.Split(strings.TrimPrefix(option, "lowerdir="), ":")
			for pathIndex, path := range paths {
				paths[pathIndex], err = seal(path)
				if err != nil {
					return nil, files, err
				}
			}
			item.Options[optionIndex] = "lowerdir=" + strings.Join(paths, ":")
		}
		result[index] = item
	}
	return result, files, nil
}
