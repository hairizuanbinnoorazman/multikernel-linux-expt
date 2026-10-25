//go:build linux

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectProcessPreservesStaleEvidenceAndBackendState(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	value := validBackendLease("/unavailable/backing.ext4")
	backend := &LinuxBackend{RuntimeDir: directory}
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	record := processRecord{Version: 4, PID: os.Getpid(), StartTime: start + 1, Path: value.Path, Port: value.Port,
		ImageID: value.ImageID, ExportGeneration: value.ExportGeneration, PeerCID: value.PeerCID,
		ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode, BinaryDevice: 3, BinaryInode: 4}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := backend.paths(value)
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	// The numeric PID belongs to this test; the wrong incarnation is never
	// signalled. Unlike Observe, inspection leaves even stale evidence intact.
	for i := 0; i < 3; i++ {
		got, err := backend.InspectProcess(context.Background(), value)
		if err != nil || got.Status != "process-absent" || got.Generation != value.ExportGeneration {
			t.Fatalf("inspection = %+v, %v", got, err)
		}
		current, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(current, data) {
			t.Fatal("stale evidence changed")
		}
	}
	if backend.Binary != "" || backend.managed != nil || backend.runtime.Device != 0 {
		t.Fatal("inspection initialized backend state")
	}
	backend.mu.Lock()
	_, err = backend.InspectProcess(context.Background(), value)
	backend.mu.Unlock()
	if err == nil {
		t.Fatal("busy backend did not refuse")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = backend.InspectProcess(ctx, value); err != context.Canceled {
		t.Fatalf("cancel error = %v", err)
	}
	conflict := value
	conflict.PeerCID++
	if _, err = backend.InspectProcess(context.Background(), conflict); err == nil {
		t.Fatal("conflicting CID accepted")
	}
	if err = os.Link(name, name+".link"); err != nil {
		t.Fatal(err)
	}
	if _, err = backend.InspectProcess(context.Background(), value); err == nil {
		t.Fatal("multiply linked record accepted")
	}
	if err = os.Remove(name + ".link"); err != nil {
		t.Fatal(err)
	}
	// Recovery's quarantine fallback is deliberately excluded from inspection.
	if err = os.Rename(name, name+".quarantined"); err != nil {
		t.Fatal(err)
	}
	got, err := backend.InspectProcess(context.Background(), value)
	if err != nil || got.Status != "record-missing" {
		t.Fatalf("missing exact record = %+v, %v", got, err)
	}
	current, err := os.ReadFile(name + ".quarantined")
	if err != nil || !bytes.Equal(current, data) {
		t.Fatal("quarantined record changed")
	}
	if err = os.Symlink(name+".quarantined", name); err != nil {
		t.Fatal(err)
	}
	if _, err = backend.InspectProcess(context.Background(), value); err == nil {
		t.Fatal("symlink record accepted")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatalf("inspection created or removed files: %v, %v", entries, err)
	}
}

func TestInspectProcessMissingRuntimeDoesNotCreateDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "missing")
	backend := &LinuxBackend{RuntimeDir: directory}
	got, err := backend.InspectProcess(context.Background(), validBackendLease("/unavailable/backing.ext4"))
	if err != nil || got.Status != "record-missing" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	if _, err = os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("missing directory changed: %v", err)
	}
}

// Recovery legitimately removes this stale record; diagnostic inspection must
// preserve it even when an exact clean terminal log is available.
func TestInspectProcessDoesNotPerformClosedRecordRecovery(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	value := validBackendLease("/unavailable/backing.ext4")
	backend := &LinuxBackend{RuntimeDir: directory}
	record := processRecord{Version: 4, PID: 1 << 30, StartTime: 1, Path: value.Path, Port: value.Port,
		ImageID: value.ImageID, ExportGeneration: value.ExportGeneration, PeerCID: value.PeerCID,
		ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode, BinaryDevice: 3, BinaryInode: 4}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	name, logName := backend.paths(value)
	if err = os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	log := append(readyMarker(value), []byte("MKNBD_SERVER_CLOSED synced=1 reads=0 read_bytes=0 writes=0 write_bytes=0 flushes=0\n")...)
	if err = os.WriteFile(logName, log, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := backend.InspectProcess(t.Context(), value)
	if err != nil || got.Status != "process-absent" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	retained, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(retained, data) {
		t.Fatal("inspection recovered stale record")
	}
	retained, err = os.ReadFile(logName)
	if err != nil || !bytes.Equal(retained, log) {
		t.Fatal("inspection changed terminal log")
	}
	observed, err := backend.Observe(t.Context(), value)
	if err != nil || !observed.Closed {
		t.Fatalf("recovery control = %+v, %v", observed, err)
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("control did not demonstrate recovery mutation")
	}
}
