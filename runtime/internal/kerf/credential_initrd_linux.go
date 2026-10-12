package kerf

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// credentialInitrd appends a private bootstrap-only cpio archive without
// modifying the approved image or putting the credential in a pathname/argv.
// Linux unpacks concatenated compressed initramfs archives in order.
func credentialInitrd(source *os.File, token []byte) (*os.File, error) {
	fd, err := unix.MemfdCreate("mk-agent-initrd", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "mk-agent-initrd")
	failed := true
	defer func() {
		if failed {
			file.Close()
		}
	}()
	if err = file.Chmod(0600); err != nil {
		return nil, err
	}
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > 16<<30 {
		return nil, errors.New("initramfs exceeds credential handoff limit")
	}
	if _, err = io.Copy(file, io.NewSectionReader(source, 0, info.Size())); err != nil {
		return nil, err
	}
	var archive bytes.Buffer
	entry := func(name string, mode uint32, data []byte) {
		fmt.Fprintf(&archive, "070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x", 0, mode, 0, 0, 1, 0, len(data), 0, 0, 0, 0, len(name)+1, 0)
		archive.WriteString(name)
		archive.WriteByte(0)
		for archive.Len()%4 != 0 {
			archive.WriteByte(0)
		}
		archive.Write(data)
		for archive.Len()%4 != 0 {
			archive.WriteByte(0)
		}
	}
	entry(".mk-agent", unix.S_IFDIR|0700, nil)
	entry(".mk-agent/token", unix.S_IFREG|0600, token)
	entry("TRAILER!!!", 0, nil)
	for archive.Len()%512 != 0 {
		archive.WriteByte(0)
	}
	compressed := gzip.NewWriter(file)
	if _, err = compressed.Write(archive.Bytes()); err != nil {
		return nil, err
	}
	if err = compressed.Close(); err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err = unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_WRITE|unix.F_SEAL_GROW|unix.F_SEAL_SHRINK|unix.F_SEAL_SEAL); err != nil {
		return nil, err
	}
	failed = false
	return file, nil
}
