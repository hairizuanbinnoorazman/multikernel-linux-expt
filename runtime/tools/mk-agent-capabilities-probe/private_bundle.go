//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"golang.org/x/sys/unix"
)

const privateBundleName = "stopped-output.json"
const privateBundleLimit = 256 << 10

type privateBundleTarget struct{ directory *safefile.Directory }

func openPrivateBundleTarget(path string) (*privateBundleTarget, error) {
	directory, err := safefile.OpenDirectory(path, false)
	if err != nil {
		return nil, errors.New("private bundle directory unavailable")
	}
	_, exists, err := directory.EntryIdentity(privateBundleName)
	if err != nil || exists {
		directory.Close()
		return nil, errors.New("private bundle target unavailable")
	}
	return &privateBundleTarget{directory: directory}, nil
}
func (target *privateBundleTarget) close() { target.directory.Close() }

// This type deliberately cannot contain credentials, a client/descriptor,
// arbitrary errors or environment. Raw workload output is private payload and
// must never be printed or included in a public evidence archive.
type privateStoppedOutput struct {
	Version       int                 `json:"version"`
	Scope         string              `json:"scope"`
	SandboxID     string              `json:"sandbox_id"`
	Generation    string              `json:"generation"`
	Endpoint      uint                `json:"endpoint"`
	ProcessSHA256 string              `json:"expected_process_id_sha256"`
	Process       *processObservation `json:"process"`
	Output        *outputObservation  `json:"output"`
	Stdout        []byte              `json:"stdout"`
	Stderr        []byte              `json:"stderr"`
}

func (target *privateBundleTarget) publish(ctx context.Context, r report, sample outputSample) report {
	fail := func(code string) report { r.Status = "inconclusive"; r.Code = code; return r }
	if r.Output == nil || !r.Output.StateStable || r.Process == nil || r.Process.Status != "STOPPED" || len(sample.Stdout) > outputSampleLimit || len(sample.Stderr) > outputSampleLimit {
		return fail("PRIVATE_BUNDLE_PRECONDITION")
	}
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	data, err := json.Marshal(privateStoppedOutput{Version: 1, Scope: "private-stopped-output-window", SandboxID: r.SandboxID, Generation: r.Generation, Endpoint: r.Endpoint, ProcessSHA256: r.ExpectedProcessSHA256, Process: r.Process, Output: r.Output, Stdout: sample.Stdout, Stderr: sample.Stderr})
	if err != nil || len(data) > privateBundleLimit {
		clear(data)
		return fail("PRIVATE_BUNDLE_LIMIT")
	}
	defer clear(data)
	if ctx.Err() != nil {
		return fail("DEADLINE_OR_CANCELED")
	}
	created, identity, err := target.directory.PublishExclusiveIdentity(privateBundleName, data, 0600)
	if err != nil {
		return fail("PRIVATE_BUNDLE_COMMIT_UNCONFIRMED")
	}
	if !created {
		return fail("PRIVATE_BUNDLE_TARGET_EXISTS")
	}
	if identity.UID != uint32(os.Geteuid()) || identity.Mode&unix.S_IFMT != unix.S_IFREG || identity.Mode&0777 != 0600 || identity.Links != 1 || identity.Size != int64(len(data)) {
		return fail("PRIVATE_BUNDLE_COMMIT_UNCONFIRMED")
	}
	r.PrivateBundleWritten = true
	r.PrivateBundleBytes = len(data)
	r.Code = "PRIVATE_STOPPED_OUTPUT_BUNDLE_WRITTEN"
	return r
}
