package protocol

import (
	"encoding/json"
	"errors"
)

const DockerEngineObservationCapability = "docker-engine-observation-v1"

// DockerEngineObservation is a point-in-time observation of the supervisor's
// fixed child-local API and foreground launcher. It reserves no build capacity.
type DockerEngineObservation struct {
	Version int    `json:"version"`
	Source  string `json:"source"`
	Status  string `json:"status"`
}

func (r DockerEngineObservation) Validate() error {
	if r.Version != 1 || r.Source != "owned-child-docker-api-and-launcher" {
		return errors.New("invalid Docker engine observation")
	}
	switch r.Status {
	case "responding", "unavailable", "exited", "stopping", "stopped":
		return nil
	}
	return errors.New("invalid Docker engine observation status")
}

func (r *DockerEngineObservation) UnmarshalJSON(data []byte) error {
	*r = DockerEngineObservation{}
	var fields map[string]json.RawMessage
	if len(data) > 1024 || StrictDecode(data, &fields) != nil || len(fields) != 3 {
		return errors.New("invalid Docker engine observation fields")
	}
	for _, name := range []string{"version", "source", "status"} {
		if _, ok := fields[name]; !ok {
			return errors.New("invalid Docker engine observation fields")
		}
	}
	// Pointers reject null as well as omitted fields. Exact keys above also
	// exclude case variants accepted by encoding/json's struct matching.
	var wire struct {
		Version *int    `json:"version"`
		Source  *string `json:"source"`
		Status  *string `json:"status"`
	}
	if StrictDecode(data, &wire) != nil || wire.Version == nil || wire.Source == nil || wire.Status == nil {
		return errors.New("incomplete Docker engine observation")
	}
	v := DockerEngineObservation{Version: *wire.Version, Source: *wire.Source, Status: *wire.Status}
	if err := v.Validate(); err != nil {
		return err
	}
	*r = v
	return nil
}
