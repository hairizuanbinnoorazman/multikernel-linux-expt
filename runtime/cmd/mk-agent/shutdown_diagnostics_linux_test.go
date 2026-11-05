//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const shutdownSyntheticSecret = "SYNTHETIC_PRIVATE_DOCKER_OUTPUT /private/guest/env=secret"

type shutdownMarkerCase struct {
	name, marker string
	stderr       bool
}

var shutdownMarkerCases = []shutdownMarkerCase{
	{"docker-stop", "MK_DOCKER_STOP_FAIL errno=1\n", true},
	{"docker-storage", "MK_DOCKER_STORAGE_QUIESCE_FAIL\n", true},
	{"root-fail", "MK_STORAGE_ROOT_QUIESCE_FAIL stage=remount-ro errno=1\n", false},
	{"root-pass", "MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0\n", false},
	{"disconnect-fail", "MK_STORAGE_NBD_DISCONNECT_FAIL device=/dev/nbd0\n", false},
	{"disconnect-pass", "MK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0\n", false},
	{"combined", "MK_STORAGE_ROOT_QUIESCE_PASS stages=sync,remount-ro,sync device=/dev/nbd0\nMK_STORAGE_NBD_DISCONNECT_PASS device=/dev/nbd0\n", false},
}

// All platform operations are callbacks; no sync, mount, NBD ioctl, engine stop
// or poweroff is performed by these controls, including when run as root.
func runShutdownMarkerFixture(name string) bool {
	var calls []string
	injected := errors.Join(unix.EPERM, errors.New(shutdownSyntheticSecret))
	platform := shutdownPlatform{
		syncFilesystem:      func() { calls = append(calls, "sync") },
		remountRootReadonly: func() error { calls = append(calls, "remount"); return nil },
		disconnectRootNBD:   func() error { calls = append(calls, "disconnect"); return nil },
		poweroff:            func() error { calls = append(calls, "poweroff"); return nil },
	}
	stop := func(context.Context) error { calls = append(calls, "stop"); return nil }
	var err error
	var want []string
	switch name {
	case "docker-stop":
		err = stopDockerBeforeStorage(func(context.Context) error { calls = append(calls, "stop"); return injected }, func() error { calls = append(calls, "UNSAFE-QUIESCE"); return nil })
		return err != nil && err.Error() == "Docker service has not quiesced" && slices.Equal(calls, []string{"stop"})
	case "docker-storage":
		err = stopDockerBeforeStorage(stop, func() error { calls = append(calls, "quiesce"); return injected })
		return errors.Is(err, injected) && slices.Equal(calls, []string{"stop", "quiesce"})
	case "root-fail":
		platform.remountRootReadonly = func() error { calls = append(calls, "remount"); return injected }
		err = quiesceMediatedRoot(platform, os.Stdout)
		return errors.Is(err, injected) && slices.Equal(calls, []string{"sync", "remount"})
	case "root-pass":
		err = quiesceMediatedRoot(platform, os.Stdout)
		want = []string{"sync", "remount", "sync"}
	case "disconnect-fail":
		platform.disconnectRootNBD = func() error { calls = append(calls, "disconnect"); return injected }
		err = finishGuestShutdown(true, platform, os.Stdout)
		return errors.Is(err, injected) && slices.Equal(calls, []string{"disconnect"})
	case "disconnect-pass":
		err = finishGuestShutdown(true, platform, os.Stdout)
		want = []string{"disconnect", "poweroff"}
	case "combined":
		err = stopDockerBeforeStorage(stop, func() error { return quiesceMediatedRoot(platform, os.Stdout) })
		if err == nil {
			err = finishGuestShutdown(true, platform, os.Stdout)
		}
		want = []string{"stop", "sync", "remount", "sync", "disconnect", "poweroff"}
	default:
		return false
	}
	return err == nil && slices.Equal(calls, want)
}

func fillShutdownPipe(t *testing.T, writer *os.File) {
	t.Helper()
	fd := int(writer.Fd())
	if err := unix.SetNonblock(fd, true); err != nil {
		t.Fatal(err)
	}
	padding := bytes.Repeat([]byte("x"), 4096)
	for {
		_, err := unix.Write(fd, padding)
		if errors.Is(err, unix.EAGAIN) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownDiagnosticsRealDescriptors(t *testing.T) {
	if scenario := os.Getenv("MK_G8_SHUTDOWN_MARKER_CHILD"); scenario != "" {
		if !runShutdownMarkerFixture(scenario) {
			os.Exit(43)
		}
		os.Exit(37)
	}
	for _, tc := range shutdownMarkerCases {
		for _, mode := range []string{"capture", "blocked", "closed"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownDiagnosticsRealDescriptors$")
				cmd.Env = append(os.Environ(), "MK_G8_SHUTDOWN_MARKER_CHILD="+tc.name, "GORACE=atexit_sleep_ms=0")
				var output, other bytes.Buffer
				var destination io.Writer = &output
				if mode != "capture" {
					reader, writer, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					defer reader.Close()
					defer writer.Close()
					if mode == "closed" {
						reader.Close()
					} else {
						fillShutdownPipe(t, writer)
					}
					destination = writer
				}
				if tc.stderr {
					cmd.Stderr = destination
					cmd.Stdout = &other
				} else {
					cmd.Stdout = destination
					cmd.Stderr = &other
				}
				started := time.Now()
				err := cmd.Run()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 37 || ctx.Err() != nil || time.Since(started) > 3*time.Second {
					t.Fatal("diagnostic changed shutdown control flow or bounded completion", err)
				}
				if other.Len() != 0 {
					t.Fatal("shutdown diagnostic crossed streams")
				}
				if mode == "capture" && output.String() != tc.marker {
					t.Fatal("legacy evidence marker changed")
				}
				if bytes.Contains(output.Bytes(), []byte(shutdownSyntheticSecret)) {
					t.Fatal("private error reached output")
				}
			})
		}
	}
}

type shutdownBlockedWriter struct {
	entered, release, finished chan struct{}
	calls                      atomic.Int64
}

func (w *shutdownBlockedWriter) Write(data []byte) (int, error) {
	w.calls.Add(1)
	close(w.entered)
	<-w.release
	close(w.finished)
	return len(data), nil
}
func TestShutdownDiagnosticLanesBoundPressureAndStageQuota(t *testing.T) {
	stdout := &shutdownBlockedWriter{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	stderr := &shutdownBlockedWriter{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	defer func() { close(stdout.release); close(stderr.release); <-stdout.finished; <-stderr.finished }()
	reporter := shutdownDiagnosticReporter{stdout: shutdownDiagnosticLane{writer: stdout}, stderr: shutdownDiagnosticLane{writer: stderr}}
	var wg sync.WaitGroup
	wg.Go(func() {
		if reporter.report(rootQuiescePassed, 0) {
			t.Error("blocked stdout claimed delivery")
		}
	})
	wg.Go(func() {
		if reporter.report(dockerStopFailed, 1) {
			t.Error("blocked stderr claimed delivery")
		}
	})
	for _, entered := range []chan struct{}{stdout.entered, stderr.entered} {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("blocked stream did not start")
		}
	}
	wg.Wait()
	for range 1000 {
		for kind := dockerStopFailed; kind <= nbdDisconnectPassed; kind++ {
			wg.Go(func() {
				if reporter.report(kind, 1) {
					t.Error("pressure claimed delivery")
				}
			})
		}
	}
	wg.Wait()
	if stdout.calls.Load() != 1 || stderr.calls.Load() != 1 || reporter.attempted.Load() != 63 {
		t.Fatal("blocked stream accumulated workers or retried stages")
	}
}

type shutdownFailedWriter struct {
	short bool
	calls int
}

func (w *shutdownFailedWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.short {
		return len(data) - 1, nil
	}
	return 0, io.ErrClosedPipe
}
func TestShutdownDiagnosticFixedRecordsAndFailedDelivery(t *testing.T) {
	for _, errno := range []uint64{0, 1, 4095, 4096, ^uint64(0)} {
		for kind := dockerStopFailed; kind <= nbdDisconnectPassed; kind++ {
			data, stderr := shutdownDiagnosticRecord(kind, errno)
			if len(data) > shutdownDiagnosticLimit || bytes.Count(data, []byte("\n")) != 1 || stderr != (kind == dockerStopFailed || kind == dockerStorageQuiesceFailed) {
				t.Fatal("invalid fixed marker")
			}
			hasErrno := bytes.Contains(data, []byte(" errno="))
			if hasErrno != ((kind == dockerStopFailed || kind == rootQuiesceFailed) && errno > 0 && errno <= 4095) {
				t.Fatal("unbounded errno accepted")
			}
		}
	}
	for _, short := range []bool{false, true} {
		writer := &shutdownFailedWriter{short: short}
		r := shutdownDiagnosticReporter{stdout: shutdownDiagnosticLane{writer: writer}, stderr: shutdownDiagnosticLane{writer: writer}}
		if r.report(0, 0) || r.report(255, 0) || r.attempted.Load() != 0 {
			t.Fatal("invalid stage consumed quota")
		}
		if r.report(rootQuiescePassed, 0) || r.report(rootQuiescePassed, 0) || writer.calls != 1 {
			t.Fatal("failed marker retried or claimed delivery")
		}
	}
	var r *shutdownDiagnosticReporter
	if r.report(rootQuiescePassed, 0) {
		t.Fatal("nil reporter accepted")
	}
	empty := shutdownDiagnosticReporter{}
	if empty.report(rootQuiescePassed, 0) || empty.attempted.Load() == 0 {
		t.Fatal("missing writer did not consume attempt")
	}
}

func TestShutdownDiagnosticsBlockedDescriptorsArePrivateAndCloexec(t *testing.T) {
	if os.Getenv("MK_G8_SHUTDOWN_FD_CHILD") == "1" {
		processShutdownDiagnostics.report(rootQuiescePassed, 0)
		processShutdownDiagnostics.report(dockerStopFailed, 1)
		for range 1000 {
			for kind := dockerStopFailed; kind <= nbdDisconnectPassed; kind++ {
				processShutdownDiagnostics.report(kind, 1)
			}
		}
		for _, standard := range []string{"1", "2"} {
			target, err := os.Readlink("/proc/self/fd/" + standard)
			if err != nil {
				os.Exit(44)
			}
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				os.Exit(44)
			}
			aliases := 0
			for _, entry := range entries {
				fd, err := strconv.Atoi(entry.Name())
				if err != nil || fd < 3 {
					continue
				}
				link, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
				if err != nil || link != target {
					continue
				}
				aliases++
				flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					os.Exit(45)
				}
			}
			if aliases != 1 {
				os.Exit(46)
			}
		}
		os.Exit(37)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestShutdownDiagnosticsBlockedDescriptorsArePrivateAndCloexec$")
	cmd.Env = append(os.Environ(), "MK_G8_SHUTDOWN_FD_CHILD=1", "GORACE=atexit_sleep_ms=0")
	for _, stream := range []string{"stdout", "stderr"} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		defer writer.Close()
		fillShutdownPipe(t, writer)
		if stream == "stdout" {
			cmd.Stdout = writer
		} else {
			cmd.Stderr = writer
		}
	}
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 || ctx.Err() != nil {
		t.Fatal("blocked descriptor bounds or CLOEXEC failed", err)
	}
}

func TestShutdownDiagnosticHealthyStreamsPreserveDistinctMarkers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := shutdownDiagnosticReporter{stdout: shutdownDiagnosticLane{writer: &stdout}, stderr: shutdownDiagnosticLane{writer: &stderr}}
	for kind := dockerStopFailed; kind <= nbdDisconnectPassed; kind++ {
		if !r.report(kind, 1) || r.report(kind, 1) {
			t.Fatal("stage delivery/quota failed", kind)
		}
	}
	if strings.Count(stdout.String(), "\n") != 4 || strings.Count(stderr.String(), "\n") != 2 {
		t.Fatal("wrong stream/stage counts")
	}
}
