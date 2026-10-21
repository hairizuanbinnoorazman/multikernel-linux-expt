package kernelmanifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dockerGuestFixture(t *testing.T) (Resolver, string, Manifest) {
	t.Helper()
	resolver, path := manifestFixture(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	bootstrap := filepath.Join(resolver.Directory, "docker-bootstrap")
	if err = os.WriteFile(bootstrap, []byte("#!/bin/busybox sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	profile := &DockerGuest{Kind: "docker-rootless-offline-v1", RootfsManifestSHA256: strings.Repeat("a", 64), Bootstrap: Artifact{Path: bootstrap, SHA256: digest(t, bootstrap)}, Modules: map[string]Artifact{}}
	for _, name := range []string{"llc", "stp", "bridge", "veth"} {
		path := filepath.Join(resolver.Directory, name+".ko")
		writeModuleFixture(t, path, "name="+name+"\x00vermagic=test-release SMP\x00")
		profile.Modules[name] = Artifact{Path: path, SHA256: digest(t, path)}
	}
	manifest.DockerGuest = profile
	return resolver, path, manifest
}

func TestResolveDockerGuestApproval(t *testing.T) {
	resolver, path, manifest := dockerGuestFixture(t)
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := resolver.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.Close()
	if artifacts.ManifestSHA256 != digest(t, path) {
		t.Fatal("profile approval not bound to manifest bytes")
	}
}

func TestResolveDockerGuestRejectsInvalidApproval(t *testing.T) {
	for _, mode := range []string{"kind", "digest", "uppercase", "missing-module", "extra-module", "alias", "release", "bootstrap-digest", "interpreter", "writable", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			resolver, path, manifest := dockerGuestFixture(t)
			profile := manifest.DockerGuest
			switch mode {
			case "kind":
				profile.Kind = "docker-rootful-v1"
			case "digest":
				profile.RootfsManifestSHA256 = "latest"
			case "uppercase":
				profile.RootfsManifestSHA256 = strings.Repeat("A", 64)
			case "missing-module":
				delete(profile.Modules, "bridge")
			case "extra-module":
				profile.Modules["other"] = profile.Modules["bridge"]
			case "alias":
				profile.Modules["bridge"] = profile.Modules["veth"]
			case "release":
				value := profile.Modules["veth"]
				writeModuleFixture(t, value.Path, "name=veth\x00vermagic=other-release SMP\x00")
				value.SHA256 = digest(t, value.Path)
				profile.Modules["veth"] = value
			case "bootstrap-digest":
				os.WriteFile(profile.Bootstrap.Path, []byte("#!/bin/busybox sh\nexit 1\n"), 0755)
			case "interpreter":
				os.WriteFile(profile.Bootstrap.Path, []byte("#!/bin/sh\nexit 0\n"), 0755)
				profile.Bootstrap.SHA256 = digest(t, profile.Bootstrap.Path)
			case "writable":
				os.Chmod(profile.Bootstrap.Path, 0777)
			case "symlink":
				original := profile.Bootstrap.Path
				profile.Bootstrap.Path = original + "-alias"
				if err := os.Symlink(original, profile.Bootstrap.Path); err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(manifest)
			if err := os.WriteFile(path, raw, 0644); err != nil {
				t.Fatal(err)
			}
			artifacts, err := resolver.Resolve("test")
			if err == nil {
				artifacts.Close()
				t.Fatal("invalid Docker approval accepted")
			}
		})
	}
}

func TestDockerGuestJSONRefusesNullAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"docker_guest":null}`, `{"DOCKER_GUEST":null}`, `{"docker_guest":{"unknown":true}}`, `{"docker_guest":{},"docker_guest":{}}`} {
		var manifest Manifest
		if json.Unmarshal([]byte(raw), &manifest) == nil {
			t.Fatalf("unsafe approval JSON accepted: %s", raw)
		}
	}
}

func TestDockerGuestJSONCanonicalNames(t *testing.T) {
	t.Run("root shadow", func(t *testing.T) {
		resolver, path, manifest := dockerGuestFixture(t)
		manifest.DockerGuest.Bootstrap.SHA256 = strings.Repeat("0", 64)
		raw, _ := json.Marshal(manifest)
		raw = append(raw[:len(raw)-1], []byte(`,"DOCKER_GUEST":null}`)...)
		if err := os.WriteFile(path, raw, 0644); err != nil {
			t.Fatal(err)
		}
		artifacts, err := resolver.Resolve("test")
		if err == nil {
			artifacts.Close()
			t.Fatal("case alias discarded an invalid approval")
		}
	})
	for _, mode := range []string{"kind", "bootstrap path", "module digest", "null module"} {
		t.Run(mode, func(t *testing.T) {
			_, _, manifest := dockerGuestFixture(t)
			raw, _ := json.Marshal(manifest.DockerGuest)
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "kind":
				fields["KIND"] = fields["kind"]
				delete(fields, "kind")
			case "bootstrap path":
				artifact := fields["bootstrap"].(map[string]any)
				artifact["PATH"] = artifact["path"]
				delete(artifact, "path")
			case "module digest":
				artifact := fields["modules"].(map[string]any)["llc"].(map[string]any)
				artifact["SHA256"] = artifact["sha256"]
				delete(artifact, "sha256")
			case "null module":
				fields["modules"].(map[string]any)["llc"] = nil
			}
			raw, _ = json.Marshal(fields)
			var profile DockerGuest
			if json.Unmarshal(raw, &profile) == nil {
				t.Fatal("noncanonical Docker approval accepted")
			}
		})
	}
}

// The explicit opt-in binds portable parsing tests to protected, installed
// artifacts on a qualified host without loading modules or booting a child.
func TestInstalledDockerGuestApproval(t *testing.T) {
	path := os.Getenv("MK_TEST_DOCKER_GUEST_MANIFEST")
	if path == "" {
		t.Skip("set MK_TEST_DOCKER_GUEST_MANIFEST for installed artifact qualification")
	}
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".json" {
		t.Fatal("canonical manifest path required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	expectRefusal := os.Getenv("MK_TEST_DOCKER_GUEST_EXPECT_REFUSAL") == "1"
	if err = json.Unmarshal(raw, &manifest); err != nil {
		if expectRefusal {
			t.Log("installed approval refused during parsing")
			return
		}
		t.Fatal(err)
	}
	if manifest.DockerGuest == nil {
		t.Fatalf("Docker approval missing or invalid: %v", err)
	}
	resolver := Resolver{Directory: filepath.Dir(path), RequiredUID: 0}
	artifacts, err := resolver.Resolve(strings.TrimSuffix(filepath.Base(path), ".json"))
	if expectRefusal {
		if err == nil {
			artifacts.Close()
			t.Fatal("invalid installed approval accepted")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = artifacts.Close(); err != nil {
		t.Fatal(err)
	}
}
