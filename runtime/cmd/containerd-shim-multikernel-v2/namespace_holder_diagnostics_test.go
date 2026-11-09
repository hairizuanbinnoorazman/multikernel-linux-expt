//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type namespaceDiagnosticHolderFixture struct {
	validation, stop error
	stops            int
}

func (h *namespaceDiagnosticHolderFixture) PID() uint32           { return 73 }
func (h *namespaceDiagnosticHolderFixture) Validate(string) error { return h.validation }
func (h *namespaceDiagnosticHolderFixture) Stop() error           { h.stops++; return h.stop }

func TestNamespaceHolderDiagnosticsRetainFailureAndOwnership(t *testing.T) {
	for _, kind := range []string{"factory-error", "nil-holder", "validation-error", "replace-stop-error", "new-success", "cached-success"} {
		t.Run(kind, func(t *testing.T) {
			records := make(chan []byte, 2)
			sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
			defer sink.close()
			endpoint := validShimEndpoint("container", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/SYNTHETIC_PRIVATE_NAMESPACE")
			candidate := &namespaceDiagnosticHolderFixture{}
			s := &service{netEndpoint: endpoint, processDiagnosticSink: sink}
			factoryCalls := 0
			factoryError := errors.New("SYNTHETIC_PRIVATE_FACTORY_CAUSE")
			s.newNamespaceHolder = func(string) (namespaceHolder, error) {
				factoryCalls++
				switch kind {
				case "factory-error":
					return nil, factoryError
				case "nil-holder":
					return nil, nil
				case "validation-error":
					candidate.validation = factoryError
				}
				return candidate, nil
			}
			previous := &namespaceDiagnosticHolderFixture{}
			if kind == "replace-stop-error" {
				previous.validation = errors.New("stale")
				previous.stop = factoryError
				s.namespaceHolder = previous
			}
			if kind == "cached-success" {
				s.namespaceHolder = previous
			}
			err := s.ensureNamespaceHolder()
			failed := kind != "new-success" && kind != "cached-success"
			if (err != nil) != failed {
				t.Fatal("holder outcome changed")
			}
			if kind == "factory-error" || kind == "validation-error" || kind == "replace-stop-error" {
				if !errors.Is(err, factoryError) {
					t.Fatal("internal error cause lost")
				}
			}
			if kind == "validation-error" && candidate.stops != 1 {
				t.Fatal("rejected holder was not stopped exactly once")
			}
			if kind == "replace-stop-error" && (s.namespaceHolder != previous || previous.stops != 1 || factoryCalls != 0) {
				t.Fatal("failed stop lost prior ownership or launched replacement")
			}
			if kind == "cached-success" && (s.namespaceHolder != previous || factoryCalls != 0) {
				t.Fatal("cached holder contract changed")
			}
			if kind == "new-success" && s.namespaceHolder != candidate {
				t.Fatal("new holder was not retained")
			}
			if failed {
				select {
				case raw := <-records:
					var record processDiagnosticRecord
					var fields map[string]any
					if json.Unmarshal(raw, &record) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 8 || !validProcessDiagnosticRecord(record) || strings.Contains(string(raw), "SYNTHETIC_PRIVATE") {
						t.Fatal("unsafe holder diagnostic")
					}
					if record.Code != "NETWORK_NAMESPACE_HOLDER_FAILED" || record.Operation != "prepare-network-namespace-holder" || record.SandboxID != "box" || record.Generation != endpoint.SandboxGeneration || record.ProcessID != "init" {
						t.Fatal("holder diagnostic has wrong ownership")
					}
				case <-time.After(time.Second):
					t.Fatal("missing fixed holder diagnostic")
				}
			}
			sink.close()
			<-sink.done
			if len(records) != 0 {
				t.Fatal("extra or success diagnostic")
			}
		})
	}
}

func TestNamespaceHolderFailureDoesNotWaitForDiagnosticWriter(t *testing.T) {
	w := &diagnosticBlockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	sink := newProcessDiagnosticSink(w)
	defer func() { close(w.release); sink.close(); <-sink.done }()
	s := &service{netEndpoint: validShimEndpoint("container", "box", strings.Repeat("a", 32), strings.Repeat("b", 32), "/private"), processDiagnosticSink: sink,
		newNamespaceHolder: func(string) (namespaceHolder, error) { return nil, errors.New("private cause") }}
	if s.ensureNamespaceHolder() == nil {
		t.Fatal("factory refusal accepted")
	}
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("writer did not block")
	}
	done := make(chan error, 1)
	go func() { done <- s.ensureNamespaceHolder() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("factory refusal accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("holder failure waited on diagnostic writer")
	}
}
