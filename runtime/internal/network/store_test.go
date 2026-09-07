package network

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRejectsUntrustedDurableState(t *testing.T) {
	for name, content := range map[string]string{
		"unknown field":       `{"version":1,"endpoints":{},"extra":true}`,
		"duplicate field":     `{"version":1,"version":1,"endpoints":{}}`,
		"unsupported version": `{"version":2,"endpoints":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "state.json"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenStore(directory); err == nil {
				t.Fatal("untrusted state accepted")
			}
		})
	}
}

func TestStoreRejectsForgedEndpointIdentityBeforeReconciliation(t *testing.T) {
	directory := t.TempDir()
	endpoint := Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box", Owner: "cni",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "ALLOCATING",
	}
	data, err := json.Marshal(diskState{Version: 1, Endpoints: map[string]Endpoint{"forged-key": endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenStore(directory); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("forged endpoint error = %v", err)
	}
	store, err := OpenStore(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint.NetNS = "/tmp/attacker-controlled"
	if err = store.Put(endpoint); err == nil {
		t.Fatal("unsafe endpoint entered durable store")
	}
}

func TestStoreRejectsSymlinkAndPermissiveState(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte(`{"version":1,"endpoints":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(directory, "state.json")
	if err := os.Symlink(target, state); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(directory); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("symlink state error=%v", err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte(`{"version":1,"endpoints":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(directory); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("permissive state error=%v", err)
	}
}
