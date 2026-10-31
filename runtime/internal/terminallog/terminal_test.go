package terminallog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTerminalCodesHaveOnlyFixedMetadata(t *testing.T) {
	seen := map[string]bool{}
	for code := Code(1); int(code) < len(specs); code++ {
		var out bytes.Buffer
		r := &reporter{writer: &out}
		if !r.report(code) || r.report(code) {
			t.Fatal("terminal quota not enforced", code)
		}
		raw := out.Bytes()
		if len(raw) > RecordLimit || bytes.Count(raw, []byte("\n")) != 1 {
			t.Fatal("record is not bounded")
		}
		var got record
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &got) != nil || json.Unmarshal(raw, &fields) != nil {
			t.Fatal("invalid record")
		}
		spec := specs[code]
		if len(fields) != 8 || got.Version != 1 || got.Scope != "daemon-terminal" || got.Component != spec.component || got.Code != spec.code || got.Operation != spec.operation || got.PID != os.Getpid() || got.OwnerKnown {
			t.Fatal("invalid fixed metadata", code)
		}
		at, err := time.Parse(time.RFC3339Nano, got.At)
		if err != nil || at.UTC().Format(time.RFC3339Nano) != got.At {
			t.Fatal("noncanonical timestamp")
		}
		if seen[got.Code] {
			t.Fatal("duplicate code")
		}
		seen[got.Code] = true
	}
}

type blockingWriter struct {
	start   chan struct{}
	release chan struct{}
	done    chan struct{}
	calls   atomic.Int64
}

func (w *blockingWriter) Write(b []byte) (int, error) {
	w.calls.Add(1)
	close(w.start)
	<-w.release
	close(w.done)
	return len(b), nil
}
func TestTerminalBlockedWriterHasOneAttemptAndBoundedWait(t *testing.T) {
	w := &blockingWriter{start: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
	defer func() { close(w.release); <-w.done }()
	r := &reporter{writer: w}
	returned := make(chan bool, 1)
	go func() { returned <- r.report(RuntimeReconcile) }()
	select {
	case <-w.start:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	select {
	case ok := <-returned:
		if ok {
			t.Fatal("blocked delivery claimed success")
		}
	case <-time.After(time.Second):
		t.Fatal("terminal diagnostic blocked caller")
	}
	var wg sync.WaitGroup
	for range 1000 {
		wg.Go(func() {
			if r.report(NetworkListen) {
				t.Error("later terminal attempt accepted")
			}
		})
	}
	wg.Wait()
	if w.calls.Load() != 1 {
		t.Fatal("writers accumulated")
	}
}

type failedWriter struct {
	short bool
	calls int
}

func (w *failedWriter) Write(b []byte) (int, error) {
	w.calls++
	if w.short {
		return len(b) - 1, nil
	}
	return 0, io.ErrClosedPipe
}
func TestTerminalFailureDoesNotRetryAndInvalidCodesDoNotConsumeQuota(t *testing.T) {
	for _, short := range []bool{false, true} {
		w := &failedWriter{short: short}
		r := &reporter{writer: w}
		if r.report(0) || r.report(255) || r.attempted.Load() {
			t.Fatal("invalid code consumed quota")
		}
		if r.report(NetworkReconcile) || r.report(RuntimeListen) || w.calls != 1 {
			t.Fatal("failed write retried")
		}
	}
	var r *reporter
	if r.report(RuntimeListen) {
		t.Fatal("nil reporter accepted")
	}
	disabled := &reporter{}
	if disabled.report(RuntimeListen) || !disabled.attempted.Load() {
		t.Fatal("disabled writer quota")
	}
}
func TestTerminalConcurrentCodesWriteOnlyOneRecord(t *testing.T) {
	var out bytes.Buffer
	r := &reporter{writer: &out}
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for range 1000 {
		wg.Go(func() {
			if r.report(NetworkReconcile) {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 || bytes.Count(out.Bytes(), []byte("\n")) != 1 {
		t.Fatal("process quota failed")
	}
}

func TestTerminalClosedRealStderrPreservesExitStatus(t *testing.T) {
	if os.Getenv("MK_G8_TERMINAL_CLOSED_CHILD") == "1" {
		Report(NetworkListen)
		os.Exit(17)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalClosedRealStderrPreservesExitStatus$")
	cmd.Env = append(os.Environ(), "MK_G8_TERMINAL_CLOSED_CHILD=1")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()
	defer writer.Close()
	cmd.Stderr = writer
	err = cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 17 || ctx.Err() != nil {
		t.Fatal("diagnostic changed terminal exit status", err)
	}
}

func TestTerminalBlockedRealStderrRetainsOnlyOnePrivateCloexecDescriptor(t *testing.T) {
	if os.Getenv("MK_G8_TERMINAL_DESCRIPTOR_CHILD") == "1" {
		flags, err := unix.FcntlInt(2, unix.F_GETFL, 0)
		if err != nil {
			t.Fatal(err)
		}
		if Report(RuntimeListen) {
			t.Fatal("full stderr delivery succeeded")
		}
		for range 1000 {
			if Report(NetworkListen) {
				t.Fatal("second process write accepted")
			}
		}
		after, err := unix.FcntlInt(2, unix.F_GETFL, 0)
		if err != nil || after != flags {
			t.Fatal("inherited flags changed")
		}
		target, err := os.Readlink("/proc/self/fd/2")
		if err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		aliases := 0
		for _, entry := range entries {
			fd, e := strconv.Atoi(entry.Name())
			if e != nil || fd < 3 {
				continue
			}
			link, e := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
			if e != nil || link != target {
				continue
			}
			aliases++
			closeFlags, e := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
			if e != nil || closeFlags&unix.FD_CLOEXEC == 0 {
				t.Fatal("private stderr descriptor could survive exec")
			}
		}
		if aliases != 1 {
			t.Fatal("wrong private stderr descriptor count", aliases)
		}
		os.Exit(19)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	fd := int(writer.Fd())
	if err = unix.SetNonblock(fd, true); err != nil {
		t.Fatal(err)
	}
	for {
		_, err = unix.Write(fd, bytes.Repeat([]byte("x"), 4096))
		if errors.Is(err, unix.EAGAIN) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = unix.SetNonblock(fd, false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalBlockedRealStderrRetainsOnlyOnePrivateCloexecDescriptor$")
	cmd.Env = append(os.Environ(), "MK_G8_TERMINAL_DESCRIPTOR_CHILD=1")
	cmd.Stderr = writer
	err = cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 19 || ctx.Err() != nil {
		t.Fatal("private descriptor control failed", err)
	}
}
