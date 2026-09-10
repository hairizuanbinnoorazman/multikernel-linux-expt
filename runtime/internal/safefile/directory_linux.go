package safefile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if info.Mode().Perm() != 0700 {
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
	}
	stable := Identity{Device: value.Device, Inode: value.Inode, UID: value.UID, Mode: value.Mode}
	return &Directory{file: current, identity: stable}, nil
}

func (d *Directory) Close() error       { return d.file.Close() }
func (d *Directory) Identity() Identity { return d.identity }

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
	data, _, err := ReadOpened(file, limit)
	return data, true, err
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
	if filepath.Base(name) != name || name == "." || mode.Perm()&0077 != 0 {
		return false, errors.New("invalid private publication name or mode")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, retErr = io.ReadFull(rand.Reader, random); retErr != nil {
			return false, retErr
		}
		temporaryName = "." + name + "." + hex.EncodeToString(random)
		fd, err := unix.Openat(int(d.file.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return false, err
		}
		temporary = os.NewFile(uintptr(fd), temporaryName)
		break
	}
	if temporary == nil {
		return false, errors.New("could not allocate a unique publication file")
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
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err = unix.Renameat2(int(d.file.Fd()), temporaryName, int(d.file.Fd()), name, unix.RENAME_NOREPLACE); errors.Is(err, syscall.EEXIST) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	renamed = true
	return true, d.file.Sync()
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
