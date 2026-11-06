package diagnostics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func healthBundleFixture(mode string) HealthSnapshot {
	boot := "00000000-0000-0000-0000-000000000001"
	op := &doctor.OperationalReport{Version: 7, Scope: "resource-network-pool-and-compiled-component-observations", Status: "consistent", BootID: boot, BootStable: true}
	r := doctor.HealthReport{Version: 8, Scope: "operational-journal-and-recorded-network-health-observations", Status: "consistent", BootID: boot, BootStable: true, Operational: op, Journal: &protocol.JournalStatusReport{Version: 1, Source: "in-process-lifecycle-journal", Status: "idle"}, Network: &network.HealthReport{Version: 1, Source: "host-network-backend-check", PrimaryBootID: boot, Status: "idle", Endpoints: []network.EndpointHealth{}}}
	result := HealthSnapshot{Components: doctor.ComponentsReport{Version: 1, Scope: "compiled-runtime-network-build-self-reports", Status: "consistent"}, Health: r}
	switch mode {
	case "pending":
		result.Health.Journal.Status = "pending"
		result.Health.Journal.Sequence = 1
		result.Health.Journal.PendingCompletions = 1
		result.Health.Status = "inconclusive"
	case "failed":
		result.Health.Journal.Status = "failed"
		result.Health.Journal.JournalFailed = true
		result.Health.Status = "degraded"
	case "network-failed":
		result.Health.Network.Status = "degraded"
		result.Health.Network.Endpoints = []network.EndpointHealth{{EndpointGeneration: strings.Repeat("a", 32), DescriptorSHA256: strings.Repeat("b", 64), Code: "BACKEND_CHECK_FAILED"}}
		result.Health.Status = "degraded"
	case "component-unknown":
		result.Components.Status = "inconclusive"
	}
	return result
}

func TestHealthBundleSeverityPrivatePublicationAndExactReceipt(t *testing.T) {
	for _, mode := range []string{"consistent", "pending", "failed", "network-failed", "component-unknown"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(privateDirectory(t), "health.json")
			snapshot := healthBundleFixture(mode)
			receipt, err := CollectHealth(t.Context(), path, func(context.Context) HealthSnapshot { return snapshot })
			want := "consistent"
			if mode == "pending" || mode == "component-unknown" {
				want = "inconclusive"
			}
			if mode == "failed" || mode == "network-failed" {
				want = "degraded"
			}
			if err != nil || receipt.Version != 2 || receipt.Scope != HealthScope || receipt.ObservationStatus != want || receipt.CollectionStatus != "captured" {
				t.Fatal("health receipt lost severity/version", receipt, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if receipt.Bytes != len(data) || receipt.SHA256 != hex.EncodeToString(sum[:]) {
				t.Fatal("health receipt lost byte binding")
			}
			var bundle HealthBundle
			if json.Unmarshal(data, &bundle) != nil || bundle.Version != 2 || bundle.Scope != HealthScope || len(bundle.Excluded) != 12 || bundle.StartedAt == "" || bundle.FinishedAt == "" || bundle.Snapshot.Health.Status != snapshot.Health.Status {
				t.Fatal("health bundle lost contract")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("health bundle not private")
			}
			var raw map[string]json.RawMessage
			if json.Unmarshal(data, &raw) != nil {
				t.Fatal("invalid JSON")
			}
			var sections map[string]json.RawMessage
			_ = json.Unmarshal(raw["snapshot"], &sections)
			if len(sections) != 2 || sections["components"] == nil || sections["health"] == nil {
				t.Fatal("health format mixed with legacy snapshot")
			}
		})
	}
}

func TestHealthBundleCanceledBudgetRetainsUnavailableReports(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path := filepath.Join(privateDirectory(t), "canceled.json")
	receipt, err := CollectHealthWithBudget(ctx, path, DirectoryBudget{MaxFiles: 1, MaxBytes: 1 << 20}, func(got context.Context) HealthSnapshot {
		if got != ctx {
			t.Fatal("health capture extended context")
		}
		return HealthSnapshot{Components: doctor.InspectComponents(got, doctor.ComponentInputs{}, "fixture"), Health: doctor.InspectHealth(got, doctor.CombinedInputs{}, doctor.PoolInputs{}, doctor.ComponentInputs{}, doctor.HealthInputs{}, "fixture")}
	})
	if err != nil || receipt.ObservationStatus != "inconclusive" {
		t.Fatal("canceled diagnostic lost", receipt, err)
	}
	data, _ := os.ReadFile(path)
	var bundle HealthBundle
	if json.Unmarshal(data, &bundle) != nil || bundle.Snapshot.Health.Operational != nil || bundle.Snapshot.Health.Findings[0].Code != "DEADLINE_OR_CANCELED" || bundle.Snapshot.Components.Findings[0].Code != "DEADLINE_OR_CANCELED" {
		t.Fatal("unavailable scope became a successful empty view")
	}
}

func TestHealthBundleInvalidReportsRefuseWithoutPublication(t *testing.T) {
	for _, mode := range []string{"version", "scope", "status", "admission", "partial", "no-payload-consistent", "boot", "op-version", "op-attestation", "journal", "network", "component-version", "component-status", "component-attestation", "masked-journal"} {
		t.Run(mode, func(t *testing.T) {
			s := healthBundleFixture("consistent")
			switch mode {
			case "version":
				s.Health.Version = 7
			case "scope":
				s.Health.Scope = "private-SYNTHETIC"
			case "status":
				s.Health.Status = "ready"
			case "admission":
				s.Health.AdmissionGuaranteed = true
			case "partial":
				s.Health.Journal = nil
			case "no-payload-consistent":
				s.Health.Operational = nil
				s.Health.Journal = nil
				s.Health.Network = nil
				s.Health.BootStable = false
			case "boot":
				s.Health.BootID = "11111111-1111-4111-8111-111111111111"
			case "op-version":
				s.Health.Operational.Version = 6
			case "op-attestation":
				s.Health.Operational.ArtifactVerified = true
			case "journal":
				s.Health.Journal.Version = 0
			case "network":
				s.Health.Network.Version = 0
			case "component-version":
				s.Components.Version = 0
			case "component-status":
				s.Components.Status = "ready"
			case "component-attestation":
				s.Components.ArtifactVerified = true
			case "masked-journal":
				s.Health.Journal.JournalFailed = true
				s.Health.Journal.Status = "failed"
			}
			dir := privateDirectory(t)
			_, err := CollectHealth(t.Context(), filepath.Join(dir, "refused.json"), func(context.Context) HealthSnapshot { return s })
			entries, _ := os.ReadDir(dir)
			if err == nil || len(entries) != 0 || strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("invalid health capture published or leaked", err)
			}
		})
	}
}

func TestHealthBundleSharesRetentionAndLockWithLegacy(t *testing.T) {
	dir := privateDirectory(t)
	path := filepath.Join(dir, "health.json")
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := CollectHealth(t.Context(), path, func(context.Context) HealthSnapshot {
			close(entered)
			<-release
			return healthBundleFixture("consistent")
		})
		done <- err
	}()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("health owner did not enter")
	}
	called := false
	if _, err := Collect(t.Context(), filepath.Join(dir, "legacy.json"), func(ctx context.Context) Snapshot { called = true; return fixture(ctx) }); err == nil || called {
		t.Fatal("legacy bypassed health directory lock")
	}
	close(release)
	released = true
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("health owner did not finish")
	}
	before, _ := os.ReadFile(path)
	if receipt, err := CollectWithBudget(t.Context(), filepath.Join(dir, "legacy.json"), DirectoryBudget{MaxFiles: 2, MaxBytes: 1 << 20}, fixture); err != nil || receipt.Version != 1 {
		t.Fatal("legacy capture changed", err)
	}
	called = false
	if _, err := CollectHealthWithBudget(t.Context(), filepath.Join(dir, "full.json"), DirectoryBudget{MaxFiles: 2, MaxBytes: 1 << 20}, func(context.Context) HealthSnapshot { called = true; return healthBundleFixture("consistent") }); err == nil || called {
		t.Fatal("mixed-format quota bypassed")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("health evidence changed")
	}
}

func TestHealthBundleReusesUnsafeDestinationAndSizeGuards(t *testing.T) {
	for _, mode := range []string{"symlink", "existing", "oversize", "invalid-budget", "nil-inspector"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateDirectory(t)
			path := filepath.Join(dir, "capture.json")
			sentinel := filepath.Join(dir, "sentinel.json")
			_ = os.WriteFile(sentinel, []byte("SYNTHETIC_PRIVATE_SENTINEL"), 0600)
			if mode == "symlink" {
				if err := os.Symlink(sentinel, path); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "existing" {
				_ = os.WriteFile(path, []byte("existing"), 0600)
			}
			called := false
			inspect := func(context.Context) HealthSnapshot {
				called = true
				s := healthBundleFixture("consistent")
				if mode == "oversize" {
					s.Components.Revision = strings.Repeat("x", MaxBundleBytes)
				}
				return s
			}
			var err error
			if mode == "invalid-budget" {
				_, err = CollectHealthWithBudget(t.Context(), path, DirectoryBudget{}, inspect)
			} else if mode == "nil-inspector" {
				_, err = CollectHealth(t.Context(), path, nil)
			} else {
				_, err = CollectHealth(t.Context(), path, inspect)
			}
			if err == nil || strings.Contains(err.Error(), dir) {
				t.Fatal("guard bypassed or reflected destination", err)
			}
			if called != (mode == "oversize") {
				t.Fatal("wrong health inspection boundary")
			}
			data, _ := os.ReadFile(sentinel)
			if string(data) != "SYNTHETIC_PRIVATE_SENTINEL" {
				t.Fatal("existing evidence changed")
			}
			if mode == "oversize" {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("truncated health artifact published")
				}
			}
		})
	}
}

func TestHealthBundleByteBudgetUsesEncodedBytesWithoutRemovingEvidence(t *testing.T) {
	dir := privateDirectory(t)
	first := filepath.Join(dir, "first.json")
	receipt, err := CollectHealth(t.Context(), first, func(context.Context) HealthSnapshot { return healthBundleFixture("consistent") })
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(first)
	for _, mode := range []string{"already-full", "insufficient-remaining"} {
		t.Run(mode, func(t *testing.T) {
			budget := DirectoryBudget{MaxFiles: 3, MaxBytes: int64(receipt.Bytes)}
			if mode == "insufficient-remaining" {
				budget.MaxBytes++
			}
			calls := 0
			_, err := CollectHealthWithBudget(t.Context(), filepath.Join(dir, "next.json"), budget, func(context.Context) HealthSnapshot { calls++; return healthBundleFixture("consistent") })
			want := 0
			if mode == "insufficient-remaining" {
				want = 1
			}
			if err == nil || calls != want {
				t.Fatal("health byte budget bypassed", err, calls)
			}
			after, _ := os.ReadFile(first)
			entries, _ := os.ReadDir(dir)
			if !bytes.Equal(before, after) || len(entries) != 1 {
				t.Fatal("byte budget modified evidence")
			}
		})
	}
}
