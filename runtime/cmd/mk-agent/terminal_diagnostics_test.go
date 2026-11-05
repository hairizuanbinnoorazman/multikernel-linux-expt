//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentTerminalDiagnosticsRealStderr(t *testing.T) {
	if os.Getenv("MK_G8_AGENT_TERMINAL_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("mk-agent", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"mk-agent"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		os.Exit(0)
	}
	cases := []struct {
		name, code string
		exit       int
	}{
		{"bootstrap", "AGENT_DOCKER_BOOTSTRAP_FAILED", 126},
		{"credential-exec", "AGENT_CREDENTIAL_EXEC_FAILED", 126},
		{"oci-exec", "AGENT_OCI_EXEC_FAILED", 126},
		{"arguments", "AGENT_ARGUMENTS_INVALID", 2},
		{"credential", "AGENT_CREDENTIAL_REFUSED", 2},
		{"socket", "AGENT_SOCKET_REQUIRED", 2},
		{"docker-root", "AGENT_DOCKER_ROOT_REFUSED", 2},
		{"listen", "AGENT_LISTEN_FAILED", 1},
	}
	for _, tc := range cases {
		for _, mode := range []string{"capture", "blocked", "closed"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				directory := t.TempDir()
				if err := os.Chmod(directory, 0700); err != nil {
					t.Fatal(err)
				}
				token := filepath.Join(directory, "SYNTHETIC_AGENT_PRIVATE_TOKEN")
				if err := os.WriteFile(token, []byte(strings.Repeat("c", 64)+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"--sandbox-id=fixture", "--generation=" + strings.Repeat("a", 32), "--port=4050", "--token-file=" + token}
				switch tc.name {
				case "bootstrap":
					args = []string{"__docker_bootstrap"}
				case "credential-exec":
					args = []string{"__oci_exec_fd", "SYNTHETIC_PRIVATE_DESCRIPTOR"}
				case "oci-exec":
					args = []string{"__oci_exec", "SYNTHETIC_PRIVATE_CONSTRAINTS!", "/SYNTHETIC_PRIVATE_WORKLOAD"}
				case "arguments":
					args = nil
				case "credential":
					args[3] = "--token-file=" + filepath.Join(directory, "SYNTHETIC_PRIVATE_MISSING", "token")
				case "socket":
				case "docker-root":
					args = append(args, "--unix-socket="+filepath.Join(directory, "control.sock"), "--docker-guest", "--test-no-chroot")
				case "listen":
					args = append(args, "--unix-socket="+filepath.Join(directory, "SYNTHETIC_PRIVATE_SOCKET_PARENT", "control.sock"))
				}
				ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestAgentTerminalDiagnosticsRealStderr$", "--"}, args...)...)
				cmd.Env = append(os.Environ(), "MK_G8_AGENT_TERMINAL_CHILD=1", "GORACE=atexit_sleep_ms=0")
				// An inherited /dev/null forces bootstrap refusal before any exec,
				// filesystem setup or seccomp installation, including on a root test host.
				null, err := os.Open(os.DevNull)
				if err != nil {
					t.Fatal(err)
				}
				defer null.Close()
				cmd.ExtraFiles = []*os.File{null}
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				if mode == "capture" {
					cmd.Stderr = &stderr
				} else {
					reader, writer, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					defer reader.Close()
					defer writer.Close()
					if mode == "closed" {
						reader.Close()
					} else {
						fd := int(writer.Fd())
						if err := unix.SetNonblock(fd, true); err != nil {
							t.Fatal(err)
						}
						padding := bytes.Repeat([]byte("x"), 4096)
						for {
							_, err = unix.Write(fd, padding)
							if errors.Is(err, unix.EAGAIN) {
								break
							}
							if err != nil {
								t.Fatal(err)
							}
						}
						if err := unix.SetNonblock(fd, false); err != nil {
							t.Fatal(err)
						}
					}
					cmd.Stderr = writer
				}
				started := time.Now()
				err = cmd.Run()
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != tc.exit || ctx.Err() != nil || time.Since(started) > 3*time.Second || stdout.Len() != 0 {
					t.Fatal("terminal failure did not preserve bounded exit/status", err)
				}
				if mode == "capture" {
					var record struct {
						Version    int    `json:"version"`
						Component  string `json:"component"`
						Scope      string `json:"scope"`
						Code       string `json:"code"`
						Operation  string `json:"operation"`
						PID        int    `json:"pid"`
						OwnerKnown bool   `json:"owner_known"`
						At         string `json:"at"`
					}
					var fields map[string]json.RawMessage
					if json.Unmarshal(stderr.Bytes(), &record) != nil || json.Unmarshal(stderr.Bytes(), &fields) != nil || len(fields) != 8 || stderr.Len() > 1024 || bytes.Count(stderr.Bytes(), []byte("\n")) != 1 {
						t.Fatal("wrong terminal diagnostic schema")
					}
					if record.Version != 1 || record.Component != "mk-agent" || record.Scope != "daemon-terminal" || record.Code != tc.code || record.Operation == "" || record.PID != cmd.Process.Pid || record.OwnerKnown {
						t.Fatal("wrong terminal diagnostic metadata")
					}
					if _, err := time.Parse(time.RFC3339Nano, record.At); err != nil {
						t.Fatal("invalid timestamp")
					}
					if bytes.Contains(stderr.Bytes(), []byte("SYNTHETIC_")) || bytes.Contains(stderr.Bytes(), []byte(directory)) || bytes.Contains(stderr.Bytes(), []byte(strings.Repeat("c", 64))) {
						t.Fatal("private invocation or credential reached terminal diagnostic")
					}
				}
				// Diagnostics must preserve one-shot credential consumption.
				_, statErr := os.Stat(token)
				consumed := tc.name == "socket" || tc.name == "docker-root" || tc.name == "listen"
				if consumed != errors.Is(statErr, os.ErrNotExist) {
					t.Fatal("credential consumption contract changed", statErr)
				}
			})
		}
	}
}
