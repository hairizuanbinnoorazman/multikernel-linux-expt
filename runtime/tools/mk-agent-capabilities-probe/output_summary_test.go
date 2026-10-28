//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
)

func TestOutputProjectionAllowsOnlyCompleteConstantStdoutMarkers(t *testing.T) {
	raw := []byte("G8_BUILD_WORKLOAD_PASS\nsecret=hidden\nG8_BUILD_WORKLOAD_BEGIN\nG8_BUILD_WORKLOAD_BEGIN\nsecret-G8_REPEAT_BUILD_PASS\nG8_BUILD_WORKLOAD_PASS")
	p := projectOutput(outputSample{Status: "STOPPED", Stdout: raw, Stderr: []byte("G8_REPEAT_BUILD_PASS\nsecret=hidden"), StdoutOffset: uint64(len(raw)) + 7, StderrOffset: 999})
	if p == nil || strings.Join(p.Markers, ",") != "G8_BUILD_WORKLOAD_BEGIN" || p.StdoutStart != 7 {
		t.Fatal("partial, duplicate, stderr, suffixed or unterminated marker accepted")
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "secret") {
		t.Fatal("raw output escaped projection")
	}
}
func TestStoppedOutputSummaryProtocolGatesAndBounds(t *testing.T) {
	for _, mode := range []string{"observed", "private-observed", "missing-field", "oversized", "offset-underflow", "wrong-output-state", "changed-final-pid", "running", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "mk-out-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "relay.sock")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			os.Chmod(path, 0600)
			o := options{socket: path, id: "box", generation: "0123456789abcdef0123456789abcdef", release: "test-release", processID: "private-target", stoppedOutputSummary: true, endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, timeout: 150 * time.Millisecond}
			if mode == "private-observed" {
				o.privateBundleDir = filepath.Join(dir, "private")
				if os.Mkdir(o.privateBundleDir, 0700) != nil {
					t.Fatal("private directory unavailable")
				}
			}
			served := make(chan []string, 1)
			go func() {
				methods := []string{}
				defer func() { served <- methods }()
				c, e := listener.Accept()
				if e != nil {
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				var last uint64
				for {
					var h [4]byte
					if _, e = io.ReadFull(c, h[:]); e != nil {
						return
					}
					n := binary.BigEndian.Uint32(h[:])
					if n > 4096 {
						methods = append(methods, "INVALID")
						return
					}
					b := make([]byte, n)
					if _, e = io.ReadFull(c, b); e != nil {
						return
					}
					var request agent.Envelope
					if json.Unmarshal(b, &request) != nil {
						return
					}
					signed := request
					agent.Sign(&signed, []byte("01234567890123456789012345678901"))
					if signed.MAC != request.MAC || request.Sequence <= last || request.SandboxID != o.id || request.Generation != o.generation || request.Endpoint != uint32(o.endpoint) {
						methods = append(methods, "INVALID")
						return
					}
					last = request.Sequence
					methods = append(methods, request.Method)
					if len(methods) > 4 {
						return
					}
					var body any
					switch request.Method {
					case "Capabilities":
						body = map[string]any{"protocol": 1, "kernel": map[string]any{"architecture": "amd64", "release": o.release}, "agent": map[string]any{"uid": 0, "gid": 0}}
					case "StateProcess":
						var q map[string]string
						if json.Unmarshal(request.Body, &q) != nil || len(q) != 1 || q["id"] != o.processID {
							methods = append(methods, "INVALID")
							return
						}
						state := agent.ProcessState{ID: o.processID, Status: "STOPPED", PID: 616, ExitCode: 137, Stdout: "secret-state-data"}
						if mode == "running" {
							state.Status = "RUNNING"
							state.ExitCode = 0
						}
						if mode == "changed-final-pid" && len(methods) == 4 {
							state.PID = 617
						}
						body = state
					case "ReadProcessOutput":
						var q map[string]any
						if json.Unmarshal(request.Body, &q) != nil || len(q) != 4 || q["id"] != o.processID || q["stdout_offset"] != float64(0) || q["stderr_offset"] != float64(0) || q["limit"] != float64(outputSampleLimit) {
							methods = append(methods, "INVALID")
							return
						}
						if mode == "deadline" {
							var closed [1]byte
							c.Read(closed[:])
							return
						}
						out := []byte("secret-output\nG8_BUILD_WORKLOAD_BEGIN\nG8_STORAGE_PRESSURE_FULL\n")
						stderr := []byte("secret-error\n")
						if mode == "oversized" {
							out = make([]byte, outputSampleLimit+1)
						}
						sample := map[string]any{"stdout": out, "stderr": stderr, "stdout_offset": len(out), "stderr_offset": len(stderr), "status": "STOPPED", "stdout_truncated": false, "stderr_truncated": false}
						if mode == "missing-field" {
							delete(sample, "stderr_truncated")
						}
						if mode == "offset-underflow" {
							sample["stdout_offset"] = 0
						}
						if mode == "wrong-output-state" {
							sample["status"] = "RUNNING"
						}
						body = sample
					default:
						methods = append(methods, "INVALID")
						return
					}
					response, _ := json.Marshal(agent.Reply{Version: 1, Sequence: request.Sequence, Body: body})
					binary.BigEndian.PutUint32(h[:], uint32(len(response)))
					c.Write(h[:])
					c.Write(response)
				}
			}()
			r := inspect(context.Background(), o, credential(t, true))
			listener.Close()
			methods := <-served
			want := []string{"Capabilities", "StateProcess", "ReadProcessOutput", "StateProcess"}
			switch mode {
			case "running":
				want = want[:2]
			case "missing-field", "oversized", "offset-underflow", "wrong-output-state", "deadline":
				want = want[:3]
			}
			if strings.Join(methods, ",") != strings.Join(want, ",") {
				t.Fatalf("RPC budget violated: %v", methods)
			}
			b, _ := json.Marshal(r)
			for _, secret := range []string{"secret", "private-target", "01234567890123456789012345678901"} {
				if strings.Contains(string(b), secret) {
					t.Fatal("private content escaped summary")
				}
			}
			version := 3
			if mode == "private-observed" {
				version = 4
			}
			if r.Version != version || r.MutatingMethodAttempted {
				t.Fatal("summary version or mutation contract violated")
			}
			if mode == "observed" || mode == "private-observed" {
				code := "STOPPED_OUTPUT_SUMMARY_OBSERVED"
				if mode == "private-observed" {
					code = "PRIVATE_STOPPED_OUTPUT_BUNDLE_WRITTEN"
				}
				if r.Status != "observed" || r.Code != code || r.Output == nil || !r.Output.StateStable || strings.Join(r.Output.Markers, ",") != "G8_BUILD_WORKLOAD_BEGIN,G8_STORAGE_PRESSURE_FULL" {
					t.Fatalf("summary missing: %+v", r)
				}
				if mode == "private-observed" {
					data, e := os.ReadFile(filepath.Join(o.privateBundleDir, privateBundleName))
					if e != nil {
						t.Fatal(e)
					}
					var stored privateStoppedOutput
					if json.Unmarshal(data, &stored) != nil || !strings.Contains(string(stored.Stdout), "secret-output") || !strings.Contains(string(stored.Stderr), "secret-error") || stored.Generation != o.generation || stored.ProcessSHA256 != r.ExpectedProcessSHA256 || !r.PrivateBundleWritten || r.PrivateBundleBytes != len(data) {
						t.Fatal("private payload capture lost binding or bytes")
					}
					clear(data)
					clear(stored.Stdout)
					clear(stored.Stderr)
				}
			} else if r.Status != "inconclusive" || r.Output != nil {
				t.Fatal("incomplete observation published output")
			}
			if mode == "deadline" && r.Code != "DEADLINE_OR_CANCELED" {
				t.Fatal("shared deadline classification lost")
			}
			if mode == "changed-final-pid" && r.Process != nil {
				t.Fatal("changed process identity retained as authority")
			}
		})
	}
}
func TestOutputSummaryRequiresExplicitTarget(t *testing.T) {
	r := inspect(context.Background(), options{stoppedOutputSummary: true}, nil)
	if r.Code != "INVALID_PROBE_INPUT" || r.RequestAttempted {
		t.Fatal("summary accepted missing process target")
	}
}
