//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	containerdlog "github.com/containerd/log"
	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/sirupsen/logrus"
)

type libraryForbiddenValue struct{}

func (libraryForbiddenValue) Error() string  { panic("private error formatter must not run") }
func (libraryForbiddenValue) String() string { panic("private field formatter must not run") }

type libraryForbiddenFormatter struct{}

func (libraryForbiddenFormatter) Format(*logrus.Entry) ([]byte, error) {
	panic("upstream formatter must not receive a library entry")
}

func libraryTestLogger(t *testing.T, sink *processDiagnosticSink) (*logrus.Logger, *libraryLogBoundary) {
	t.Helper()
	logger := logrus.New()
	logger.SetLevel(logrus.TraceLevel)
	logger.SetReportCaller(true)
	logger.SetOutput(io.Discard)
	logger.SetFormatter(libraryForbiddenFormatter{})
	boundary := newLibraryLogBoundary(sink)
	logger.AddHook(boundary)
	return logger, boundary
}

func libraryReadRecord(t *testing.T, records <-chan []byte) libraryDiagnosticRecord {
	t.Helper()
	select {
	case raw := <-records:
		var record libraryDiagnosticRecord
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if len(raw) > processDiagnosticRecordLimit || bytes.Contains(raw, []byte("SYNTHETIC_PRIVATE")) || decoder.Decode(&record) != nil || !validLibraryDiagnosticRecord(record) || record.PID != uint32(os.Getpid()) {
			t.Fatal("invalid fixed library diagnostic")
		}
		var fields map[string]any
		if json.Unmarshal(raw, &fields) != nil || len(fields) != 9 {
			t.Fatal("unexpected library diagnostic fields")
		}
		return record
	case <-time.After(2 * time.Second):
		t.Fatal("fixed library diagnostic missing")
		return libraryDiagnosticRecord{}
	}
}

func TestLibraryDiagnosticsRemoveFieldsMessagesHiddenErrorsAndCaller(t *testing.T) {
	records := make(chan []byte, 16)
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	defer sink.close()
	logger, _ := libraryTestLogger(t, sink)
	ctx := context.WithValue(t.Context(), struct{}{}, "SYNTHETIC_PRIVATE_CONTEXT")
	parent := logrus.NewEntry(logger).WithContext(ctx).WithError(libraryForbiddenValue{}).
		WithField("SYNTHETIC_PRIVATE_FUNCTION", func() {}).WithFields(logrus.Fields{
		"path": "/SYNTHETIC_PRIVATE/socket", "nested": map[string]any{"token": libraryForbiddenValue{}},
		"code": "SYNTHETIC_PRIVATE_FORGED_CODE", "owner_known": true,
	})
	// Logrus carries invalid-field keys in an unexported error string as well as
	// Data. Replacing only Data/Message would leave that string available.
	parent.Error(strings.Repeat("SYNTHETIC_PRIVATE_MESSAGE", 10000))
	if record := libraryReadRecord(t, records); record.Code != "SHIM_LIBRARY_EVENT" || record.Severity != "error" || record.OwnerKnown {
		t.Fatal("unknown library text was not a fixed generic event")
	}
	if parent.Context != ctx || parent.Logger != logger || len(parent.Data) != 5 {
		t.Fatal("redaction mutated the caller's retained entry")
	}
	// Match upstream's later logger reconfiguration; the boundary must remain
	// independent of both formatter and output replacement.
	var upstream bytes.Buffer
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(&upstream)
	parent.Warn("failed sending message on channel")
	if record := libraryReadRecord(t, records); record.Code != "SHIM_RPC_SEND_FAILED" || record.Severity != "warning" {
		t.Fatal("fixed transport class/severity changed")
	}
	if upstream.Len() != 0 {
		t.Fatal("upstream writer received a library entry")
	}
	// Panic semantics remain intact, but the recovered object must also be safe
	// if formatted by a different formatter after leaving logrus.
	var recovered any
	func() { defer func() { recovered = recover() }(); parent.Panic("SYNTHETIC_PRIVATE_PANIC") }()
	entry, ok := recovered.(*logrus.Entry)
	if !ok || entry.Level != logrus.PanicLevel || entry.Context != nil || entry.Caller != nil || len(entry.Data) != 0 || !entry.Time.IsZero() {
		t.Fatal("panic policy changed or retained private entry state")
	}
	raw, err := (&logrus.JSONFormatter{}).Format(entry)
	if err != nil || bytes.Contains(raw, []byte("SYNTHETIC_PRIVATE")) || bytes.Contains(raw, []byte("logrus_error")) {
		t.Fatal("recovered panic retained hidden private formatting state")
	}
	if record := libraryReadRecord(t, records); record.Severity != "panic" {
		t.Fatal("panic diagnostic severity changed")
	}
}

func TestLibraryDiagnosticsConcurrentClassesStayBoundedWithBlockedWriter(t *testing.T) {
	w := &diagnosticBlockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	sink := newProcessDiagnosticSink(w)
	defer func() {
		close(w.release)
		sink.close()
		select {
		case <-sink.done:
		case <-time.After(time.Second):
			t.Error("released library diagnostic writer did not stop")
		}
	}()
	logger, _ := libraryTestLogger(t, sink)
	logger.Error("SYNTHETIC_PRIVATE_FIRST")
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("diagnostic writer did not block")
	}
	messages := []string{"SYNTHETIC_PRIVATE_OTHER", "error receiving message", "failed sending message on channel",
		"ttrpc: received message on inactive stream", "ttrpc: failed to handle message", "ttrpc: refusing connection after handshake",
		"ttrpc: create connection failed", "containerd-shim: ttrpc server failure", "reap exit status", "serving api on socket",
		"starting signal loop", "Caught exit signal", "=== BEGIN goroutine stack dump ===\nSYNTHETIC_PRIVATE_STACK\n=== END goroutine stack dump ==="}
	levels := []logrus.Level{logrus.ErrorLevel, logrus.WarnLevel, logrus.InfoLevel, logrus.DebugLevel, logrus.TraceLevel}
	done := make(chan struct{})
	go func() {
		var workers sync.WaitGroup
		for worker := 0; worker < 4; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := 0; i < 32; i++ {
					for _, message := range messages {
						for _, level := range levels {
							logger.WithField("SYNTHETIC_PRIVATE", libraryForbiddenValue{}).Log(level, message)
						}
					}
				}
			}()
		}
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("library logging waited on the blocked writer")
	}
	// Thirteen classes times five nonterminal severities: one blocked write
	// plus the shared fixed queue. Repetition or input changes add no records.
	if len(sink.queue) != processDiagnosticQueueSize || sink.dropped.Load() != 0 || sink.tryRecord(diagnosticTestRecord()) || sink.dropped.Load() != 1 {
		t.Fatal("library events escaped deduplication or shared queue limits")
	}
	sink.close() // Close must not wait for that writer either.
}

type libraryFirstHandshakeRefused struct{ calls atomic.Uint32 }

func (h *libraryFirstHandshakeRefused) Handshake(ctx context.Context, conn net.Conn) (net.Conn, interface{}, error) {
	if h.calls.Add(1) == 1 {
		return nil, nil, errors.New("SYNTHETIC_PRIVATE_PEER /private/socket credential")
	}
	return ttrpc.UnixSocketRequireSameUser().Handshake(ctx, conn)
}

func TestLibraryDiagnosticsRealUnixHandshakeRefusalKeepsInspectionResponsive(t *testing.T) {
	records := make(chan []byte, 16)
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	defer sink.close()
	logger, _ := libraryTestLogger(t, sink)
	address := filepath.Join(privateTestDirectory(t), "peer.sock")
	listener, err := net.Listen("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(address, 0600)
	server, err := ttrpc.NewServer(ttrpc.WithServerHandshaker(&libraryFirstHandshakeRefused{}))
	if err != nil {
		t.Fatal(err)
	}
	inspection.Register(server, func(_ context.Context, q inspection.Request) (inspection.Report, error) { return routerReport(q), nil })
	ctx, cancel := context.WithCancel(containerdlog.WithLogger(t.Context(), logrus.NewEntry(logger).WithField("socket", address)))
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
			t.Error("synthetic library server did not drain")
		}
		select {
		case <-done:
		case <-shutdown.Done():
			t.Error("synthetic library listener did not stop")
		}
	})
	refused, err := net.DialTimeout("unix", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer refused.Close()
	if record := libraryReadRecord(t, records); record.Code != "SHIM_RPC_PEER_REFUSED" || record.Severity != "error" {
		t.Fatal("actual TTRPC refusal did not cross the library boundary")
	}
	observation, err := inspection.ObserveSocket(t.Context(), address, routerOwner())
	if err != nil || observation != routerReport(routerOwner()) {
		t.Fatal("transport diagnostic changed subsequent owning-shim inspection")
	}
}

func TestLibraryDiagnosticsFatalKeepsOriginalExitPolicy(t *testing.T) {
	if os.Getenv("MK_G8_LIBRARY_FATAL_CHILD") == "1" {
		logrus.AddHook(newLibraryLogBoundary(nil))
		_, _ = os.Stdout.WriteString("LIBRARY_FATAL_POLICY_NEXT\n")
		logrus.WithField("SYNTHETIC_PRIVATE_FIELD", func() {}).WithError(errors.New("SYNTHETIC_PRIVATE_ERROR")).Fatal("SYNTHETIC_PRIVATE_MESSAGE")
		os.Exit(99)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLibraryDiagnosticsFatalKeepsOriginalExitPolicy$")
	command.Env = append(os.Environ(), "MK_G8_LIBRARY_FATAL_CHILD=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || ctx.Err() != nil || stdout.String() != "LIBRARY_FATAL_POLICY_NEXT\n" || bytes.Contains(stderr.Bytes(), []byte("SYNTHETIC_PRIVATE")) || stderr.Len() > processDiagnosticRecordLimit {
		t.Fatal("fatal exit policy changed or private data escaped")
	}
	// Delivery is deliberately lossy and may be preempted by the unchanged
	// process exit. Any delivered record must still have the exact contract.
	if stderr.Len() != 0 {
		var r libraryDiagnosticRecord
		if json.Unmarshal(stderr.Bytes(), &r) != nil || !validLibraryDiagnosticRecord(r) || r.Severity != "fatal" {
			t.Fatal("fatal diagnostic was not a fixed record")
		}
	}
}
