// Package buildinfo exposes the immutable identity injected into release
// binaries. Development builds deliberately use explicit, recognizable
// defaults rather than inferring mutable state at runtime.
package buildinfo

import (
	"fmt"
	"io"
)

var (
	Version  = "0.0.0-dev"
	Revision = "unknown"
)

// PrintRequested handles the conventional standalone version invocations.
// Requiring exactly one argument avoids stealing flags from normal commands.
func PrintRequested(out io.Writer, component string, arguments []string) bool {
	if len(arguments) != 1 || (arguments[0] != "--version" && arguments[0] != "-v" && arguments[0] != "version") {
		return false
	}
	fmt.Fprintf(out, "%s version=%s revision=%s\n", component, Version, Revision)
	return true
}
