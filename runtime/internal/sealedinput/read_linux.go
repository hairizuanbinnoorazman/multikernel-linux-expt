//go:build linux

// Package sealedinput reads bounded caller-owned anonymous immutable inputs.
package sealedinput

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const Seals = unix.F_SEAL_SEAL | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_WRITE

// Read accepts only a private, single-owner, sealed anonymous regular descriptor.
// The caller must disable dumpability before reading credentials and clear them
// after use. This does not establish the authority/provenance of input contents.
func Read(file *os.File, minimum, maximum int64) ([]byte, error) {
	fail := func() ([]byte, error) { return nil, errors.New("sealed input descriptor rejected") }
	if file == nil || minimum < 1 || maximum < minimum || maximum > 64<<10 {
		return fail()
	}
	var st unix.Stat_t
	if unix.Fstat(int(file.Fd()), &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 0 || st.Size < minimum || st.Size > maximum {
		return fail()
	}
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || seals&Seals != Seals {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) != st.Size {
		clear(data)
		return fail()
	}
	return data, nil
}
