//go:build linux

package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

// InspectProcess never invokes Observe: recovery may remove stale records.
// It creates no directories, changes no records, takes no image lock, and runs
// no filesystem check. It does not read mutable image contents or server logs.
// Ordinary filesystem access-time updates remain possible.
func (b *LinuxBackend) InspectProcess(ctx context.Context, value Export) (ProcessInspection, error) {
	empty := ProcessInspection{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := validateBackendLease(value); err != nil {
		return empty, err
	}
	if value.ImageIdentity.Device == 0 || value.ImageIdentity.Inode == 0 {
		return empty, errors.New("storage image identity is unavailable")
	}
	if !b.mu.TryLock() {
		return empty, errors.New("storage process inspection is busy")
	}
	binary, runtimeDir, requiredUID, pinned := b.Binary, b.RuntimeDir, b.RequiredUID, b.runtime
	b.mu.Unlock()
	if binary == "" {
		binary = "/usr/local/libexec/multikernel/mkvsock-nbd"
	}
	if runtimeDir == "" {
		runtimeDir = "/run/mkstorage"
	}
	directory, err := safefile.OpenDirectory(runtimeDir, false)
	if errors.Is(err, os.ErrNotExist) {
		// A missing previously pinned directory is a replacement, not mere absence.
		if pinned != (safefile.Identity{}) {
			return empty, errors.New("storage runtime directory identity changed")
		}
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		return ProcessInspection{Status: "record-missing", Generation: value.ExportGeneration}, nil
	}
	if err != nil {
		return empty, errors.New("storage runtime directory is unavailable")
	}
	defer directory.Close()
	if pinned != (safefile.Identity{}) && pinned != directory.Identity() {
		return empty, errors.New("storage runtime directory identity changed")
	}
	name := value.SandboxID + "-" + value.SandboxGeneration[:12] + ".json"
	data, found, identity, err := directory.ReadPrivateIdentity(name, 4096)
	if err != nil {
		return empty, errors.New("storage process record is unavailable")
	}
	result := ProcessInspection{Status: "record-missing", Generation: value.ExportGeneration}
	if found {
		record, err := decodeProcessRecord(data, value)
		if err != nil {
			return empty, err
		}
		handle, matches, err := openProcessHandle(record, binary)
		if err != nil {
			return empty, err
		}
		if handle != nil {
			defer handle.Close()
		}
		result.Status = "process-absent"
		if matches {
			// A private copy avoids initializing backend defaults/runtime/managed state.
			config := LinuxBackend{RequiredUID: requiredUID}
			image, err := config.openPreparedImage(value.PreparedImage, false)
			if err != nil {
				return empty, errors.New("storage backing identity is unavailable")
			}
			defer image.Close()
			if image.identity.Device != value.ImageIdentity.Device || image.identity.Inode != value.ImageIdentity.Inode {
				return empty, errors.New("storage backing identity conflicts with its lease")
			}
			// Image contents/mtime may change during normal I/O; compare object identity
			// and declared size only. Reopen the parent to detect pathname replacement.
			parent, err := safefile.OpenOwnedDirectory(filepath.Dir(value.Path))
			if err != nil {
				return empty, errors.New("storage backing parent is unavailable")
			}
			sameParent := parent.Identity() == image.directory.Identity()
			parent.Close()
			named, present, err := image.directory.EntryIdentity(filepath.Base(value.Path))
			if err != nil || !present || !sameParent || !safefile.SameObject(named, image.identity) || named.Size != image.identity.Size {
				return empty, errors.New("storage backing pathname changed during inspection")
			}
			if err := unix.PidfdSendSignal(handle.pidfd, 0, nil, 0); err != nil {
				return empty, errors.New("storage process changed during inspection")
			}
			result.Status = "process-matched"
		}
	}
	// Bracket the observation with exact record and runtime directory identity.
	_, stillFound, current, err := directory.ReadPrivateIdentity(name, 4096)
	if err != nil || stillFound != found || (found && current != identity) {
		return empty, errors.New("storage process record changed during inspection")
	}
	currentDir, err := safefile.OpenDirectory(runtimeDir, false)
	if err != nil {
		return empty, errors.New("storage runtime directory changed during inspection")
	}
	same := currentDir.Identity() == directory.Identity()
	currentDir.Close()
	if !same {
		return empty, errors.New("storage runtime directory changed during inspection")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return result, nil
}
