package network

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

var privateMutationFailure = errors.New(strings.Repeat("SYNTHETIC_NETWORK_MUTATION_TOKEN /private/network/credential\n", 256))

func requirePrivateMutationOmitted(t *testing.T, issue *APIError, code string, retry bool) {
	t.Helper()
	if issue == nil || issue.Code != code || issue.Retryable != retry {
		t.Fatal("mutation code or retryability changed")
	}
	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	if len(issue.Message) > 160 || bytes.Contains(data, []byte("SYNTHETIC_NETWORK_MUTATION_TOKEN")) || bytes.Contains(data, []byte("/private/network/credential")) {
		t.Fatal("mutation error reflects private backend or persistence data")
	}
}

func TestNetworkMutationErrorsOmitPrivateBackendAndStoreFailures(t *testing.T) {
	for _, operation := range []string{"add", "add-replay", "check", "bind", "delete", "release", "persist-add", "persist-bind", "persist-unbind", "persist-report", "persist-delete", "persist-release"} {
		t.Run(operation, func(t *testing.T) {
			backend := &fakeBackend{}
			s := service(t, "172.31.0.0/30", backend)
			ctx := context.Background()
			if operation == "add" {
				backend.failAdd = privateMutationFailure
				_, issue := s.Add(ctx, endpoint("one"))
				requirePrivateMutationOmitted(t, issue, "INTERNAL", true)
				if len(s.List()) != 0 || len(backend.deletes) != 1 {
					t.Fatal("ADD rollback changed")
				}
				return
			}
			if operation == "persist-add" {
				s.Store.persistFault = func() error { return privateMutationFailure }
				_, issue := s.Add(ctx, endpoint("one"))
				requirePrivateMutationOmitted(t, issue, "INTERNAL", true)
				if len(backend.adds) != 0 || len(s.List()) != 0 {
					t.Fatal("backend ran before durable allocation")
				}
				return
			}
			var value Endpoint
			var issue *APIError
			if operation == "release" || operation == "persist-release" {
				req := endpoint("one")
				req.NetNS = ""
				req.SandboxID = "box"
				req.SandboxGeneration = strings.Repeat("a", 32)
				value, issue = s.Provision(ctx, req)
			} else {
				value, issue = s.Add(ctx, endpoint("one"))
			}
			if issue != nil {
				t.Fatal(issue)
			}
			owner := Endpoint{NetNS: value.NetNS, Generation: value.Generation, SandboxID: "box", SandboxGeneration: strings.Repeat("a", 32)}
			switch operation {
			case "add-replay":
				backend.failCheck = privateMutationFailure
				_, issue = s.Add(ctx, endpoint("one"))
			case "check":
				backend.failCheck = privateMutationFailure
				_, issue = s.Check(ctx, value)
			case "bind":
				backend.failCheck = privateMutationFailure
				_, issue = s.Bind(ctx, owner)
			case "delete":
				backend.failDelete = privateMutationFailure
				issue = s.Delete(ctx, value)
			case "release":
				backend.failDelete = privateMutationFailure
				issue = s.Release(ctx, owner)
			default:
				if operation == "persist-unbind" || operation == "persist-report" {
					value, issue = s.Bind(ctx, owner)
					if issue != nil {
						t.Fatal(issue)
					}
				}
				s.Store.persistFault = func() error { return privateMutationFailure }
				switch operation {
				case "persist-bind":
					_, issue = s.Bind(ctx, owner)
				case "persist-unbind":
					issue = s.Unbind(owner)
				case "persist-report":
					value.RXPackets = 1
					issue = s.Report(value)
				case "persist-delete":
					issue = s.Delete(ctx, value)
				case "persist-release":
					issue = s.Release(ctx, owner)
				}
			}
			code := "INTERNAL"
			if operation == "add-replay" || operation == "check" || operation == "bind" {
				code = "FAILED_PRECONDITION"
			}
			requirePrivateMutationOmitted(t, issue, code, true)
			remaining := s.List()
			if len(remaining) != 1 || remaining[0].Generation != value.Generation {
				t.Fatal("failure lost exact allocation ownership")
			}
			if operation == "delete" || operation == "release" {
				if remaining[0].State != "DELETING" {
					t.Fatal("failed deletion lost transition")
				}
				backend.failDelete = nil
				if operation == "delete" {
					issue = s.Delete(ctx, value)
				} else {
					issue = s.Release(ctx, owner)
				}
				if issue != nil || len(s.List()) != 0 {
					t.Fatal("generation-bound retry did not clean up")
				}
			}
		})
	}
}

func TestNetworkAttachWireFailureOmitsPrivateDeviceError(t *testing.T) {
	backend := &fakeBackend{}
	s := service(t, "172.31.0.0/30", backend)
	value, issue := s.Add(context.Background(), endpoint("one"))
	if issue != nil {
		t.Fatal(issue)
	}
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	rpc := &Server{Service: s, OpenTUN: func(Endpoint) (*os.File, error) { return nil, privateMutationFailure }}
	go func() { defer close(done); rpc.handle(context.Background(), server) }()
	request := Request{Version: 1, RequestID: "private-device-failure", Method: "ATTACH", Endpoint: &Endpoint{NetNS: value.NetNS, Generation: value.Generation, SandboxID: "box", SandboxGeneration: strings.Repeat("a", 32)}}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
	raw, err := bufio.NewReader(client).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var response Response
	if json.Unmarshal(raw, &response) != nil {
		t.Fatal("invalid ATTACH response")
	}
	requirePrivateMutationOmitted(t, response.Error, "INTERNAL", true)
	if response.Endpoint != nil || response.RequestID != request.RequestID || len(raw) > 512 {
		t.Fatal("failed ATTACH returned endpoint data or oversized frame")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ATTACH error retained handler")
	}
	saved := s.List()
	if len(saved) != 1 || saved[0].Generation != value.Generation || saved[0].SandboxID != "box" {
		t.Fatal("device-open failure changed binding")
	}
}

func TestNetworkAllocationErrorPreservesRollbackOutcome(t *testing.T) {
	for _, failure := range []string{"backend-clean", "backend-cleanup", "record-cleanup", "readiness-persistence"} {
		t.Run(failure, func(t *testing.T) {
			backend := &fakeBackend{}
			s := service(t, "172.31.0.0/30", backend)
			if failure == "readiness-persistence" {
				backend.addHook = func() {
					s.Store.persistFault = func() error {
						s.Store.persistFault = nil
						return privateMutationFailure
					}
				}
			} else {
				backend.failAdd = privateMutationFailure
				if failure == "backend-cleanup" {
					backend.failDelete = privateMutationFailure
				}
				if failure == "record-cleanup" {
					backend.addHook = func() { s.Store.persistFault = func() error { return privateMutationFailure } }
				}
			}
			_, issue := s.Add(context.Background(), endpoint("one"))
			requirePrivateMutationOmitted(t, issue, "INTERNAL", true)
			incomplete := failure == "backend-cleanup" || failure == "record-cleanup"
			if strings.Contains(issue.Message, "rollback incomplete; reconciliation required") != incomplete || strings.Contains(issue.Message, "allocation rolled back") == incomplete {
				t.Fatal("message misrepresents allocation cleanup")
			}
			if len(backend.deletes) != 1 {
				t.Fatal("cleanup attempt changed")
			}
			remaining := s.List()
			if incomplete {
				if len(remaining) != 1 || remaining[0].Generation != backend.adds[0].Generation || remaining[0].State != "ALLOCATING" {
					t.Fatal("incomplete rollback lost durable generation")
				}
				s.Store.persistFault = nil
				backend.failDelete = nil
				if err := s.Reconcile(context.Background()); err != nil || len(s.List()) != 0 {
					t.Fatal("reconciliation failed to recover retained allocation")
				}
			} else if len(remaining) != 0 {
				t.Fatal("completed rollback retained allocation")
			}
		})
	}
}
