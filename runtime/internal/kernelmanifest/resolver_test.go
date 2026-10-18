package kernelmanifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func digest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func manifestFixture(t *testing.T) (Resolver, string) {
	t.Helper()
	directory := t.TempDir()
	var err error
	kernel := filepath.Join(directory, "vmlinux")
	agent := filepath.Join(directory, "mk-agent")
	relay := filepath.Join(directory, "mkvsock-relay")
	module := filepath.Join(directory, "mk_transport.ko")
	initrd := filepath.Join(directory, "initramfs")
	writeKernelFixture(t, kernel, "Linux version test-release (fixture)\n\x00")
	// Structural ELF fixtures avoid repeatedly compressing the entire test binary.
	// Configured root qualification separately checks actual installed artifacts.
	writeKernelFixture(t, agent, "agent fixture\x00")
	writeKernelFixture(t, relay, "relay fixture\x00")
	for _, executable := range []string{agent, relay} {
		if err = os.Chmod(executable, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeModuleFixture(t, module, "name=mk_transport\x00vermagic=test-release SMP\x00")
	writeArchive(t, initrd, baseEntries(t, directory))
	if err = os.WriteFile(filepath.Join(directory, "config-test-release"), []byte("CONFIG_MULTIKERNEL=y\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: 1, Name: "test", Architecture: "amd64", KernelRelease: "test-release",
		Compatibility: Compatibility{MultikernelRevision: multikernelRevision, KerfVersion: "v0.2.0", KerfRevision: kerfRevision},
		Kernel:        Artifact{Path: kernel, SHA256: digest(t, kernel)}, Initramfs: Artifact{Path: initrd, SHA256: digest(t, initrd)}, Agent: Artifact{Path: agent, SHA256: digest(t, agent)}, Relay: Artifact{Path: relay, SHA256: digest(t, relay)},
		Transport: Transport{Module: Artifact{Path: module, SHA256: digest(t, module)}, ModuleName: "mk_transport", SocketOption: 9, TransportID: 1, PrimaryRole: "server", ChildRole: "client"},
		Protocol:  Protocol{Min: 1, Max: 1}, RequiredConfig: []string{"CONFIG_MULTIKERNEL=y"}, OCIFeatures: []string{"argv"},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "test.json")
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return Resolver{Directory: directory, RequiredUID: uint32(os.Getuid())}, path
}

func TestResolveVerifiesApprovedArtifacts(t *testing.T) {
	resolver, manifestPath := manifestFixture(t)
	artifacts, err := resolver.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	if artifacts.Kernel == "" || artifacts.Initrd == "" {
		t.Fatalf("artifacts = %+v", artifacts)
	}
	if artifacts.ManifestSHA256 != digest(t, manifestPath) {
		t.Fatal("resolved policy is not bound to the exact manifest bytes")
	}
}

func TestResolveReturnsExactVerifiedArtifactDescriptors(t *testing.T) {
	resolver, manifestPath := manifestFixture(t)
	var manifest Manifest
	raw, err := os.ReadFile(manifestPath)
	if err != nil || json.Unmarshal(raw, &manifest) != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	artifacts, err := resolver.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	originalKernel, err := os.ReadFile(manifest.Kernel.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(manifest.Kernel.Path, manifest.Kernel.Path+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifest.Kernel.Path, []byte("substitute-kernel"), 0755); err != nil {
		t.Fatal(err)
	}
	resolvedKernel, err := io.ReadAll(artifacts.KernelFile)
	if err != nil || !bytes.Equal(resolvedKernel, originalKernel) {
		t.Fatalf("resolved kernel changed after pathname replacement: bytes=%d err=%v", len(resolvedKernel), err)
	}
	if replacement, err := os.ReadFile(manifest.Kernel.Path); err != nil || string(replacement) != "substitute-kernel" {
		t.Fatalf("replacement kernel = %q, %v", replacement, err)
	}
}

func TestResolveRejectsTamperingAndUnsafeFiles(t *testing.T) {
	t.Run("digest", func(t *testing.T) {
		resolver, path := manifestFixture(t)
		var manifest Manifest
		raw, _ := os.ReadFile(path)
		json.Unmarshal(raw, &manifest)
		if err := os.WriteFile(manifest.Initramfs.Path, []byte("tampered"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := resolver.Resolve("test"); err == nil {
			t.Fatal("tampered artifact accepted")
		}
	})
	t.Run("mode", func(t *testing.T) {
		resolver, path := manifestFixture(t)
		if err := os.Chmod(path, 0666); err != nil {
			t.Fatal(err)
		}
		if _, err := resolver.Resolve("test"); err == nil {
			t.Fatal("writable manifest accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		resolver, path := manifestFixture(t)
		target := path + ".target"
		if err := os.Rename(path, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, err := resolver.Resolve("test"); err == nil {
			t.Fatal("symlink manifest accepted")
		}
	})
}
