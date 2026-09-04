package kerf

import (
	"context"
	"errors"
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

func TestMalformedObservationIsRejected(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "act", "unknown"} {
		if err := os.WriteFile(filepath.Join(instance, "status"), []byte(status), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := (&CLI{Sysfs: root}).Observe(context.Background(), "box"); err == nil {
			t.Fatalf("malformed status %q accepted", status)
		}
	}
}

func TestCreatedObservationVocabulary(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"created", "ready"} {
		if err := os.WriteFile(filepath.Join(instance, "status"), []byte(status+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if got, err := (&CLI{Sysfs: root}).Observe(context.Background(), "box"); err != nil || got != "CREATED" {
			t.Fatalf("Observe(%q) = %q, %v; want CREATED", status, got, err)
		}
	}
}

func TestCommittedNonzeroTransitionsAreObserved(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	sandbox := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{Bundle: t.TempDir()}}
	tests := []struct {
		name, status string
		call         func() error
	}{
		{"load", "loaded", func() error { return client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline") }},
		{"start", "active", func() error { return client.Start(context.Background(), sandbox) }},
		{"stop", "loaded", func() error { return client.Stop(context.Background(), sandbox) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(instance, "status"), []byte(test.status), 0644); err != nil {
				t.Fatal(err)
			}
			if err := test.call(); err != nil {
				t.Fatalf("committed nonzero result rejected: %v", err)
			}
		})
	}
}

func TestDeleteAcceptsCommittedNonzeroExit(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "status"), []byte("created"), 0644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "kerf")
	contents := "#!/bin/sh\n/usr/bin/unlink '" + filepath.Join(instance, "status") + "'\n/usr/bin/rmdir '" + instance + "'\nexit 1\n"
	if err := os.WriteFile(script, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	if err := client.Delete(context.Background(), protocol.Sandbox{ID: "box"}); err != nil {
		t.Fatalf("committed delete rejected: %v", err)
	}
}

func TestBackendTimeoutIsBounded(t *testing.T) {
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\n"), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Timeout: 10 * time.Millisecond}
	started := time.Now()
	err := client.EnsurePool(context.Background())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func TestEveryUncommittedNonzeroExitIsRejected(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 9\n"), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	sandbox := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{Bundle: t.TempDir()}}
	tests := []struct {
		name, status string
		call         func() error
	}{
		{"ensure-pool", "created", func() error { return client.EnsurePool(context.Background()) }},
		{"create", "", func() error { return client.Create(context.Background(), protocol.Sandbox{ID: "missing"}) }},
		{"load", "created", func() error { return client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline") }},
		{"start", "loaded", func() error { return client.Start(context.Background(), sandbox) }},
		{"stop", "active", func() error { return client.Stop(context.Background(), sandbox) }},
		{"delete", "created", func() error { return client.Delete(context.Background(), sandbox) }},
		{"release-pool", "created", func() error { return client.ReleasePool(context.Background()) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.status != "" {
				if err := os.WriteFile(filepath.Join(instance, "status"), []byte(test.status), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := test.call(); err == nil {
				t.Fatal("uncommitted nonzero exit accepted")
			}
		})
	}
}
