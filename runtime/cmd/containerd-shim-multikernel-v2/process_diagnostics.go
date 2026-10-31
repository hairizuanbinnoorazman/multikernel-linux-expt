//go:build linux

package main

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const processDiagnosticQueueSize = 64
const processDiagnosticRecordLimit = 1024

type processDiagnosticCode uint8

const (
	diagnosticStdinReplay processDiagnosticCode = iota + 1
	diagnosticStdinIntent
	diagnosticStdinDelivery
	diagnosticStdinRead
	diagnosticStdinDrain
	diagnosticStdinClose
	diagnosticOutputRead
	diagnosticOutputAck
	diagnosticStdoutPressure
	diagnosticStderrPressure
	diagnosticStdoutTruncated
	diagnosticStderrTruncated
	diagnosticProcessWait
	diagnosticProcessWaitInvalid
	diagnosticExitPersist
	diagnosticSignalAck
	diagnosticSignalAckPersist
	diagnosticExitEventQueue
	diagnosticExitEventAck
	diagnosticNetworkRead
	diagnosticNetworkWrite
	diagnosticNetworkReport
	diagnosticGuestTeardownWarnings
	diagnosticEventPublicationDeferred
	diagnosticEventReplayDeferred
)

var processDiagnosticSpecs = [...]struct{ code, operation string }{
	{},
	{"STDIN_REPLAY_FAILED", "replay-stdin"},
	{"STDIN_INTENT_PERSIST_FAILED", "persist-stdin-intent"},
	{"STDIN_DELIVERY_FAILED", "deliver-stdin"},
	{"STDIN_READ_FAILED", "read-stdin"},
	{"STDIN_DRAIN_LOST", "drain-stdin"},
	{"STDIN_CLOSE_FAILED", "close-stdin"},
	{"OUTPUT_READ_FAILED", "read-process-output"},
	{"OUTPUT_ACK_PERSIST_FAILED", "persist-output-ack"},
	{"STDOUT_PRESSURE_DROPPED", "deliver-stdout"},
	{"STDERR_PRESSURE_DROPPED", "deliver-stderr"},
	{"STDOUT_RETENTION_TRUNCATED", "read-process-output"},
	{"STDERR_RETENTION_TRUNCATED", "read-process-output"},
	{"PROCESS_WAIT_FAILED", "wait-process"},
	{"PROCESS_WAIT_INVALID", "validate-process-wait"},
	{"PROCESS_EXIT_PERSIST_FAILED", "persist-process-exit"},
	{"SIGNAL_ACK_FAILED", "acknowledge-process-signal"},
	{"SIGNAL_ACK_PERSIST_FAILED", "persist-signal-ack"},
	{"EXIT_EVENT_QUEUE_FAILED", "queue-exit-event"},
	{"EXIT_EVENT_ACK_PERSIST_FAILED", "persist-exit-event-ack"},
	{"NETWORK_DEVICE_READ_FAILED", "read-network-device"},
	{"NETWORK_DEVICE_WRITE_FAILED", "write-network-device"},
	{"NETWORK_REPORT_FAILED", "report-network-state"},
	{"GUEST_TEARDOWN_WARNINGS", "teardown-guest"},
	{"EVENT_PUBLICATION_DEFERRED", "publish-task-event"},
	{"EVENT_REPLAY_DEFERRED", "replay-task-events"},
}

type processDiagnosticRecord struct {
	Version    int    `json:"version"`
	Component  string `json:"component"`
	Operation  string `json:"operation"`
	Code       string `json:"code"`
	SandboxID  string `json:"sandbox_id"`
	Generation string `json:"generation"`
	ProcessID  string `json:"process_id"`
	At         string `json:"at"`
	Bytes      uint64 `json:"bytes,omitempty"`
}

// One worker and a fixed queue isolate process monitors from a blocked writer.
// This is lossy diagnostic delivery, never a durable task/event journal. A short
// write or writer error disables the sink instead of retrying or appending later
// records to a possibly incomplete JSON line. Close never waits on writer I/O.
type processDiagnosticSink struct {
	queue     chan []byte
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	disabled  atomic.Bool
	dropped   atomic.Uint64
}

func newProcessDiagnosticSink(writer io.Writer) *processDiagnosticSink {
	s := &processDiagnosticSink{queue: make(chan []byte, processDiagnosticQueueSize), stop: make(chan struct{}), done: make(chan struct{})}
	if writer == nil {
		s.disabled.Store(true)
		close(s.done)
		return s
	}
	go func() {
		defer close(s.done)
		for {
			select {
			case <-s.stop:
				return
			case data := <-s.queue:
				n, err := writer.Write(data)
				if err != nil || n != len(data) {
					s.disabled.Store(true)
					return
				}
			}
		}
	}()
	return s
}
func (s *processDiagnosticSink) close() {
	s.disabled.Store(true)
	s.closeOnce.Do(func() { close(s.stop) })
}
func (s *processDiagnosticSink) tryRecord(record processDiagnosticRecord) bool {
	if s == nil {
		return false
	}
	if s.disabled.Load() || !validProcessDiagnosticRecord(record) {
		s.dropped.Add(1)
		return false
	}
	data, err := json.Marshal(record)
	if err != nil {
		s.dropped.Add(1)
		return false
	}
	return s.tryEncodedDiagnostic(data)
}

// Only strict, typed diagnostic producers call this shared bounded transport.
// It is not an input interface for guest data or pre-serialized RPC payloads.
func (s *processDiagnosticSink) tryEncodedDiagnostic(data []byte) bool {
	if s == nil {
		return false
	}
	if s.disabled.Load() || len(data)+1 > processDiagnosticRecordLimit {
		s.dropped.Add(1)
		return false
	}
	data = append(data, '\n')
	select {
	case <-s.stop:
		s.dropped.Add(1)
		return false
	case s.queue <- data:
		return true
	default:
		s.dropped.Add(1)
		return false
	}
}

var defaultProcessDiagnosticOnce sync.Once
var defaultProcessDiagnosticSink *processDiagnosticSink

func processDiagnosticDefault() *processDiagnosticSink {
	defaultProcessDiagnosticOnce.Do(func() { defaultProcessDiagnosticSink = newProcessDiagnosticSink(os.Stderr) })
	return defaultProcessDiagnosticSink
}

func diagnosticIdentifier(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	first := value[0]
	if !(first >= 'a' && first <= 'z' || first >= 'A' && first <= 'Z' || first >= '0' && first <= '9') {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func validProcessDiagnosticRecord(record processDiagnosticRecord) bool {
	if record.Version != 1 || record.Component != "containerd-shim-multikernel-v2" ||
		!diagnosticIdentifier(record.SandboxID) || !diagnosticGeneration(record.Generation) || !diagnosticIdentifier(record.ProcessID) ||
		len(record.At) == 0 || len(record.At) > 35 {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, record.At)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != record.At {
		return false
	}
	for _, spec := range processDiagnosticSpecs[1:] {
		if record.Code == spec.code && record.Operation == spec.operation {
			return true
		}
	}
	return false
}
func diagnosticGeneration(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

type processDiagnosticReporter struct {
	sink                         *processDiagnosticSink
	sandbox, generation, process string
	attempted                    uint64
}

func (s *service) processDiagnostics(agentID string) processDiagnosticReporter {
	s.mu.Lock()
	sandbox, generation, sink := s.sandbox.ID, s.sandbox.Generation, s.processDiagnosticSink
	s.mu.Unlock()
	return newProcessDiagnosticReporter(sandbox, generation, agentID, sink)
}

func newProcessDiagnosticReporter(sandbox, generation, agentID string, sink *processDiagnosticSink) processDiagnosticReporter {
	if !diagnosticIdentifier(sandbox) || !diagnosticGeneration(generation) || !diagnosticIdentifier(agentID) {
		return processDiagnosticReporter{}
	}
	if sink == nil {
		sink = processDiagnosticDefault()
	}
	return processDiagnosticReporter{sink: sink, sandbox: sandbox, generation: generation, process: agentID}
}

// A code gets one enqueue attempt per monitor invocation, including when the
// sink is full/disabled. No error or output payload can be passed to this API.
func (r *processDiagnosticReporter) emit(code processDiagnosticCode, bytes uint64) bool {
	if code == 0 || int(code) >= len(processDiagnosticSpecs) || code >= 64 {
		return false
	}
	bit := uint64(1) << code
	if r.attempted&bit != 0 {
		return false
	}
	r.attempted |= bit
	if r.sink == nil {
		return false
	}
	spec := processDiagnosticSpecs[code]
	return r.sink.tryRecord(processDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2", Operation: spec.operation, Code: spec.code, SandboxID: r.sandbox, Generation: r.generation, ProcessID: r.process, At: time.Now().UTC().Format(time.RFC3339Nano), Bytes: bytes})
}
