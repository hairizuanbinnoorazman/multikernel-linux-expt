//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPublishShimFileIsExclusiveAndOwnsExactCreatedIdentity(t *testing.T) {
	directory := privateTestDirectory(t)
	path := filepath.Join(directory, "address")
	if err := os.WriteFile(path, []byte("pre-existing"), 0644); err != nil {
		t.Fatal(err)
	}
	if owner, err := publishShimFile(path, []byte("new")); err == nil || owner != nil {
		t.Fatalf("exclusive publication = %v, %v", owner, err)
	}
	if value, err := os.ReadFile(path); err != nil || string(value) != "pre-existing" {
		t.Fatalf("pre-existing publication = %q, %v", value, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	owner, err := publishShimFile(path, []byte("owned"))
	if err != nil {
		t.Fatal(err)
	}
	original := path + ".original"
	if err = os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = owner.close(true); err == nil {
		t.Fatal("rollback removed a replacement")
	}
	if value, err := os.ReadFile(path); err != nil || string(value) != "replacement" {
		t.Fatalf("replacement publication = %q, %v", value, err)
	}
	if value, err := os.ReadFile(original); err != nil || string(value) != "owned" {
		t.Fatalf("owned publication = %q, %v", value, err)
	}
}

func TestLaunchShimWorkerCleansProcessGroupAndOwnedArtifactsOnPIDFailure(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	address := "unix://" + filepath.Join(directory, "shim.sock")
	inheritedPath := filepath.Join(directory, "inherited-descriptor")
	if err := os.WriteFile(inheritedPath, []byte("descriptor"), 0600); err != nil {
		t.Fatal(err)
	}
	addressPath := filepath.Join(directory, "address")
	pidPath := filepath.Join(directory, "shim.pid")
	if err := os.Mkdir(pidPath, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	launchErr := launchShimWorker(context.Background(), cmd, inheritedFileSocket(inheritedPath), address, addressPath, pidPath)
	if launchErr == nil {
		t.Fatal("PID-file failure was accepted")
	}
	if _, statErr := os.Lstat(addressPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial address file survived: stat=%v launch=%v", statErr, launchErr)
	}
	if info, statErr := os.Stat(pidPath); statErr != nil || !info.IsDir() {
		t.Fatalf("pre-existing PID path was removed or changed: %+v, %v", info, statErr)
	}
	if cmd.Process == nil {
		t.Fatal("worker did not reach the injected post-start failure")
	}
	deadline := time.Now().Add(time.Second)
	for {
		killErr := syscall.Kill(-cmd.Process.Pid, 0)
		if errors.Is(killErr, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker process group survived cleanup: %v", killErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLaunchShimWorkerPreservesReplacedAddressOnFailure(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	addressPath := filepath.Join(directory, "address")
	pidPath := filepath.Join(directory, "shim.pid")
	originalPath := addressPath + ".original"
	socket := shimSocketFileFunc(func() (*os.File, error) {
		if err := os.Rename(addressPath, originalPath); err != nil {
			return nil, err
		}
		if err := os.WriteFile(addressPath, []byte("replacement"), 0644); err != nil {
			return nil, err
		}
		return nil, errors.New("injected inherited-socket failure")
	})
	err := launchShimWorker(context.Background(), exec.Command("/bin/true"), socket,
		"unix:///owned", addressPath, pidPath)
	if err == nil || !strings.Contains(err.Error(), "file identity changed before removal") {
		t.Fatalf("launch replacement error = %v", err)
	}
	if value, readErr := os.ReadFile(addressPath); readErr != nil || string(value) != "replacement" {
		t.Fatalf("replacement address = %q, %v", value, readErr)
	}
	if value, readErr := os.ReadFile(originalPath); readErr != nil || string(value) != "unix:///owned" {
		t.Fatalf("original address = %q, %v", value, readErr)
	}
}
