//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
)

func intentTestOwner(t *testing.T) *service {
	t.Helper()
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), processes: map[string]*process{}}
	t.Cleanup(func() {
		if s.bundleDirectory != nil {
			_ = s.bundleDirectory.Close()
		}
	})
	return s
}

func TestCreateIntentPrivatePublicationAndExactRemoval(t *testing.T) {
	s := intentTestOwner(t)
	if err := s.beginCreateIntent(); err != nil {
		t.Fatal("intent publication failed")
	}
	path := filepath.Join(s.bundle, createIntentName)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("intent mode is not private")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 1024 {
		t.Fatal("intent record exceeds bound")
	}
	var record map[string]any
	if json.Unmarshal(raw, &record) != nil || len(record) != 4 || record["version"] != float64(1) || record["namespace"] != "default" || record["task"] != "task" || record["bundle_identity"] == nil {
		t.Fatal("unsafe intent schema")
	}
	if err = s.refusePendingCreate(); err == nil {
		t.Fatal("own pending intent accepted")
	}
	if err = s.clearCreateIntent(); err != nil || s.createIntentIdentity != nil {
		t.Fatal("exact intent removal failed")
	}
	if err = s.refusePendingCreate(); err != nil {
		t.Fatal("completed intent still blocks")
	}
	if err = s.clearCreateIntent(); err != nil {
		t.Fatal("completed removal not idempotent")
	}
}

func TestCreateIntentRefusesForeignChangedAndQuarantinedEntries(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "directory", "replacement", "quarantine", "inventory-bound"} {
		t.Run(kind, func(t *testing.T) {
			s := intentTestOwner(t)
			path := filepath.Join(s.bundle, createIntentName)
			switch kind {
			case "file":
				if err := os.WriteFile(path, []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink("/nonexistent/private", path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "replacement", "quarantine":
				if err := s.beginCreateIntent(); err != nil {
					t.Fatal(err)
				}
				target := path + ".held"
				if kind == "quarantine" {
					digest := sha256.Sum256([]byte(createIntentName))
					i := s.createIntentIdentity
					target = filepath.Join(s.bundle, fmt.Sprintf(".mklinux-remove-%s-%016x-%016x", hex.EncodeToString(digest[:8]), i.Device, i.Inode))
				}
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if kind == "replacement" {
					if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
						t.Fatal(err)
					}
					if s.clearCreateIntent() == nil {
						t.Fatal("replacement was removed")
					}
				}
			case "inventory-bound":
				for i := 0; i < 257; i++ {
					if err := os.WriteFile(filepath.Join(s.bundle, fmt.Sprint(i)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := s.refusePendingCreate(); err == nil || strings.Contains(err.Error(), s.bundle) {
				t.Fatal("uncertain intent not privately refused")
			}
			if err := s.beginCreateIntent(); err == nil {
				t.Fatal("existing or uncertain intent overwritten")
			}
		})
	}
}

func TestCreateIntentSurvivesOwnedProcessExit(t *testing.T) {
	if os.Getenv("MK_CREATE_INTENT_EXIT_CHILD") == "1" {
		s := &service{id: "task", namespace: "default", bundle: os.Getenv("MK_CREATE_INTENT_BUNDLE")}
		if s.beginCreateIntent() != nil {
			os.Exit(31)
		}
		os.Exit(23)
	}
	s := intentTestOwner(t)
	command := exec.Command(os.Args[0], "-test.run=^TestCreateIntentSurvivesOwnedProcessExit$")
	command.Env = append(os.Environ(), "MK_CREATE_INTENT_EXIT_CHILD=1", "MK_CREATE_INTENT_BUNDLE="+s.bundle)
	output, err := command.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 23 || len(output) != 0 {
		t.Fatal("owned child failed intent publication")
	}
	if s.refusePendingCreate() == nil || s.createIntentIdentity != nil {
		t.Fatal("new service forgot or adopted crashed intent")
	}
	if s.clearCreateIntent() != nil {
		t.Fatal("unowned clear should be a no-op")
	}
	if s.refusePendingCreate() == nil {
		t.Fatal("new service removed crashed intent")
	}
}

func TestPendingCreateBlocksStartAndTaskDelete(t *testing.T) {
	s := intentTestOwner(t)
	s.createCleanupPending = true
	p := &process{status: tasktypes.Status_CREATED}
	s.processes[""] = p
	if result, err := s.Start(t.Context(), &taskapi.StartRequest{ID: "task"}); result != nil || err == nil {
		t.Fatal("incomplete Create admitted Start")
	}
	if result, err := s.Delete(t.Context(), &taskapi.DeleteRequest{ID: "task"}); result != nil || err == nil {
		t.Fatal("incomplete Create admitted Delete")
	}
	if s.processes[""] != p || p.deleting {
		t.Fatal("refusal changed task")
	}
}
