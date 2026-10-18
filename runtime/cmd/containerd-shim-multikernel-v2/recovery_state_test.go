//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	tasktypes "github.com/containerd/containerd/api/types/task"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func TestRecoveryStatePersistsGenerationProcessesAndOffsetsAtomically(t *testing.T) {
	bundle := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	stdout := filepath.Join(bundle, "stdout")
	if err := os.WriteFile(stdout, nil, 0600); err != nil {
		t.Fatal(err)
	}
	stdoutIdentity, err := inspectBoundProcessIOPath(stdout, false)
	if err != nil {
		t.Fatal(err)
	}
	stdin := filepath.Join(bundle, "stdin")
	if err = syscall.Mkfifo(stdin, 0600); err != nil {
		t.Fatal(err)
	}
	stdinIdentity, err := inspectBoundProcessIOPath(stdin, true)
	if err != nil {
		t.Fatal(err)
	}
	s := &service{
		bundle: bundle, bundleIdentity: testBundleIdentity(t, bundle),
		sandbox: protocol.Sandbox{ID: "box", Generation: "0123456789abcdef0123456789abcdef"},
		processes: map[string]*process{
			"": {id: "", pid: 7, status: tasktypes.Status_RUNNING, stdin: stdin, stdinIdentity: stdinIdentity,
				stdout: stdout, stdoutIdentity: stdoutIdentity, stdinClosed: true,
				stdinOffset: 5, stdinDrainRemaining: 9, stdinPending: []byte("pending"), stdoutOffset: 123, stderrOffset: 45,
				exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{})},
		},
	}
	if err := s.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SchemaVersion != 4 || saved.BundleIdentity != s.bundleIdentity || saved.Generation != s.sandbox.Generation || len(saved.Processes) != 1 || saved.Processes[0].PID != 7 || !saved.Processes[0].StdinClosed || saved.Processes[0].StdinCloseAcked || !saved.Processes[0].ExitEventQueued || !saved.Processes[0].DeleteEventQueued || saved.Processes[0].StdinOffset != 5 || saved.Processes[0].StdinDrainRemaining != 9 || string(saved.Processes[0].StdinPending) != "pending" || saved.Processes[0].StdinIdentity != stdinIdentity || saved.Processes[0].StdoutOffset != 123 || saved.Processes[0].StdoutIdentity != stdoutIdentity {
		t.Fatalf("persisted recovery = %+v", saved)
	}
	temporary, err := filepath.Glob(filepath.Join(bundle, ".multikernel", ".sandbox.json.*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary recovery files = %v, error = %v", temporary, err)
	}
}

func TestRecoveryStateRejectsRuntimeDirectoryReplacement(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	s := &service{bundle: bundle, sandbox: protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)},
		processes: map[string]*process{"": {pid: 7, status: tasktypes.Status_RUNNING}}}
	if err := s.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(bundle, ".multikernel-held")
	if err := os.Rename(runtimeDir, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	s.processes[""].pid = 8
	if err := s.persistRecovery(); err == nil || !strings.Contains(err.Error(), "runtime directory identity changed") {
		t.Fatalf("recovery write after runtime directory replacement = %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "sandbox.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement runtime directory received recovery state: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(held, "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.PID != 7 {
		t.Fatalf("held recovery PID = %d, want original 7", saved.PID)
	}
}

func TestRuntimeDirectoryHandoffRejectsPreparedReplacement(t *testing.T) {
	bundle := privateTestDirectory(t)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	expected := testBundleIdentity(t, runtimeDir)
	if err := os.Rename(runtimeDir, runtimeDir+"-prepared"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	s := &service{bundle: bundle}
	directory, err := s.ensureRuntimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyRuntimeDirectoryHandoff(directory, expected); err == nil || !strings.Contains(err.Error(), "rootfs handoff") {
		t.Fatalf("prepared runtime replacement = %v", err)
	}
}

func validPersistedRecovery(namespace, task string) persisted {
	return persisted{
		SchemaVersion:  3,
		BundleIdentity: rootfspkg.DirectoryIdentity{Device: 1, Inode: 2, UID: uint32(os.Geteuid())},
		ID:             sandboxID(namespace, task),
		Generation:     strings.Repeat("a", 32),
		TaskIdentity:   storageTaskIdentity(namespace, task),
		Processes:      []persistedProcess{{ID: "", Status: tasktypes.Status_STOPPED}},
	}
}

func TestRecoveryStateIsBoundedStrictAndIdentityBound(t *testing.T) {
	namespace, task := "default", "task-a"
	valid := validPersistedRecovery(namespace, task)
	encode := func(t *testing.T, value persisted) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, found, err := loadPersistedRecovery(path, namespace, task)
		if err != nil || !found || loaded.ID != valid.ID || loaded.Generation != valid.Generation {
			t.Fatalf("loaded recovery = %+v, found=%v error=%v", loaded, found, err)
		}
	})

	t.Run("schema 4 close drain", func(t *testing.T) {
		value := valid
		value.SchemaVersion = 4
		value.Processes = append([]persistedProcess(nil), valid.Processes...)
		process := &value.Processes[0]
		process.Status = tasktypes.Status_RUNNING
		process.Stdin = "/run/containerd/synthetic-stdin"
		process.StdinIdentity = processIOIdentity{Device: 1, Inode: 3, UID: uint32(os.Geteuid()), Links: 1, Mode: unix.S_IFIFO | 0600}
		process.StdinClosed, process.StdinDrainRemaining = true, 9
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, found, err := loadPersistedRecovery(path, namespace, task)
		if err != nil || !found || loaded.Processes[0].StdinDrainRemaining != 9 {
			t.Fatalf("close drain loaded=%+v found=%v err=%v", loaded, found, err)
		}
		value.SchemaVersion = 3
		if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("schema 3 accepted a schema 4 close drain")
		}
	})

	t.Run("valid stopped signal acknowledgement", func(t *testing.T) {
		value := valid
		value.Processes = append([]persistedProcess(nil), valid.Processes...)
		value.Processes[0].PendingSignal = uint32(syscall.SIGTERM)
		value.Processes[0].PendingSignalOp = strings.Repeat("f", 32)
		value.Processes[0].PendingSignalAck = true
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := loadPersistedRecovery(path, namespace, task); err != nil || !found {
			t.Fatalf("stopped signal acknowledgement found=%v error=%v", found, err)
		}
	})

	for name, mutate := range map[string]func(*persisted){
		"legacy schema":    func(value *persisted) { value.SchemaVersion = 1 },
		"wrong sandbox":    func(value *persisted) { value.ID = sandboxID(namespace, "other") },
		"wrong generation": func(value *persisted) { value.Generation = "short" },
		"wrong task owner": func(value *persisted) { value.TaskIdentity = storageTaskIdentity(namespace, "other") },
		"partial network":  func(value *persisted) { value.Network.SandboxID = value.ID },
		"duplicate process": func(value *persisted) {
			value.Processes = append(value.Processes, value.Processes[0])
		},
		"invalid state": func(value *persisted) { value.Processes[0].Status = tasktypes.Status_UNKNOWN },
		"stdio without ownership": func(value *persisted) {
			value.Processes[0].Stdout = "/run/containerd/fifo"
		},
		"stdin offset without stream": func(value *persisted) {
			value.Processes[0].StdinOffset = 1
		},
		"oversized pending stdin": func(value *persisted) {
			value.Processes[0].StdinPending = make([]byte, (32<<10)+1)
		},
		"oversized close drain": func(value *persisted) {
			value.Processes[0].StdinDrainRemaining = maxStdinCloseDrain + 1
		},
		"drain without close": func(value *persisted) {
			value.Processes[0].StdinDrainRemaining = 1
		},
		"signal without operation": func(value *persisted) {
			value.Processes[0].PendingSignal = uint32(syscall.SIGTERM)
		},
		"signal acknowledgement without operation": func(value *persisted) {
			value.Processes[0].PendingSignalAck = true
		},
		"malformed signal operation": func(value *persisted) {
			value.Processes[0].Status = tasktypes.Status_RUNNING
			value.Processes[0].PendingSignalOp = "short"
		},
		"signal operation on stopped process": func(value *persisted) {
			value.Processes[0].PendingSignal = uint32(syscall.SIGTERM)
			value.Processes[0].PendingSignalOp = strings.Repeat("e", 32)
		},
		"unsupported pending signal": func(value *persisted) {
			value.Processes[0].Status = tasktypes.Status_RUNNING
			value.Processes[0].PendingSignal = 65
			value.Processes[0].PendingSignalOp = strings.Repeat("e", 32)
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			value.Processes = append([]persistedProcess(nil), valid.Processes...)
			mutate(&value)
			path := filepath.Join(t.TempDir(), "sandbox.json")
			if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
				t.Fatalf("unsafe recovery accepted: %+v", value)
			}
		})
	}

	t.Run("unknown field", func(t *testing.T) {
		data := encode(t, valid)
		data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("recovery with unknown field was accepted")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		if err := os.WriteFile(target, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "sandbox.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("symlinked recovery was accepted")
		}
	})
}
