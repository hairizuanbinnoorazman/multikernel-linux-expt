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
	"golang.org/x/sys/unix"
)

func credential(t *testing.T, sealed bool) *os.File {
	t.Helper()
	fd, err := unix.MemfdCreate("probe-test", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(fd), "probe-test")
	t.Cleanup(func() { f.Close() })
	if err = f.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("01234567890123456789012345678901")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if sealed {
		if _, err = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, tokenSeals); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func TestCredentialRequiresSealedPrivateDescriptor(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		f := credential(t, sealed)
		key, err := readToken(f)
		if sealed && (err != nil || len(key) != 32) {
			t.Fatalf("sealed descriptor rejected: %v", err)
		}
		if !sealed && (err == nil || key != nil) {
			t.Fatal("unsealed credential accepted")
		}
		clear(key)
	}
	f, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write([]byte("01234567890123456789012345678901"))
	f.Seek(0, 0)
	if _, err = readToken(f); err == nil {
		t.Fatal("named regular file accepted")
	}
}
func TestProbeSingleRequestAndSanitizedOutcomes(t *testing.T) {
	for _, mode := range []string{"matched", "missing-root-claims", "wrong-release", "malformed", "timeout", "wrong-peer", "unsealed"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "mk-caps-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "relay.sock")
			l, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			os.Chmod(path, 0600)
			o := options{socket: path, id: "box", generation: "0123456789abcdef0123456789abcdef", release: "test-release", endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, timeout: 100 * time.Millisecond}
			if mode == "wrong-peer" {
				o.peerPID += 100000
			}
			served := make(chan int, 1)
			go func() {
				c, err := l.Accept()
				if err != nil {
					served <- 0
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				var h [4]byte
				if _, err = io.ReadFull(c, h[:]); err != nil {
					served <- 0
					return
				}
				n := binary.BigEndian.Uint32(h[:])
				if n > 4096 {
					served <- -1
					return
				}
				b := make([]byte, n)
				io.ReadFull(c, b)
				var e agent.Envelope
				if json.Unmarshal(b, &e) != nil {
					served <- -1
					return
				}
				signed := e
				agent.Sign(&signed, []byte("01234567890123456789012345678901"))
				if e.MAC != signed.MAC || e.Method != "Capabilities" || e.SandboxID != o.id || e.Generation != o.generation || e.Endpoint != uint32(o.endpoint) {
					served <- -1
					return
				}
				body := map[string]any{"protocol": 1, "kernel": map[string]any{"architecture": "amd64", "release": "test-release"}, "agent": map[string]any{"uid": 0, "gid": 0}}
				if mode == "missing-root-claims" {
					delete(body, "agent")
				}
				if mode == "wrong-release" {
					body["kernel"] = map[string]any{"architecture": "amd64", "release": "secret-unexpected-release"}
				}
				response, _ := json.Marshal(agent.Reply{Version: 1, Sequence: e.Sequence, Body: body})
				if mode == "malformed" {
					response = []byte("secret-invalid-reply")
				}
				if mode == "timeout" {
					var buf [1]byte
					c.Read(buf[:])
					served <- 1
					return
				}
				binary.BigEndian.PutUint32(h[:], uint32(len(response)))
				c.Write(h[:])
				c.Write(response)
				var extra [1]byte
				nread, _ := c.Read(extra[:])
				if nread != 0 {
					served <- -1
					return
				}
				served <- 1
			}()
			r := inspect(context.Background(), o, credential(t, mode != "unsealed"))
			l.Close()
			count := <-served
			want := 1
			if mode == "wrong-peer" || mode == "unsealed" {
				want = 0
			}
			if count != want || r.RequestAttempted != (want == 1) || r.MutatingMethodAttempted {
				t.Fatalf("request contract: count=%d report=%+v", count, r)
			}
			if (r.Status == "matched") != (mode == "matched") {
				t.Fatalf("unexpected result %+v", r)
			}
			if mode == "missing-root-claims" && r.GuestRootClaimMatches {
				t.Fatal("omitted root identity accepted")
			}
			b, _ := json.Marshal(r)
			if strings.Contains(string(b), "secret") || strings.Contains(string(b), "01234567890123456789012345678901") {
				t.Fatal("unsafe reply or credential appeared in report")
			}
		})
	}
}

func TestProcessInspectionIsBoundedGatedAndRedacted(t *testing.T) {
	for _, mode := range []string{"running", "created", "stopped", "not-found", "wrong-id", "invalid-state", "invalid-pid", "remote-error", "deadline", "capability-mismatch", "invalid-target"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "mk-state-")
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
			o := options{socket: path, id: "box", generation: "0123456789abcdef0123456789abcdef", release: "test-release", processID: "private-target", endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, timeout: 150 * time.Millisecond}
			if mode == "invalid-target" {
				o.processID = "../private-target"
			}
			done := make(chan []string, 1)
			go func() {
				methods := []string{}
				defer func() { done <- methods }()
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
						methods = append(methods, "INVALID")
						return
					}
					var request agent.Envelope
					if json.Unmarshal(b, &request) != nil {
						methods = append(methods, "INVALID")
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
					if len(methods) > 2 {
						return
					}
					reply := map[string]any{"version": 1, "sequence": request.Sequence}
					switch request.Method {
					case "Capabilities":
						release := "test-release"
						if mode == "capability-mismatch" {
							release = "secret-other-kernel"
						}
						reply["body"] = map[string]any{"protocol": 1, "kernel": map[string]any{"architecture": "amd64", "release": release}, "agent": map[string]any{"uid": 0, "gid": 0}}
					case "StateProcess":
						var query map[string]string
						if json.Unmarshal(request.Body, &query) != nil || len(query) != 1 || query["id"] != o.processID {
							methods = append(methods, "INVALID")
							return
						}
						if mode == "deadline" {
							var closed [1]byte
							c.Read(closed[:])
							return
						}
						state := agent.ProcessState{ID: o.processID, Status: "RUNNING", PID: 123, Stdout: "secret-private-output", Stderr: "secret-private-error"}
						switch mode {
						case "created":
							state.Status = "CREATED"
							state.PID = 0
						case "stopped":
							state.Status = "STOPPED"
							state.ExitCode = 7
						case "wrong-id":
							state.ID = "secret-wrong-process"
						case "invalid-state":
							state.Status = "secret-unknown-state"
						case "invalid-pid":
							state.PID = -1
						}
						if mode == "not-found" || mode == "remote-error" {
							code := "NOT_FOUND"
							if mode == "remote-error" {
								code = "INTERNAL"
							}
							reply["error"] = map[string]any{"code": code, "message": "secret-private-error", "operation_id": "request", "retryable": false}
						} else {
							reply["body"] = state
						}
					default:
						methods = append(methods, "INVALID")
						return
					}
					response, _ := json.Marshal(reply)
					binary.BigEndian.PutUint32(h[:], uint32(len(response)))
					c.Write(h[:])
					c.Write(response)
				}
			}()
			started := time.Now()
			r := inspect(context.Background(), o, credential(t, true))
			listener.Close()
			methods := <-done
			expected := []string{"Capabilities", "StateProcess"}
			if mode == "invalid-target" {
				expected = nil
			}
			if mode == "capability-mismatch" {
				expected = []string{"Capabilities"}
			}
			if strings.Join(methods, ",") != strings.Join(expected, ",") {
				t.Fatalf("unexpected RPC sequence %v", methods)
			}
			if mode != "invalid-target" && r.Version != 2 {
				t.Fatal("explicit process mode did not version its report")
			}
			b, _ := json.Marshal(r)
			for _, secret := range []string{"secret", "private-target", "01234567890123456789012345678901"} {
				if strings.Contains(string(b), secret) {
					t.Fatal("private target, guest output or credential escaped projection")
				}
			}
			if r.MutatingMethodAttempted {
				t.Fatal("mutating method reported")
			}
			switch mode {
			case "running", "created", "stopped":
				if r.Status != "observed" || r.Code != "PROCESS_STATE_OBSERVED" || r.Process == nil || r.ExpectedProcessSHA256 == "" {
					t.Fatalf("state not observed: %+v", r)
				}
				if mode == "stopped" && (r.Process.ExitCode == nil || *r.Process.ExitCode != 7) {
					t.Fatal("exit observation lost")
				}
			case "not-found":
				if r.Status != "observed" || r.Code != "PROCESS_NOT_FOUND" || r.Process == nil || r.Process.Status != "NOT_FOUND" {
					t.Fatalf("absence classification %+v", r)
				}
			case "deadline":
				if r.Code != "DEADLINE_OR_CANCELED" || time.Since(started) > time.Second {
					t.Fatal("whole-probe deadline not enforced")
				}
			case "capability-mismatch", "invalid-target":
				if r.ProcessQueryAttempted || r.Process != nil {
					t.Fatal("process query bypassed prerequisite")
				}
			default:
				if r.Status != "inconclusive" || r.Process != nil {
					t.Fatal("unsafe state/error accepted")
				}
			}
		})
	}
}
