package safefile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// OpenTrustedExecutable resolves managed links through held directory and
// symlink descriptors, then opens the exact regular executable inode. System
// ancestors may be root-owned; other objects must be root- or caller-owned.
// Root-owned sticky directories permit private fixtures beneath /tmp while
// preserving protection against replacement by other directory writers.
func OpenTrustedExecutable(path string, limit int64) (*os.File, error) {
	return openTrustedExecutable(path, limit, false)
}

// System multicall binaries can have many package-owned hardlinks. Interpreter
// selection permits those only for root-owned ELF files; configured launch
// artifacts and scripts retain the single-link requirement.
func openTrustedInterpreter(path string, limit int64) (*os.File, error) {
	return openTrustedExecutable(path, limit, true)
}

func openTrustedExecutable(path string, limit int64, interpreter bool) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || limit <= 0 {
		return nil, errors.New("executable requires a canonical absolute path and positive size limit")
	}
	rootFD, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	directories := []*os.File{os.NewFile(uintptr(rootFD), "/")}
	defer func() {
		for _, directory := range directories {
			_ = directory.Close()
		}
	}()
	trustedOwner := func(uid uint32) bool { return uid == 0 || uid == uint32(os.Geteuid()) }
	checkDirectory := func(file *os.File) error {
		info, statErr := file.Stat()
		value, ok := identity(info)
		if statErr != nil || !ok || !info.IsDir() || !trustedOwner(value.UID) {
			return errors.New("executable directory has an unsafe type or owner")
		}
		if info.Mode().Perm()&0022 != 0 && !(value.UID == 0 && info.Mode()&os.ModeSticky != 0) {
			return fmt.Errorf("executable directory %q is writable without trusted sticky ownership (uid=%d mode=%s)", file.Name(), value.UID, info.Mode())
		}
		return nil
	}
	if err = checkDirectory(directories[0]); err != nil {
		return nil, err
	}
	remaining := strings.Split(strings.TrimPrefix(path, "/"), "/")
	links, steps := 0, 0
	for len(remaining) > 0 {
		steps++
		if steps > 4096 {
			return nil, errors.New("executable traversal exceeds component limit")
		}
		name := remaining[0]
		remaining = remaining[1:]
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			if len(directories) > 1 {
				_ = directories[len(directories)-1].Close()
				directories = directories[:len(directories)-1]
			}
			continue
		}
		parent := directories[len(directories)-1]
		fd, openErr := unix.Openat(int(parent.Fd()), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil {
			return nil, openErr
		}
		object := os.NewFile(uintptr(fd), name)
		info, statErr := object.Stat()
		value, ok := identity(info)
		if statErr != nil || !ok || !trustedOwner(value.UID) {
			_ = object.Close()
			return nil, errors.New("executable path object has an unsafe owner")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 40 {
				_ = object.Close()
				return nil, errors.New("executable traversal exceeds symlink limit")
			}
			buffer := make([]byte, 4096)
			count, readErr := unix.Readlinkat(fd, "", buffer)
			_ = object.Close()
			if readErr != nil || count <= 0 || count == len(buffer) {
				return nil, errors.Join(errors.New("executable link target is unavailable or exceeds limit"), readErr)
			}
			target := string(buffer[:count])
			if filepath.IsAbs(target) {
				for len(directories) > 1 {
					_ = directories[len(directories)-1].Close()
					directories = directories[:len(directories)-1]
				}
				target = strings.TrimPrefix(target, "/")
			}
			remaining = append(strings.Split(target, "/"), remaining...)
			continue
		}
		if len(remaining) > 0 {
			if err = checkDirectory(object); err != nil {
				_ = object.Close()
				return nil, err
			}
			directories = append(directories, object)
			continue
		}
		if !info.Mode().IsRegular() || !trustedExecutableLinkCount(value, interpreter) || value.Size <= 0 || value.Size > limit ||
			info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0100 == 0 {
			_ = object.Close()
			return nil, errors.New("executable has an unsafe type, mode, links or size")
		}
		// Reopen the held O_PATH inode, never its public name. Type inspection
		// precedes this read open, so FIFOs/devices cannot block or run drivers.
		readFD, readErr := unix.Open(fmt.Sprintf("/proc/self/fd/%d", fd), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		_ = object.Close()
		if readErr != nil {
			return nil, readErr
		}
		file := os.NewFile(uintptr(readFD), path)
		openedInfo, statErr := file.Stat()
		opened, ok := identity(openedInfo)
		if statErr != nil || !ok || opened != value {
			_ = file.Close()
			return nil, errors.Join(errors.New("executable identity changed during open"), statErr)
		}
		if value.Links > 1 {
			header := make([]byte, 4)
			count, readErr := unix.Pread(readFD, header, 0)
			afterInfo, statErr := file.Stat()
			after, ok := identity(afterInfo)
			if readErr != nil || count != 4 || !bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}) || statErr != nil || !ok || after != value {
				file.Close()
				return nil, errors.New("multiply linked interpreter must be a stable root-owned ELF file")
			}
		}
		return file, nil
	}
	return nil, errors.New("executable path does not resolve to a regular file")
}

func trustedExecutableLinkCount(value Identity, interpreter bool) bool {
	return value.Links == 1 || (interpreter && value.UID == 0 && value.Links > 1)
}
