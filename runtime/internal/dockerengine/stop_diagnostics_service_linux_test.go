//go:build linux

package dockerengine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
	"golang.org/x/sys/unix"
)

func fillScopeStopPipe(t *testing.T, writer *os.File) {
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

func TestDockerScopeStopDiagnosticRealStderr(t *testing.T) {
	if os.Getenv("MK_G8_SCOPE_STOP_DIAGNOSTIC_CHILD") == "1" {
		// A zero-authority scope must fail before any cgroup/filesystem operation.
		// No pidfd/process is installed, so this fixture cannot signal anything.
		before, flagErr := unix.FcntlInt(2, unix.F_GETFL, 0)
		if flagErr != nil {
			os.Exit(44)
		}
		scope := &guestscope.Scope{}
		service := &Service{scope: scope, pidfd: -1}
		for range 1000 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := service.Stop(ctx)
			cancel()
			if err == nil || err.Error() != "Docker descendants have not quiesced" || service.closed.Load() || service.pidfd != -1 || service.scope != scope || service.filesystem != nil {
				os.Exit(43)
			}
		}
		after, err := unix.FcntlInt(2, unix.F_GETFL, 0)
		if err != nil || before != after {
			os.Exit(44)
		}
		if os.Getenv("MK_G8_SCOPE_STOP_DESCRIPTOR_CHECK") == "1" {
			target, err := os.Readlink("/proc/self/fd/2")
			if err != nil {
				os.Exit(45)
			}
			entries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				os.Exit(45)
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
					os.Exit(46)
				}
			}
			if aliases != 1 {
				os.Exit(47)
			}
		}
		os.Exit(37)
	}
	for _, mode := range []string{"capture", "blocked", "closed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDockerScopeStopDiagnosticRealStderr$")
			cmd.Env = append(os.Environ(), "MK_G8_SCOPE_STOP_DIAGNOSTIC_CHILD=1", "GORACE=atexit_sleep_ms=0")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			if mode == "capture" {
				cmd.Stderr = &stderr
			} else {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				if mode == "closed" {
					reader.Close()
				} else {
					fillScopeStopPipe(t, writer)
					cmd.Env = append(cmd.Env, "MK_G8_SCOPE_STOP_DESCRIPTOR_CHECK=1")
				}
				cmd.Stderr = writer
			}
			started := time.Now()
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 37 || ctx.Err() != nil || time.Since(started) > 3*time.Second {
				t.Fatal("scope-stop diagnostic blocked failure return or changed exit", err)
			}
			if stdout.Len() != 0 {
				t.Fatal("scope-stop diagnostic crossed streams")
			}
			if mode == "capture" && stderr.String() != "MK_DOCKER_SCOPE_STOP_FAIL timeout=false canceled=false identity=true errno=0\n" {
				t.Fatal("scope-stop marker or process quota changed")
			}
		})
	}
}
