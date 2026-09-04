package kernelmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func copyFile(t *testing.T, source, destination string, mode os.FileMode) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
}

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
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	kernel := filepath.Join(directory, "vmlinux")
	agent := filepath.Join(directory, "mk-agent")
	relay := filepath.Join(directory, "mkvsock-relay")
	module := filepath.Join(directory, "mk_transport.ko")
	initrd := filepath.Join(directory, "initramfs")
	copyFile(t, executable, kernel, 0755)
	copyFile(t, executable, agent, 0755)
	copyFile(t, executable, relay, 0755)
	if err = os.WriteFile(module, []byte("module"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(initrd, []byte("initramfs"), 0644); err != nil {
		t.Fatal(err)
	}
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
	resolver, _ := manifestFixture(t)
	artifacts, err := resolver.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	if artifacts.Kernel == "" || artifacts.Initrd == "" {
		t.Fatalf("artifacts = %+v", artifacts)
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
