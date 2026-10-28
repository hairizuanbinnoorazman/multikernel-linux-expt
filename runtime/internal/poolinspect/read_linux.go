//go:build linux

package poolinspect

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var ErrKernelPoolUnavailable = errors.New("kernel pool metadata is unavailable")

type Snapshot struct {
	CPUs   CPUs   `json:"cpus"`
	Memory Memory `json:"memory"`
}

// Read observes the explicit kernel pool free list and memory resource map.
// It never invokes Kerf, initializes a pool or reconciles ownership. Callers
// must bracket/repeat observations to detect drift; these files are not atomic.
func Read(ctx context.Context, sysfs string) (Snapshot, error) {
	if !filepath.IsAbs(sysfs) || filepath.Clean(sysfs) != sysfs {
		return Snapshot{}, ErrKernelPoolUnavailable
	}
	tree, err := readBounded(ctx, filepath.Join(sysfs, "device_tree"), 1<<20, 0)
	if err != nil {
		return Snapshot{}, err
	}
	cpus, err := ParseCPUs(tree)
	if err != nil {
		return Snapshot{}, err
	}
	iomem, err := readBounded(ctx, "/proc/iomem", maxMemoryMapBytes, 0)
	if err != nil {
		return Snapshot{}, err
	}
	memory, err := ParseMemory(iomem)
	if err != nil {
		return Snapshot{}, err
	}
	if err = ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{CPUs: cpus, Memory: memory}, nil
}

func readBounded(ctx context.Context, path string, maximum int64, uid uint32) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, ErrKernelPoolUnavailable
	}
	file := os.NewFile(uintptr(fd), "kernel-pool-metadata")
	defer file.Close()
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Uid != uid || before.Nlink != 1 || before.Mode&0o022 != 0 || before.Size > maximum {
		return nil, ErrKernelPoolUnavailable
	}
	data := make([]byte, 0, 4096)
	buffer := make([]byte, 4096)
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := file.Read(buffer)
		if int64(len(data))+int64(n) > maximum {
			return nil, ErrKernelPoolUnavailable
		}
		data = append(data, buffer[:n]...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, ErrKernelPoolUnavailable
		}
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || before.Ctim != after.Ctim {
		return nil, ErrKernelPoolUnavailable
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
