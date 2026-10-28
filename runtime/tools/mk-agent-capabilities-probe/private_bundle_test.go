//go:build linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPrivateBundlePublicationIsExclusivePrivateAndCredentialFree(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	target, err := openPrivateBundleTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	r := report{Version: 4, Status: "observed", SandboxID: "box", Generation: strings.Repeat("a", 32), Endpoint: 7200, ExpectedProcessSHA256: strings.Repeat("b", 64), Process: &processObservation{Status: "STOPPED", GuestPID: 616}, Output: &outputObservation{StateStable: true}}
	sample := outputSample{Stdout: []byte("private workload stdout"), Stderr: []byte("private workload stderr")}
	captured := target.publish(t.Context(), r, sample)
	if !captured.PrivateBundleWritten || captured.Status != "observed" {
		t.Fatalf("private capture failed: %+v", captured)
	}
	path := filepath.Join(dir, privateBundleName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil || st.Mode&0777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Size != int64(captured.PrivateBundleBytes) {
		t.Fatal("private output file ownership/mode/link/size contract failed")
	}
	var content map[string]json.RawMessage
	if json.Unmarshal(data, &content) != nil {
		t.Fatal("private capture is not JSON")
	}
	for _, key := range []string{"token", "credential", "token_fd", "environment", "client"} {
		if _, ok := content[key]; ok {
			t.Fatal("caller credentials or environment entered bundle")
		}
	}
	if again := target.publish(t.Context(), r, outputSample{Stdout: []byte("replacement")}); again.Status != "inconclusive" || again.PrivateBundleWritten {
		t.Fatal("private capture overwrote existing entry")
	}
	unchanged, _ := os.ReadFile(path)
	if string(data) != string(unchanged) {
		t.Fatal("private artifact changed on repeat")
	}
	if _, err = openPrivateBundleTarget(dir); err == nil {
		t.Fatal("existing private artifact admitted")
	}
	clear(data)
	clear(unchanged)
}
func TestPrivateBundleRejectsUnsafeTargetsAndCanceledPublication(t *testing.T) {
	base := t.TempDir()
	os.Chmod(base, 0700)
	for _, mode := range []string{"shared", "missing", "symlink", "occupied-symlink"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(base, mode)
			if mode == "missing" {
			} else if mode == "symlink" {
				os.Symlink(base, dir)
			} else {
				os.Mkdir(dir, 0700)
			}
			if mode == "shared" {
				os.Chmod(dir, 0755)
			}
			if mode == "occupied-symlink" {
				os.Symlink("/must-not-follow", filepath.Join(dir, privateBundleName))
			}
			if target, err := openPrivateBundleTarget(dir); err == nil {
				target.close()
				t.Fatal("unsafe target admitted")
			}
			if mode == "shared" {
				st, _ := os.Stat(dir)
				if st.Mode().Perm() != 0755 {
					t.Fatal("inspection changed unsafe directory mode")
				}
			}
		})
	}
	dir := filepath.Join(base, "canceled")
	os.Mkdir(dir, 0700)
	target, err := openPrivateBundleTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer target.close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := report{Process: &processObservation{Status: "STOPPED"}, Output: &outputObservation{StateStable: true}}
	result := target.publish(ctx, r, outputSample{})
	if result.Code != "DEADLINE_OR_CANCELED" || result.PrivateBundleWritten {
		t.Fatal("canceled publication accepted")
	}
	if _, err = os.Lstat(filepath.Join(dir, privateBundleName)); !os.IsNotExist(err) {
		t.Fatal("canceled publication created artifact")
	}
}

func TestCanceledPrivateInspectionDoesNotReadCredential(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	file := credential(t, true)
	o := options{id: "box", generation: strings.Repeat("a", 32), release: "test-release", processID: "init", stoppedOutputSummary: true, privateBundleDir: "/must-not-open", endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, timeout: time.Second}
	result := inspect(ctx, o, file)
	offset, err := file.Seek(0, 1)
	if err != nil || offset != 0 || result.Code != "DEADLINE_OR_CANCELED" || result.RequestAttempted || result.CredentialDescriptorVerified {
		t.Fatal("canceled inspection advanced into private target or credential handling")
	}
}
