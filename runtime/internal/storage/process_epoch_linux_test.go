//go:build linux

package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func currentStorageBoot(t *testing.T) string {
	t.Helper()
	value, err := storageBootIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// A canonical close log must not turn a prior-boot record into a recovered
// session. Exercise all consumers, including their durable record side effects.
func TestStorageProcessEpochRefusesHistoricalRecovery(t *testing.T) {
	boot := currentStorageBoot(t)
	other := "11111111-1111-4111-8111-111111111111"
	if other == boot {
		other = "22222222-2222-4222-8222-222222222222"
	}
	for _, tc := range []struct {
		name    string
		version int
		boot    string
	}{
		{"prior-boot", 5, other}, {"legacy-v4", 4, ""},
		{"missing-boot", 5, ""}, {"malformed-boot", 5, "private-invalid-canary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			value := validBackendLease("/unavailable/backing.ext4")
			backend := &LinuxBackend{RuntimeDir: directory}
			// This PID cannot exist. A boot check after pidfd lookup would silently
			// treat the process as absent and consume the terminal close log.
			record := processRecord{Version: tc.version, BootID: tc.boot, PID: 1 << 30, StartTime: 1,
				Path: value.Path, Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration,
				PeerCID: value.PeerCID, ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode,
				BinaryDevice: 3, BinaryInode: 4}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			path, logPath := backend.paths(value)
			log := append(readyMarker(value), []byte("MKNBD_SERVER_CLOSED synced=1 reads=0 read_bytes=0 writes=0 write_bytes=0 flushes=0\n")...)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(logPath, log, 0600); err != nil {
				t.Fatal(err)
			}
			for _, operation := range []string{"inspect", "observe", "stop"} {
				var err error
				switch operation {
				case "inspect":
					_, err = backend.InspectProcess(t.Context(), value)
				case "observe":
					_, err = backend.Observe(t.Context(), value)
				case "stop":
					_, err = backend.Stop(t.Context(), value)
				}
				if err == nil || strings.Contains(err.Error(), "private-invalid-canary") {
					t.Fatalf("%s did not privately refuse historical evidence: %v", operation, err)
				}
				for name, want := range map[string][]byte{path: data, logPath: log} {
					got, err := os.ReadFile(name)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("%s changed retained evidence", operation)
					}
				}
			}
		})
	}
}

func TestStorageProcessEpochCheckedBeforeLivePIDIdentity(t *testing.T) {
	boot := currentStorageBoot(t)
	other := "11111111-1111-4111-8111-111111111111"
	if boot == other {
		other = "22222222-2222-4222-8222-222222222222"
	}
	start, err := processStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	record := processRecord{Version: 5, BootID: other, PID: os.Getpid(), StartTime: start}
	handle, matches, err := openProcessHandle(record, "private-command-canary")
	if handle != nil {
		handle.Close()
		t.Fatal("historical record opened a process handle")
	}
	if matches || err == nil || err.Error() != "storage process record lacks current host boot identity" {
		t.Fatalf("epoch refusal = %v, %v", matches, err)
	}
	// Same-boot lookup still checks the existing process command identity.
	record.BootID = boot
	handle, matches, err = openProcessHandle(record, "private-command-canary")
	if handle != nil {
		handle.Close()
	}
	if matches || err == nil || err.Error() != "storage server command identity conflicts with its record" {
		t.Fatalf("same-boot command control = %v, %v", matches, err)
	}
}
