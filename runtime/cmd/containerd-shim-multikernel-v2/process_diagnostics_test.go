//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type diagnosticChannelWriter struct{ records chan []byte }

func (w diagnosticChannelWriter) Write(p []byte) (int, error) {
	w.records <- append([]byte(nil), p...)
	return len(p), nil
}

type diagnosticBlockedWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *diagnosticBlockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(p), nil
}

func diagnosticTestRecord() processDiagnosticRecord {
	return processDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2", Operation: "read-process-output", Code: "OUTPUT_READ_FAILED", SandboxID: "box", Generation: strings.Repeat("a", 32), ProcessID: "init", At: time.Now().UTC().Format(time.RFC3339Nano)}
}

func TestProcessDiagnosticsBoundedQueueDoesNotWaitForBlockedWriter(t *testing.T) {
	w := &diagnosticBlockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	sink := newProcessDiagnosticSink(w)
	defer func() {
		close(w.release)
		sink.close()
		select {
		case <-sink.done:
		case <-time.After(time.Second):
			t.Error("writer did not stop after release")
		}
	}()
	record := diagnosticTestRecord()
	if !sink.tryRecord(record) {
		t.Fatal("initial record refused")
	}
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not block")
	}
	start := time.Now()
	accepted := 0
	for i := 0; i < 2048; i++ {
		if sink.tryRecord(record) {
			accepted++
		}
	}
	if accepted != processDiagnosticQueueSize || len(sink.queue) != processDiagnosticQueueSize || sink.dropped.Load() != 2048-processDiagnosticQueueSize || time.Since(start) > 2*time.Second {
		t.Fatal("queue grew or producer blocked", accepted, len(sink.queue), sink.dropped.Load())
	}
	sink.close() // Must return even with the writer still blocked.
}

func TestProcessDiagnosticsOneAttemptPerCodeAndFixedMetadata(t *testing.T) {
	records := make(chan []byte, 64)
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	defer sink.close()
	s := &service{sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, processDiagnosticSink: sink}
	reporter := s.processDiagnostics("exec-1")
	for code := processDiagnosticCode(1); int(code) < len(processDiagnosticSpecs); code++ {
		if !reporter.emit(code, 7) {
			t.Fatal("first event refused", code)
		}
		for i := 0; i < 100; i++ {
			if reporter.emit(code, 999) {
				t.Fatal("duplicate code enqueued", code)
			}
		}
	}
	if reporter.emit(0, 0) || reporter.emit(255, 0) {
		t.Fatal("unknown code accepted")
	}
	for i := 1; i < len(processDiagnosticSpecs); i++ {
		select {
		case raw := <-records:
			if len(raw) > processDiagnosticRecordLimit || bytes.Contains(raw, []byte("private-credential")) {
				t.Fatal("unsafe frame")
			}
			var record processDiagnosticRecord
			if err := json.Unmarshal(raw, &record); err != nil || !validProcessDiagnosticRecord(record) || record.Bytes != 7 || record.SandboxID != "box" || record.ProcessID != "exec-1" {
				t.Fatal("invalid fixed metadata record")
			}
			var fields map[string]any
			_ = json.Unmarshal(raw, &fields)
			if len(fields) != 9 {
				t.Fatal("unexpected event fields")
			}
		case <-time.After(time.Second):
			t.Fatal("missing accepted event")
		}
	}
	select {
	case <-records:
		t.Fatal("extra records")
	default:
	}
}

func TestProcessDiagnosticsRejectInvalidContextAndMetadataWithoutReflection(t *testing.T) {
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{make(chan []byte, 64)})
	defer sink.close()
	for _, mode := range []string{"sandbox", "generation", "process", "code", "operation", "at", "component", "version"} {
		r := diagnosticTestRecord()
		switch mode {
		case "sandbox":
			r.SandboxID = "private/path"
		case "generation":
			r.Generation = "private-credential"
		case "process":
			r.ProcessID = strings.Repeat("x", 129)
		case "code":
			r.Code = "private-credential"
		case "operation":
			r.Operation = "repair"
		case "at":
			r.At = "private-credential"
		case "component":
			r.Component = "guest"
		case "version":
			r.Version = 2
		}
		if sink.tryRecord(r) {
			t.Fatal("invalid metadata accepted", mode)
		}
	}
	s := &service{sandbox: protocol.Sandbox{ID: "box", Generation: "private-credential"}, processDiagnosticSink: sink}
	reporter := s.processDiagnostics("init")
	if reporter.emit(diagnosticOutputRead, 0) || len(sink.queue) != 0 {
		t.Fatal("invalid context reached sink")
	}
}

type diagnosticFailWriter struct {
	calls atomic.Int32
	short bool
}

func (w *diagnosticFailWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("private-writer-credential")
}
func TestProcessDiagnosticsDisableOnWriteErrorOrShortWrite(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			w := &diagnosticFailWriter{short: short}
			sink := newProcessDiagnosticSink(w)
			defer sink.close()
			if !sink.tryRecord(diagnosticTestRecord()) {
				t.Fatal("initial enqueue failed")
			}
			select {
			case <-sink.done:
			case <-time.After(time.Second):
				t.Fatal("sink did not stop on writer failure")
			}
			if !sink.disabled.Load() || sink.tryRecord(diagnosticTestRecord()) || w.calls.Load() != 1 {
				t.Fatal("failed writer retried")
			}
		})
	}
}

const diagnosticSyntheticSecret = "SYNTHETIC_CHILD_ERROR_MUST_NOT_REACH_HOST_LOG"

type diagnosticFaultAgent struct {
	recovered atomic.Bool
	reads     atomic.Int32
}

func (f *diagnosticFaultAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *diagnosticFaultAgent) CallContext(_ context.Context, method string, request, response any) error {
	switch method {
	case "ReadProcessOutput":
		f.reads.Add(1)
		if !f.recovered.Load() {
			return errors.New(diagnosticSyntheticSecret + " /private/guest/path")
		}
		return setJSONResponse(response, processOutput{Status: "STOPPED"})
	case "WaitProcess":
		return setStoppedResponse(request, response, 41, 37)
	}
	return errors.New("unexpected diagnostic fixture method")
}
func (*diagnosticFaultAgent) Close() error                                   { return nil }
func (*diagnosticFaultAgent) Reconnect(string) error                         { return nil }
func (*diagnosticFaultAgent) ReconnectContext(context.Context, string) error { return nil }

// A subprocess isolates real stderr redirection from other tests. In blocked
// mode the parent fills a real OS pipe and never drains it while the child
// retries the guest and commits the exact exit. No kernel/daemon is needed.
func TestProcessDiagnosticsRealStderrRedactionAndBlockedPipeProgress(t *testing.T) {
	if mode := os.Getenv("MK_G8_DIAGNOSTIC_CHILD"); mode != "" {
		bundle := privateTestDirectory(t)
		if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
			t.Fatal(err)
		}
		a := &diagnosticFaultAgent{}
		p := &process{pid: 41, status: tasktypes.Status_RUNNING, done: make(chan struct{})}
		s := &service{id: "task", namespace: "tests", bundle: bundle, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}, agent: a, processes: map[string]*process{"": p}, publisher: &fakePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
		go s.waitProcess("init", "", p)
		deadline := time.Now().Add(2 * time.Second)
		for a.reads.Load() < 4 {
			if time.Now().After(deadline) {
				t.Fatal("diagnostic writer blocked repeated guest observations")
			}
			time.Sleep(5 * time.Millisecond)
		}
		select {
		case <-p.done:
			t.Fatal("failure fabricated exit")
		default:
		}
		a.recovered.Store(true)
		select {
		case <-p.done:
		case <-time.After(time.Second):
			t.Fatal("diagnostic pressure blocked exact completion")
		}
		s.mu.Lock()
		status, exit, queued := p.status, p.exit, p.exitEventQueued
		s.mu.Unlock()
		if status != tasktypes.Status_STOPPED || exit != 37 || !queued {
			t.Fatal("wrong exit after diagnostic pressure")
		}
		if mode == "blocked" {
			sink := processDiagnosticDefault()
			if sink.disabled.Load() {
				t.Fatal("pipe write was not blocked")
			}
			select {
			case <-sink.done:
				t.Fatal("blocked writer exited")
			default:
			}
		}
		t.Log("PROCESS_MONITOR_RETRIED_PRIVATE_ERRORS_AND_COMMITTED_EXACT_EXIT_WITH_BOUNDED_DIAGNOSTICS")
		return
	}
	for _, mode := range []string{"capture", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessDiagnosticsRealStderrRedactionAndBlockedPipeProgress$", "-test.v")
			cmd.Env = append(os.Environ(), "MK_G8_DIAGNOSTIC_CHILD="+mode)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			if mode == "blocked" {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				pipeFD := int(writer.Fd())
				if err := unix.SetNonblock(pipeFD, true); err != nil {
					t.Fatal(err)
				}
				padding := bytes.Repeat([]byte("x"), 4096)
				for {
					_, err := unix.Write(pipeFD, padding)
					if errors.Is(err, unix.EAGAIN) {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				if err := unix.SetNonblock(pipeFD, false); err != nil {
					t.Fatal(err)
				}
				cmd.Stderr = writer
			} else {
				cmd.Stderr = &stderr
			}
			if err := cmd.Run(); err != nil {
				t.Fatal("isolated diagnostic fixture failed", err)
			}
			if !strings.Contains(stdout.String(), "PROCESS_MONITOR_RETRIED_PRIVATE_ERRORS_AND_COMMITTED_EXACT_EXIT_WITH_BOUNDED_DIAGNOSTICS") {
				t.Fatal("missing exact-exit proof")
			}
			if mode == "capture" {
				raw := stderr.Bytes()
				if bytes.Contains(raw, []byte(diagnosticSyntheticSecret)) || bytes.Contains(raw, []byte("/private/guest/path")) {
					t.Fatal("private guest error leaked to host stderr")
				}
				lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
				if len(lines) != 1 {
					t.Fatal("retry diagnostic volume was not bounded", len(lines))
				}
				var record processDiagnosticRecord
				if json.Unmarshal(lines[0], &record) != nil || !validProcessDiagnosticRecord(record) || record.Code != "OUTPUT_READ_FAILED" {
					t.Fatal("invalid real stderr event")
				}
			}
		})
	}
}
