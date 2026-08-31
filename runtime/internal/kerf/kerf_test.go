package kerf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestCreateAcceptsCommittedNonzeroExit(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "status"), []byte("created\n"), 0644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	s := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 2}}
	if err := c.Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRejectsNonzeroWithoutInstance(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	c := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	s := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 2}}
	if err := c.Create(context.Background(), s); err == nil {
		t.Fatal("uncommitted create accepted")
	}
}
