//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	types "github.com/containerd/containerd/api/types"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestLongDaemonTimeoutsCoverServerBoundsWithoutChangingDefault(t *testing.T) {
	base := daemon.Client{Timeout: 30 * time.Second}
	rootfsClient := daemonCallerWithTimeout(base, rootfsPrepareDaemonTimeout).(daemon.Client)
	createClient := daemonCallerWithTimeout(base, createSandboxDaemonTimeout).(daemon.Client)
	if base.Timeout != 30*time.Second {
		t.Fatalf("base timeout changed to %s", base.Timeout)
	}
	if rootfsClient.Timeout <= 10*time.Minute {
		t.Fatalf("rootfs timeout %s does not cover the ten-minute builder bound", rootfsClient.Timeout)
	}
	if createClient.Timeout <= time.Hour {
		t.Fatalf("create timeout %s does not cover the one-hour backend bound", createClient.Timeout)
	}
	fake := daemonCallFunc(func(context.Context, protocol.Request, any) *protocol.Error { return nil })
	if _, ok := daemonCallerWithTimeout(fake, rootfsPrepareDaemonTimeout).(daemonCallFunc); !ok {
		t.Fatal("injected daemon caller was replaced")
	}
}

func privateTestDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestWriteRuntimeInfo(t *testing.T) {
	for _, test := range []struct {
		name    string
		options *anypb.Any
	}{
		{name: "empty"},
		{name: "options", options: &anypb.Any{TypeUrl: "types.example/options", Value: []byte("exact")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var input, output bytes.Buffer
			if test.options != nil {
				data, err := proto.Marshal(test.options)
				if err != nil {
					t.Fatal(err)
				}
				input.Write(data)
			}
			if err := writeRuntimeInfo(&input, &output); err != nil {
				t.Fatal(err)
			}
			var info types.RuntimeInfo
			if err := proto.Unmarshal(output.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
			if info.Name != runtimeName || info.Version == nil || info.Version.Version == "" || info.Version.Revision == "" ||
				!proto.Equal(info.Options, test.options) {
				t.Fatalf("runtime info name=%q version=%v options=%v", info.Name, info.Version, info.Options)
			}
			if info.Features != nil {
				t.Fatalf("unsubstantiated OCI features advertised: %+v", info.Features)
			}
		})
	}
}

func TestWriteRuntimeInfoRejectsMalformedAndOversizedOptions(t *testing.T) {
	for _, input := range [][]byte{{0xff}, bytes.Repeat([]byte("x"), runtimeInfoInputLimit+1)} {
		var output bytes.Buffer
		if err := writeRuntimeInfo(bytes.NewReader(input), &output); err == nil || output.Len() != 0 {
			t.Fatalf("input length %d: error=%v output=%d", len(input), err, output.Len())
		}
	}
}

func testBundleIdentity(t *testing.T, path string) rootfspkg.DirectoryIdentity {
	t.Helper()
	directory, err := safefile.OpenOwnedDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	identity := directory.Identity()
	if err = directory.Close(); err != nil {
		t.Fatal(err)
	}
	return rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}
}

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == fifoKeeperMode {
		os.Exit(runOutputFIFOKeeper(os.Args[2:]))
	}
	if output := os.Getenv("MK_SHIM_SUPERVISOR_IDENTITY_OUTPUT"); output != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		info, err := os.Stat(".")
		if err != nil {
			os.Exit(97)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			os.Exit(96)
		}
		value := fmt.Sprintf("%d %d %d %s %s %s\n", stat.Dev, stat.Ino, stat.Uid,
			os.Getenv("MK_SHIM_BUNDLE_DEVICE"), os.Getenv("MK_SHIM_BUNDLE_INODE"), os.Getenv("MK_SHIM_BUNDLE_UID"))
		if os.WriteFile(output, []byte(value), 0600) != nil {
			os.Exit(95)
		}
		os.Exit(0)
	}
	if marker := os.Getenv("MK_SHIM_SUPERVISOR_TEST_MARKER"); marker != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("first-worker-signaled\n"), 0600)
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			os.Exit(99)
		}
		os.Exit(0)
	}
	if marker := os.Getenv("MK_SHIM_SUPERVISOR_BLOCK_WORKER"); marker != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		_ = os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
		// Keep an actual pending timer: an empty select makes an otherwise
		// idle non-race helper fail the runtime's deadlock detector. The
		// supervisor owns termination; race instrumentation must not affect it.
		for {
			time.Sleep(time.Hour)
		}
	}
	if marker := os.Getenv("MK_SHIM_SUPERVISOR_PROXY_FIRST_MARKER"); marker != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("first-worker-proxy-first\n"), 0600)
			_ = syscall.Close(3)
			time.Sleep(50 * time.Millisecond)
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			os.Exit(99)
		}
		file, _ := os.OpenFile(marker, os.O_APPEND|os.O_WRONLY, 0600)
		if file != nil {
			_, _ = file.WriteString("second-worker-clean\n")
			_ = file.Close()
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
