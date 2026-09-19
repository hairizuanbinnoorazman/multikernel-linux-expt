//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	mediatedRootDevice = "/dev/nbd0"
	nbdBlockMajor      = 43
	nbdDisconnectIOCTL = 0xab08
)

type nbdDeviceOps struct {
	open  func(string, int, uint32) (int, error)
	fstat func(int, *unix.Stat_t) error
	ioctl func(int, uint, int) error
	close func(int) error
}

func disconnectNBD(path string, operations nbdDeviceOps) error {
	if path != mediatedRootDevice {
		return errors.New("refuse to disconnect an unexpected NBD device")
	}
	descriptor, err := operations.open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	var status unix.Stat_t
	if err = operations.fstat(descriptor, &status); err != nil {
		return errors.Join(err, operations.close(descriptor))
	}
	if status.Mode&unix.S_IFMT != unix.S_IFBLK || unix.Major(uint64(status.Rdev)) != nbdBlockMajor || unix.Minor(uint64(status.Rdev)) != 0 {
		return errors.Join(errors.New("mediated root device identity is not NBD 0"), operations.close(descriptor))
	}
	err = operations.ioctl(descriptor, nbdDisconnectIOCTL, 0)
	return errors.Join(err, operations.close(descriptor))
}

type shutdownPlatform struct {
	syncFilesystem      func()
	remountRootReadonly func() error
	disconnectRootNBD   func() error
	poweroff            func() error
}

func linuxShutdownPlatform() shutdownPlatform {
	return shutdownPlatform{
		syncFilesystem: syscall.Sync,
		remountRootReadonly: func() error {
			return unix.Mount("", "/", "", unix.MS_REMOUNT|unix.MS_RDONLY, "")
		},
		disconnectRootNBD: func() error {
			return disconnectNBD(mediatedRootDevice, nbdDeviceOps{
				open: unix.Open, fstat: unix.Fstat, ioctl: unix.IoctlSetInt, close: unix.Close,
			})
		},
		poweroff: func() error { return unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF) },
	}
}

func quiesceMediatedRoot(platform shutdownPlatform) error {
	platform.syncFilesystem()
	if err := platform.remountRootReadonly(); err != nil {
		return err
	}
	platform.syncFilesystem()
	return nil
}

func finishGuestShutdown(mediated bool, platform shutdownPlatform, evidence io.Writer) error {
	if mediated {
		if err := platform.disconnectRootNBD(); err != nil {
			fmt.Fprintln(evidence, "MK_STORAGE_NBD_DISCONNECT_FAIL device=/dev/nbd0")
			return fmt.Errorf("disconnect mediated root: %w", err)
		}
		fmt.Fprintln(evidence, "MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0")
	} else {
		platform.syncFilesystem()
	}
	return platform.poweroff()
}
