package diagnostics

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func budgetFixture() DirectoryBudget { return DirectoryBudget{MaxFiles: 4, MaxBytes: 1 << 20} }
func TestDiagnosticRetentionLimitsPreservePriorEvidence(t *testing.T) {
	dir := privateDirectory(t)
	first := filepath.Join(dir, "first.json")
	receipt, err := CollectWithBudget(t.Context(), first, budgetFixture(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(original)
	if receipt.SHA256 != hex.EncodeToString(hash[:]) || receipt.Bytes != len(original) {
		t.Fatal("bounded receipt lost original byte binding")
	}
	for _, kind := range []string{"count", "bytes-full", "bytes-insufficient"} {
		t.Run(kind, func(t *testing.T) {
			budget := budgetFixture()
			if kind == "count" {
				budget.MaxFiles = 1
			} else if kind == "bytes-full" {
				budget.MaxBytes = int64(receipt.Bytes)
			} else {
				budget.MaxBytes = int64(receipt.Bytes) + 1
			}
			called := 0
			_, err := CollectWithBudget(t.Context(), filepath.Join(dir, "next.json"), budget, func(ctx context.Context) Snapshot { called++; return fixture(ctx) })
			if err == nil || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "first.json") {
				t.Fatal("retention limit accepted or exposed path", err)
			}
			wantCalls := 0
			if kind == "bytes-insufficient" {
				wantCalls = 1
			}
			if called != wantCalls {
				t.Fatal("wrong pre/post-inspection budget boundary", called)
			}
			data, _ := os.ReadFile(first)
			if !bytes.Equal(original, data) {
				t.Fatal("prior evidence changed")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 || entries[0].Name() != "first.json" {
				t.Fatal("budget failure published or removed evidence")
			}
		})
	}
}

func TestDiagnosticRetentionInvalidBudgetRefusesBeforeInspection(t *testing.T) {
	for _, budget := range []DirectoryBudget{{}, {MaxFiles: 1}, {MaxBytes: 1}, {MaxFiles: -1, MaxBytes: 1}, {MaxFiles: MaxDirectoryFiles + 1, MaxBytes: 1}, {MaxFiles: 1, MaxBytes: -1}, {MaxFiles: 1, MaxBytes: MaxDirectoryBytes + 1}} {
		called := false
		_, err := CollectWithBudget(t.Context(), "/SYNTHETIC_PRIVATE_PATH/capture.json", budget, func(ctx context.Context) Snapshot { called = true; return fixture(ctx) })
		if err == nil || called || strings.Contains(err.Error(), "SYNTHETIC") {
			t.Fatal("invalid budget inspected or reflected private input", err)
		}
	}
}

func TestDiagnosticRetentionUnsafeInventoryRefusesWithoutPayloadRead(t *testing.T) {
	for _, kind := range []string{"unknown-name", "symlink", "hardlink", "directory", "fifo", "shared-file", "readonly-file", "oversized", "too-many"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDirectory(t)
			path := filepath.Join(dir, "SYNTHETIC_PRIVATE_ENTRY.json")
			secret := []byte("SYNTHETIC_PRIVATE_PAYLOAD_MUST_NOT_BE_PARSED_OR_REPORTED")
			switch kind {
			case "unknown-name":
				path = filepath.Join(dir, "SYNTHETIC_PRIVATE_ENTRY.log")
				if err := os.WriteFile(path, secret, 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink", "hardlink":
				outside := filepath.Join(privateDirectory(t), "outside.json")
				if err := os.WriteFile(outside, secret, 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(outside, path)
				} else {
					err = os.Link(outside, path)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "shared-file", "readonly-file":
				if err := os.WriteFile(path, secret, 0600); err != nil {
					t.Fatal(err)
				}
				mode := os.FileMode(0644)
				if kind == "readonly-file" {
					mode = 0400
				}
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(MaxBundleBytes + 1)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "too-many":
				for n := 0; n <= MaxDirectoryFiles; n++ {
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("entry-%03d.json", n)), nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			called := false
			started := time.Now()
			_, err := CollectWithBudget(t.Context(), filepath.Join(dir, "capture.json"), budgetFixture(), func(ctx context.Context) Snapshot { called = true; return fixture(ctx) })
			if err == nil || called || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("unsafe inventory inspected, accepted or reflected", err)
			}
			if time.Since(started) > time.Second {
				t.Fatal("nonregular inventory blocked")
			}
			if _, err := os.Lstat(filepath.Join(dir, "capture.json")); !os.IsNotExist(err) {
				t.Fatal("unsafe inventory published")
			}
			if _, err := os.Lstat(path); kind != "too-many" && err != nil {
				t.Fatal("collector removed unsafe evidence", err)
			}
		})
	}
}

func TestDiagnosticRetentionDetectsMetadataDriftWithoutRepair(t *testing.T) {
	for _, kind := range []string{"append", "replace-identical", "add", "remove", "chmod"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDirectory(t)
			old := filepath.Join(dir, "old.json")
			original := []byte("SYNTHETIC_PRIVATE_EVIDENCE")
			if err := os.WriteFile(old, original, 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "capture.json")
			_, err := CollectWithBudget(t.Context(), path, budgetFixture(), func(ctx context.Context) Snapshot {
				switch kind {
				case "append":
					f, e := os.OpenFile(old, os.O_APPEND|os.O_WRONLY, 0)
					if e != nil {
						t.Fatal(e)
					}
					_, e = f.Write([]byte("x"))
					f.Close()
					if e != nil {
						t.Fatal(e)
					}
				case "replace-identical":
					if e := os.Rename(old, filepath.Join(privateDirectory(t), "retained.json")); e != nil {
						t.Fatal(e)
					}
					if e := os.WriteFile(old, original, 0600); e != nil {
						t.Fatal(e)
					}
				case "add":
					if e := os.WriteFile(filepath.Join(dir, "added.json"), original, 0600); e != nil {
						t.Fatal(e)
					}
				case "remove":
					if e := os.Remove(old); e != nil {
						t.Fatal(e)
					}
				case "chmod":
					if e := os.Chmod(old, 0644); e != nil {
						t.Fatal(e)
					}
				}
				return fixture(ctx)
			})
			if err == nil || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("changed inventory accepted or exposed", err)
			}
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("inventory drift published")
			}
		})
	}
}

func TestDiagnosticRetentionCanceledObservationsRemainPublishable(t *testing.T) {
	dir := privateDirectory(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	receipt, err := CollectWithBudget(ctx, filepath.Join(dir, "capture.json"), budgetFixture(), func(got context.Context) Snapshot {
		if got != ctx {
			t.Fatal("caller context replaced")
		}
		return fixture(got)
	})
	if err != nil || receipt.ObservationStatus != "inconclusive" || receipt.CollectionStatus != "captured" {
		t.Fatal("quota mode promoted or discarded canceled observation", receipt, err)
	}
}

func TestDiagnosticRetentionProcessLockAndQuota(t *testing.T) {
	if dir := os.Getenv("MK_G8_RETENTION_PROCESS_DIR"); dir != "" {
		_, err := CollectWithBudget(context.Background(), filepath.Join(dir, "first.json"), DirectoryBudget{MaxFiles: 1, MaxBytes: 1 << 20}, func(ctx context.Context) Snapshot {
			fmt.Println("RETENTION_INSPECTION_HELD")
			if _, e := bufio.NewReader(os.Stdin).ReadString('\n'); e != nil {
				os.Exit(43)
			}
			return fixture(ctx)
		})
		if err != nil {
			os.Exit(44)
		}
		os.Exit(37)
	}
	dir := privateDirectory(t)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDiagnosticRetentionProcessLockAndQuota$")
	cmd.Env = append(os.Environ(), "MK_G8_RETENTION_PROCESS_DIR="+dir, "GORACE=atexit_sleep_ms=0")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		input.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan bool, 1)
	go func() {
		line, e := bufio.NewReader(output).ReadString('\n')
		ready <- e == nil && line == "RETENTION_INSPECTION_HELD\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("owner did not hold inspection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("owner inspection deadline")
	}
	for _, legacy := range []bool{false, true} {
		called := false
		inspect := func(ctx context.Context) Snapshot { called = true; return fixture(ctx) }
		if legacy {
			_, err = Collect(t.Context(), filepath.Join(dir, "second.json"), inspect)
		} else {
			_, err = CollectWithBudget(t.Context(), filepath.Join(dir, "second.json"), budgetFixture(), inspect)
		}
		if err == nil || called {
			t.Fatal("competing process bypassed directory lock", legacy, err)
		}
	}
	_, _ = input.Write([]byte("\n"))
	input.Close()
	err = cmd.Wait()
	waited = true
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 || ctx.Err() != nil {
		t.Fatal("capture owner did not publish and exit", err)
	}
	called := false
	_, err = CollectWithBudget(t.Context(), filepath.Join(dir, "second.json"), DirectoryBudget{MaxFiles: 1, MaxBytes: 1 << 20}, func(ctx context.Context) Snapshot { called = true; return fixture(ctx) })
	if err == nil || called {
		t.Fatal("completed owner was not charged to retention budget", err)
	}
	if _, err = CollectWithBudget(t.Context(), filepath.Join(dir, "second.json"), budgetFixture(), fixture); err != nil {
		t.Fatal("closed owner retained its lock", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("wrong retained count", len(entries))
	}
}
