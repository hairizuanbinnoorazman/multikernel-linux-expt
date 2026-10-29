package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func fixture(ctx context.Context) Snapshot {
	// Exercise the real producers' cancellation path, without any RPC, kernel
	// inventory, private state, or backend available to consult.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	return Snapshot{Components: doctor.InspectComponents(canceled, doctor.ComponentInputs{}, "abcdef0"),
		Operational: doctor.InspectOperational(canceled, doctor.CombinedInputs{}, doctor.PoolInputs{}, doctor.ComponentInputs{}, "abcdef0")}
}

func privateDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCanceledUnavailableObservationsAreRetainedAndHashed(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "snapshot.json")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	receipt, err := Collect(ctx, path, func(got context.Context) Snapshot {
		if got != ctx {
			t.Fatal("collection replaced the caller context")
		}
		return fixture(got)
	})
	if err != nil || receipt.CollectionStatus != "captured" || receipt.ObservationStatus != "inconclusive" {
		t.Fatal(receipt, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if receipt.Bytes != len(data) || receipt.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("receipt does not describe exact artifact bytes")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("bundle is not private", err)
	}
	var bundle Bundle
	if err = json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Version != 1 || bundle.Scope != Scope || bundle.CollectionStatus != "captured" || bundle.StartedAt == "" || bundle.FinishedAt == "" || len(bundle.Excluded) != 12 {
		t.Fatal(bundle)
	}
	if bundle.Snapshot.Operational.Findings[0].Code != "DEADLINE_OR_CANCELED" || bundle.Snapshot.Components.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal("lost unavailable report")
	}
	if bundle.Snapshot.Operational.AdmissionGuaranteed || bundle.Snapshot.Operational.ArtifactVerified || bundle.Snapshot.Operational.KernelVerified {
		t.Fatal("collection promoted observation to attestation")
	}
}

func TestUnsafeDestinationsRefuseBeforeInspectionWithoutChanges(t *testing.T) {
	for _, kind := range []string{"existing", "symlink", "directory", "shared-parent", "symlink-parent", "missing-parent", "relative", "noncanonical", "wrong-extension"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDirectory(t)
			path := filepath.Join(dir, "capture.json")
			sentinel := filepath.Join(dir, "sentinel.json")
			if err := os.WriteFile(sentinel, []byte("private-sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "existing":
				if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(sentinel, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "shared-parent":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink-parent":
				alias := filepath.Join(privateDirectory(t), "alias")
				if err := os.Symlink(dir, alias); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(alias, "capture.json")
			case "missing-parent":
				path = filepath.Join(dir, "missing", "capture.json")
			case "relative":
				path = "capture.json"
			case "noncanonical":
				path = dir + "/../" + filepath.Base(dir) + "/capture.json"
			case "wrong-extension":
				path = filepath.Join(dir, "capture.txt")
			}
			called := false
			_, err := Collect(t.Context(), path, func(ctx context.Context) Snapshot { called = true; return fixture(ctx) })
			if err == nil || called || strings.Contains(err.Error(), dir) {
				t.Fatal("unsafe destination accepted, inspected, or reflected", err)
			}
			data, _ := os.ReadFile(sentinel)
			if string(data) != "private-sentinel" {
				t.Fatal("sentinel changed")
			}
			if kind == "shared-parent" {
				info, _ := os.Stat(dir)
				if info.Mode().Perm() != 0755 {
					t.Fatal("parent was chmodded")
				}
			}
			if kind == "existing" {
				data, _ := os.ReadFile(path)
				if string(data) != "existing" {
					t.Fatal("existing file changed")
				}
			}
		})
	}
}

func TestOversizeAndInvalidReportsPublishNothing(t *testing.T) {
	for _, kind := range []string{"oversize", "version", "scope", "status"} {
		t.Run(kind, func(t *testing.T) {
			dir := privateDirectory(t)
			_, err := Collect(t.Context(), filepath.Join(dir, "capture.json"), func(ctx context.Context) Snapshot {
				s := fixture(ctx)
				switch kind {
				case "oversize":
					s.Operational.Revision = strings.Repeat("x", MaxBundleBytes)
				case "version":
					s.Operational.Version = 8
				case "scope":
					s.Components.Scope = "private-unrecognized-scope"
				case "status":
					s.Components.Status = "private-unrecognized-status"
				}
				return s
			})
			if err == nil || strings.Contains(err.Error(), "private-") {
				t.Fatal("invalid output accepted or reflected", err)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("failed encoding left published or temporary files")
			}
		})
	}
}

func TestConcurrentCollectorsNeverOverwrite(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "capture.json")
	var ready sync.WaitGroup
	ready.Add(2)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := Collect(t.Context(), path, func(ctx context.Context) Snapshot { ready.Done(); ready.Wait(); return fixture(ctx) })
			results <- err
		}()
	}
	success := 0
	for range 2 {
		if <-results == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("publication was not exclusive", success)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 || entries[0].Name() != "capture.json" {
		t.Fatal("unexpected artifacts", entries)
	}
}

func TestReplacementDuringInspectionIsPreserved(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "capture.json")
	_, err := Collect(t.Context(), path, func(ctx context.Context) Snapshot {
		if err := os.WriteFile(path, []byte("raced-file"), 0600); err != nil {
			t.Fatal(err)
		}
		return fixture(ctx)
	})
	data, _ := os.ReadFile(path)
	if err == nil || string(data) != "raced-file" {
		t.Fatal("raced destination was overwritten", err)
	}
}

func TestReportSeverityDoesNotBecomeHealth(t *testing.T) {
	for _, status := range []string{"consistent", "degraded", "inconclusive"} {
		path := filepath.Join(privateDirectory(t), "capture.json")
		r, err := Collect(t.Context(), path, func(ctx context.Context) Snapshot {
			s := fixture(ctx)
			s.Components.Status = status
			s.Operational.Status = "consistent"
			return s
		})
		if err != nil || r.ObservationStatus != status || r.CollectionStatus != "captured" {
			t.Fatal(r, err)
		}
	}
}

func TestRealInspectorsRedactPrivateSourceFailures(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "capture.json")
	_, err := Collect(t.Context(), path, func(ctx context.Context) Snapshot {
		boot := func() (string, error) { return "3d317685-064f-4751-8589-a1f4799525b9", nil }
		private := errors.New("private-token=secret /private/state environment=secret")
		components := doctor.InspectComponents(ctx, doctor.ComponentInputs{Boot: boot, Runtime: func(context.Context) (protocol.ComponentBuildReport, error) {
			return protocol.ComponentBuildReport{}, private
		}}, "abcdef0")
		operational := doctor.InspectOperational(ctx, doctor.CombinedInputs{Boot: boot, Native: func(context.Context) (protocol.NativeResourceReport, error) {
			return protocol.NativeResourceReport{}, private
		}}, doctor.PoolInputs{}, doctor.ComponentInputs{}, "abcdef0")
		return Snapshot{Components: components, Operational: operational}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "secret") || strings.Contains(string(data), "/private/") || strings.Contains(string(data), "private-token") {
		t.Fatal("private error was retained", err)
	}
	if !strings.Contains(string(data), "RUNTIME_BUILD_UNAVAILABLE") || !strings.Contains(string(data), "RESOURCE_OBSERVATION_UNAVAILABLE") {
		t.Fatal("diagnostic failure codes lost")
	}
}

func TestRenamedParentIsNotRedirectedOrPublished(t *testing.T) {
	base := privateDirectory(t)
	dir := filepath.Join(base, "original")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := Collect(t.Context(), filepath.Join(dir, "capture.json"), func(ctx context.Context) Snapshot {
		if err := os.Rename(dir, filepath.Join(base, "moved")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		return fixture(ctx)
	})
	if err == nil {
		t.Fatal("parent substitution went unnoticed")
	}
	for _, path := range []string{dir, filepath.Join(base, "moved")} {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatal("published into replaced parent", err)
		}
	}
}
