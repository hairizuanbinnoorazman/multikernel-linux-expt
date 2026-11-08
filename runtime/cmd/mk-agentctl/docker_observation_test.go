package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestAgentctlObservationWorker(t *testing.T) {
	if os.Getenv("MK_TEST_AGENTCTL_OBSERVATION") != "1" {
		t.Skip("subprocess fixture only")
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet("mk-agentctl", flag.ExitOnError)
	main()
	os.Exit(0)
}

func TestAgentctlObservationReadOnlyMode(t *testing.T) {
	for _, status := range []string{"responding", "unavailable", "unsupported"} {
		t.Run(status, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			socket := filepath.Join(dir, "agent.sock")
			token := bytes.Repeat([]byte{1}, 32)
			keyfile := filepath.Join(dir, "token")
			if err := os.WriteFile(keyfile, []byte(hex.EncodeToString(token)+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			server := &agent.Server{Manager: agent.NewManager(true), SandboxID: "box", Generation: "generation", Endpoint: 7001, Token: token}
			calls := 0
			if status != "unsupported" {
				server.ObserveDockerEngine = func(context.Context) (protocol.DockerEngineObservation, error) {
					calls++
					return protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: status}, nil
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- server.Serve(ctx, listener) }()
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(exe, "-test.run=^TestAgentctlObservationWorker$", "--", "--inspect-docker-engine", "--sandbox-id=box", "--generation=generation", "--port=7001", "--token-file="+keyfile, "--unix-socket="+socket)
			command.Env = append(os.Environ(), "MK_TEST_AGENTCTL_OBSERVATION=1")
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err = command.Run()
			if status == "responding" && err != nil || status != "responding" && err == nil {
				t.Fatalf("incorrect observation exit: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if status == "unsupported" {
				if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Docker observation failed") {
					t.Fatalf("unsupported success output: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			} else {
				var report protocol.DockerEngineObservation
				if json.Unmarshal(stdout.Bytes(), &report) != nil || report.Status != status {
					t.Fatalf("invalid observation output: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			}
			cancel()
			listener.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("server did not stop")
			}
			want := 1
			if status == "unsupported" {
				want = 0
			}
			if calls != want || !server.Manager.Quiescent() {
				t.Fatal("observation performed workload operations")
			}
		})
	}
}
