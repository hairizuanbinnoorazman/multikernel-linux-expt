package safefile

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Identity is the stable identity of an opened filesystem object.
type Identity struct {
	Device uint64
	Inode  uint64
	UID    uint32
	Mode   uint32
	Links  uint64
	Size   int64
	MTime  syscall.Timespec
	CTime  syscall.Timespec
}

// Directory is a no-symlink, descriptor-anchored directory.
type Directory struct {
	file     *os.File
	identity Identity
}

func identity(info os.FileInfo) (Identity, bool) {
	value, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Identity{}, false
	}
	return Identity{Device: uint64(value.Dev), Inode: value.Ino, UID: value.Uid, Mode: value.Mode,
		Links: value.Nlink, Size: value.Size, MTime: value.Mtim, CTime: value.Ctim}, true
}

// OpenDirectory walks path from / without following any symlink. Missing
// components are created only when create is true.
func OpenDirectory(path string, create bool) (*Directory, error) {
	return openDirectory(path, create, true)
}

// OpenOwnedDirectory opens an existing descriptor-anchored directory without
// changing its mode. Shared read/traverse bits are allowed, but group/other
// write access is not.
func OpenOwnedDirectory(path string) (*Directory, error) {
	return openDirectory(path, false, false)
}

// OpenCurrentOwnedDirectory binds the process's actual cwd inode even if its
// public pathname is concurrently renamed or replaced.
func OpenCurrentOwnedDirectory() (*Directory, error) {
	fd, err := unix.Open(".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	current := os.NewFile(uintptr(fd), ".")
	info, err := current.Stat()
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	value, ok := identity(info)
	if !ok || !info.IsDir() || value.UID != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		_ = current.Close()
		return nil, errors.New("current directory must be caller-owned and not group/other-writable")
	}
	stable := Identity{Device: value.Device, Inode: value.Inode, UID: value.UID, Mode: value.Mode}
	return &Directory{file: current, identity: stable}, nil
}

func openDirectory(path string, create, requirePrivate bool) (*Directory, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
		return nil, errors.New("directory must be canonical, absolute, and below the filesystem root")
	}
	rootFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	current := os.NewFile(uintptr(rootFD), string(filepath.Separator))
	for _, component := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		nextFD, openErr := unix.Openat(int(current.Fd()), component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, syscall.ENOENT) && create {
			if mkdirErr := unix.Mkdirat(int(current.Fd()), component, 0700); mkdirErr != nil && !errors.Is(mkdirErr, syscall.EEXIST) {
				_ = current.Close()
				return nil, mkdirErr
			}
			nextFD, openErr = unix.Openat(int(current.Fd()), component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = current.Close()
			return nil, fmt.Errorf("open directory component %q: %w", component, openErr)
		}
		_ = current.Close()
		current = os.NewFile(uintptr(nextFD), component)
	}
	info, err := current.Stat()
	if err != nil {
		_ = current.Close()
		return nil, err
	}
	value, ok := identity(info)
	if !ok || !info.IsDir() || value.UID != uint32(os.Geteuid()) {
		_ = current.Close()
		return nil, fmt.Errorf("directory must be caller-owned (mode=%#o owner=%d caller=%d)",
			info.Mode().Perm(), value.UID, os.Geteuid())
	}
	if requirePrivate && info.Mode().Perm() != 0700 {
		if !create {
			_ = current.Close()
			return nil, fmt.Errorf("directory mode must remain private (mode=%#o)", info.Mode().Perm())
		}
		if err = current.Chmod(0700); err != nil {
			_ = current.Close()
			return nil, err
		}
		info, err = current.Stat()
		if err != nil {
			_ = current.Close()
			return nil, err
		}
		value, ok = identity(info)
		if !ok || info.Mode().Perm() != 0700 {
			_ = current.Close()
			return nil, errors.New("directory mode could not be bound after opening")
		}
	} else if !requirePrivate && info.Mode().Perm()&0022 != 0 {
		_ = current.Close()
		return nil, fmt.Errorf("directory must not be group/other-writable (mode=%#o)", info.Mode().Perm())
	}
	stable := Identity{Device: value.Device, Inode: value.Inode, UID: value.UID, Mode: value.Mode}
	return &Directory{file: current, identity: stable}, nil
}

func (d *Directory) Close() error       { return d.file.Close() }
func (d *Directory) Identity() Identity { return d.identity }
func (d *Directory) Sync() error        { return d.file.Sync() }
func (d *Directory) ProcPath() string   { return fmt.Sprintf("/proc/self/fd/%d", d.file.Fd()) }

// OpenChildDirectory opens one exact child directory relative to the held
// parent and validates its ownership and mode without following it.
func (d *Directory) OpenChildDirectory(name string, mode os.FileMode) (*Directory, error) {
	if filepath.Base(name) != name || name == "." || mode != mode.Perm() {
		return nil, errors.New("invalid child directory request")
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	child := os.NewFile(uintptr(fd), name)
	info, err := child.Stat()
	if err != nil {
		_ = child.Close()
		return nil, err
	}
	value, ok := identity(info)
	if !ok || !info.IsDir() || info.Mode().Perm() != mode.Perm() || value.UID != uint32(os.Geteuid()) {
		_ = child.Close()
		return nil, errors.New("child directory has an unsafe identity or mode")
	}
	stable := Identity{Device: value.Device, Inode: value.Inode, UID: value.UID, Mode: value.Mode}
	return &Directory{file: child, identity: stable}, nil
}

// TryLockExclusive attempts to lock the held directory inode without
// blocking. Closing Directory releases the lock.
func (d *Directory) TryLockExclusive() (bool, error) {
	err := unix.Flock(int(d.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

// EntryIdentity observes a simple name without following it. Callers must use
// a type-specific capture method before trusting contents or removing it.
func (d *Directory) EntryIdentity(name string) (Identity, bool, error) {
	if filepath.Base(name) != name || name == "." {
		return Identity{}, false, errors.New("invalid entry name")
	}
	entry, err := identityAt(d.file, name)
	if errors.Is(err, syscall.ENOENT) {
		return Identity{}, false, nil
	}
	return entry, err == nil, err
}

func inspectPrivate(file *os.File, limit int64) (Identity, error) {
	if limit <= 0 {
		return Identity{}, errors.New("private file size limit must be positive")
	}
	info, err := file.Stat()
	if err != nil {
		return Identity{}, err
	}
	value, ok := identity(info)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || value.UID != uint32(os.Geteuid()) ||
		value.Links != 1 || info.Size() < 0 || info.Size() > limit {
		return Identity{}, errors.New("file must be private, caller-owned, single-link, regular, and bounded")
	}
	return value, nil
}

// InspectOpened validates a private opened file and returns its current identity.
func InspectOpened(file *os.File, limit int64) (Identity, error) { return inspectPrivate(file, limit) }

// SameObject compares the immutable ownership identity of two observations.
func SameObject(first, second Identity) bool {
	return first.Device == second.Device && first.Inode == second.Inode && first.UID == second.UID &&
		first.Mode == second.Mode && first.Links == second.Links
}

// ReadOpened reads a private opened file without changing its current offset.
func ReadOpened(file *os.File, limit int64) ([]byte, Identity, error) {
	before, err := inspectPrivate(file, limit)
	if err != nil {
		return nil, Identity{}, err
	}
	data := make([]byte, before.Size)
	if len(data) != 0 {
		if _, err = file.ReadAt(data, 0); err != nil {
			return nil, Identity{}, err
		}
	}
	after, err := inspectPrivate(file, limit)
	if err != nil || before != after {
		return nil, Identity{}, errors.New("file identity changed while reading")
	}
	return data, before, nil
}

func (d *Directory) ReadPrivate(name string, limit int64) ([]byte, bool, error) {
	data, present, _, err := d.ReadPrivateIdentity(name, limit)
	return data, present, err
}

// ReadPrivateIdentity reads a private file and returns the identity of the
// exact descriptor from which the bytes were read.
func (d *Directory) ReadPrivateIdentity(name string, limit int64) ([]byte, bool, Identity, error) {
	if filepath.Base(name) != name || name == "." || limit <= 0 {
		return nil, false, Identity{}, errors.New("invalid private file name or size limit")
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, false, Identity{}, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return nil, false, Identity{}, errors.New("file must be a private regular file, not a symlink")
	}
	if err != nil {
		return nil, false, Identity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	data, opened, err := ReadOpened(file, limit)
	return data, true, opened, err
}

// CaptureRegularIdentity binds later cleanup to an exact regular file without
// requiring the file itself to be private. The containing Directory remains
// the private ownership boundary.
func (d *Directory) CaptureRegularIdentity(name string, mode os.FileMode, limit int64) (Identity, error) {
	_, opened, err := d.ReadRegularIdentity(name, mode, limit)
	return opened, err
}

// ReadRegularIdentity reads an exact-mode regular file from the same stable
// descriptor whose identity it returns.
func (d *Directory) ReadRegularIdentity(name string, mode os.FileMode, limit int64) ([]byte, Identity, error) {
	if filepath.Base(name) != name || name == "." || mode.Perm() != mode || limit <= 0 {
		return nil, Identity{}, errors.New("invalid regular-file read request")
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, Identity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, inspectErr := file.Stat()
	opened, ok := identity(info)
	if inspectErr != nil || !ok || !info.Mode().IsRegular() || info.Mode().Perm() != mode.Perm() ||
		opened.UID != uint32(os.Geteuid()) || opened.Links != 1 || opened.Size < 0 || opened.Size > limit {
		return nil, Identity{}, errors.Join(errors.New("file must be caller-owned, single-link, regular, exact-mode, and bounded"), inspectErr)
	}
	data := make([]byte, opened.Size)
	if len(data) != 0 {
		if _, err = file.ReadAt(data, 0); err != nil {
			return nil, Identity{}, err
		}
	}
	afterInfo, err := file.Stat()
	after, afterOK := identity(afterInfo)
	if err != nil || !afterOK || opened != after {
		return nil, Identity{}, errors.Join(errors.New("file identity changed while reading"), err)
	}
	named, err := identityAt(d.file, name)
	if err != nil || named != opened {
		return nil, Identity{}, errors.Join(errors.New("file identity changed while reading"), err)
	}
	return data, opened, nil
}

// CaptureSymlinkIdentity returns the exact bounded symlink target and identity
// without following the link.
func (d *Directory) CaptureSymlinkIdentity(name string, limit int) (string, Identity, error) {
	if filepath.Base(name) != name || name == "." || limit <= 0 {
		return "", Identity{}, errors.New("invalid symlink capture request")
	}
	before, err := identityAt(d.file, name)
	if err != nil || before.Mode&unix.S_IFMT != unix.S_IFLNK || before.UID != uint32(os.Geteuid()) || before.Links != 1 || before.Size < 0 || before.Size > int64(limit) {
		return "", Identity{}, errors.Join(errors.New("entry must be a caller-owned, single-link, bounded symlink"), err)
	}
	buffer := make([]byte, limit+1)
	n, err := unix.Readlinkat(int(d.file.Fd()), name, buffer)
	if err != nil || n > limit {
		return "", Identity{}, errors.Join(errors.New("read bounded symlink target"), err)
	}
	after, err := identityAt(d.file, name)
	if err != nil || before != after {
		return "", Identity{}, errors.Join(errors.New("symlink identity changed while capturing"), err)
	}
	return string(buffer[:n]), before, nil
}

func removalPrefix(name string) string {
	digest := sha256.Sum256([]byte(name))
	return ".mklinux-remove-" + hex.EncodeToString(digest[:8]) + "-"
}

func removalQuarantine(name string, expected Identity) string {
	return fmt.Sprintf("%s%016x-%016x", removalPrefix(name), expected.Device, expected.Inode)
}

// ReadPrivateOrQuarantineIdentity recovers an interrupted
// identity-conditioned removal. A quarantined result is returned only when its
// encoded name matches the exact file identity read from that entry.
func (d *Directory) ReadPrivateOrQuarantineIdentity(name string, limit int64) ([]byte, bool, bool, Identity, error) {
	data, present, opened, err := d.ReadPrivateIdentity(name, limit)
	if err != nil || present {
		return data, present, false, opened, err
	}
	duplicateFD, err := unix.Openat(int(d.file.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, false, false, Identity{}, err
	}
	listing := os.NewFile(uintptr(duplicateFD), d.file.Name())
	names, readErr := listing.Readdirnames(-1)
	closeErr := listing.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return nil, false, false, Identity{}, err
	}
	prefix := removalPrefix(name)
	var candidates []string
	for _, candidate := range names {
		if strings.HasPrefix(candidate, prefix) {
			candidates = append(candidates, candidate)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return nil, false, false, Identity{}, nil
	}
	if len(candidates) != 1 {
		return nil, false, false, Identity{}, errors.New("multiple removal quarantines exist for one private file")
	}
	data, present, opened, err = d.ReadPrivateIdentity(candidates[0], limit)
	if err != nil || !present {
		return nil, false, false, Identity{}, errors.Join(errors.New("removal quarantine could not be read"), err)
	}
	if removalQuarantine(name, opened) != candidates[0] {
		return nil, false, false, Identity{}, errors.New("removal quarantine name does not match its inode identity")
	}
	return data, true, true, opened, nil
}

// ReadPrivateSnapshot reads the validated prefix present when an append-only
// private file is opened. Appends after the initial inspection are deliberately
// excluded, while the opened descriptor prevents pathname replacement from
// redirecting the read.
func (d *Directory) ReadPrivateSnapshot(name string, limit int64) ([]byte, bool, error) {
	if filepath.Base(name) != name || name == "." || limit <= 0 {
		return nil, false, errors.New("invalid private file name or size limit")
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, false, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return nil, false, errors.New("file must be a private regular file, not a symlink")
	}
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	value, err := inspectPrivate(file, limit)
	if err != nil {
		return nil, true, err
	}
	data := make([]byte, value.Size)
	if len(data) != 0 {
		if _, err = file.ReadAt(data, 0); err != nil {
			return nil, true, err
		}
	}
	return data, true, nil
}

// InspectPrivate opens a named file relative to the directory and validates it
// without reading its contents.
func (d *Directory) InspectPrivate(name string, limit int64) (Identity, bool, error) {
	if filepath.Base(name) != name || name == "." || limit <= 0 {
		return Identity{}, false, errors.New("invalid private file name or size limit")
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return Identity{}, false, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return Identity{}, false, errors.New("file must be a private regular file, not a symlink")
	}
	if err != nil {
		return Identity{}, false, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	value, err := inspectPrivate(file, limit)
	return value, true, err
}

// OpenAppend opens or exclusively creates a bounded private append-only file.
func (d *Directory) OpenAppend(name string, limit int64, mode os.FileMode) (*os.File, Identity, error) {
	if filepath.Base(name) != name || name == "." || mode.Perm()&0077 != 0 || limit <= 0 {
		return nil, Identity{}, errors.New("invalid private append file name, mode, or limit")
	}
	created := false
	fd, err := unix.Openat(int(d.file.Fd()), name,
		unix.O_RDWR|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err == nil {
		created = true
	} else if errors.Is(err, syscall.EEXIST) {
		fd, err = unix.Openat(int(d.file.Fd()), name, unix.O_RDWR|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, Identity{}, err
	}
	file := os.NewFile(uintptr(fd), name)
	value, inspectErr := inspectPrivate(file, limit)
	if inspectErr != nil {
		_ = file.Close()
		if created {
			_ = unix.Unlinkat(int(d.file.Fd()), name, 0)
		}
		return nil, Identity{}, inspectErr
	}
	if created {
		if err = d.file.Sync(); err != nil {
			_ = file.Close()
			_ = unix.Unlinkat(int(d.file.Fd()), name, 0)
			return nil, Identity{}, err
		}
	}
	return file, value, nil
}

// Replace publishes data atomically relative to the already opened directory.
func (d *Directory) Replace(name string, data []byte, mode os.FileMode) (retErr error) {
	if filepath.Base(name) != name || name == "." || mode.Perm()&0077 != 0 {
		return errors.New("invalid private replacement name or mode")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, err := io.ReadFull(rand.Reader, random); err != nil {
			return err
		}
		temporaryName = "." + name + "." + hex.EncodeToString(random)
		fd, err := unix.Openat(int(d.file.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return err
		}
		temporary = os.NewFile(uintptr(fd), temporaryName)
		break
	}
	if temporary == nil {
		return errors.New("could not allocate a unique replacement file")
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = unix.Unlinkat(int(d.file.Fd()), temporaryName, 0)
		}
	}()
	written, err := temporary.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = unix.Renameat(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name); err != nil {
		return err
	}
	renamed = true
	return d.file.Sync()
}

// PublishExclusive durably creates name without replacing an existing entry.
// It returns false without error when name already exists.
func (d *Directory) PublishExclusive(name string, data []byte, mode os.FileMode) (created bool, retErr error) {
	created, _, retErr = d.PublishExclusiveIdentity(name, data, mode)
	return created, retErr
}

// PublishExclusiveIdentity is PublishExclusive plus the identity of the exact
// inode published by this call.
func (d *Directory) PublishExclusiveIdentity(name string, data []byte, mode os.FileMode) (created bool, published Identity, retErr error) {
	if mode.Perm()&0077 != 0 {
		return false, Identity{}, errors.New("invalid private publication name or mode")
	}
	return d.publishExclusiveIdentity(name, data, mode)
}

// PublishExclusiveRegularIdentity publishes a caller-owned regular file with
// an exact non-writable group/other mode without replacing an existing entry.
func (d *Directory) PublishExclusiveRegularIdentity(name string, data []byte, mode os.FileMode) (created bool, published Identity, retErr error) {
	if mode.Perm()&0022 != 0 {
		return false, Identity{}, errors.New("regular publication mode may not be group/other-writable")
	}
	return d.publishExclusiveIdentity(name, data, mode)
}

func (d *Directory) publishExclusiveIdentity(name string, data []byte, mode os.FileMode) (created bool, published Identity, retErr error) {
	if filepath.Base(name) != name || name == "." || mode != mode.Perm() {
		return false, Identity{}, errors.New("invalid publication name or mode")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, retErr = io.ReadFull(rand.Reader, random); retErr != nil {
			return false, Identity{}, retErr
		}
		temporaryName = "." + name + "." + hex.EncodeToString(random)
		fd, err := unix.Openat(int(d.file.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return false, Identity{}, err
		}
		temporary = os.NewFile(uintptr(fd), temporaryName)
		break
	}
	if temporary == nil {
		return false, Identity{}, errors.New("could not allocate a unique publication file")
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = unix.Unlinkat(int(d.file.Fd()), temporaryName, 0)
		}
	}()
	written, err := temporary.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = temporary.Chmod(mode.Perm())
	}
	if err == nil {
		err = temporary.Sync()
	}
	if err == nil {
		info, statErr := temporary.Stat()
		err = statErr
		if err == nil {
			var ok bool
			published, ok = identity(info)
			if !ok {
				err = errors.New("published file identity is unavailable")
			}
		}
	}
	closeErr := temporary.Close()
	if err != nil {
		return false, Identity{}, err
	}
	if closeErr != nil {
		return false, Identity{}, closeErr
	}
	if err = unix.Renameat2(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name, unix.RENAME_NOREPLACE); errors.Is(err, syscall.EEXIST) {
		return false, Identity{}, nil
	} else if err != nil {
		return false, Identity{}, err
	}
	renamed = true
	return true, published, d.file.Sync()
}

// PublishExclusiveSymlinkIdentity creates a bounded symlink without replacing
// an existing entry and returns its exact identity.
func (d *Directory) PublishExclusiveSymlinkIdentity(name, target string, limit int) (bool, Identity, error) {
	if filepath.Base(name) != name || name == "." || target == "" || len(target) > limit || limit <= 0 {
		return false, Identity{}, errors.New("invalid symlink publication")
	}
	if err := unix.Symlinkat(target, int(d.file.Fd()), name); errors.Is(err, syscall.EEXIST) {
		return false, Identity{}, nil
	} else if err != nil {
		return false, Identity{}, err
	}
	published, err := identityAt(d.file, name)
	if err != nil || published.Mode&unix.S_IFMT != unix.S_IFLNK || published.UID != uint32(os.Geteuid()) || published.Links != 1 {
		return true, published, errors.Join(errors.New("published symlink identity could not be verified"), err)
	}
	return true, published, d.file.Sync()
}

// ReplaceIdentity atomically publishes a private regular file. When expected
// is non-nil, an exchange makes replacement conditional on that exact inode;
// a raced substitute is exchanged back intact.
func (d *Directory) ReplaceIdentity(name string, data []byte, mode os.FileMode, expected *Identity) (Identity, error) {
	return d.replaceIdentityWithHook(name, data, mode, expected, nil)
}

func (d *Directory) replaceIdentityWithHook(name string, data []byte, mode os.FileMode, expected *Identity, beforeExchange func()) (published Identity, retErr error) {
	if filepath.Base(name) != name || name == "." || mode != mode.Perm() || mode.Perm()&0077 != 0 {
		return Identity{}, errors.New("invalid identity-bound replacement")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, retErr = io.ReadFull(rand.Reader, random); retErr != nil {
			return Identity{}, retErr
		}
		temporaryName = "." + name + ".exchange." + hex.EncodeToString(random)
		fd, err := unix.Openat(int(d.file.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return Identity{}, err
		}
		temporary = os.NewFile(uintptr(fd), temporaryName)
		break
	}
	if temporary == nil {
		return Identity{}, errors.New("could not allocate identity-bound replacement")
	}
	temporaryPresent := true
	defer func() {
		if temporaryPresent {
			_ = unix.Unlinkat(int(d.file.Fd()), temporaryName, 0)
		}
	}()
	written, err := temporary.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = temporary.Chmod(mode.Perm())
	}
	if err == nil {
		err = temporary.Sync()
	}
	if err == nil {
		info, statErr := temporary.Stat()
		err = statErr
		if err == nil {
			var ok bool
			published, ok = identity(info)
			if !ok {
				err = errors.New("replacement identity is unavailable")
			}
		}
	}
	closeErr := temporary.Close()
	if err != nil || closeErr != nil {
		return Identity{}, errors.Join(err, closeErr)
	}
	if expected == nil {
		if err = unix.Renameat2(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name, unix.RENAME_NOREPLACE); err != nil {
			return Identity{}, err
		}
		temporaryPresent = false
		return published, d.file.Sync()
	}
	current, err := identityAt(d.file, name)
	if err != nil || !SameObject(current, *expected) {
		return Identity{}, errors.Join(errors.New("replacement target identity changed"), err)
	}
	if beforeExchange != nil {
		beforeExchange()
	}
	if err = unix.Renameat2(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name, unix.RENAME_EXCHANGE); err != nil {
		return Identity{}, err
	}
	// The temporary name now refers to a formerly public inode. Do not unlink it
	// unless its identity is proved or a compensating exchange succeeds.
	temporaryPresent = false
	old, oldErr := identityAt(d.file, temporaryName)
	installed, installedErr := identityAt(d.file, name)
	if oldErr != nil || installedErr != nil || !SameObject(old, *expected) || !SameObject(installed, published) {
		rollbackErr := unix.Renameat2(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name, unix.RENAME_EXCHANGE)
		if rollbackErr == nil {
			temporaryPresent = true
		}
		return Identity{}, errors.Join(errors.New("replacement target changed during exchange"), oldErr, installedErr, rollbackErr)
	}
	removed, err := d.RemoveIfIdentity(temporaryName, *expected)
	if err != nil || !removed {
		return Identity{}, errors.Join(errors.New("remove exchanged prior identity"), err)
	}
	return published, d.file.Sync()
}

// Remove unlinks a simple name relative to the opened directory and syncs the
// directory when an entry was removed.
func (d *Directory) Remove(name string) (bool, error) {
	if filepath.Base(name) != name || name == "." {
		return false, errors.New("invalid removal name")
	}
	if err := unix.Unlinkat(int(d.file.Fd()), name, 0); errors.Is(err, syscall.ENOENT) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, d.file.Sync()
}

func identityAt(directory *os.File, name string) (Identity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(directory.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return Identity{}, err
	}
	return Identity{Device: uint64(stat.Dev), Inode: stat.Ino, UID: stat.Uid, Mode: stat.Mode,
		Links: stat.Nlink, Size: stat.Size,
		MTime: syscall.Timespec{Sec: stat.Mtim.Sec, Nsec: stat.Mtim.Nsec},
		CTime: syscall.Timespec{Sec: stat.Ctim.Sec, Nsec: stat.Ctim.Nsec}}, nil
}

func (d *Directory) removeIfIdentityWithHook(name string, expected Identity, beforeRename func()) (bool, error) {
	if filepath.Base(name) != name || name == "." || expected.Device == 0 || expected.Inode == 0 {
		return false, errors.New("invalid identity-conditioned removal")
	}
	quarantine := removalQuarantine(name, expected)
	quarantined, quarantineErr := identityAt(d.file, quarantine)
	if quarantineErr == nil {
		if !SameObject(quarantined, expected) {
			return false, errors.New("removal quarantine has a conflicting identity")
		}
	} else if errors.Is(quarantineErr, syscall.ENOENT) {
		current, inspectErr := identityAt(d.file, name)
		if errors.Is(inspectErr, syscall.ENOENT) {
			return false, nil
		}
		if inspectErr != nil || !SameObject(current, expected) {
			return false, errors.Join(errors.New("file identity changed before removal"), inspectErr)
		}
		if beforeRename != nil {
			beforeRename()
		}
		if err := unix.Renameat2(int(d.file.Fd()), name, int(d.file.Fd()), quarantine, unix.RENAME_NOREPLACE); err != nil {
			return false, err
		}
		quarantined, quarantineErr = identityAt(d.file, quarantine)
		if quarantineErr != nil || !SameObject(quarantined, expected) {
			restoreErr := unix.Renameat2(int(d.file.Fd()), quarantine, int(d.file.Fd()), name, unix.RENAME_NOREPLACE)
			return false, errors.Join(errors.New("file changed before removal quarantine"), quarantineErr, restoreErr)
		}
	} else {
		return false, quarantineErr
	}
	fd, err := unix.Openat(int(d.file.Fd()), quarantine, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	opened := os.NewFile(uintptr(fd), quarantine)
	openedInfo, inspectErr := opened.Stat()
	openedIdentity, ok := identity(openedInfo)
	closeErr := opened.Close()
	if inspectErr != nil || !ok || !SameObject(openedIdentity, expected) {
		return false, errors.Join(errors.New("opened removal quarantine has a conflicting identity"), inspectErr, closeErr)
	}
	if closeErr != nil {
		return false, closeErr
	}
	quarantined, err = identityAt(d.file, quarantine)
	if err != nil || !SameObject(quarantined, expected) {
		return false, errors.Join(errors.New("removal quarantine changed before unlink"), err)
	}
	if err = unix.Unlinkat(int(d.file.Fd()), quarantine, 0); err != nil {
		return false, err
	}
	if current, inspectErr := identityAt(d.file, name); inspectErr == nil {
		return false, fmt.Errorf("file pathname was replaced during removal by inode %d", current.Inode)
	} else if !errors.Is(inspectErr, syscall.ENOENT) {
		return false, inspectErr
	}
	return true, d.file.Sync()
}

// RemoveIfIdentity removes only the exact object represented by expected.
// A raced replacement is restored or preserved and reported as an error.
func (d *Directory) RemoveIfIdentity(name string, expected Identity) (bool, error) {
	return d.removeIfIdentityWithHook(name, expected, nil)
}
