package buildinfo

import "github.com/hairizuan/multikernel-linux-expt/runtime/protocol"

// Component reports only compiled labels. Caller-supplied fields, disk paths,
// environment, VCS working state and external commands are never inspected.
func Component(name string) (protocol.ComponentBuildReport, error) {
	r := protocol.ComponentBuildReport{Version: 1, Source: "compiled-component-self-report", Component: name, BuildVersion: Version, BuildRevision: Revision}
	if err := r.Validate(); err != nil {
		return protocol.ComponentBuildReport{}, err
	}
	return r, nil
}
