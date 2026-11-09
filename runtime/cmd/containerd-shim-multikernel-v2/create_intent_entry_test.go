//go:build linux

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/namespaces"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// This file also runs as the sole test overlay on old production sources.
// It deliberately uses only entry points that existed before the new marker.
func TestPendingCreateIntentRefusesFreshEntry(t *testing.T) {
	const marker = ".multikernel-create.json"
	if os.Getenv("MK_CREATE_INTENT_REOPEN_CHILD") == "1" {
		ctx := namespaces.WithNamespace(context.Background(), "default")
		if owner, err := newServiceOwner(ctx, "task", &fakePublisher{}, func() {}); err == nil || owner != nil {
			os.Exit(31)
		}
		os.Exit(23)
	}
	for _, entry := range []string{"Create", "Cleanup", "worker", "delete-worker"} {
		t.Run(entry, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			config := `{"ociVersion":"1.1.0","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(bundle, marker)
			if err := os.WriteFile(path, []byte("SYNTHETIC_PRIVATE_PENDING_INTENT"), 0600); err != nil {
				t.Fatal(err)
			}
			if entry == "worker" || entry == "delete-worker" {
				args := []string{"-test.run=^TestPendingCreateIntentRefusesFreshEntry$"}
				if entry == "delete-worker" {
					args = append(args, "delete")
				}
				command := exec.Command(os.Args[0], args...)
				command.Dir = bundle
				command.Env = append(os.Environ(), "MK_CREATE_INTENT_REOPEN_CHILD=1", "MK_DAEMON_SOCKET=/nonexistent/intent-fixture", "MK_NETWORK_SOCKET=/nonexistent/intent-fixture")
				var stdout, stderr bytes.Buffer
				command.Stdout = &stdout
				command.Stderr = &stderr
				err := command.Run()
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 23 || stdout.Len() != 0 || stderr.Len() != 0 {
					t.Fatal("pending intent admitted a fresh worker")
				}
			} else {
				calls := 0
				s := &service{id: "task", namespace: "default", bundle: bundle, processes: map[string]*process{}, daemon: daemonCallFunc(func(context.Context, protocol.Request, any) *protocol.Error {
					calls++
					return &protocol.Error{Code: "VALIDATION_SENTINEL"}
				})}
				t.Cleanup(func() {
					if s.bundleDirectory != nil {
						_ = s.bundleDirectory.Close()
					}
				})
				if entry == "Create" {
					result, err := s.Create(t.Context(), &taskapi.CreateTaskRequest{ID: "task", Bundle: bundle})
					if err == nil || result != nil || calls != 0 {
						t.Fatal("pending intent admitted backend setup")
					}
				} else {
					result, err := s.Cleanup(t.Context())
					if err == nil || result != nil || calls != 0 {
						t.Fatal("pending intent admitted cleanup")
					}
				}
			}
			raw, err := os.ReadFile(path)
			if err != nil || string(raw) != "SYNTHETIC_PRIVATE_PENDING_INTENT" {
				t.Fatal("refusal changed pending intent")
			}
		})
	}
}
