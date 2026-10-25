package protocol

import (
	"strings"
	"testing"
)

func TestNativeResourceIdentityValidation(t *testing.T) {
	for _, mode := range []string{"matched", "absent", "missing", "unavailable", "released", "nil", "generation", "export", "unknown", "live-skipped", "released-matched"} {
		t.Run(mode, func(t *testing.T) {
			r := NativeResourceReport{Version: 1, Source: "durable-resources-and-native-process-identity", Resources: resourceFixture()}
			v := r.Resources.Storage.Exports[0]
			r.Processes = []StorageProcessIdentity{{SandboxID: v.SandboxID, Generation: v.SandboxGeneration, ExportGeneration: v.ExportGeneration, Status: "process-matched"}}
			good := true
			switch mode {
			case "absent":
				r.Processes[0].Status = "process-absent"
			case "missing":
				r.Processes[0].Status = "record-missing"
			case "unavailable":
				r.Processes[0].Status = "unavailable"
			case "released", "released-matched":
				r.Resources.Storage.Exports[0].State = "RELEASED"
				r.Resources.Storage.Exports[0].OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("a", 64)
				if mode == "released" {
					r.Processes[0].Status = "not-probed-released"
				} else {
					good = false
				}
			case "nil":
				r.Processes = nil
				good = false
			case "generation":
				r.Processes[0].Generation = strings.Repeat("f", 32)
				good = false
			case "export":
				r.Processes[0].ExportGeneration = strings.Repeat("f", 32)
				good = false
			case "unknown":
				r.Processes[0].Status = "PRIVATE_SECRET"
				good = false
			case "live-skipped":
				r.Processes[0].Status = "not-probed-released"
				good = false
			}
			if (r.Validate() == nil) != good {
				t.Fatalf("validation = %v", r.Validate())
			}
		})
	}
}
