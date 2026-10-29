package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComponentBuildStrictBoundedSelfReportedContract(t *testing.T) {
	good := ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: "mkruntimed", BuildVersion: "0.0.0-dev", BuildRevision: strings.Repeat("a", 40)}
	if good.Validate() != nil {
		t.Fatal("valid build refused")
	}
	for _, component := range []string{"mkruntimed", "mknetd"} {
		v := good
		v.Component = component
		v.BuildRevision = "unknown"
		if v.Validate() != nil {
			t.Fatal(v)
		}
	}
	for name, change := range map[string]func(*ComponentBuildReport){
		"version":            func(r *ComponentBuildReport) { r.Version = 2 },
		"source":             func(r *ComponentBuildReport) { r.Source = "attested" },
		"component":          func(r *ComponentBuildReport) { r.Component = "private-process" },
		"version-path":       func(r *ComponentBuildReport) { r.BuildVersion = "/private/path" },
		"version-bound":      func(r *ComponentBuildReport) { r.BuildVersion = "1.2.3-" + strings.Repeat("x", 64) },
		"revision-path":      func(r *ComponentBuildReport) { r.BuildRevision = "/private/path" },
		"revision-short":     func(r *ComponentBuildReport) { r.BuildRevision = "abc123" },
		"revision-bound":     func(r *ComponentBuildReport) { r.BuildRevision = strings.Repeat("a", 41) },
		"revision-uppercase": func(r *ComponentBuildReport) { r.BuildRevision = strings.Repeat("A", 40) },
		"artifact-claim":     func(r *ComponentBuildReport) { r.ArtifactVerified = true },
		"kernel-claim":       func(r *ComponentBuildReport) { r.KernelVerified = true },
	} {
		t.Run(name, func(t *testing.T) {
			v := good
			change(&v)
			if v.Validate() == nil {
				t.Fatal("invalid build accepted")
			}
		})
	}
	raw, _ := json.Marshal(good)
	for _, data := range [][]byte{append([]byte(`{"version":1,`), raw[1:]...), []byte(strings.TrimSuffix(string(raw), "}") + `,"private":"synthetic-secret"}`), []byte(strings.Replace(string(raw), `"version":1`, `"version":true`, 1))} {
		var v ComponentBuildReport
		if StrictDecode(data, &v) == nil {
			t.Fatal("ambiguous/extra/type-confused report decoded")
		}
	}
}
