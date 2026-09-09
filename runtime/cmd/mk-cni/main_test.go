package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
)

type fakeCaller struct {
	requests []network.Request
	response network.Response
	err      error
}

func (f *fakeCaller) Call(_ context.Context, request network.Request) (network.Response, error) {
	f.requests = append(f.requests, request)
	return f.response, f.err
}

func validConfig(t *testing.T) []byte {
	t.Helper()
	cache := t.TempDir()
	if err := os.Chmod(cache, 0700); err != nil {
		t.Fatal(err)
	}
	return []byte(fmt.Sprintf(`{"cniVersion":"1.0.0","name":"multikernel","type":"multikernel","socket":"/run/mknetd.sock","cacheDir":%q}`, cache))
}

func TestAddReturnsAllocatedInterfaceIPAndDNS(t *testing.T) {
	fake := &fakeCaller{response: network.Response{Endpoint: &network.Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box",
		Owner: "cni", Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400,
		State: "READY", DNS: network.DNS{Nameservers: []string{"1.1.1.1"}},
	}}}
	value, err := run(context.Background(), validConfig(t), environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, fake)
	if err != nil {
		t.Fatal(err)
	}
	result := value.(cniResult)
	if result.IPs[0].Address != "172.31.0.2/30" || result.Interfaces[0].Sandbox != "/run/netns/box" || result.DNS.Nameservers[0] != "1.1.1.1" {
		t.Fatalf("ADD result = %+v", result)
	}
	if len(fake.requests) != 1 || fake.requests[0].Method != "ADD" {
		t.Fatalf("requests = %+v", fake.requests)
	}
}

func TestAddRejectsMismatchedResponseAndRollsBackExactGeneration(t *testing.T) {
	allocated := &network.Endpoint{
		ContainerID: "other", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box", Owner: "cni",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "READY",
	}
	fake := &fakeCaller{response: network.Response{Endpoint: allocated}}
	_, err := run(context.Background(), validConfig(t), environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, fake)
	if err == nil || !strings.Contains(err.Error(), "identity differs") {
		t.Fatalf("mismatched ADD response error = %v", err)
	}
	if len(fake.requests) != 2 || fake.requests[0].Method != "ADD" || fake.requests[1].Method != "DEL" || fake.requests[1].Endpoint != allocated {
		t.Fatalf("ADD rollback requests = %+v", fake.requests)
	}
}

func TestAddRejectsSymlinkCacheWithoutChangingTargetAndRollsBack(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0755); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "cache")
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	input := []byte(fmt.Sprintf(`{"cniVersion":"1.0.0","name":"multikernel","type":"multikernel","socket":"/run/mknetd.sock","cacheDir":%q}`, linked))
	allocated := &network.Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box", Owner: "cni",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "READY",
	}
	fake := &fakeCaller{response: network.Response{Endpoint: allocated}}
	_, err := run(context.Background(), input, environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, fake)
	if err == nil || !strings.Contains(err.Error(), "symlinks or non-directories") {
		t.Fatalf("symlink cache error = %v", err)
	}
	info, statErr := os.Stat(target)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("symlink target mode changed: mode=%v", info.Mode().Perm())
	}
	if len(fake.requests) != 2 || fake.requests[1].Method != "DEL" {
		t.Fatalf("symlink cache rollback requests = %+v", fake.requests)
	}
}

func TestCheckAndDeleteProduceNoResultAndDeleteAllowsEmptyNetNS(t *testing.T) {
	input := validConfig(t)
	configuration, err := validateConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	env := environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}
	if err = writeCache(configuration, env, cacheRecord{Version: 1, Generation: "0123456789abcdef0123456789abcdef", NetNS: env.NetNS}); err != nil {
		t.Fatal(err)
	}
	fake := &fakeCaller{}
	for _, test := range []environment{
		{Command: "CHECK", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"},
		{Command: "DEL", ContainerID: "box", IfName: "eth0"},
	} {
		value, err := run(context.Background(), input, test, fake)
		if err != nil || value != nil {
			t.Fatalf("%s value=%v error=%v", test.Command, value, err)
		}
	}
	if len(fake.requests) != 2 || fake.requests[0].Endpoint.Generation == "" || fake.requests[1].Endpoint.NetNS != "/run/netns/box" {
		t.Fatalf("cached requests = %+v", fake.requests)
	}
}

func TestVersionDoesNotRequireConfigOrDaemon(t *testing.T) {
	value, err := run(context.Background(), nil, environment{Command: "VERSION"}, nil)
	if err != nil || len(value.(cniVersions).SupportedVersions) != 1 {
		t.Fatalf("VERSION value=%+v error=%v", value, err)
	}
}

func TestCNIRejectsUnknownFieldsUnsafeIdentityAndDaemonErrors(t *testing.T) {
	valid := validConfig(t)
	badConfig := append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"extra":true}`)...)
	if _, err := run(context.Background(), badConfig, environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{}); err == nil {
		t.Fatal("unknown config field accepted")
	}
	if _, err := run(context.Background(), valid, environment{Command: "ADD", ContainerID: "../box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{}); err == nil {
		t.Fatal("unsafe container ID accepted")
	}
	injected := &network.APIError{Code: "RESOURCE_EXHAUSTED", Message: "full"}
	if _, err := run(context.Background(), valid, environment{Command: "ADD", ContainerID: "box", IfName: "eth0", NetNS: "/run/netns/box"}, &fakeCaller{err: injected}); !errors.Is(err, injected) || errorCode(err) != 11 {
		t.Fatalf("daemon error=%v code=%d", err, errorCode(err))
	}
}

func TestCNIInputRejectsOversizedValidPrefix(t *testing.T) {
	prefix := validConfig(t)
	input := append(prefix, bytes.Repeat([]byte(" "), (1<<20)+1)...)
	if _, err := readCNIInput(bytes.NewReader(input)); err == nil {
		t.Fatal("oversized CNI input with a valid JSON prefix was accepted")
	}
	if data, err := readCNIInput(bytes.NewReader(prefix)); err != nil || !bytes.Equal(data, prefix) {
		t.Fatalf("bounded CNI input = %q, %v", data, err)
	}
}

func TestCacheCreationRejectsSymlinkAncestorWithoutMutation(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "linked")
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	configuration := config{Name: "multikernel", CacheDir: filepath.Join(linked, "new-cache")}
	env := environment{ContainerID: "box", IfName: "eth0"}
	err := writeCache(configuration, env, cacheRecord{Version: 1, Generation: "0123456789abcdef0123456789abcdef", NetNS: "/run/netns/box"})
	if err == nil {
		t.Fatal("cache beneath symlinked ancestor was accepted")
	}
	if _, statErr := os.Stat(filepath.Join(target, "new-cache")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cache creation mutated symlink target: %v", statErr)
	}
}

func TestCachePublicationIsIdempotentAndGenerationExclusive(t *testing.T) {
	input := validConfig(t)
	configuration, err := validateConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	env := environment{ContainerID: "box", IfName: "eth0"}
	first := cacheRecord{Version: 1, Generation: "0123456789abcdef0123456789abcdef", NetNS: "/run/netns/box"}
	if err = writeCache(configuration, env, first); err != nil {
		t.Fatal(err)
	}
	if err = writeCache(configuration, env, first); err != nil {
		t.Fatalf("exact cache replay failed: %v", err)
	}
	conflict := first
	conflict.Generation = "abcdef0123456789abcdef0123456789"
	if err = writeCache(configuration, env, conflict); err == nil || !strings.Contains(err.Error(), "different generation") {
		t.Fatalf("conflicting cache generation error = %v", err)
	}
	observed, err := readCache(configuration, env)
	if err != nil || observed != first {
		t.Fatalf("cache after conflict = %+v, %v", observed, err)
	}
	entries, err := os.ReadDir(configuration.CacheDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(cachePath(configuration, env)) {
		t.Fatalf("cache directory after replay = %v, %v", entries, err)
	}
	linked := filepath.Join(configuration.CacheDir, "attacker-link")
	if err = os.Link(cachePath(configuration, env), linked); err != nil {
		t.Fatal(err)
	}
	if _, err = readCache(configuration, env); err == nil {
		t.Fatal("hard-linked cache ownership record was accepted")
	}
}
