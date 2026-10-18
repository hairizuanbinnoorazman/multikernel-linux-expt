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
		"null":                `null`,
		"empty object":        `{}`,
		"missing version":     `{"endpoints":{}}`,
		"missing endpoints":   `{"version":1}`,
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

func TestStoreRejectsDirectoryReplacementBeforePersistence(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	store, err := OpenStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(directory, directory+".moved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	endpoint := Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box", Owner: "cni",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "ALLOCATING",
	}
	if err = store.Put(endpoint); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("directory replacement error = %v", err)
	}
	if entries, readErr := os.ReadDir(directory); readErr != nil || len(entries) != 0 {
		t.Fatalf("replacement directory was mutated: %v, %v", entries, readErr)
	}
}

func TestStoreOwnsDNSCopies(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	endpoint := Endpoint{
		ContainerID: "box", NetworkName: "multikernel", IfName: "eth0", NetNS: "/run/netns/box", Owner: "cni",
		Generation: "0123456789abcdef0123456789abcdef", Address: "172.31.0.2/30", Gateway: "172.31.0.1", MTU: 1400, State: "ALLOCATING",
		DNS: DNS{Nameservers: []string{"1.1.1.1"}, Search: []string{"example.test"}, Options: []string{"ndots:1"}},
	}
	if err := store.Put(endpoint); err != nil {
		t.Fatal(err)
	}
	mutate := func(value Endpoint) {
		value.DNS.Nameservers[0] = "invalid"
		value.DNS.Search[0] = "changed.test"
		value.DNS.Options[0] = "ndots:9"
	}
	check := func(store *Store) {
		t.Helper()
		value, ok := store.Get("multikernel", "box", "eth0")
		if !ok || value.DNS.Nameservers[0] != "1.1.1.1" || value.DNS.Search[0] != "example.test" || value.DNS.Options[0] != "ndots:1" {
			t.Fatalf("caller mutated stored DNS: %+v", value.DNS)
		}
	}
	mutate(endpoint)
	check(store)
	value, _ := store.Get("multikernel", "box", "eth0")
	mutate(value)
	check(store)
	mutate(store.List()[0])
	check(store)
	// Persist a subsequent read to ensure aliased changes cannot enter disk state.
	value, _ = store.Get("multikernel", "box", "eth0")
	if err := store.Put(value); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened)
}

func TestCloneEndpointPreservesNilDNS(t *testing.T) {
	value := cloneEndpoint(Endpoint{})
	if value.DNS.Nameservers != nil || value.DNS.Search != nil || value.DNS.Options != nil {
		t.Fatal("absent DNS slices became non-nil")
	}
}
