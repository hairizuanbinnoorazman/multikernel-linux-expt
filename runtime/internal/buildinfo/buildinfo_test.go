package buildinfo

import (
	"bytes"
	"testing"
)

func TestPrintRequested(t *testing.T) {
	oldVersion, oldRevision := Version, Revision
	Version, Revision = "1.2.3", "0123456789abcdef"
	t.Cleanup(func() { Version, Revision = oldVersion, oldRevision })
	for _, argument := range []string{"--version", "-v", "version"} {
		var output bytes.Buffer
		if !PrintRequested(&output, "component", []string{argument}) {
			t.Fatalf("%q was not recognized", argument)
		}
		if got, want := output.String(), "component version=1.2.3 revision=0123456789abcdef\n"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	for _, arguments := range [][]string{nil, {"--version", "extra"}, {"--help"}} {
		var output bytes.Buffer
		if PrintRequested(&output, "component", arguments) || output.Len() != 0 {
			t.Fatalf("unexpected match for %#v", arguments)
		}
	}
}
