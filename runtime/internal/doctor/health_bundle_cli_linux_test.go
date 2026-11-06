//go:build linux

package doctor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Decode the public bundle wire as an independent CLI consumer. Importing the
// publisher from its producer's package would create a test import cycle.
type healthBundleCLIReceipt struct {
	Version           int    `json:"version"`
	Scope             string `json:"scope"`
	CollectionStatus  string `json:"collection_status"`
	ObservationStatus string `json:"observation_status"`
	Bytes             int    `json:"bytes"`
	SHA256            string `json:"sha256"`
}
type healthBundleCLIEnvelope struct {
	Version          int    `json:"version"`
	Scope            string `json:"scope"`
	CollectionStatus string `json:"collection_status"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	Snapshot         struct {
		Components ComponentsReport `json:"components"`
		Health     HealthReport     `json:"health"`
	} `json:"snapshot"`
	Excluded []string `json:"excluded"`
}
type legacyBundleCLIEnvelope struct {
	Version          int    `json:"version"`
	Scope            string `json:"scope"`
	CollectionStatus string `json:"collection_status"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	Snapshot         struct {
		Components  ComponentsReport  `json:"components"`
		Operational OperationalReport `json:"operational"`
	} `json:"snapshot"`
	Excluded []string `json:"excluded"`
}

func TestInstalledHealthBundleProductionRPCAndJournalFailure(t *testing.T) {
	cli, f := newInstalledHealthCLIFixture(t)
	observations := f.history.observations.Load()
	for _, name := range []string{"legacy-bundles", "health-bundles"} {
		if err := os.Mkdir(filepath.Join(f.root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	capture := func(mode, wantStatus string, version, wantExit int) []byte {
		t.Helper()
		raw := runHealthCLI(t, f, cli, mode, wantExit, "")
		var receipt healthBundleCLIReceipt
		if protocol.StrictDecode(raw, &receipt) != nil || receipt.Version != version || receipt.CollectionStatus != "captured" || receipt.ObservationStatus != wantStatus {
			t.Fatal("bundle CLI receipt lost version or aggregate severity")
		}
		path := filepath.Join(f.root, "health-bundles", mode+".json")
		scope := "operational-health-and-component-observation-snapshot"
		if version == 1 {
			path = filepath.Join(f.root, "legacy-bundles/legacy.json")
			scope = "operational-and-component-observation-snapshot"
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if receipt.Scope != scope || receipt.Bytes != len(data) || receipt.SHA256 != hex.EncodeToString(sum[:]) || len(data) > 8<<20 {
			t.Fatal("bundle CLI receipt does not bind exact private bytes")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("bundle CLI did not publish privately")
		}
		if bytes.Contains(data, []byte("SYNTHETIC_HEALTH_PRIVATE_CANARY")) {
			t.Fatal("bundle captured private state or labels")
		}
		if version == 2 {
			var bundle healthBundleCLIEnvelope
			if protocol.StrictDecode(data, &bundle) != nil || bundle.Version != 2 || bundle.Scope != scope || len(bundle.Excluded) != 12 || bundle.Snapshot.Components.Version != 1 || bundle.Snapshot.Health.Version != 8 || bundle.Snapshot.Health.Status != wantStatus {
				t.Fatal("health bundle lost source scopes or health severity")
			}
			h := bundle.Snapshot.Health
			if h.AdmissionGuaranteed || h.Journal == nil || h.Operational == nil || h.Network == nil {
				t.Fatal("health bundle incomplete or promoted to readiness")
			}
			if wantStatus == "degraded" && (!h.Journal.JournalFailed || !hasFinding(h.Findings, "JOURNAL_FAILED")) {
				t.Fatal("health bundle masked failed journal")
			}
			if wantStatus != "consistent" && (len(h.Operational.Unified.Pool.CPUCandidates) != 0 || h.Operational.Unified.Pool.MemoryCandidateUpperBoundBytes != 0) {
				t.Fatal("health bundle exposed unsafe candidates")
			}
		} else {
			var bundle legacyBundleCLIEnvelope
			if protocol.StrictDecode(data, &bundle) != nil || bundle.Version != 1 || bundle.Snapshot.Operational.Version != 7 {
				t.Fatal("default legacy bundle changed")
			}
		}
		return data
	}
	capture("legacy-bundle", "consistent", 1, 0)
	idle := capture("health-bundle-idle", "consistent", 2, 0)
	f.pending(t)
	pending := capture("health-bundle-pending", "inconclusive", 2, 1)
	f.failJournal(t)
	failed := capture("health-bundle-failed", "degraded", 2, 1)
	if output := runHealthCLI(t, f, cli, "health-bundle-full", 1, "diagnostic directory retention limit reached\n"); len(output) != 0 {
		t.Fatal("full health directory emitted a receipt")
	}
	for name, before := range map[string][]byte{"health-bundle-idle": idle, "health-bundle-pending": pending, "health-bundle-failed": failed} {
		after, err := os.ReadFile(filepath.Join(f.root, "health-bundles", name+".json"))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("health retention changed prior evidence")
		}
	}
	entries, err := os.ReadDir(filepath.Join(f.root, "health-bundles"))
	if err != nil || len(entries) != 3 {
		t.Fatal("health retention published beyond its budget")
	}
	if f.history.observations.Load() != observations || f.network.calls.Load() != 0 {
		t.Fatal("health bundle invoked a resource backend")
	}
	t.Log("HEALTH_BUNDLE_V2_IDLE_PENDING_FAILED_EXACT_PRIVATE_RECEIPTS_AND_LEGACY_V1_PASS")
	t.Log("HEALTH_BUNDLE_DIRECTORY_FULL_REFUSES_WITHOUT_RECEIPT_AND_PRESERVES_THREE_PRIOR_ARTIFACTS")
}
