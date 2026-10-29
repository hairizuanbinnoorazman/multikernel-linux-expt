package protocol

import (
	"errors"
	"regexp"
)

const ComponentBuildCapability = "component-build-v1"

// ComponentBuildReport is immutable compiled metadata, not evidence of the
// running executable's digest, kernel identity, process incarnation or health.
// No paths, configuration, arguments or environment values belong here.
type ComponentBuildReport struct {
	Version          int    `json:"version"`
	Source           string `json:"source"`
	Component        string `json:"component"`
	BuildVersion     string `json:"build_version"`
	BuildRevision    string `json:"build_revision"`
	ArtifactVerified bool   `json:"artifact_verified"`
	KernelVerified   bool   `json:"kernel_verified"`
}

var componentBuildVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
var componentBuildRevision = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func (r ComponentBuildReport) Validate() error {
	if r.Version != 1 || r.Source != "compiled-component-self-report" ||
		(r.Component != "mkruntimed" && r.Component != "mknetd") ||
		len(r.BuildVersion) > 64 || !componentBuildVersion.MatchString(r.BuildVersion) ||
		(len(r.BuildRevision) > 40) ||
		(r.BuildRevision != "unknown" && !componentBuildRevision.MatchString(r.BuildRevision)) ||
		r.ArtifactVerified || r.KernelVerified {
		return errors.New("invalid component build report")
	}
	return nil
}
