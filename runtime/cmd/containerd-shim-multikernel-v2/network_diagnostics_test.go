//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const networkDiagnosticSecret = "SYNTHETIC_GUEST_NETWORK_TOKEN /private/network/credential"

type diagnosticReportFailureClient struct {
	fakeNetworkClient
	calls atomic.Int32
}

func (c *diagnosticReportFailureClient) Call(_ context.Context, _ mknetwork.Request) (mknetwork.Response, error) {
	c.calls.Add(1)
	return mknetwork.Response{}, errors.New(networkDiagnosticSecret)
}

func diagnosticSinkForTest(t *testing.T, blocked bool) (*processDiagnosticSink, chan []byte) {
	t.Helper()
	records := make(chan []byte, 64)
	if blocked {
		writer := &diagnosticBlockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
		sink := newProcessDiagnosticSink(writer)
		t.Cleanup(func() {
			sink.close()
			close(writer.release)
			select {
			case <-sink.done:
			case <-time.After(time.Second):
				t.Error("writer failed to stop")
			}
		})
		// Pre-fill one record and confirm the writer is actually blocked before
		// exercising lifecycle work, rather than merely using an idle test writer.
		if !sink.tryRecord(diagnosticTestRecord()) {
			t.Fatal("initial frame rejected")
		}
		select {
		case <-writer.entered:
		case <-time.After(time.Second):
			t.Fatal("writer did not block")
		}
		return sink, records
	}
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	t.Cleanup(sink.close)
	return sink, records
}
func requireNetworkDiagnostic(t *testing.T, records chan []byte, code string) {
	t.Helper()
	select {
	case raw := <-records:
		var r processDiagnosticRecord
		if bytes.Contains(raw, []byte(networkDiagnosticSecret)) || json.Unmarshal(raw, &r) != nil || !validProcessDiagnosticRecord(r) || r.Code != code || r.ProcessID != "init" || r.SandboxID != "box" {
			t.Fatal("private error escaped or invalid diagnostic")
		}
	case <-time.After(time.Second):
		t.Fatal("missing diagnostic")
	}
	select {
	case <-records:
		t.Fatal("duplicate diagnostic")
	default:
	}
}
func TestNetworkDiagnosticsRepeatedPrivateReportErrorsDoNotBlockJoin(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "capture"
		if blocked {
			name = "blocked"
		}
		t.Run(name, func(t *testing.T) {
			sink, records := diagnosticSinkForTest(t, blocked)
			client := &diagnosticReportFailureClient{}
			s := &service{netDone: make(chan struct{}), netReports: make(chan string, 1), netClient: client,
				netEndpoint: validShimEndpoint("task", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/test"), processDiagnosticSink: sink}
			// The service lock stays held through start and join, as it can during
			// lifecycle cleanup. A network goroutine must never wait for that lock.
			s.mu.Lock()
			defer s.mu.Unlock()
			s.startNetworkReporter()
			s.queueNetworkReport("DISCONNECTED")
			waitNetworkCondition(t, func() bool { return client.calls.Load() >= 4 }, "private REPORT retries")
			close(s.netDone)
			joined := make(chan struct{})
			go func() { s.netWG.Wait(); close(joined) }()
			select {
			case <-joined:
			case <-time.After(time.Second):
				t.Fatal("diagnostics blocked reporter join")
			}
			if blocked {
				if len(sink.queue) != 1 {
					t.Fatal("retry diagnostics were repeated", len(sink.queue))
				}
			} else {
				requireNetworkDiagnostic(t, records, "NETWORK_REPORT_FAILED")
			}
		})
	}
}
func TestNetworkDiagnosticsInvalidEndpointContextIsSuppressed(t *testing.T) {
	sink := newProcessDiagnosticSink(nil)
	defer sink.close()
	s := &service{netEndpoint: validShimEndpoint("task", "bad\ncontext", strings.Repeat("a", 32), strings.Repeat("b", 32), "/private/guest/path"), processDiagnosticSink: sink}
	r := s.networkDiagnostics()
	if r.sink != nil || r.emit(diagnosticNetworkReport, 0) {
		t.Fatal("invalid context reached sink")
	}
}
func TestTeardownDiagnosticsRedactPrivateWarningAndPreserveExactDelete(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "capture"
		if blocked {
			name = "blocked"
		}
		t.Run(name, func(t *testing.T) {
			sink, records := diagnosticSinkForTest(t, blocked)
			sandbox := protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}
			fake := &fakeAgentClient{fail: map[string]error{"CloseNetwork": errors.New(networkDiagnosticSecret)}}
			p := &process{pid: 17, status: tasktypes.Status_STOPPED, exit: 37, exited: time.Unix(123, 0).UTC(), exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{})}
			close(p.done)
			var calls []string
			s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), agent: fake, sandbox: sandbox,
				processes: map[string]*process{"": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}, processDiagnosticSink: sink,
				daemon: daemonCallFunc(func(_ context.Context, r protocol.Request, _ any) *protocol.Error {
					calls = append(calls, r.Method)
					return nil
				})}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			response, err := s.Delete(ctx, &taskapi.DeleteRequest{ID: "task"})
			if err != nil || response.ExitStatus != 37 || s.processes[""] != nil || strings.Join(calls, ",") != "StopSandbox,DeleteSandbox" {
				t.Fatal("diagnostics changed exact host cleanup", err)
			}
			if blocked {
				if len(sink.queue) != 1 {
					t.Fatal("wrong teardown diagnostic count")
				}
			} else {
				requireNetworkDiagnostic(t, records, "GUEST_TEARDOWN_WARNINGS")
			}
		})
	}
}

func TestNetworkDiagnosticsDeviceErrorsDoNotBlockPumpJoin(t *testing.T) {
	for _, writeFailure := range []bool{false, true} {
		name := "read"
		code := "NETWORK_DEVICE_READ_FAILED"
		if writeFailure {
			name = "write"
			code = "NETWORK_DEVICE_WRITE_FAILED"
		}
		t.Run(name, func(t *testing.T) {
			for _, blocked := range []bool{false, true} {
				name := "capture"
				if blocked {
					name = "blocked"
				}
				t.Run(name, func(t *testing.T) {
					sink, records := diagnosticSinkForTest(t, blocked)
					device, err := os.Open("/dev/null")
					if err != nil {
						t.Fatal(err)
					}
					defer device.Close()
					if !writeFailure {
						if err := device.Close(); err != nil {
							t.Fatal(err)
						}
					}
					endpoint := validShimEndpoint("task", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/test")
					// Isolate the local device fault; no backend endpoint has been provisioned.
					endpoint.Generation = ""
					agent := &networkPumpAgent{handler: func(int, []byte) ([]byte, error) { return []byte{1}, nil }}
					s := &service{agent: agent, netDevice: device, netEndpoint: endpoint, processDiagnosticSink: sink}
					s.mu.Lock()
					defer s.mu.Unlock()
					s.startNetworkPump()
					waitNetworkCondition(t, func() bool { return s.netErrors.Load() == 1 }, "fatal device fault")
					close(s.netDone)
					joined := make(chan struct{})
					go func() { s.netWG.Wait(); close(joined) }()
					select {
					case <-joined:
					case <-time.After(time.Second):
						t.Fatal("diagnostics blocked network pump join")
					}
					if s.netTXPackets.Load() != 0 {
						t.Fatal("failed device write fabricated packet delivery")
					}
					if blocked {
						if len(sink.queue) != 1 {
							t.Fatal("wrong device diagnostic count")
						}
					} else {
						requireNetworkDiagnostic(t, records, code)
					}
				})
			}
		})
	}
}
