//go:build linux

package network

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	tunSetIFF = 0x400454ca
	iffTun    = 0x0001
	iffNoPI   = 0x1000
)

// OpenTUN enters the endpoint's primary-owned network namespace only on a
// locked helper thread, attaches to the already-created TUN, and restores the
// caller's namespace before returning the descriptor.
func OpenTUN(endpoint Endpoint) (result *os.File, retErr error) {
	if _, err := netnsTarget(endpoint.NetNS); err != nil {
		return nil, err
	}
	if !identifier.MatchString(endpoint.IfName) || len(endpoint.IfName) > 15 {
		return nil, errors.New("invalid endpoint TUN name")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	current, err := os.Open("/proc/self/task/" + strconv.Itoa(unix.Gettid()) + "/ns/net")
	if err != nil {
		return nil, err
	}
	defer current.Close()
	target, err := os.Open(endpoint.NetNS)
	if err != nil {
		return nil, err
	}
	defer target.Close()
	if err = unix.Setns(int(target.Fd()), unix.CLONE_NEWNET); err != nil {
		return nil, err
	}
	defer func() {
		if restoreErr := unix.Setns(int(current.Fd()), unix.CLONE_NEWNET); restoreErr != nil {
			if result != nil {
				_ = result.Close()
				result = nil
			}
			retErr = errors.Join(retErr, fmt.Errorf("restore primary network namespace: %w", restoreErr))
		}
	}()
	if _, err = os.Stat("/sys/class/net/" + endpoint.IfName); err != nil {
		return nil, fmt.Errorf("CNI TUN is absent: %w", err)
	}
	device, err := os.OpenFile("/dev/net/tun", os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	request, err := unix.NewIfreq(endpoint.IfName)
	if err != nil {
		device.Close()
		return nil, err
	}
	request.SetUint16(iffTun | iffNoPI)
	if err = unix.IoctlIfreq(int(device.Fd()), tunSetIFF, request); err != nil {
		device.Close()
		return nil, err
	}
	return device, nil
}
