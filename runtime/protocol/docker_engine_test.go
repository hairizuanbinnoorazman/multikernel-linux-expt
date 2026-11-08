package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDockerEngineObservationWireContract(t *testing.T) {
	valid := `{"version":1,"source":"owned-child-docker-api-and-launcher","status":"responding"}`
	for _, status := range []string{"responding", "unavailable", "exited", "stopping", "stopped"} {
		var r DockerEngineObservation
		if err := json.Unmarshal([]byte(strings.Replace(valid, "responding", status, 1)), &r); err != nil || r.Status != status {
			t.Fatalf("valid %s refused: %v", status, err)
		}
	}
	for _, input := range []string{
		`{}`, `null`, `[]`, valid + valid,
		strings.Replace(valid, `"version":1`, `"version":null`, 1),
		strings.Replace(valid, `"version":1`, `"version":1.0`, 1),
		strings.Replace(valid, `"version":1`, `"Version":1`, 1),
		strings.Replace(valid, `"status":"responding"`, `"status":null`, 1),
		strings.Replace(valid, `"status":"responding"`, `"status":"ready"`, 1),
		strings.Replace(valid, `"status":"responding"`, `"status":"unavailable","status":"responding"`, 1),
		strings.Replace(valid, `"source":"owned-child-docker-api-and-launcher",`, ``, 1),
		strings.Replace(valid, `"source":"owned-child-docker-api-and-launcher"`, `"source":"host-docker"`, 1),
		strings.Replace(valid, `}`, `,"output":"private"}`, 1),
		strings.Replace(valid, "owned-child-docker-api-and-launcher", strings.Repeat("x", 1025), 1),
	} {
		r := DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"}
		if json.Unmarshal([]byte(input), &r) == nil {
			t.Fatalf("invalid observation accepted: %s", input)
		}
		// Direct UnmarshalJSON clears a previously successful value on refusal.
		if r.UnmarshalJSON([]byte(input)) == nil || r != (DockerEngineObservation{}) {
			t.Fatal("refusal retained stale success")
		}
	}
}
