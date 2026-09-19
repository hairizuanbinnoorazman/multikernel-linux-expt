//go:build linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDisconnectNBDRequiresExactNoFollowBlockDevice(t *testing.T) {
	var calls []string
	operations := nbdDeviceOps{
		open: func(path string, flags int, mode uint32) (int, error) {
			calls = append(calls, fmt.Sprintf("open:%s:%d:%d", path, flags, mode))
			if flags != unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW || mode != 0 {
				t.Fatalf("open options = %#x, %#o", flags, mode)
			}
			return 17, nil
		},
		fstat: func(descriptor int, status *unix.Stat_t) error {
			calls = append(calls, fmt.Sprintf("fstat:%d", descriptor))
			status.Mode = unix.S_IFBLK | 0600
			status.Rdev = unix.Mkdev(nbdBlockMajor, 0)
			return nil
		},
		ioctl: func(descriptor int, request uint, value int) error {
			calls = append(calls, fmt.Sprintf("ioctl:%d:%#x:%d", descriptor, request, value))
			return nil
		},
		close: func(descriptor int) error {
			calls = append(calls, fmt.Sprintf("close:%d", descriptor))
			return nil
		},
	}
	if err := disconnectNBD("/dev/nbd1", operations); err == nil || len(calls) != 0 {
		t.Fatalf("unexpected-device result = calls:%v err:%v", calls, err)
	}
	if err := disconnectNBD(mediatedRootDevice, operations); err != nil {
		t.Fatal(err)
	}
	want := []string{
		fmt.Sprintf("open:/dev/nbd0:%d:0", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW),
		"fstat:17", fmt.Sprintf("ioctl:17:%#x:0", nbdDisconnectIOCTL), "close:17",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("disconnect calls = %v, want %v", calls, want)
	}

	calls = nil
	operations.fstat = func(descriptor int, status *unix.Stat_t) error {
		calls = append(calls, fmt.Sprintf("fstat:%d", descriptor))
		status.Mode = unix.S_IFREG | 0600
		status.Rdev = unix.Mkdev(nbdBlockMajor, 0)
		return nil
	}
	if err := disconnectNBD(mediatedRootDevice, operations); err == nil || slices.ContainsFunc(calls, func(value string) bool { return len(value) >= 6 && value[:6] == "ioctl:" }) {
		t.Fatalf("non-block device result = calls:%v err:%v", calls, err)
	}
}

func TestMediatedShutdownOrderingAndFailureBoundaries(t *testing.T) {
	var events []string
	platform := shutdownPlatform{
		syncFilesystem:      func() { events = append(events, "sync") },
		remountRootReadonly: func() error { events = append(events, "remount-ro"); return nil },
		disconnectRootNBD:   func() error { events = append(events, "disconnect-nbd"); return nil },
		poweroff:            func() error { events = append(events, "poweroff"); return nil },
	}
	if err := quiesceMediatedRoot(platform); err != nil {
		t.Fatal(err)
	}
	var evidence bytes.Buffer
	if err := finishGuestShutdown(true, platform, &evidence); err != nil {
		t.Fatal(err)
	}
	want := []string{"sync", "remount-ro", "sync", "disconnect-nbd", "poweroff"}
	if !slices.Equal(events, want) || evidence.String() != "MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0\n" {
		t.Fatalf("shutdown result = events:%v evidence:%q", events, evidence.String())
	}

	events = nil
	injected := errors.New("injected disconnect failure")
	platform.remountRootReadonly = func() error { events = append(events, "remount-ro"); return injected }
	if err := quiesceMediatedRoot(platform); !errors.Is(err, injected) {
		t.Fatalf("remount failure = %v", err)
	}
	if !slices.Equal(events, []string{"sync", "remount-ro"}) {
		t.Fatalf("failed remount events = %v", events)
	}

	events = nil
	platform.remountRootReadonly = func() error { events = append(events, "remount-ro"); return nil }
	platform.disconnectRootNBD = func() error { events = append(events, "disconnect-nbd"); return injected }
	evidence.Reset()
	if err := finishGuestShutdown(true, platform, &evidence); !errors.Is(err, injected) {
		t.Fatalf("disconnect failure = %v", err)
	}
	if !slices.Equal(events, []string{"disconnect-nbd"}) || evidence.String() != "MK_STORAGE_NBD_DISCONNECT_FAIL device=/dev/nbd0\n" {
		t.Fatalf("failed shutdown result = events:%v evidence:%q", events, evidence.String())
	}

	events = nil
	evidence.Reset()
	if err := finishGuestShutdown(false, platform, &evidence); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(events, []string{"sync", "poweroff"}) || evidence.Len() != 0 {
		t.Fatalf("non-mediated shutdown result = events:%v evidence:%q", events, evidence.String())
	}
}
