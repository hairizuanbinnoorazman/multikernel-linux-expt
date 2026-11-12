//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/terminallog"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const runtimeName = "io.containerd.multikernel.v2"

const runtimeInfoInputLimit = 1 << 20

var _ taskapi.TaskService = (*service)(nil)

func writeRuntimeInfo(input io.Reader, output io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(input, runtimeInfoInputLimit+1))
	if err != nil {
		return fmt.Errorf("read runtime options: %w", err)
	}
	if len(data) > runtimeInfoInputLimit {
		return errors.New("runtime options exceed the one-MiB limit")
	}
	var options *anypb.Any
	if len(data) != 0 {
		options = &anypb.Any{}
		if err = proto.Unmarshal(data, options); err != nil {
			return fmt.Errorf("decode runtime options: %w", err)
		}
	}
	payload, err := proto.Marshal(&types.RuntimeInfo{
		Name: runtimeName,
		Version: &types.RuntimeVersion{
			Version:  buildinfo.Version,
			Revision: buildinfo.Revision,
		},
		Options: options,
	})
	if err != nil {
		return fmt.Errorf("encode runtime info: %w", err)
	}
	written, err := output.Write(payload)
	if err == nil && written != len(payload) {
		err = io.ErrShortWrite
	}
	return err
}

func main() {
	if len(os.Args) == 3 && os.Args[1] == fifoKeeperMode {
		os.Exit(runOutputFIFOKeeper(os.Args[2:]))
	}
	if len(os.Args) == 2 && os.Args[1] == namespaceHolderMode {
		os.Exit(runNamespaceHolder())
	}
	if buildinfo.PrintRequested(os.Stdout, "containerd-shim-multikernel-v2", os.Args[1:]) {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--inspect-create-intent" {
		os.Exit(runCreateIntentInspection(os.Args[2:], intentStdoutWriter{}))
	}
	// The containerd 2.x discovery probe is a command-like flag. Intercept it
	// before the vendored 1.7 shim runner parses flags.
	if len(os.Args) == 2 && os.Args[1] == "-info" {
		if err := writeRuntimeInfo(os.Stdin, os.Stdout); err != nil {
			terminallog.Report(terminallog.ShimRuntimeInfo)
			os.Exit(1)
		}
		return
	}
	// Keep the vendored shim's FIFO setup and process semantics. Its library
	// records must cross the fixed, bounded boundary before formatting/writing.
	logrus.AddHook(newLibraryLogBoundary(nil))
	if serverInvocation() && os.Getenv("MK_SHIM_WORKER") != "1" {
		os.Exit(superviseShimWorker())
	}
	shim.Run(runtimeName, protectShimEntry(inspectionInitializer(plugin.Register)))
}
