package protocol

import "errors"

const NativeResourcesCapability = "native-resource-identity-v1"

type NativeResourceReport struct {
	Version   int                      `json:"version"`
	Source    string                   `json:"source"`
	Resources ResourceOwnershipReport  `json:"resources"`
	Processes []StorageProcessIdentity `json:"processes"`
}
type StorageProcessIdentity struct {
	SandboxID        string `json:"sandbox_id"`
	Generation       string `json:"generation"`
	ExportGeneration string `json:"export_generation"`
	Status           string `json:"status"`
}

func (r NativeResourceReport) Validate() error {
	if r.Version != 1 || r.Source != "durable-resources-and-native-process-identity" || r.Resources.Validate() != nil || r.Processes == nil || len(r.Processes) != len(r.Resources.Storage.Exports) {
		return errors.New("invalid native resource identity report")
	}
	for i, p := range r.Processes {
		e := r.Resources.Storage.Exports[i]
		if p.SandboxID != e.SandboxID || p.Generation != e.SandboxGeneration || p.ExportGeneration != e.ExportGeneration {
			return errors.New("invalid native process binding")
		}
		if e.State == "RELEASED" {
			if p.Status != "not-probed-released" {
				return errors.New("invalid released process scope")
			}
		} else {
			switch p.Status {
			case "process-matched", "record-missing", "process-absent", "unavailable":
			default:
				return errors.New("invalid native process status")
			}
		}
	}
	return nil
}
