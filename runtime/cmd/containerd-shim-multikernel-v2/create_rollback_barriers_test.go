//go:build linux

package main

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCreateRollbackStopsAtEveryUncertainOwner(t *testing.T) {
	for _, stage := range []string{"network-release", "sandbox-delete", "cancel-create", "cancel-not-safe", "rootfs-cleanup", "pre-sandbox-rootfs", "delete-not-found", "network-not-found"} {
		t.Run(stage, func(t *testing.T) {
			injected := errors.New("synthetic network refusal")
			pending := stage == "cancel-create" || stage == "cancel-not-safe"
			preSandbox := stage == "pre-sandbox-rootfs"
			generation := strings.Repeat("a", 32)
			sandbox := protocol.Sandbox{ID: "box", Generation: generation}
			endpoint := validShimEndpoint("task", "box", generation, strings.Repeat("b", 32), "/run/netns/test")
			p := &process{pid: 17}
			s := &service{sandbox: sandbox, netEndpoint: endpoint, token: []byte("private"), processes: map[string]*process{"": p}}
			s.setEventDiagnosticOwner(s.sandbox)
			if preSandbox {
				s.sandbox = protocol.Sandbox{}
				s.netEndpoint = mknetwork.Endpoint{}
				s.token = nil
				delete(s.processes, "")
				s.setEventDiagnosticOwner(s.sandbox)
			}
			initialSandbox, initialToken := s.sandbox, append([]byte(nil), s.token...)
			diagnosticOwner := s.eventDiagnosticOwner.Load()
			config := protocol.SandboxConfig{ID: "box", CPUs: []int{8}, AgentPort: 7200, ChildCID: 40}
			prepared := &rootfspkg.CleanupRequest{Version: 1, Bundle: "/private/bundle", TaskIdentity: "task", StorageSHA256: strings.Repeat("c", 64)}
			calls := []string{}
			failing := true
			s.netClient = retentionNetworkCaller(func(_ context.Context, r mknetwork.Request) (mknetwork.Response, error) {
				calls = append(calls, r.Method)
				if r.Endpoint == nil || r.Endpoint.SandboxID != "box" || r.Endpoint.SandboxGeneration != generation {
					t.Fatal("network cleanup lost exact owner")
				}
				if stage == "network-not-found" {
					return mknetwork.Response{}, &mknetwork.APIError{Code: "NOT_FOUND"}
				}
				if failing && stage == "network-release" {
					return mknetwork.Response{}, injected
				}
				return mknetwork.Response{}, nil
			})
			s.daemon = daemonCallFunc(func(_ context.Context, r protocol.Request, out any) *protocol.Error {
				calls = append(calls, r.Method)
				switch r.Method {
				case "DeleteSandbox":
					if r.SandboxID != "box" || r.Generation != generation || r.IdempotencyKey != "shim-create-rollback-delete-"+generation {
						t.Fatal("sandbox cleanup lost exact identity/key")
					}
					if stage == "delete-not-found" {
						return &protocol.Error{Code: "NOT_FOUND"}
					}
					if failing && (stage == "sandbox-delete" || stage == "network-not-found") {
						return &protocol.Error{Code: "SYNTHETIC_REFUSAL"}
					}
				case "CancelCreateSandbox":
					var body protocol.SandboxConfig
					if protocol.StrictDecode(r.Body, &body) != nil || !reflect.DeepEqual(body, config) || r.IdempotencyKey != "original-key" {
						t.Fatal("cancellation changed original request")
					}
					if failing && stage == "cancel-create" {
						return &protocol.Error{Code: "SYNTHETIC_REFUSAL"}
					}
					if err := setJSONResponse(out, map[string]bool{"safe_to_cleanup": !(failing && stage == "cancel-not-safe")}); err != nil {
						t.Fatal(err)
					}
				case "CleanupRootfs":
					var body rootfspkg.CleanupRequest
					if protocol.StrictDecode(r.Body, &body) != nil || body != *prepared {
						t.Fatal("prepared-root cleanup identity changed")
					}
					if failing && (stage == "rootfs-cleanup" || stage == "pre-sandbox-rootfs" || stage == "delete-not-found") {
						return &protocol.Error{Code: "SYNTHETIC_REFUSAL"}
					}
				default:
					t.Fatal("unexpected cleanup method")
				}
				return nil
			})
			err := s.rollbackCreate(t.Context(), prepared, pending, config, "original-key")
			if err == nil {
				t.Fatal("uncertain cleanup accepted")
			}
			expected := []string{"RELEASE", "DeleteSandbox"}
			switch stage {
			case "network-release":
				expected = []string{"RELEASE"}
			case "cancel-create", "cancel-not-safe":
				expected = []string{"RELEASE", "CancelCreateSandbox"}
			case "rootfs-cleanup", "delete-not-found":
				expected = append(expected, "CleanupRootfs")
			case "pre-sandbox-rootfs":
				expected = []string{"CleanupRootfs"}
			}
			if !slices.Equal(calls, expected) {
				t.Fatal("rollback crossed the failed cleanup stage")
			}
			if stage == "network-release" && !errors.Is(err, injected) {
				t.Fatal("original network error lost")
			}
			if !reflect.DeepEqual(s.sandbox, initialSandbox) || !reflect.DeepEqual(s.token, initialToken) || s.eventDiagnosticOwner.Load() != diagnosticOwner {
				t.Fatal("incomplete rollback discarded retry context")
			}
			if !preSandbox && s.processes[""] != p {
				t.Fatal("incomplete rollback discarded task record")
			}
			if stage == "network-release" {
				if !reflect.DeepEqual(s.netEndpoint, endpoint) {
					t.Fatal("failed network release lost its owner")
				}
			} else if s.netEndpoint.SandboxID != "" {
				t.Fatal("successful network release was not recorded")
			}
			failing = false
			before := len(calls)
			if err = s.rollbackCreate(t.Context(), prepared, pending, config, "original-key"); err != nil {
				t.Fatal("exact cleanup retry failed")
			}
			retry := []string{"DeleteSandbox", "CleanupRootfs"}
			if pending {
				retry = []string{"CancelCreateSandbox", "CleanupRootfs"}
			}
			if preSandbox {
				retry = []string{"CleanupRootfs"}
			}
			if stage == "network-release" {
				retry = append([]string{"RELEASE"}, retry...)
			}
			if !slices.Equal(calls[before:], retry) || s.sandbox.ID != "" || s.token != nil || len(s.processes) != 0 || s.eventDiagnosticOwner.Load() != nil {
				t.Fatal("cleanup retry did not complete in order")
			}
		})
	}
}
