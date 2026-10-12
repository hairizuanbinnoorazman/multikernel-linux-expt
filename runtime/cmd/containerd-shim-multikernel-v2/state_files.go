//go:build linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

func randomToken() ([]byte, string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, "", err
	}
	return b, hex.EncodeToString(b), nil
}

func newSignalOperationID() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func loadExistingToken(runtimeDir string) ([]byte, string, error) {
	path := filepath.Join(runtimeDir, "token")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 65 ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return nil, "", errors.New("existing runtime token is unsafe")
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	opened, statErr := file.Stat()
	if statErr != nil || !sameProcessIOIdentity(info, opened) {
		_ = file.Close()
		return nil, "", errors.New("runtime token identity changed while opening")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 66))
	after, afterErr := file.Stat()
	closeErr := file.Close()
	if err = errors.Join(readErr, afterErr, closeErr); err != nil {
		return nil, "", err
	}
	if !sameProcessIOIdentity(opened, after) {
		return nil, "", errors.New("runtime token identity changed while reading")
	}
	if len(data) != 65 || data[64] != '\n' {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	token, err := hex.DecodeString(string(data[:64]))
	if err != nil || len(token) != 32 {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	return token, string(data[:64]), nil
}

func loadExistingTokenFromDirectory(directory *safefile.Directory) ([]byte, string, error) {
	data, present, _, err := directory.ReadPrivateIdentity("token", 65)
	if err != nil {
		return nil, "", err
	}
	if !present {
		return nil, "", os.ErrNotExist
	}
	if len(data) != 65 || data[64] != '\n' {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	token, err := hex.DecodeString(string(data[:64]))
	if err != nil || len(token) != 32 {
		return nil, "", errors.New("existing runtime token is malformed")
	}
	return token, string(data[:64]), nil
}

func loadOrCreateToken(runtimeDir string) (_ []byte, _ string, retErr error) {
	if token, encoded, err := loadExistingToken(runtimeDir); err == nil {
		return token, encoded, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	token, encoded, err := randomToken()
	if err != nil {
		return nil, "", err
	}
	directory, err := openStateDirectory(runtimeDir)
	if err != nil {
		return nil, "", err
	}
	defer func() { retErr = errors.Join(retErr, directory.Close()) }()
	descriptor, err := unix.Openat(int(directory.Fd()), "token",
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(descriptor), "token")
	createdInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	createdIdentity, ok := createdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		_ = file.Close()
		return nil, "", errors.New("created runtime token identity is unavailable")
	}
	remove := true
	defer func() {
		if remove {
			var current unix.Stat_t
			if err := unix.Fstatat(int(directory.Fd()), "token", &current, unix.AT_SYMLINK_NOFOLLOW); err == nil &&
				current.Dev == createdIdentity.Dev && current.Ino == createdIdentity.Ino {
				_ = unix.Unlinkat(int(directory.Fd()), "token", 0)
			}
		}
	}()
	written, err := file.WriteString(encoded + "\n")
	if err == nil && written != 65 {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return nil, "", err
	}
	if closeErr != nil {
		return nil, "", closeErr
	}
	err = directory.Sync()
	if err != nil {
		return nil, "", err
	}
	remove = false
	return token, encoded, nil
}

func loadOrCreateTokenFromDirectory(directory *safefile.Directory) ([]byte, string, error) {
	if token, encoded, err := loadExistingTokenFromDirectory(directory); err == nil {
		return token, encoded, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	token, encoded, err := randomToken()
	if err != nil {
		return nil, "", err
	}
	created, _, err := directory.PublishExclusiveIdentity("token", []byte(encoded+"\n"), 0600)
	if err != nil {
		return nil, "", err
	}
	if !created {
		return nil, "", errors.New("runtime token appeared during exclusive creation")
	}
	return token, encoded, nil
}

func openStateDirectory(directory string) (*os.File, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, errors.New("state directory must be canonical and absolute")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("state directory identity is unavailable")
	}
	if !info.IsDir() || info.Mode().Perm()&0002 != 0 || identity.Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("state directory must be caller-owned with safe mode (mode=%#o owner=%d caller=%d)",
			info.Mode().Perm(), identity.Uid, os.Geteuid())
	}
	descriptor, err := unix.Openat2(unix.AT_FDCWD, directory, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(descriptor), directory)
	opened, err := dir.Stat()
	if err != nil || !sameProcessIOIdentity(info, opened) {
		_ = dir.Close()
		return nil, errors.New("state directory identity changed while opening")
	}
	return dir, nil
}

func stateIdentity(info os.FileInfo) (stateFileIdentity, bool) {
	value, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return stateFileIdentity{}, false
	}
	return stateFileIdentity{device: uint64(value.Dev), inode: value.Ino, uid: value.Uid,
		mode: value.Mode, links: value.Nlink}, true
}

func inspectStateFileAt(dir *os.File, base string, mode os.FileMode) (stateFileIdentity, bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		return stateFileIdentity{}, false, nil
	}
	if err != nil {
		return stateFileIdentity{}, false, err
	}
	identity := stateFileIdentity{device: uint64(stat.Dev), inode: stat.Ino, uid: stat.Uid,
		mode: stat.Mode, links: stat.Nlink}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 ||
		os.FileMode(stat.Mode).Perm() != mode.Perm() {
		return stateFileIdentity{}, true, errors.New("state file must be a private caller-owned single-link regular file")
	}
	return identity, true, nil
}

func removeStateFile(path string, expected *stateFileIdentity, mode os.FileMode) (retErr error) {
	dir, err := openStateDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, dir.Close()) }()
	current, found, err := inspectStateFileAt(dir, filepath.Base(path), mode)
	if err != nil {
		return err
	}
	if !found {
		if expected != nil {
			return errors.New("owned state file disappeared before removal")
		}
		return nil
	}
	if expected == nil || current != *expected {
		return errors.New("refusing to remove replaced or unowned state file")
	}
	if err = unix.Unlinkat(int(dir.Fd()), filepath.Base(path), 0); err != nil {
		return err
	}
	return dir.Sync()
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) (retErr error) {
	return atomicWriteFileOwned(path, data, mode, nil, nil)
}

func atomicWriteFileOwned(path string, data []byte, mode os.FileMode, expected, published *stateFileIdentity) (retErr error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || mode.Perm()&0077 != 0 {
		return errors.New("atomic state path and mode must be canonical, absolute, and private")
	}
	directory := filepath.Dir(path)
	dir, err := openStateDirectory(directory)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, dir.Close()) }()
	base := filepath.Base(path)
	current, found, err := inspectStateFileAt(dir, base, mode)
	if err != nil {
		return err
	}
	if published != nil && (expected == nil && found || expected != nil && (!found || current != *expected)) {
		return errors.New("refusing to replace an unowned or changed state file")
	}
	var temporary *os.File
	var temporaryName string
	for attempt := 0; attempt < 16; attempt++ {
		random := make([]byte, 8)
		if _, err = io.ReadFull(rand.Reader, random); err != nil {
			return err
		}
		temporaryName = "." + base + "." + hex.EncodeToString(random)
		temporaryDescriptor, openErr := unix.Openat(int(dir.Fd()), temporaryName,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
		if errors.Is(openErr, syscall.EEXIST) {
			continue
		}
		if openErr != nil {
			return openErr
		}
		temporary = os.NewFile(uintptr(temporaryDescriptor), temporaryName)
		break
	}
	if temporary == nil {
		return errors.New("could not allocate a unique atomic state temporary file")
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = unix.Unlinkat(int(dir.Fd()), temporaryName, 0)
		}
	}()
	if err = temporary.Chmod(mode); err == nil {
		var written int
		written, err = temporary.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = temporary.Sync()
	}
	var temporaryIdentity stateFileIdentity
	if err == nil {
		var info os.FileInfo
		info, err = temporary.Stat()
		var ok bool
		if err == nil {
			temporaryIdentity, ok = stateIdentity(info)
			if !ok {
				err = errors.New("temporary state identity is unavailable")
			}
		}
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = unix.Renameat(int(dir.Fd()), temporaryName, int(dir.Fd()), base); err != nil {
		return err
	}
	renamed = true
	err = dir.Sync()
	if err == nil && published != nil {
		*published = temporaryIdentity
	}
	return err
}
