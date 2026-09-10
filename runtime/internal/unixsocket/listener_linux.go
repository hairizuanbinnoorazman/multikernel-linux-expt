package unixsocket

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type identity struct {
	device uint64
	inode  uint64
	uid    uint32
	mode   uint32
	links  uint64
}

type Listener struct {
	listener *net.UnixListener
	dir      *os.File
	base     string
	identity identity
	once     sync.Once
	err      error
}

// Path owns the identity of an already-published Unix socket relative to a
// held parent descriptor. It is used when a supervised helper, rather than Go,
// performed bind(2).
type Path struct {
	dir      *os.File
	base     string
	identity identity
	mu       sync.Mutex
	closed   bool
}

func socketIdentity(stat unix.Stat_t) identity {
	return identity{device: uint64(stat.Dev), inode: stat.Ino, uid: stat.Uid, mode: stat.Mode, links: stat.Nlink}
}

func inspectAt(dir *os.File, base string, expectedMode os.FileMode) (identity, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return identity{}, false, nil
	}
	if err != nil {
		return identity{}, false, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFSOCK || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		os.FileMode(stat.Mode).Perm() != expectedMode.Perm() {
		return identity{}, true, errors.New("Unix socket path must be a caller-owned single-link socket with the expected mode")
	}
	return socketIdentity(stat), true, nil
}

func inspectSafeAt(dir *os.File, base string) (identity, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return identity{}, false, nil
	}
	if err != nil {
		return identity{}, false, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFSOCK || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		os.FileMode(stat.Mode).Perm()&0022 != 0 {
		return identity{}, true, errors.New("Unix socket path must be a caller-owned single-link socket with safe mode")
	}
	return socketIdentity(stat), true, nil
}

func openParent(path string) (*os.File, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) == "." {
		return nil, "", errors.New("Unix socket path must be canonical and absolute")
	}
	parent, base := filepath.Dir(path), filepath.Base(path)
	fd, err := unix.Openat2(unix.AT_FDCWD, parent, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", err
	}
	dir := os.NewFile(uintptr(fd), parent)
	info, err := dir.Stat()
	if err != nil {
		_ = dir.Close()
		return nil, "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		_ = dir.Close()
		return nil, "", errors.New("Unix socket parent must be caller-owned and not group/other-writable")
	}
	return dir, base, nil
}

// Listen removes a safely identifiable stale socket and binds a replacement
// relative to a held no-symlink parent descriptor.
func Listen(path string, mode os.FileMode) (*Listener, error) {
	if mode.Perm()&0007 != 0 {
		return nil, errors.New("Unix socket mode may not grant access to other users")
	}
	dir, base, err := openParent(path)
	if err != nil {
		return nil, err
	}
	cleanupDir := true
	defer func() {
		if cleanupDir {
			_ = dir.Close()
		}
	}()
	if _, found, inspectErr := inspectAt(dir, base, mode); inspectErr != nil {
		return nil, inspectErr
	} else if found {
		if err = unix.Unlinkat(int(dir.Fd()), base, 0); err != nil {
			return nil, err
		}
		if err = dir.Sync(); err != nil {
			return nil, err
		}
	}
	anchoredPath := fmt.Sprintf("/proc/self/fd/%d/%s", dir.Fd(), base)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: anchoredPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	if err = unix.Fchmodat(int(dir.Fd()), base, uint32(mode.Perm()), 0); err != nil {
		_ = listener.Close()
		_ = unix.Unlinkat(int(dir.Fd()), base, 0)
		return nil, err
	}
	created, found, err := inspectAt(dir, base, mode)
	if err != nil || !found {
		_ = listener.Close()
		_ = unix.Unlinkat(int(dir.Fd()), base, 0)
		return nil, errors.New("created Unix socket identity could not be verified")
	}
	cleanupDir = false
	return &Listener{listener: listener, dir: dir, base: base, identity: created}, nil
}

// Capture binds cleanup authority to the current inode at path. A missing path
// returns os.ErrNotExist so a supervisor can distinguish startup progress from
// an unsafe published object.
func Capture(path string) (*Path, error) {
	dir, base, err := openParent(path)
	if err != nil {
		return nil, err
	}
	current, found, err := inspectSafeAt(dir, base)
	if err != nil {
		_ = dir.Close()
		return nil, err
	}
	if !found {
		_ = dir.Close()
		return nil, os.ErrNotExist
	}
	return &Path{dir: dir, base: base, identity: current}, nil
}

// Remove unlinks only the inode captured by Capture. A mismatch remains
// retryable: the parent descriptor stays open and the replacement is preserved.
func (p *Path) Remove() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	current, found, err := inspectSafeAt(p.dir, p.base)
	if err != nil {
		return err
	}
	if found && current != p.identity {
		return errors.New("refusing to remove replaced Unix socket path")
	}
	if found {
		if err = unix.Unlinkat(int(p.dir.Fd()), p.base, 0); err != nil {
			return err
		}
		if err = p.dir.Sync(); err != nil {
			return err
		}
	}
	err = p.dir.Close()
	if err == nil {
		p.closed = true
	}
	return err
}

func (l *Listener) Accept() (net.Conn, error) { return l.listener.Accept() }
func (l *Listener) Addr() net.Addr            { return l.listener.Addr() }
func (l *Listener) Owner() uint32             { return l.identity.uid }

func (l *Listener) Close() error {
	l.once.Do(func() {
		closeErr := l.listener.Close()
		current, found, inspectErr := inspectAt(l.dir, l.base, os.FileMode(l.identity.mode))
		var removeErr error
		if inspectErr != nil {
			removeErr = inspectErr
		} else if found && current != l.identity {
			removeErr = errors.New("refusing to remove replaced Unix socket path")
		} else if found {
			if removeErr = unix.Unlinkat(int(l.dir.Fd()), l.base, 0); removeErr == nil {
				removeErr = l.dir.Sync()
			}
		}
		l.err = errors.Join(closeErr, removeErr, l.dir.Close())
	})
	return l.err
}
