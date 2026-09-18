package unixsocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// Close releases a captured path without removing the socket. This is used
// after Dial proves that a live service already owns the published endpoint.
func (p *Path) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	err := p.dir.Close()
	if err == nil {
		p.closed = true
	}
	return err
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

func rawIdentityAt(dir *os.File, base string) (identity, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return identity{}, false, nil
	}
	if err != nil {
		return identity{}, false, err
	}
	return socketIdentity(stat), true, nil
}

func removeIdentityAtWithHook(dir *os.File, base string, expected identity, beforeRename func()) (bool, error) {
	quarantine := fmt.Sprintf(".mklinux-socket-%016x-%016x", expected.device, expected.inode)
	quarantined, found, err := rawIdentityAt(dir, quarantine)
	if err != nil {
		return false, err
	}
	if found {
		if quarantined != expected {
			return false, errors.New("Unix socket removal quarantine has a conflicting identity")
		}
	} else {
		current, present, inspectErr := rawIdentityAt(dir, base)
		if inspectErr != nil {
			return false, inspectErr
		}
		if !present {
			return false, nil
		}
		if current != expected {
			return false, errors.New("refusing to remove replaced Unix socket path")
		}
		if beforeRename != nil {
			beforeRename()
		}
		if err = unix.Renameat2(int(dir.Fd()), base, int(dir.Fd()), quarantine, unix.RENAME_NOREPLACE); err != nil {
			return false, err
		}
		quarantined, found, err = rawIdentityAt(dir, quarantine)
		if err != nil || !found || quarantined != expected {
			restoreErr := unix.Renameat2(int(dir.Fd()), quarantine, int(dir.Fd()), base, unix.RENAME_NOREPLACE)
			return false, errors.Join(errors.New("Unix socket changed before removal quarantine"), err, restoreErr)
		}
	}
	descriptor, err := unix.Openat(int(dir.Fd()), quarantine, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	var opened unix.Stat_t
	statErr := unix.Fstat(descriptor, &opened)
	closeErr := unix.Close(descriptor)
	if statErr != nil || socketIdentity(opened) != expected {
		return false, errors.Join(errors.New("opened Unix socket quarantine has a conflicting identity"), statErr, closeErr)
	}
	if closeErr != nil {
		return false, closeErr
	}
	quarantined, found, err = rawIdentityAt(dir, quarantine)
	if err != nil || !found || quarantined != expected {
		return false, errors.Join(errors.New("Unix socket quarantine changed before unlink"), err)
	}
	if err = unix.Unlinkat(int(dir.Fd()), quarantine, 0); err != nil {
		return false, err
	}
	if _, present, inspectErr := rawIdentityAt(dir, base); inspectErr != nil {
		return false, inspectErr
	} else if present {
		return false, errors.New("Unix socket pathname was replaced during removal")
	}
	return true, dir.Sync()
}

func removeIdentityAt(dir *os.File, base string, expected identity) (bool, error) {
	return removeIdentityAtWithHook(dir, base, expected, nil)
}

func openParent(path string) (*os.File, string, error) {
	if !validPath(path) {
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

func validPath(path string) bool {
	base := filepath.Base(path)
	return filepath.IsAbs(path) && filepath.Clean(path) == path && base != "." && base != string(filepath.Separator)
}

// EnsureParent creates missing socket-parent components without following
// symlinks or changing permissions on pre-existing ancestors.
func EnsureParent(path string) error {
	if !validPath(path) {
		return errors.New("Unix socket path must be canonical and absolute")
	}
	parent := filepath.Dir(path)
	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	current := os.NewFile(uintptr(rootFD), "/")
	defer func() { _ = current.Close() }()
	for _, component := range strings.Split(strings.TrimPrefix(parent, "/"), "/") {
		if component == "" {
			continue
		}
		created := false
		nextFD, openErr := unix.Openat(int(current.Fd()), component,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, syscall.ENOENT) {
			if mkdirErr := unix.Mkdirat(int(current.Fd()), component, 0700); mkdirErr != nil && !errors.Is(mkdirErr, syscall.EEXIST) {
				return mkdirErr
			} else if mkdirErr == nil {
				created = true
			}
			nextFD, openErr = unix.Openat(int(current.Fd()), component,
				unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			return openErr
		}
		if created {
			if chmodErr := unix.Fchmod(nextFD, 0755); chmodErr != nil {
				_ = unix.Close(nextFD)
				return chmodErr
			}
		}
		next := os.NewFile(uintptr(nextFD), component)
		if err = current.Close(); err != nil {
			_ = next.Close()
			return err
		}
		current = next
	}
	info, err := current.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		return errors.New("Unix socket parent must be caller-owned and not group/other-writable")
	}
	return current.Sync()
}

// Listen removes a safely identifiable stale socket and binds a replacement
// relative to a held no-symlink parent descriptor.
func Listen(path string, mode os.FileMode) (*Listener, error) {
	return listen(path, mode, true)
}

// ListenExclusive binds only when no socket exists. The returned listener
// owns exact-identity cleanup, including when its public name is replaced.
func ListenExclusive(path string, mode os.FileMode) (*Listener, error) {
	return listen(path, mode, false)
}

func listen(path string, mode os.FileMode, removeStale bool) (*Listener, error) {
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
	if stale, found, inspectErr := inspectAt(dir, base, mode); inspectErr != nil {
		return nil, inspectErr
	} else if found && removeStale {
		if _, err = removeIdentityAt(dir, base, stale); err != nil {
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
	if _, err := removeIdentityAt(p.dir, p.base, p.identity); err != nil {
		return err
	}
	err := p.dir.Close()
	if err == nil {
		p.closed = true
	}
	return err
}

// Dial connects through the held parent descriptor only while the published
// socket still has the identity captured by Capture.
func (p *Path) Dial() (net.Conn, error) {
	return p.DialContext(context.Background())
}

// DialContext is Dial with caller-controlled cancellation.
func (p *Path) DialContext(ctx context.Context) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("Unix socket path owner is closed")
	}
	current, found, err := inspectSafeAt(p.dir, p.base)
	if err != nil {
		return nil, err
	}
	if !found || current != p.identity {
		return nil, errors.New("refusing to dial replaced Unix socket path")
	}
	anchoredPath := fmt.Sprintf("/proc/self/fd/%d/%s", p.dir.Fd(), p.base)
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", anchoredPath)
	if err != nil {
		return nil, err
	}
	current, found, err = inspectSafeAt(p.dir, p.base)
	if err != nil || !found || current != p.identity {
		_ = connection.Close()
		return nil, errors.Join(errors.New("Unix socket path changed while dialing"), err)
	}
	return connection, nil
}

func (l *Listener) Accept() (net.Conn, error) { return l.listener.Accept() }
func (l *Listener) Addr() net.Addr            { return l.listener.Addr() }
func (l *Listener) Owner() uint32             { return l.identity.uid }
func (l *Listener) File() (*os.File, error)   { return l.listener.File() }

func (l *Listener) Close() error {
	l.once.Do(func() {
		closeErr := l.listener.Close()
		_, removeErr := removeIdentityAt(l.dir, l.base, l.identity)
		l.err = errors.Join(closeErr, removeErr, l.dir.Close())
	})
	return l.err
}
