//go:build linux

package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestObserveCanonicalRefusalEvidenceAndRetainAmbiguity(t *testing.T) {
	refusal := "MKNBD_SERVER_REFUSED reason=identity\n"
	ioRefusal := "MKNBD_SERVER_REFUSED reason=handshake-io\n"
	notice := "MKNBD_SERVER_REFUSALS_SUPPRESSED further_details=omitted\n"
	accepted := "MKNBD_SERVER_CLIENT_ACCEPTED\n"
	for _, tc := range []struct {
		name, body     string
		pre, lost, bad bool
	}{
		{name: "ready only", pre: true},
		{name: "identity refusal", body: refusal, pre: true},
		{name: "mixed refusal", body: refusal + ioRefusal, pre: true},
		{name: "detail bound", body: strings.Repeat(refusal, 16), pre: true},
		{name: "suppression", body: strings.Repeat(refusal, 16) + notice, pre: true},
		{name: "accepted after refusal", body: refusal + accepted, lost: true},
		{name: "accepted after suppression", body: strings.Repeat(refusal, 16) + notice + accepted, lost: true},
		{name: "accepted partial tail", body: accepted + "partial crash output", lost: true},
		{name: "unknown before acceptance", body: "unknown\n" + accepted, bad: true},
		{name: "embedded acceptance", body: "diagnostic " + accepted, bad: true},
		{name: "partial refusal", body: strings.TrimSuffix(refusal, "\n"), bad: true},
		{name: "unknown refusal", body: "MKNBD_SERVER_REFUSED reason=unknown\n", bad: true},
		{name: "early suppression", body: notice, bad: true},
		{name: "duplicate suppression", body: strings.Repeat(refusal, 16) + notice + notice, bad: true},
		{name: "excess details", body: strings.Repeat(refusal, 17), bad: true},
		{name: "detail after suppression", body: strings.Repeat(refusal, 16) + notice + refusal, bad: true},
		{name: "duplicate ready", body: "MKNBD_SERVER_READY bogus\n" + accepted, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			value := validBackendLease("/var/lib/multikernel/root.ext4")
			backend := &LinuxBackend{RuntimeDir: dir, RequiredUID: os.Getuid()}
			recordPath, logPath := backend.paths(value)
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
			got, err := backend.Observe(t.Context(), value)
			if (err != nil) != tc.bad || !tc.bad && (got.PreAcceptance != tc.pre || got.ClientLost != tc.lost || got.Active || got.Closed) {
				t.Fatalf("observation=%+v err=%v", got, err)
			}
			retained, readErr := os.ReadFile(recordPath)
			if tc.bad || tc.lost {
				if readErr != nil || !bytes.Equal(retained, original) {
					t.Fatalf("ambiguity/client-loss record changed: %q %v", retained, readErr)
				}
			} else if !os.IsNotExist(readErr) {
				t.Fatalf("pre-acceptance record not reclaimed: %v", readErr)
			}
		})
	}
}

func TestCounterEvidenceRejectsEmbeddedAndDuplicateReadiness(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	value := validBackendLease("/var/lib/multikernel/root.ext4")
	other := value
	other.ImageID = "other-image"
	closeRecord := []byte("MKNBD_SERVER_CLOSED synced=1 reads=0 read_bytes=0 writes=0 write_bytes=0 flushes=0\n")
	for name, data := range map[string][]byte{
		"embedded ready":              append(append([]byte("prefix "), readyMarker(value)...), closeRecord...),
		"foreign then expected ready": append(append(readyMarker(other), readyMarker(value)...), closeRecord...),
		"duplicate ready":             append(append(readyMarker(value), readyMarker(value)...), closeRecord...),
		"embedded close":              append(append(readyMarker(value), []byte("diagnostic ")...), closeRecord...),
	} {
		t.Run(name, func(t *testing.T) {
			path := dir + "/server.log"
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := parseCounters(path, value); err == nil {
				t.Fatal("ambiguous counter evidence accepted")
			}
		})
	}
}
