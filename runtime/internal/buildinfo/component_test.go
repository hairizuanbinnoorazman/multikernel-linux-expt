package buildinfo

import (
	"strings"
	"testing"
)

func TestComponentReturnsCompiledLabelsOnlyAndRefusesInvalidInjection(t *testing.T) {
	oldVersion, oldRevision := Version, Revision
	t.Cleanup(func() { Version, Revision = oldVersion, oldRevision })
	Version, Revision = "1.2.3-test", "abcdef0"
	for _, name := range []string{"mkruntimed", "mknetd"} {
		r, err := Component(name)
		if err != nil || r.Validate() != nil || r.Component != name || r.BuildVersion != Version || r.BuildRevision != Revision || r.ArtifactVerified || r.KernelVerified {
			t.Fatalf("%+v %v", r, err)
		}
	}
	Revision = "private/secret/token"
	r, err := Component("mkruntimed")
	if err == nil || r.Component != "" || strings.Contains(err.Error(), "private") {
		t.Fatal("invalid injected label accepted or reflected")
	}
}
