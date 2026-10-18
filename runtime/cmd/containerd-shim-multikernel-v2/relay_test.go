//go:build linux

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestRelayOwnershipKillsDescendantsAndRetainsSocketCleanup(t *testing.T) {
	t.Run("failed capture never publishes typed nil", func(t *testing.T) {
		service := &service{newRelayOwner: func(string) (relayPathOwner, error) {
			var owner *unixsocket.Path
			return owner, os.ErrNotExist
		}}
		if err := service.acquireRelaySocketOwner("/run/missing.sock"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("capture error = %v", err)
		}
		if service.relayOwner != nil {
			t.Fatalf("failed capture published owner %#v", service.relayOwner)
		}
	})

	t.Run("default is approved artifact location", func(t *testing.T) {
		t.Setenv("MK_RELAY", "")
		command := (&service{}).relayCommand(7001, "/tmp/mk-relay-test.sock")
		if command.Path != "/opt/mkruntime/bin/mkvsock-relay" {
			t.Fatalf("default relay path = %q", command.Path)
		}
		t.Setenv("MK_RELAY", "/opt/test/explicit-relay")
		command = (&service{}).relayCommand(7001, "/tmp/mk-relay-test.sock")
		if command.Path != "/opt/test/explicit-relay" {
			t.Fatalf("explicit relay path = %q", command.Path)
		}
	})

	t.Run("command has private process group", func(t *testing.T) {
		command := newRelayCommand("/bin/true", 7001, "/tmp/mk-relay-test.sock")
		if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
			t.Fatalf("relay process attributes = %+v", command.SysProcAttr)
		}
	})

	t.Run("stop kills group and removes socket", func(t *testing.T) {
		command := exec.Command("/bin/sh", "-c", "sleep 60 & wait")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		processGroup := command.Process.Pid
		socketDirectory := t.TempDir()
		if err := os.Chmod(socketDirectory, 0700); err != nil {
			t.Fatal(err)
		}
		socket := filepath.Join(socketDirectory, "relay.sock")
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
		if errors.Is(err, syscall.EPERM) {
			t.Skip("sandbox forbids Unix pathname listeners")
		}
		if err != nil {
			t.Fatal(err)
		}
		listener.SetUnlinkOnClose(false)
		if err = os.Chmod(socket, 0755); err != nil {
			t.Fatal(err)
		}
		if err = listener.Close(); err != nil {
			t.Fatal(err)
		}
		s := &service{relay: command, relaySocket: socket}
		if err := s.stopRelay(); err != nil {
			t.Fatal(err)
		}
		if s.relay != nil || s.relaySocket != "" {
			t.Fatalf("relay ownership remains: command=%v socket=%q", s.relay, s.relaySocket)
		}
		if _, err := os.Stat(socket); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("relay socket remains: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			err := syscall.Kill(-processGroup, syscall.Signal(0))
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("relay process group %d remains: %v", processGroup, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("socket cleanup remains retryable", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "relay.sock")
		if err := os.Mkdir(socket, 0700); err != nil {
			t.Fatal(err)
		}
		occupied := filepath.Join(socket, "occupied")
		if err := os.WriteFile(occupied, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		s := &service{relaySocket: socket}
		if err := s.stopRelay(); err == nil || s.relaySocket != socket {
			t.Fatalf("failed socket cleanup: error=%v retained=%q", err, s.relaySocket)
		}
		if err := os.Remove(occupied); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(socket); err != nil {
			t.Fatal(err)
		}
		if err := s.stopRelay(); err != nil || s.relaySocket != "" {
			t.Fatalf("socket cleanup retry: error=%v retained=%q", err, s.relaySocket)
		}
	})
}

func TestExitedRelayReplacementIsSingleOwnerAcrossConcurrentObservers(t *testing.T) {
	old := exec.Command("/bin/true")
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	if err := old.Wait(); err != nil {
		t.Fatal(err)
	}
	var countsMu sync.Mutex
	starts, oldRemovals, newCaptures, newRemovals := 0, 0, 0, 0
	s := &service{
		relay: old, relaySocket: "/run/multikernel/relay.sock",
		relayOwner: relayOwnerFunc(func() error {
			countsMu.Lock()
			oldRemovals++
			countsMu.Unlock()
			return nil
		}),
		sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{AgentPort: 7200}},
		newRelay: func(uint32, string) *exec.Cmd {
			countsMu.Lock()
			starts++
			countsMu.Unlock()
			command := exec.Command("/bin/sleep", "300")
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return command
		},
		newRelayOwner: func(string) (relayPathOwner, error) {
			countsMu.Lock()
			newCaptures++
			countsMu.Unlock()
			return relayOwnerFunc(func() error {
				countsMu.Lock()
				newRemovals++
				countsMu.Unlock()
				return nil
			}), nil
		},
	}
	const observers = 16
	errorsByObserver := make(chan error, observers)
	var observersWG sync.WaitGroup
	for range observers {
		observersWG.Add(1)
		go func() {
			defer observersWG.Done()
			replaced, err := s.replaceExitedRelay(context.Background(), 0)
			if err == nil && !replaced {
				err = errors.New("exited relay was not replaced")
			}
			errorsByObserver <- err
		}()
	}
	observersWG.Wait()
	close(errorsByObserver)
	for err := range errorsByObserver {
		if err != nil {
			t.Fatal(err)
		}
	}
	countsMu.Lock()
	observedStarts, observedOldRemovals, observedCaptures := starts, oldRemovals, newCaptures
	countsMu.Unlock()
	if observedStarts != 1 || observedOldRemovals != 1 || observedCaptures != 1 || s.relayGeneration != 1 {
		t.Fatalf("replacement ownership = starts:%d old-removals:%d captures:%d generation:%d",
			observedStarts, observedOldRemovals, observedCaptures, s.relayGeneration)
	}
	if err := s.stopRelay(); err != nil {
		t.Fatal(err)
	}
	countsMu.Lock()
	defer countsMu.Unlock()
	if newRemovals != 1 {
		t.Fatalf("replacement socket removals = %d, want 1", newRemovals)
	}
}

func TestAgentReconnectFailureReplacesExitedRelayBeforeRetry(t *testing.T) {
	old := exec.Command("/bin/true")
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	if err := old.Wait(); err != nil {
		t.Fatal(err)
	}
	client := &relayReplacementAgent{}
	starts := 0
	s := &service{
		agent: client, relay: old, relaySocket: "/run/multikernel/relay.sock", ioCallTimeout: time.Second,
		relayOwner: relayOwnerFunc(func() error { return nil }),
		sandbox:    protocol.Sandbox{Config: protocol.SandboxConfig{AgentPort: 7200}},
		newRelay: func(uint32, string) *exec.Cmd {
			starts++
			command := exec.Command("/bin/sleep", "300")
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return command
		},
		newRelayOwner: func(string) (relayPathOwner, error) {
			return relayOwnerFunc(func() error { return nil }), nil
		},
	}
	var state agent.ProcessState
	if err := s.callAgentWithReconnect("StateProcess", map[string]string{"ID": "init"}, &state); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	calls, reconnects := client.calls, client.reconnects
	client.mu.Unlock()
	if state.ID != "init" || state.PID != 41 || state.Status != "RUNNING" || calls != 3 || reconnects != 2 || starts != 1 || s.relayGeneration != 1 {
		t.Fatalf("relay recovery = state:%+v calls:%d reconnects:%d starts:%d generation:%d",
			state, calls, reconnects, starts, s.relayGeneration)
	}
	if err := s.stopRelay(); err != nil {
		t.Fatal(err)
	}
}

func TestExitedRelayReplacementCancellationReapsUnpublishedCommand(t *testing.T) {
	old := exec.Command("/bin/true")
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	if err := old.Wait(); err != nil {
		t.Fatal(err)
	}
	var replacementPID int
	pidMarker := filepath.Join(t.TempDir(), "replacement.pid")
	s := &service{
		relay: old, relaySocket: "/run/multikernel/relay.sock",
		relayOwner: relayOwnerFunc(func() error { return nil }),
		sandbox:    protocol.Sandbox{Config: protocol.SandboxConfig{AgentPort: 7200}},
		newRelay: func(uint32, string) *exec.Cmd {
			// Publish a complete PID atomically; mere file existence after
			// redirection is not evidence that printf has written its bytes.
			command := exec.Command("/bin/sh", "-c", "printf '%s\\n' \"$$\" > \"$1.tmp\"; /bin/mv -- \"$1.tmp\" \"$1\"; exec /bin/sleep 300", "relay", pidMarker)
			command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			return command
		},
		newRelayOwner: func(string) (relayPathOwner, error) { return nil, os.ErrNotExist },
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.replaceExitedRelay(ctx, 0)
		done <- err
	}()
	for {
		if data, err := os.ReadFile(pidMarker); err == nil {
			replacementPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatalf("replacement completed before publishing command: %v", err)
		case <-time.After(time.Millisecond):
		}
	}
	if err := <-done; err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled replacement error = %v", err)
	}
	if s.relay != nil || s.relayGeneration != 0 {
		t.Fatalf("cancelled replacement retained relay=%v generation=%d", s.relay, s.relayGeneration)
	}
	if err := syscall.Kill(replacementPID, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("cancelled replacement process %d remains: %v", replacementPID, err)
	}
}

func TestStaleRelayCleanupRejectsNonSocketPathsWithoutRemoval(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sock")
	if err := removeStaleRelaySocket(missing); err != nil {
		t.Fatalf("missing stale socket error = %v", err)
	}
	for name, create := range map[string]func(string) error{
		"regular":   func(path string) error { return os.WriteFile(path, []byte("owned data"), 0600) },
		"directory": func(path string) error { return os.Mkdir(path, 0700) },
		"symlink":   func(path string) error { return os.Symlink("missing-target", path) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "relay.sock")
			if err := create(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = removeStaleRelaySocket(path); err == nil {
				t.Fatal("non-socket stale relay path was accepted")
			}
			after, statErr := os.Lstat(path)
			if statErr != nil || before.Mode() != after.Mode() || !os.SameFile(before, after) {
				t.Fatalf("rejected path changed: before=%+v after=%+v error=%v", before, after, statErr)
			}
		})
	}
}

func TestStaleRelayCleanupRemovesExactSafeSocket(t *testing.T) {
	path := filepath.Join(privateTestDirectory(t), "relay.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err = removeStaleRelaySocket(path); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale relay remains: %v", err)
	}
}
