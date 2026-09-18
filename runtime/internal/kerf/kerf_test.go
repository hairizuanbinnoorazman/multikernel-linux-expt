package kerf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
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
		{"load", "loaded", func() error {
			return client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline", LoadFiles{})
		}},
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

func TestLoadPinsRuntimeInitramfsAcrossPublicDirectoryReplacement(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	movedDir := filepath.Join(bundle, ".multikernel-original")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	logicalInitrd := filepath.Join(runtimeDir, "initramfs.cpio.gz")
	if err := os.WriteFile(filepath.Join(runtimeDir, "initramfs.path"), []byte(logicalInitrd+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logicalInitrd, []byte("original-initramfs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(runtimeDir, "token"), []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtimeHandle, err := os.Open(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	defer runtimeHandle.Close()
	initrdHandle, err := os.Open(logicalInitrd)
	if err != nil {
		t.Fatal(err)
	}
	defer initrdHandle.Close()
	kernelPath := filepath.Join(bundle, "approved-kernel")
	if err = os.WriteFile(kernelPath, []byte("original-kernel\n"), 0600); err != nil {
		t.Fatal(err)
	}
	kernelHandle, err := os.Open(kernelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer kernelHandle.Close()
	if err = os.Rename(kernelPath, kernelPath+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(kernelPath, []byte("substitute-kernel\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(runtimeDir, movedDir); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(logicalInitrd, []byte("substitute-initramfs\n"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(bundle, "kerf-observed")
	script := filepath.Join(t.TempDir(), "kerf")
	contents := "#!/bin/sh\nset -eu\n" +
		"test \"$3\" = '--kernel=/proc/self/fd/4'\n" +
		"test \"$(cat /proc/self/fd/4)\" = 'original-kernel'\n" +
		"test \"$4\" = '--initrd=/proc/self/fd/3'\n" +
		"test \"$(cat /proc/self/fd/3)\" = 'original-initramfs'\n" +
		"case \"$5\" in *'mk.token=" + token + "'*) ;; *) exit 41 ;; esac\n" +
		"printf observed > '" + marker + "'\n"
	if err := os.WriteFile(script, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Timeout: time.Second}
	sandbox := protocol.Sandbox{ID: "box", Generation: "generation", Config: protocol.SandboxConfig{
		Bundle: bundle, AgentPort: 7200,
	}}
	if err := client.Load(context.Background(), sandbox, kernelPath, "/fallback-initrd", "cmdline", LoadFiles{RuntimeDir: runtimeHandle, Initramfs: initrdHandle, Kernel: kernelHandle}); err != nil {
		t.Fatal(err)
	}
	if value, err := os.ReadFile(marker); err != nil || string(value) != "observed" {
		t.Fatalf("fake Kerf observation = %q, %v", value, err)
	}
	if value, err := os.ReadFile(filepath.Join(movedDir, "initramfs.cpio.gz")); err != nil || string(value) != "original-initramfs\n" {
		t.Fatalf("original initramfs = %q, %v", value, err)
	}
	if value, err := os.ReadFile(logicalInitrd); err != nil || string(value) != "substitute-initramfs\n" {
		t.Fatalf("substitute initramfs = %q, %v", value, err)
	}
	if value, err := os.ReadFile(kernelPath); err != nil || string(value) != "substitute-kernel\n" {
		t.Fatalf("substitute kernel = %q, %v", value, err)
	}
}

func TestLoadRejectsUnsafePresentRuntimeArtifacts(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{"missing-initramfs-path", func(t *testing.T, runtimeDir string) {
			if err := os.WriteFile(filepath.Join(runtimeDir, "initramfs.cpio.gz"), []byte("initramfs"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink-initramfs", func(t *testing.T, runtimeDir string) {
			target := filepath.Join(filepath.Dir(runtimeDir), "target")
			if err := os.WriteFile(target, []byte("initramfs"), 0600); err != nil {
				t.Fatal(err)
			}
			logical := filepath.Join(runtimeDir, "initramfs.cpio.gz")
			if err := os.WriteFile(filepath.Join(runtimeDir, "initramfs.path"), []byte(logical+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, logical); err != nil {
				t.Fatal(err)
			}
		}},
		{"noncanonical-path", func(t *testing.T, runtimeDir string) {
			if err := os.WriteFile(filepath.Join(runtimeDir, "initramfs.path"), []byte("/tmp/other\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(runtimeDir, "initramfs.cpio.gz"), []byte("initramfs"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			runtimeDir := filepath.Join(bundle, ".multikernel")
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			test.setup(t, runtimeDir)
			marker := filepath.Join(bundle, "executed")
			script := filepath.Join(t.TempDir(), "kerf")
			if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
				t.Fatal(err)
			}
			client := &CLI{Path: script, Timeout: time.Second}
			sandbox := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{Bundle: bundle}}
			if err := client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline", LoadFiles{}); err == nil {
				t.Fatal("unsafe runtime artifacts accepted")
			}
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Kerf executed after artifact rejection: %v", err)
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
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60 & wait\n"), 0755); err != nil {
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

func TestBackendOutputIsBoundedAndSecretSafe(t *testing.T) {
	script := filepath.Join(t.TempDir(), "kerf")
	contents := "#!/bin/sh\nprintf super-secret >&2\nhead -c 4097 /dev/zero\nexit 9\n"
	if err := os.WriteFile(script, []byte(contents), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Timeout: time.Second, MaxOutput: 4096}
	err := client.EnsurePool(context.Background())
	if !errors.Is(err, boundedexec.ErrOutputLimit) {
		t.Fatalf("overflow error = %v", err)
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("Kerf error disclosed command output: %v", err)
	}
	if !strings.Contains(err.Error(), "retained_output_bytes=4096") || !strings.Contains(err.Error(), "retained_output_sha256=") {
		t.Fatalf("Kerf error lacks bounded output evidence: %v", err)
	}
}

func TestCallerCancellationCannotBeAcceptedAsObservedSuccess(t *testing.T) {
	root := t.TempDir()
	instance := filepath.Join(root, "instances", "box")
	if err := os.MkdirAll(instance, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "status"), []byte("created\n"), 0644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "executed")
	script := filepath.Join(t.TempDir(), "kerf")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sandbox := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 2}}
	if err := client.Create(ctx, sandbox); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create with matching observed state = %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled Kerf command executed: %v", err)
	}
	if _, err := client.Observe(ctx, sandbox.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled observation error = %v", err)
	}
	if _, err := client.ListInstances(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inventory error = %v", err)
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
		{"load", "created", func() error {
			return client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline", LoadFiles{})
		}},
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
