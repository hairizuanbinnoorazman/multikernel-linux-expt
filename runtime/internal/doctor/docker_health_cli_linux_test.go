//go:build linux

package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestInstalledDockerHealthIdleCLIAndBundle(t *testing.T) {
	cli, f := newInstalledHealthCLIFixture(t)
	if err := os.WriteFile(filepath.Join(f.root, "engine-targets.json"), []byte(`{"version":1,"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.root, "docker-bundles"), 0700); err != nil {
		t.Fatal(err)
	}
	raw := runHealthCLI(t, f, cli, "9", 0, "")
	var report DockerHealthReport
	if protocol.StrictDecode(raw, &report) != nil || report.Status != "consistent" || !report.AllRunningOwnersObserved || len(report.Engines) != 0 {
		t.Fatal("idle CLI did not cover all absent owners")
	}
	if _, ok := DockerHealthObservationStatus(report); !ok {
		t.Fatal("invalid actual CLI contract")
	}
	raw = runHealthCLI(t, f, cli, "docker-health-bundle", 0, "")
	var receipt healthBundleCLIReceipt
	if protocol.StrictDecode(raw, &receipt) != nil || receipt.Version != 3 || receipt.Scope != "operational-health-child-engine-and-component-observation-snapshot" || receipt.ObservationStatus != "consistent" {
		t.Fatal("invalid aggregate bundle receipt")
	}
	path := filepath.Join(f.root, "docker-bundles/idle.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 || receipt.Bytes != len(data) || receipt.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("bundle publication identity differs")
	}
	var bundle struct {
		Version          int    `json:"version"`
		Scope            string `json:"scope"`
		CollectionStatus string `json:"collection_status"`
		StartedAt        string `json:"started_at"`
		FinishedAt       string `json:"finished_at"`
		Snapshot         struct {
			Components   ComponentsReport   `json:"components"`
			DockerHealth DockerHealthReport `json:"docker_health"`
		} `json:"snapshot"`
		Excluded []string `json:"excluded"`
	}
	if protocol.StrictDecode(data, &bundle) != nil || bundle.Version != 3 || bundle.CollectionStatus != "captured" || bundle.Scope != receipt.Scope {
		t.Fatal("aggregate bundle lost envelope")
	}
	if _, ok := DockerHealthObservationStatus(bundle.Snapshot.DockerHealth); !ok {
		t.Fatal("aggregate bundle lost owner coverage")
	}
}
