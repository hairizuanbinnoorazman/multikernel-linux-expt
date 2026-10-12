//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleNetworkNamespaceRequiresCanonicalOCIPath(t *testing.T) {
	config := func(path string) []byte {
		return []byte(fmt.Sprintf(`{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":%q}]}}`, path))
	}
	for name, test := range map[string]struct {
		path string
		err  bool
	}{
		"runtime creates namespace": {path: ""},
		"named namespace":           {path: "/run/netns/pod-one"},
		"relative rejected":         {path: "run/netns/pod-one", err: true},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := privateTestDirectory(t)
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), config(test.path), 0600); err != nil {
				t.Fatal(err)
			}
			actual, err := bundleNetworkNamespace(bundle)
			if (err != nil) != test.err || (!test.err && actual != test.path) {
				t.Fatalf("namespace=%q error=%v", actual, err)
			}
		})
	}
	t.Run("hardlink rejected", func(t *testing.T) {
		bundle := privateTestDirectory(t)
		path := filepath.Join(bundle, "config.json")
		if err := os.WriteFile(path, config(""), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(bundle, "config.link")); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(bundle); err == nil {
			t.Fatal("hard-linked OCI config accepted")
		}
	})
	t.Run("symlinked ancestor rejected", func(t *testing.T) {
		directory := t.TempDir()
		realBundle := filepath.Join(directory, "real")
		if err := os.Mkdir(realBundle, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realBundle, "config.json"), config(""), 0600); err != nil {
			t.Fatal(err)
		}
		linkedBundle := filepath.Join(directory, "linked")
		if err := os.Symlink(realBundle, linkedBundle); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(linkedBundle); err == nil {
			t.Fatal("OCI config beneath symlinked bundle accepted")
		}
	})
	t.Run("oversized valid prefix rejected", func(t *testing.T) {
		bundle := privateTestDirectory(t)
		data := append(config(""), bytes.Repeat([]byte(" "), (1<<20)+1)...)
		if err := os.WriteFile(filepath.Join(bundle, "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := bundleNetworkNamespace(bundle); err == nil {
			t.Fatal("oversized OCI config accepted")
		}
	})
}

func TestServiceBundleIdentityPinsConfigAcrossPublicReplacement(t *testing.T) {
	base := privateTestDirectory(t)
	bundle := filepath.Join(base, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	config := func(namespace string) []byte {
		return []byte(fmt.Sprintf(`{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":%q}]}}`, namespace))
	}
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), config("/run/netns/original"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, identity, err := openServiceIdentity("task", "default", bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	service := &service{bundle: bundle, bundleDirectory: directory, bundleIdentity: identity}
	if err = os.Rename(bundle, bundle+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(bundle, "config.json"), config("/run/netns/replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = service.ensureBundleIdentity(); err == nil {
		t.Fatal("public bundle replacement retained the service identity")
	}
	if namespace, readErr := bundleNetworkNamespaceFrom(directory); readErr != nil || namespace != "/run/netns/original" {
		t.Fatalf("held bundle namespace = %q, %v", namespace, readErr)
	}
}
