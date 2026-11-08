package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/doctor"
)

func TestDockerHealthBundleRetainsInconclusiveAndRejectsForgedCoverage(t *testing.T) {
	snapshot := DockerHealthSnapshot{Components: healthBundleFixture("consistent").Components, DockerHealth: doctor.InspectDockerHealth(t.Context(), doctor.DockerHealthInputs{}, "fixture")}
	for _, mode := range []string{"valid", "forged-status", "forged-coverage", "private-finding"} {
		t.Run(mode, func(t *testing.T) {
			s := snapshot
			switch mode {
			case "forged-status":
				s.DockerHealth.Status = "consistent"
			case "forged-coverage":
				s.DockerHealth.AllRunningOwnersObserved = true
			case "private-finding":
				s.DockerHealth.Findings = []doctor.Finding{{Code: "SYNTHETIC_PRIVATE_TOKEN"}}
			}
			path := filepath.Join(privateDirectory(t), "docker.json")
			receipt, err := CollectDockerHealth(t.Context(), path, func(context.Context) DockerHealthSnapshot { return s })
			if mode != "valid" {
				if err == nil {
					t.Fatal("forged aggregate published")
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatal("invalid capture left artifact")
				}
				return
			}
			if err != nil || receipt.Version != 3 || receipt.Scope != DockerHealthScope || receipt.ObservationStatus != "inconclusive" {
				t.Fatal("failure observation lost", receipt, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			info, _ := os.Stat(path)
			if receipt.Bytes != len(data) || receipt.SHA256 != hex.EncodeToString(sum[:]) || info.Mode().Perm() != 0600 {
				t.Fatal("private publication receipt differs")
			}
			if _, err = CollectDockerHealth(t.Context(), path, func(context.Context) DockerHealthSnapshot { return snapshot }); err == nil {
				t.Fatal("existing bundle replaced")
			}
		})
	}
}
