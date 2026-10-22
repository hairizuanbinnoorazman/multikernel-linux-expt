//go:build linux

package dockerengine

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const releaseDirectory = "/run/multikernel-agent/docker-state-release"

// PreparedFilesystem owns the exact API-state bind installed before admission.
// The outer self-bind prevents RootlessKit from deleting the shared inode on
// exit. Both that mount and the inner API bind remain owned until normal release.
// An open_tree FD lets cleanup move the owned mount out of workload-writable
// paths before closing the FD and unmounting it normally. No lazy detach occurs.
type PreparedFilesystem struct {
	mu                                      sync.Mutex
	mountFD, directoryFD                    int
	source                                  *PreparedFilesystem
	destination                             string
	moved, unmounted, targetRemoved, closed bool
}

func captureStateMount() (*PreparedFilesystem, error) {
	inner, err := captureMount(imageRoot+rootlessState, releaseDirectory)
	if err != nil {
		return nil, err
	}
	source, err := captureMount(rootlessState, releaseDirectory+"-source")
	if err != nil {
		return nil, err
	}
	inner.source = source
	return inner, nil
}

func captureMount(path, destination string) (*PreparedFilesystem, error) {
	control := "/run/multikernel-agent"
	if err := os.Mkdir(control, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	st, err := directoryIdentity(control)
	if err != nil || st.Uid != 0 || st.Mode&0777 != 0700 {
		return nil, errors.New("Docker cleanup parent identity refused")
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return nil, errors.New("Docker cleanup directory is not fresh")
	}
	if err := os.Mkdir(destination+"/state", 0700); err != nil {
		return nil, err
	}
	directory, err := unix.Open(destination, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	mount, err := unix.OpenTree(unix.AT_FDCWD, path, unix.OPEN_TREE_CLOEXEC)
	if err != nil {
		unix.Close(directory)
		return nil, errors.New("Docker state mount ownership unavailable")
	}
	return &PreparedFilesystem{mountFD: mount, directoryFD: directory, destination: destination}, nil
}

// Release runs only after the owned service is empty and the launcher is reaped.
// Failed release retains ownership for retry and prevents storage quiescence.
func (p *PreparedFilesystem) Release() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	if !p.moved {
		if err := unix.MoveMount(p.mountFD, "", p.directoryFD, "state", unix.MOVE_MOUNT_F_EMPTY_PATH); err != nil {
			return fmt.Errorf("Docker owned state mount move failed: %w", err)
		}
		p.moved = true
		err := unix.Close(p.mountFD)
		p.mountFD = -1
		if err != nil {
			return fmt.Errorf("Docker owned state mount descriptor close failed: %w", err)
		}
	}
	if !p.unmounted {
		// The destination directory is root-owned/private and its FD is held.
		// User renames of the original image path cannot redirect this unmount.
		path := fmt.Sprintf("/proc/self/fd/%d/state", p.directoryFD)
		if err := unix.Unmount(path, 0); err != nil {
			return fmt.Errorf("Docker owned state mount unmount failed: %w", err)
		}
		p.unmounted = true
	}
	if !p.targetRemoved {
		if err := unix.Unlinkat(p.directoryFD, "state", unix.AT_REMOVEDIR); err != nil {
			return fmt.Errorf("Docker cleanup target removal failed: %w", err)
		}
		p.targetRemoved = true
	}
	if p.directoryFD >= 0 {
		err := unix.Close(p.directoryFD)
		p.directoryFD = -1
		if err != nil {
			return fmt.Errorf("Docker cleanup directory descriptor close failed: %w", err)
		}
	}
	if err := os.Remove(p.destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Docker cleanup directory removal failed: %w", err)
	}
	if p.source != nil {
		if err := p.source.Release(); err != nil {
			return err
		}
	}
	p.closed = true
	return nil
}
