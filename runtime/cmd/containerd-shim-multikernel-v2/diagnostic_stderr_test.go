//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

// Fake writers cannot exercise Go's special SIGPIPE policy for descriptors 1/2.
// Each child uses the actual process-wide sink and an inherited real stderr.
func TestDiagnosticStderrDefaultSurvivesReaderLoss(t *testing.T) {
	if mode := os.Getenv("MK_G8_DIAGNOSTIC_STDERR_CHILD"); mode != "" {
		fields := strings.Split(mode, ":")
		kind, transport := fields[0], fields[1]
		if transport == "closed-fd" {
			if err := unix.Close(2); err != nil {
				t.Fatal("close inherited stderr")
			}
		}
		flags, flagErr := unix.FcntlInt(2, unix.F_GETFL, 0)
		beforeFDs := diagnosticOpenFDCount(t)
		sink := processDiagnosticDefault()
		accepted := false
		switch kind {
		case "process", "network":
			r := newProcessDiagnosticReporter("box", strings.Repeat("a", 32), "init", nil)
			code := diagnosticProcessWait
			if kind == "network" {
				code = diagnosticNetworkWrite
			}
			accepted = r.emit(code, 0)
		case "event":
			s := &service{}
			s.setEventDiagnosticOwner(protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)})
			accepted = s.reportEventDiagnostic(diagnosticEventPublicationDeferred)
		case "supervisor":
			accepted = newSupervisorDiagnostics(nil).emit(supervisorProxyFailed, 0, 0, 0)
		}
		if !accepted {
			t.Fatal("first typed diagnostic refused")
		}
		deadline := time.After(2 * time.Second)
		if transport == "closed-reader" || transport == "closed-fd" {
			select {
			case <-sink.done:
			case <-deadline:
				t.Fatal("failed stderr writer did not stop")
			}
			if diagnosticOpenFDCount(t) != beforeFDs {
				t.Fatal("failed writer leaked private descriptor")
			}
			if !sink.disabled.Load() || sink.tryRecord(diagnosticTestRecord()) {
				t.Fatal("failed sink accepted another record")
			}
		} else {
			for len(sink.queue) != 0 {
				select {
				case <-deadline:
					t.Fatal("writer did not receive initial record")
				default:
					time.Sleep(time.Millisecond)
				}
			}
			if transport == "full" {
				if sink.disabled.Load() {
					t.Fatal("full pipe disabled writer")
				}
				select {
				case <-sink.done:
					t.Fatal("full pipe writer exited")
				default:
				}
				for diagnosticOpenFDCount(t) != beforeFDs+1 {
					select {
					case <-deadline:
						t.Fatal("blocked writer did not own exactly one private descriptor")
					default:
						time.Sleep(time.Millisecond)
					}
				}
				requireDiagnosticDuplicateCLOEXEC(t)
				count := 0
				for i := 0; i < 10000; i++ {
					if sink.tryRecord(diagnosticTestRecord()) {
						count++
					}
				}
				if diagnosticOpenFDCount(t) != beforeFDs+1 {
					t.Fatal("pressure created additional descriptors")
				}
				if count != processDiagnosticQueueSize || len(sink.queue) != processDiagnosticQueueSize || sink.dropped.Load() != uint64(10000-processDiagnosticQueueSize) {
					t.Fatal("full pipe queue or drop accounting is unbounded")
				}
			}
			sink.close()
			if transport == "capture" {
				select {
				case <-sink.done:
				case <-deadline:
					t.Fatal("capture writer did not join")
				}
			}
		}
		if flagErr == nil {
			after, err := unix.FcntlInt(2, unix.F_GETFL, 0)
			if err != nil || flags != after {
				t.Fatal("inherited stderr flags changed")
			}
		}
		// A full pipe can retain one writer until exit; lifecycle never joins it.
		sink.close()
		if sink.tryRecord(diagnosticTestRecord()) {
			t.Fatal("closed sink accepted record")
		}
		if kind == "event" {
			diagnosticEventJournalSurvivesDisabledSink(t)
		}
		if _, err := os.Stdout.WriteString("DIAGNOSTIC_STDERR_LIFECYCLE_PROGRESS_PASS\n"); err != nil {
			os.Exit(24)
		}
		os.Exit(23)
	}
	for _, kind := range []string{"process", "network", "event", "supervisor"} {
		for _, transport := range []string{"capture", "closed-reader", "closed-fd", "full"} {
			t.Run(kind+"/"+transport, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDiagnosticStderrDefaultSurvivesReaderLoss$")
				cmd.Env = append(os.Environ(), "MK_G8_DIAGNOSTIC_STDERR_CHILD="+kind+":"+transport)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				if transport == "capture" || transport == "closed-fd" {
					cmd.Stderr = &stderr
				} else {
					reader, writer, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					defer writer.Close()
					if transport == "closed-reader" {
						reader.Close()
					} else {
						defer reader.Close()
						fd := int(writer.Fd())
						if err = unix.SetNonblock(fd, true); err != nil {
							t.Fatal(err)
						}
						filler := bytes.Repeat([]byte("x"), 4096)
						for {
							_, err = unix.Write(fd, filler)
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
					}
					cmd.Stderr = writer
				}
				err := cmd.Run()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 23 || ctx.Err() != nil {
					t.Fatalf("diagnostic child did not retain deliberate exit: %v", err)
				}
				if stdout.String() != "DIAGNOSTIC_STDERR_LIFECYCLE_PROGRESS_PASS\n" {
					t.Fatal("missing independent lifecycle progress marker")
				}
				if transport == "capture" {
					var record map[string]any
					if json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &record) != nil || record["component"] != "containerd-shim-multikernel-v2" {
						t.Fatal("expected exactly one structured diagnostic")
					}
					if kind == "supervisor" {
						if record["code"] != "SUPERVISOR_PROXY_FAILED" {
							t.Fatal("wrong supervisor code")
						}
					} else {
						expected := map[string]string{"process": "PROCESS_WAIT_FAILED", "network": "NETWORK_DEVICE_WRITE_FAILED", "event": "EVENT_PUBLICATION_DEFERRED"}
						if record["code"] != expected[kind] || record["sandbox_id"] != "box" || record["generation"] != strings.Repeat("a", 32) {
							t.Fatal("wrong typed diagnostic metadata")
						}
					}
				} else if stderr.Len() != 0 {
					t.Fatal("failed stderr produced unexpected captured output")
				}
			})
		}
	}
}

func diagnosticOpenFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("inspect fixture descriptors")
	}
	return len(entries)
}

func requireDiagnosticDuplicateCLOEXEC(t *testing.T) {
	t.Helper()
	var original unix.Stat_t
	if unix.Fstat(2, &original) != nil {
		t.Fatal("inspect inherited stderr identity")
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal("inspect duplicate descriptors")
	}
	found := 0
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil || fd < 3 {
			continue
		}
		var identity unix.Stat_t
		if unix.Fstat(fd, &identity) != nil || identity.Dev != original.Dev || identity.Ino != original.Ino {
			continue
		}
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("diagnostic duplicate can escape through exec")
		}
		found++
	}
	if found != 1 {
		t.Fatal("expected exactly one private stderr duplicate")
	}
}

// Reader loss must not change durable event ordering or acknowledgement. The
// broker emits a private synthetic error, while the default diagnostic sink is
// disabled after the real transport test above. Recovery reads the actual file.
func diagnosticEventJournalSurvivesDisabledSink(t *testing.T) {
	t.Helper()
	bundle := privateTestDirectory(t)
	s := &service{bundle: bundle, namespace: "tests", publisher: &privateFailurePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	s.setEventDiagnosticOwner(protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)})
	s.mu.Lock()
	for i := 0; i < 8; i++ {
		if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: uint32(31 + i)}); err != nil {
			s.mu.Unlock()
			t.Fatal("durable publication failed")
		}
	}
	s.mu.Unlock()
	if len(s.events.Pending) != 8 || s.events.NextSequence != 9 {
		t.Fatal("failed broker changed event order")
	}
	publisher := &eventPayloadPublisher{}
	recovered := &service{bundle: bundle, namespace: "tests", publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if recovered.loadEventJournal() != nil || len(recovered.events.Pending) != 8 || recovered.events.NextSequence != 9 {
		t.Fatal("event reconstruction failed")
	}
	if recovered.flushEvents(context.Background()) != nil || len(recovered.events.Pending) != 0 || len(publisher.pids) != 8 {
		t.Fatal("event acknowledgement failed")
	}
	for i, pid := range publisher.pids {
		if pid != uint32(31+i) || publisher.topics[i] != ctruntime.TaskStartEventTopic {
			t.Fatal("replayed event order changed")
		}
	}
	if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("acknowledged event file remains")
	}
}
