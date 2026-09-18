package unixsocket

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func privateTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestListenerBindsAndCleansExactSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "service.sock")
	listener, err := Listen(path, 0660)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			acceptErr = connection.Close()
		}
		accepted <- acceptErr
	}()
	connection, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-accepted; err != nil {
		t.Fatal(err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed listener path remains: %v", err)
	}
}

func TestListenerRejectsUnsafeStalePaths(t *testing.T) {
	for name, install := range map[string]func(string) error{
		"regular": func(path string) error { return os.WriteFile(path, []byte("owned"), 0600) },
		"symlink": func(path string) error { return os.Symlink("missing", path) },
		"wrong mode socket": func(path string) error {
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
			if err != nil {
				return err
			}
			listener.SetUnlinkOnClose(false)
			if err = os.Chmod(path, 0600); err != nil {
				_ = listener.Close()
				return err
			}
			return listener.Close()
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(privateTempDir(t), "service.sock")
			if err := install(path); err != nil {
				if errors.Is(err, syscall.EPERM) {
					t.Skip("sandbox forbids Unix pathname listeners")
				}
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if listener, listenErr := Listen(path, 0660); listenErr == nil {
				_ = listener.Close()
				t.Fatal("unsafe stale path was replaced")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("rejected stale path changed: %v", err)
			}
		})
	}
}

func TestListenerReplacesOnlySafeStaleSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "service.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	if err = stale.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := Listen(path, 0660)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExclusiveListenerPreservesLiveSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "service.sock")
	live, err := ListenExclusive(path, 0600)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	if second, secondErr := ListenExclusive(path, 0600); !errors.Is(secondErr, syscall.EADDRINUSE) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("second exclusive listener error = %v", secondErr)
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("live listener was disrupted: %v", err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err = live.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCapturedPathClosePreservesSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "service.sock")
	live, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	live.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	owner, err := Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(path); err != nil {
		t.Fatalf("released live socket was removed: %v", err)
	}
	if err = live.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestListenerCleanupPreservesReplacement(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "service.sock")
	listener, err := Listen(path, 0660)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	moved := path + ".moved"
	if err = os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = listener.Close(); err == nil {
		t.Fatal("replaced socket cleanup succeeded")
	}
	if data, readErr := os.ReadFile(path); readErr != nil || string(data) != "replacement" {
		t.Fatalf("replacement path = %q, %v", data, readErr)
	}
}

func TestCapturedPathRemovesOnlyCapturedSocket(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "relay.sock")
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
	owner, err := Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := owner.Dial()
	if err != nil {
		t.Fatalf("dial captured socket: %v", err)
	}
	if err = connection.Close(); err != nil {
		t.Fatal(err)
	}
	moved := path + ".original"
	if err = os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	replacement.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if connection, dialErr := owner.Dial(); dialErr == nil {
		_ = connection.Close()
		t.Fatal("captured owner dialed a replacement")
	}
	if err = owner.Remove(); err == nil {
		t.Fatal("captured owner removed a replacement")
	}
	if _, err = os.Lstat(path); err != nil {
		t.Fatalf("replacement was not preserved: %v", err)
	}
	if err = replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(moved, path); err != nil {
		t.Fatal(err)
	}
	if err = owner.Remove(); err != nil {
		t.Fatal(err)
	}
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSocketRemovalQuarantineRestoresRemovalTimeReplacement(t *testing.T) {
	path := filepath.Join(privateTempDir(t), "raced.sock")
	original, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids Unix pathname listeners")
	}
	if err != nil {
		t.Fatal(err)
	}
	original.SetUnlinkOnClose(false)
	if err = os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	directory, base, err := openParent(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	expected, found, err := inspectAt(directory, base, 0660)
	if err != nil || !found {
		t.Fatalf("original socket identity = %+v, %v, %v", expected, found, err)
	}
	moved := path + ".original"
	var replacement *net.UnixListener
	removed, err := removeIdentityAtWithHook(directory, base, expected, func() {
		if renameErr := os.Rename(path, moved); renameErr != nil {
			t.Fatal(renameErr)
		}
		var listenErr error
		replacement, listenErr = net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		replacement.SetUnlinkOnClose(false)
		if chmodErr := os.Chmod(path, 0660); chmodErr != nil {
			t.Fatal(chmodErr)
		}
	})
	if removed || err == nil {
		t.Fatalf("removal-time socket replacement = removed:%v err:%v", removed, err)
	}
	if _, err = os.Lstat(path); err != nil {
		t.Fatalf("replacement socket was not restored: %v", err)
	}
	if _, err = os.Lstat(moved); err != nil {
		t.Fatalf("original socket was changed: %v", err)
	}
	if err = replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = original.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(moved); err != nil {
		t.Fatal(err)
	}
}

func TestSocketRemovalQuarantineAlgorithmWithoutListenerPrivilege(t *testing.T) {
	directoryPath := privateTempDir(t)
	path := filepath.Join(directoryPath, "synthetic.sock")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, base, err := openParent(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	expected, found, err := rawIdentityAt(directory, base)
	if err != nil || !found {
		t.Fatalf("synthetic identity = %+v, %v, %v", expected, found, err)
	}
	moved := path + ".original"
	removed, err := removeIdentityAtWithHook(directory, base, expected, func() {
		if renameErr := os.Rename(path, moved); renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(path, []byte("replacement"), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if removed || err == nil {
		t.Fatalf("synthetic removal-time replacement = removed:%v err:%v", removed, err)
	}
	if value, readErr := os.ReadFile(path); readErr != nil || string(value) != "replacement" {
		t.Fatalf("synthetic replacement = %q, %v", value, readErr)
	}
	if value, readErr := os.ReadFile(moved); readErr != nil || string(value) != "original" {
		t.Fatalf("synthetic original = %q, %v", value, readErr)
	}
}

func TestEnsureParentCreatesWithoutFollowingOrRechmoddingAncestors(t *testing.T) {
	base := privateTempDir(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "one", "two", "service.sock")
	if err := EnsureParent(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(base); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("existing ancestor mode = %v, %v", info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() || info.Mode().Perm() != 0755 {
		t.Fatalf("created parent = %v, %v", info, err)
	}
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(base, "linked")
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	if err := EnsureParent(filepath.Join(linked, "redirected", "service.sock")); err == nil {
		t.Fatal("symlinked parent ancestry was accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "redirected")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target was mutated: %v", err)
	}
}

func TestEnsureParentRejectsRootAsSocketPath(t *testing.T) {
	if err := EnsureParent("/"); err == nil {
		t.Fatal("root was accepted as a socket pathname")
	}
}
