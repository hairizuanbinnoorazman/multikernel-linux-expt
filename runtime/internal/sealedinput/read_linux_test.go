//go:build linux

package sealedinput

import (
	"bytes"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadRequiresPrivateSealedAnonymousBoundedInput(t *testing.T) {
	for _, mode := range []string{"valid", "unsealed", "wrong-mode", "too-small", "too-large", "already-read", "invalid-bounds"} {
		t.Run(mode, func(t *testing.T) {
			fd, e := unix.MemfdCreate("sealed-input-test", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
			if e != nil {
				t.Fatal(e)
			}
			f := os.NewFile(uintptr(fd), "sealed-input-test")
			defer f.Close()
			f.Chmod(0600)
			data := bytes.Repeat([]byte("x"), 32)
			if mode == "too-small" {
				data = data[:31]
			}
			if mode == "too-large" {
				data = append(data, 'x')
			}
			f.Write(data)
			f.Seek(0, 0)
			if mode == "wrong-mode" {
				f.Chmod(0644)
			}
			if mode != "unsealed" {
				if _, e = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, Seals); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "already-read" {
				f.Seek(0, 2)
			}
			maximum := int64(32)
			if mode == "invalid-bounds" {
				maximum = 64<<10 + 1
			}
			value, e := Read(f, 32, maximum)
			if mode == "valid" {
				if e != nil || !bytes.Equal(value, data) {
					t.Fatal("sealed input rejected")
				}
			} else if e == nil || value != nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
	f, e := os.CreateTemp(t.TempDir(), "named")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	f.Write(bytes.Repeat([]byte("x"), 32))
	f.Seek(0, 0)
	if _, e = Read(f, 32, 32); e == nil {
		t.Fatal("named input accepted")
	}
	if _, e = Read(nil, 32, 32); e == nil {
		t.Fatal("nil input accepted")
	}
}
