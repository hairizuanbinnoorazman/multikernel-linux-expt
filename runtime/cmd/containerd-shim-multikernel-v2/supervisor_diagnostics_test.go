//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

const supervisorDiagnosticSecret = "SYNTHETIC_SUPERVISOR_SECRET"

func requireSupervisorRecord(t *testing.T, raw []byte, code string) supervisorDiagnosticRecord {
	t.Helper()
	var r supervisorDiagnosticRecord
	if bytes.Contains(raw, []byte(supervisorDiagnosticSecret)) || json.Unmarshal(raw, &r) != nil || !validSupervisorDiagnosticRecord(r) || r.Code != code || r.OwnerKnown {
		t.Fatal("invalid/private supervisor diagnostic")
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	want := 8
	if code == "SUPERVISOR_RESTART" {
		want = 11
	}
	if len(fields) != want {
		t.Fatal("unexpected supervisor diagnostic fields")
	}
	return r
}
func TestSupervisorDiagnosticsFixedCodesQuotaAndMetadata(t *testing.T) {
	records := make(chan []byte, 64)
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	defer sink.close()
	r := newSupervisorDiagnostics(sink)
	for code := supervisorDiagnosticCode(1); int(code) < len(supervisorDiagnosticSpecs); code++ {
		var pid, attempt, signal uint32
		if code == supervisorRestart {
			pid, attempt, signal = 123, 1, 9
		}
		if !r.emit(code, pid, attempt, signal) {
			t.Fatal("valid code refused", code)
		}
		for i := 0; i < 100; i++ {
			if r.emit(code, pid, attempt, signal) {
				t.Fatal("duplicate supervisor code emitted")
			}
		}
	}
	if r.emit(0, 0, 0, 0) || r.emit(255, 0, 0, 0) {
		t.Fatal("unknown code accepted")
	}
	for code := supervisorDiagnosticCode(1); int(code) < len(supervisorDiagnosticSpecs); code++ {
		select {
		case raw := <-records:
			requireSupervisorRecord(t, raw, supervisorDiagnosticSpecs[code].code)
		case <-time.After(time.Second):
			t.Fatal("missing code")
		}
	}
}
func TestSupervisorDiagnosticsRejectInvalidMetadataAndFullQueueConsumesAttempt(t *testing.T) {
	base := supervisorDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2", Scope: "worker-supervisor", Code: "SUPERVISOR_START_FAILED", Operation: "start-worker", SupervisorPID: 1, At: time.Now().UTC().Format(time.RFC3339Nano)}
	changes := []func(*supervisorDiagnosticRecord){func(r *supervisorDiagnosticRecord) { r.OwnerKnown = true }, func(r *supervisorDiagnosticRecord) { r.Scope = supervisorDiagnosticSecret }, func(r *supervisorDiagnosticRecord) { r.SupervisorPID = 0 }, func(r *supervisorDiagnosticRecord) { r.WorkerPID = 1 << 31 }, func(r *supervisorDiagnosticRecord) { r.At = "invalid" }, func(r *supervisorDiagnosticRecord) { r.Code = "UNKNOWN" }, func(r *supervisorDiagnosticRecord) { r.Attempt = 1 }, func(r *supervisorDiagnosticRecord) { r.Signal = 9 }, func(r *supervisorDiagnosticRecord) { r.Operation = "private/path" }, func(r *supervisorDiagnosticRecord) {
		r.Code = "SUPERVISOR_RESTART"
		r.Operation = "restart-worker"
		r.WorkerPID = 1
		r.Attempt = 11
		r.Signal = 9
	}}
	sink := newProcessDiagnosticSink(nil)
	defer sink.close()
	for _, change := range changes {
		r := base
		change(&r)
		if validSupervisorDiagnosticRecord(r) || sink.trySupervisorRecord(r) {
			t.Fatal("invalid metadata accepted")
		}
	}
	blocked, _ := diagnosticSinkForTest(t, true)
	for len(blocked.queue) < processDiagnosticQueueSize {
		if !blocked.tryRecord(diagnosticTestRecord()) {
			t.Fatal("queue fill failed")
		}
	}
	r := newSupervisorDiagnostics(blocked)
	if r.emit(supervisorStartFailed, 0, 0, 0) || r.attempted&(uint64(1)<<supervisorStartFailed) == 0 {
		t.Fatal("full queue did not consume attempt")
	}
}
func TestSupervisorDiagnosticsRedactLaunchErrorsAndDoNotBlockBudgetCleanup(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "capture"
		if blocked {
			name = "blocked"
		}
		t.Run(name, func(t *testing.T) {
			sink, records := diagnosticSinkForTest(t, blocked)
			bundle := privateTestDirectory(t)
			d, err := safefile.OpenOwnedDirectory(bundle)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			listener, err := os.CreateTemp(bundle, "listener")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			reporter := newSupervisorDiagnostics(sink)
			missing := filepath.Join(bundle, supervisorDiagnosticSecret)
			if code := superviseShimWorkerWithDirectoryDiagnostics(listener, missing, nil, bundle, os.Environ(), d, reporter); code != 1 {
				t.Fatal("wrong launch failure exit")
			}
			if !blocked {
				select {
				case raw := <-records:
					requireSupervisorRecord(t, raw, "SUPERVISOR_START_FAILED")
				case <-time.After(time.Second):
					t.Fatal("missing launch diagnostic")
				}
			}
			// Ten actual signaled workers consume the unchanged restart budget. The
			// injected writer is already blocked before any worker is started.
			reporter = newSupervisorDiagnostics(sink)
			if code := superviseShimWorkerWithDirectoryDiagnostics(listener, "/bin/sh", []string{"-c", "kill -KILL $$"}, bundle, os.Environ(), d, reporter); code != 1 {
				t.Fatal("wrong exhausted-budget exit")
			}
			if _, err := os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("worker PID ownership survived budget exhaustion", err)
			}
			if blocked {
				if len(sink.queue) != 3 {
					t.Fatal("supervisor retry log volume was not bounded", len(sink.queue))
				}
			} else {
				for _, code := range []string{"SUPERVISOR_RESTART", "SUPERVISOR_BUDGET_EXHAUSTED"} {
					select {
					case raw := <-records:
						requireSupervisorRecord(t, raw, code)
					case <-time.After(time.Second):
						t.Fatal("missing budget diagnostic")
					}
				}
				select {
				case <-records:
					t.Fatal("extra restart record")
				default:
				}
			}
		})
	}
}
func TestSupervisorDiagnosticsRealStderrRestartAndPIDCleanup(t *testing.T) {
	if mode := os.Getenv("MK_G8_SUPERVISOR_DIAGNOSTIC_CHILD"); mode != "" {
		bundle := privateTestDirectory(t)
		listener, err := os.CreateTemp(bundle, "listener")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		marker := filepath.Join(bundle, "signaled")
		env := append(os.Environ(), "MK_SHIM_SUPERVISOR_TEST_MARKER="+marker)
		if code := superviseShimWorkerWith(listener, os.Args[0], []string{"-test.run=^$"}, bundle, env); code != 0 {
			t.Fatal("signaled worker recovery failed")
		}
		if _, err := os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("PID file remains", err)
		}
		if mode == "blocked" {
			sink := processDiagnosticDefault()
			deadline := time.Now().Add(time.Second)
			for len(sink.queue) != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(sink.queue) != 0 || sink.disabled.Load() {
				t.Fatal("real pipe did not block the writer")
			}
			select {
			case <-sink.done:
				t.Fatal("blocked writer exited")
			default:
			}
		}
		fmt.Fprintln(os.Stdout, "SUPERVISOR_SIGKILL_RESTART_AND_PID_CLEANUP_COMPLETE_WITH_BOUNDED_STDERR")
		return
	}
	for _, mode := range []string{"capture", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSupervisorDiagnosticsRealStderrRestartAndPIDCleanup$", "-test.v")
			cmd.Env = append(os.Environ(), "MK_G8_SUPERVISOR_DIAGNOSTIC_CHILD="+mode)
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
				fd := int(writer.Fd())
				if err = unix.SetNonblock(fd, true); err != nil {
					t.Fatal(err)
				}
				for {
					_, err = unix.Write(fd, bytes.Repeat([]byte("x"), 4096))
					if errors.Is(err, syscall.EAGAIN) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if err = unix.SetNonblock(fd, false); err != nil {
					t.Fatal(err)
				}
				cmd.Stderr = writer
			}
			if err := cmd.Run(); err != nil {
				t.Fatal("real-stderr supervisor fixture failed", err)
			}
			if !strings.Contains(stdout.String(), "SUPERVISOR_SIGKILL_RESTART_AND_PID_CLEANUP_COMPLETE_WITH_BOUNDED_STDERR") {
				t.Fatal("missing restart proof")
			}
			if mode == "capture" {
				lines := bytes.Split(bytes.TrimSpace(stderr.Bytes()), []byte("\n"))
				if len(lines) != 1 {
					t.Fatal("wrong real stderr record count", len(lines))
				}
				requireSupervisorRecord(t, lines[0], "SUPERVISOR_RESTART")
			}
		})
	}
}

type supervisorPrivateErrorConnection struct {
	net.Conn
	gate chan struct{}
	once sync.Once
}

func (c *supervisorPrivateErrorConnection) Read([]byte) (int, error) {
	<-c.gate
	return 0, errors.New(supervisorDiagnosticSecret + " /private/supervisor/proxy")
}
func (c *supervisorPrivateErrorConnection) release()     { c.once.Do(func() { close(c.gate) }) }
func (c *supervisorPrivateErrorConnection) Close() error { c.release(); return c.Conn.Close() }
func TestSupervisorDiagnosticsPrivateProxyErrorDoesNotBlockReap(t *testing.T) {
	previous := shimWorkerDisconnectGrace
	shimWorkerDisconnectGrace = 20 * time.Millisecond
	defer func() { shimWorkerDisconnectGrace = previous }()
	for _, blocked := range []bool{false, true} {
		name := "capture"
		if blocked {
			name = "blocked"
		}
		t.Run(name, func(t *testing.T) {
			sink, records := diagnosticSinkForTest(t, blocked)
			bundle := privateTestDirectory(t)
			directory, err := safefile.OpenOwnedDirectory(bundle)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			client, base := net.Pipe()
			connection := &supervisorPrivateErrorConnection{Conn: base, gate: make(chan struct{})}
			done := make(chan int, 1)
			complete := false
			t.Cleanup(func() {
				connection.Close()
				client.Close()
				if !complete {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("supervisor helper failed to stop")
					}
				}
			})
			marker := filepath.Join(bundle, "worker-pid")
			environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_BLOCK_WORKER="+marker)
			go func() {
				done <- superviseReconnectableShimWorkerWithDirectoryDiagnostics(connection, nil, os.Args[0], []string{"-test.run=^$"}, bundle, environment, directory, newSupervisorDiagnostics(sink))
			}()
			var pid int
			waitNetworkCondition(t, func() bool {
				data, err := os.ReadFile(marker)
				if err != nil {
					return false
				}
				pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
				return err == nil && pid > 0
			}, "supervised live worker")
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatal("worker was not live before proxy failure", err)
			}
			connection.release()
			select {
			case code := <-done:
				complete = true
				if code != 0 {
					t.Fatal("proxy disconnect changed cleanup result", code)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("diagnostic pressure blocked orphan reap")
			}
			if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatal("worker survived proxy failure", err)
			}
			if _, err := os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("PID ownership remains", err)
			}
			if blocked {
				if len(sink.queue) != 1 {
					t.Fatal("wrong proxy diagnostic count")
				}
			} else {
				select {
				case raw := <-records:
					requireSupervisorRecord(t, raw, "SUPERVISOR_PROXY_FAILED")
				case <-time.After(time.Second):
					t.Fatal("missing proxy failure diagnostic")
				}
			}
		})
	}
}
