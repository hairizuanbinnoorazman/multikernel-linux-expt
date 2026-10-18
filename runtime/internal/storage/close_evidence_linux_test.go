//go:build linux

package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestClosedEvidenceValidatesHistoryAndRetainsAmbiguity(t *testing.T) {
	accepted := "MKNBD_SERVER_CLIENT_ACCEPTED\n"
	refusal := "MKNBD_SERVER_REFUSED reason=identity\n"
	notice := "MKNBD_SERVER_REFUSALS_SUPPRESSED further_details=omitted\n"
	closed := func(counts string) string { return "MKNBD_SERVER_CLOSED synced=1 " + counts + "\n" }
	zero := "reads=0 read_bytes=0 writes=0 write_bytes=0 flushes=0"
	io := "reads=1 read_bytes=512 writes=1 write_bytes=512 flushes=1"
	summary := "MKNBD_SERVER_REFUSAL_SUMMARY total=1 details=1 suppressed=0\n"
	for _, tc := range []struct {
		name, body string
		good       bool
	}{
		{"unused graceful", closed(zero), true},
		{"accepted IO", accepted + closed(io), true},
		{"zero length operations", accepted + closed("reads=1 read_bytes=0 writes=1 write_bytes=0 flushes=0"), true},
		{"refused unused", refusal + summary + closed(zero), true},
		{"refused then accepted", refusal + accepted + summary + closed(io), true},
		{"suppressed then accepted", strings.Repeat(refusal, 16) + notice + accepted + "MKNBD_SERVER_REFUSAL_SUMMARY total=256 details=16 suppressed=240\n" + closed(io), true},
		{"request timeout", accepted + "MKNBD_SERVER_IO_FAILED phase=request errno=110\n" + closed(io), true},
		{"partial write", accepted + "MKNBD_SERVER_DISCONNECT_DURING_WRITE len=512 offset=0\n" + closed(io), true},
		{"unknown before close", accepted + "unknown\n" + closed(io), false},
		{"duplicate acceptance", accepted + accepted + closed(io), false},
		{"duplicate close", accepted + closed(io) + closed(io), false},
		{"unaccepted IO", closed(io), false},
		{"bytes without operations", accepted + closed("reads=0 read_bytes=512 writes=0 write_bytes=0 flushes=0"), false},
		{"too many bytes", accepted + closed(fmt.Sprintf("reads=1 read_bytes=%d writes=0 write_bytes=0 flushes=0", storageRequestLimit+1)), false},
		{"missing refusal summary", refusal + accepted + closed(io), false},
		{"summary count mismatch", refusal + accepted + "MKNBD_SERVER_REFUSAL_SUMMARY total=2 details=2 suppressed=0\n" + closed(io), false},
		{"summary without suppression", refusal + accepted + "MKNBD_SERVER_REFUSAL_SUMMARY total=2 details=1 suppressed=1\n" + closed(io), false},
		{"duplicate diagnostic", accepted + strings.Repeat("MKNBD_SERVER_IO_FAILED phase=request errno=110\n", 2) + closed(io), false},
		{"diagnostic before acceptance", "MKNBD_SERVER_IO_FAILED phase=request errno=110\n" + closed(zero), false},
		{"invalid errno", accepted + "MKNBD_SERVER_IO_FAILED phase=request errno=0\n" + closed(io), false},
		{"zero-length disconnect", accepted + "MKNBD_SERVER_DISCONNECT_DURING_WRITE len=0 offset=0\n" + closed(io), false},
		{"write overflow", accepted + "MKNBD_SERVER_DISCONNECT_DURING_WRITE len=512 offset=18446744073709551615\n" + closed(io), false},
		{"request refusal exit", accepted + "MKNBD_SERVER_REFUSED reason=request command=5 len=0 offset=0\n" + closed(io), false},
		{"refusal after acceptance", accepted + refusal + summary + closed(io), false},
		{"summary before acceptance", refusal + summary + accepted + closed(io), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			value := validBackendLease("/var/lib/multikernel/root.ext4")
			b := &LinuxBackend{RuntimeDir: dir, RequiredUID: os.Getuid()}
			recordPath, logPath := b.paths(value)
			record := processRecord{Version: 3, PID: 1 << 30, StartTime: 1, Path: value.Path, Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration, ImageDevice: value.ImageIdentity.Device, ImageInode: value.ImageIdentity.Inode, BinaryDevice: 3, BinaryInode: 4}
			original, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			original = append(original, '\n')
			if err = os.WriteFile(recordPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(logPath, append(readyMarker(value), []byte(tc.body)...), 0600); err != nil {
				t.Fatal(err)
			}
			_, err = parseCounters(logPath, value)
			if (err == nil) != tc.good {
				t.Fatalf("counter evidence: %v", err)
			}
			observed, observeErr := b.Observe(t.Context(), value)
			if tc.good {
				if observeErr != nil || !observed.Closed {
					t.Fatalf("valid close not observed: %+v %v", observed, observeErr)
				}
				return
			}
			if observed.Closed {
				t.Fatal("ambiguous history authorized graceful close")
			}
			if _, err = b.Stop(t.Context(), value); err == nil {
				t.Fatal("ambiguous history authorized stop")
			}
			retained, err := os.ReadFile(recordPath)
			if err != nil || !bytes.Equal(retained, original) {
				t.Fatalf("ambiguous record changed: %q %v", retained, err)
			}
		})
	}
}
