//go:build linux

package main

import (
	"context"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"path/filepath"
	"testing"
)

func TestAllocatedSandboxDeclaresRequiredNetworkExpectation(t *testing.T) {
	t.Setenv("MK_SHIM_LOCK", filepath.Join(privateTestDirectory(t), "allocation.lock"))
	t.Setenv("MK_SANDBOX_CPU_SETS", "8,9")
	s := &service{id: "task", namespace: "default", daemon: daemonCallFunc(func(_ context.Context, r protocol.Request, out any) *protocol.Error {
		if r.Method != "ListSandboxes" {
			t.Fatal("unexpected RPC")
		}
		*out.(*[]protocol.Sandbox) = []protocol.Sandbox{}
		return nil
	})}
	config, lock, err := s.allocate(t.Context(), "/private-bundle")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	expectation, err := protocol.NetworkExpectation(config)
	if err != nil || expectation != "required" {
		t.Fatalf("expectation=%q %v", expectation, err)
	}
}
