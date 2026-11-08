//go:build linux

package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineTargetSnapshotCanonicalPrivateAndBounded(t *testing.T) {
	_, _, targets := dockerHealthFixture(t)
	data, err := json.Marshal(struct {
		Version int            `json:"version"`
		Targets []EngineTarget `json:"targets"`
	}{1, targets})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "extra", "case", "duplicate", "null", "bad-owner", "duplicate-owner", "relative", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			bytes := append([]byte{}, data...)
			switch mode {
			case "extra":
				bytes = append(bytes[:len(bytes)-1], []byte(`,"token":"secret"}`)...)
			case "case":
				bytes = []byte(strings.Replace(string(bytes), `"version":1`, `"Version":1`, 1))
			case "duplicate":
				bytes = []byte(`{"version":1,"version":1,"targets":[]}`)
			case "null":
				bytes = []byte(`{"version":1,"targets":null}`)
			case "bad-owner":
				bytes = []byte(strings.Replace(string(bytes), `"namespace":`, `"Namespace":`, 1))
			case "duplicate-owner":
				bytes, _ = json.Marshal(struct {
					Version int            `json:"version"`
					Targets []EngineTarget `json:"targets"`
				}{1, append(append([]EngineTarget{}, targets...), targets[0])})
			case "relative":
				bytes = []byte(strings.Replace(string(bytes), `/private/secret.sock`, `relative.sock`, 1))
			case "oversized":
				bytes = make([]byte, (1<<20)+1)
			}
			got, err := DecodeEngineTargets(bytes)
			if mode == "valid" {
				if err != nil || len(got) != len(targets) {
					t.Fatal("valid target refused", err)
				}
			} else if err == nil {
				t.Fatal("unsafe targets accepted", mode)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("target input leaked")
			}
		})
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, "targets.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEngineTargets(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEngineTargets(t.Context(), path); err == nil {
		t.Fatal("public targets accepted")
	}
	_ = os.Chmod(path, 0600)
	link := filepath.Join(dir, "alias.json")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEngineTargets(t.Context(), link); err == nil {
		t.Fatal("target symlink accepted")
	}
	hard := filepath.Join(dir, "hard.json")
	if err = os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEngineTargets(t.Context(), path); err == nil {
		t.Fatal("hardlinked targets accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = LoadEngineTargets(ctx, path); err == nil {
		t.Fatal("canceled read succeeded")
	}
}
