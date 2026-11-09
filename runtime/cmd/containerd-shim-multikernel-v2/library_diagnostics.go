//go:build linux

package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

type libraryDiagnosticCode uint8

const (
	libraryEvent libraryDiagnosticCode = iota + 1
	libraryRPCReceive
	libraryRPCSend
	libraryRPCInactiveStream
	libraryRPCMessage
	libraryRPCPeer
	libraryRPCConnection
	libraryRPCServer
	libraryReaper
	libraryListener
	librarySignalLoop
	libraryExitSignal
	libraryStackDump
	libraryCodeLimit
)

var libraryDiagnosticSpecs = [...]struct{ code, operation string }{
	{},
	{"SHIM_LIBRARY_EVENT", "observe-library-event"},
	{"SHIM_RPC_RECEIVE_FAILED", "receive-rpc"},
	{"SHIM_RPC_SEND_FAILED", "send-rpc"},
	{"SHIM_RPC_INACTIVE_STREAM", "receive-rpc"},
	{"SHIM_RPC_MESSAGE_FAILED", "handle-rpc"},
	{"SHIM_RPC_PEER_REFUSED", "authenticate-rpc"},
	{"SHIM_RPC_CONNECTION_FAILED", "open-rpc"},
	{"SHIM_RPC_SERVER_FAILED", "serve-rpc"},
	{"SHIM_REAPER_FAILED", "reap-process"},
	{"SHIM_LISTENER_STARTED", "serve-rpc"},
	{"SHIM_SIGNAL_LOOP_STARTED", "observe-signal"},
	{"SHIM_EXIT_SIGNAL", "observe-signal"},
	{"SHIM_STACK_DUMP_REDACTED", "redact-stack-dump"},
}

var libraryDiagnosticSeverities = [...]string{"panic", "fatal", "error", "warning", "info", "debug", "trace"}

func libraryCode(message string) libraryDiagnosticCode {
	switch message {
	case "error receiving message":
		return libraryRPCReceive
	case "failed sending message on channel":
		return libraryRPCSend
	case "ttrpc: received message on inactive stream":
		return libraryRPCInactiveStream
	case "ttrpc: failed to handle message":
		return libraryRPCMessage
	case "ttrpc: refusing connection after handshake":
		return libraryRPCPeer
	case "ttrpc: create connection failed":
		return libraryRPCConnection
	case "containerd-shim: ttrpc server failure":
		return libraryRPCServer
	case "reap exit status":
		return libraryReaper
	case "serving api on socket":
		return libraryListener
	case "starting signal loop":
		return librarySignalLoop
	case "Caught exit signal":
		return libraryExitSignal
	default:
		if strings.HasPrefix(message, "=== BEGIN goroutine stack dump ===\n") && strings.HasSuffix(message, "\n=== END goroutine stack dump ===") {
			return libraryStackDump
		}
		return libraryEvent
	}
}

type libraryDiagnosticRecord struct {
	Version    int    `json:"version"`
	Component  string `json:"component"`
	Scope      string `json:"scope"`
	Code       string `json:"code"`
	Operation  string `json:"operation"`
	Severity   string `json:"severity"`
	PID        uint32 `json:"pid"`
	OwnerKnown bool   `json:"owner_known"`
	At         string `json:"at"`
}

func validLibraryDiagnosticRecord(r libraryDiagnosticRecord) bool {
	if r.Version != 1 || r.Component != "containerd-shim-multikernel-v2" || r.Scope != "shim-library" || r.OwnerKnown || r.PID == 0 || r.PID >= 1<<31 || len(r.At) == 0 || len(r.At) > 35 {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, r.At)
	if err != nil || at.UTC().Format(time.RFC3339Nano) != r.At {
		return false
	}
	validSeverity := false
	for _, severity := range libraryDiagnosticSeverities {
		validSeverity = validSeverity || severity == r.Severity
	}
	if !validSeverity {
		return false
	}
	for _, spec := range libraryDiagnosticSpecs[1:] {
		if r.Code == spec.code && r.Operation == spec.operation {
			return true
		}
	}
	return false
}

// Every admitted record has a strict fixed schema before it reaches the shared
// bounded sink. Neither raw messages nor entry fields/cause methods are used.
func (s *processDiagnosticSink) tryLibraryRecord(r libraryDiagnosticRecord) bool {
	if s == nil {
		return false
	}
	if !validLibraryDiagnosticRecord(r) {
		s.dropped.Add(1)
		return false
	}
	data, err := json.Marshal(r)
	if err != nil {
		s.dropped.Add(1)
		return false
	}
	return s.tryEncodedDiagnostic(data)
}

// The formatter never sees a library message or fields, including logrus's
// unexported field-formatting error. Whole-entry replacement clears that state.
// A private discard logger also prevents later upstream output/formatter changes
// from restoring synchronous writes. The original caller keeps its logger, level
// selection and Fatal exit function; logrus's panic still receives a safe entry.
type libraryDiscardFormatter struct{}

func (libraryDiscardFormatter) Format(*logrus.Entry) ([]byte, error) { return nil, nil }

type libraryLogBoundary struct {
	sink      *processDiagnosticSink
	discard   *logrus.Logger
	pid       uint32
	attempted [libraryCodeLimit]atomic.Uint32
}

func newLibraryLogBoundary(sink *processDiagnosticSink) *libraryLogBoundary {
	if sink == nil {
		sink = processDiagnosticDefault()
	}
	discard := logrus.New()
	discard.SetOutput(io.Discard)
	discard.SetFormatter(libraryDiscardFormatter{})
	return &libraryLogBoundary{sink: sink, discard: discard, pid: uint32(os.Getpid())}
}

func (*libraryLogBoundary) Levels() []logrus.Level { return logrus.AllLevels }

func (b *libraryLogBoundary) Fire(entry *logrus.Entry) error {
	code, level := libraryCode(entry.Message), entry.Level
	*entry = logrus.Entry{Logger: b.discard, Level: level, Message: "shim library diagnostic"}
	if level > logrus.TraceLevel {
		return nil
	}
	bit := uint32(1) << uint32(level)
	for {
		previous := b.attempted[code].Load()
		if previous&bit != 0 {
			return nil
		}
		if b.attempted[code].CompareAndSwap(previous, previous|bit) {
			break
		}
	}
	spec := libraryDiagnosticSpecs[code]
	b.sink.tryLibraryRecord(libraryDiagnosticRecord{Version: 1, Component: "containerd-shim-multikernel-v2",
		Scope: "shim-library", Code: spec.code, Operation: spec.operation,
		Severity: libraryDiagnosticSeverities[level], PID: b.pid, OwnerKnown: false,
		At: time.Now().UTC().Format(time.RFC3339Nano)})
	return nil
}
