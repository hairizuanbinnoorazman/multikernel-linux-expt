//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/sealedinput"
	"golang.org/x/sys/unix"
)

const secret = "SYNTHETIC_SIGNAL_PRIVATE_PAYLOAD"

func sealed(t *testing.T, data []byte, seal bool) *os.File {
	t.Helper()
	fd, e := unix.MemfdCreate("replay-test", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if e != nil {
		t.Fatal(e)
	}
	f := os.NewFile(uintptr(fd), "replay-test")
	t.Cleanup(func() { f.Close() })
	f.Chmod(0600)
	f.Write(data)
	f.Seek(0, 0)
	if seal {
		if _, e = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, sealedinput.Seals); e != nil {
			t.Fatal(e)
		}
	}
	return f
}
func inputs(t *testing.T) (options, intent, string) {
	t.Helper()
	root, e := os.MkdirTemp("/tmp", "signal-replay-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	attempt := filepath.Join(root, "attempt")
	if e = os.Mkdir(attempt, 0700); e != nil {
		t.Fatal(e)
	}
	d, e := safefile.OpenDirectory(attempt, false)
	if e != nil {
		t.Fatal(e)
	}
	x := d.Identity()
	d.Close()
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		t.Fatal(e)
	}
	ack := false
	o := options{socket: filepath.Join(root, "peer.sock"), id: "box", generation: strings.Repeat("a", 32), release: "test-release", attemptDir: attempt, endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, intentFD: 4, timeout: 200 * time.Millisecond}
	v := intent{Version: 1, Scope: "persisted-shim-pending-signal", BootID: strings.TrimSpace(string(boot)), SandboxID: o.id, Generation: o.generation, Endpoint: 7200, Release: o.release, ProcessID: "init", GuestPID: 613, Signal: 9, OperationID: strings.Repeat("b", 32), Acknowledged: &ack, RecordSHA256: strings.Repeat("c", 64)}
	v.AttemptDirectory.Device = x.Device
	v.AttemptDirectory.Inode = x.Inode
	v.AttemptDirectory.UID = x.UID
	v.RecordDirectory = filepath.Join(root, "source")
	if e = os.Mkdir(v.RecordDirectory, 0700); e != nil {
		t.Fatal(e)
	}
	source, e := safefile.OpenDirectory(v.RecordDirectory, false)
	if e != nil {
		t.Fatal(e)
	}
	identity := source.Identity()
	source.Close()
	v.RecordDirectoryIdentity.Device = identity.Device
	v.RecordDirectoryIdentity.Inode = identity.Inode
	v.RecordDirectoryIdentity.UID = identity.UID
	data := sourceRecord(v, false)
	if e = os.WriteFile(filepath.Join(v.RecordDirectory, "sandbox.json"), data, 0600); e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(data)
	v.RecordSHA256 = hex.EncodeToString(digest[:])
	v.RecordIdentity = captureRecord(t, v.RecordDirectory)
	return o, v, root
}
func captureRecord(t *testing.T, path string) recordIdentity {
	t.Helper()
	d, e := safefile.OpenDirectory(path, false)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	_, found, x, e := d.ReadPrivateIdentity("sandbox.json", 8<<20)
	if e != nil || !found {
		t.Fatal("fixture source identity unavailable")
	}
	return bindRecord(x)
}
func sourceRecord(v intent, ack bool) []byte {
	data, _ := json.Marshal(map[string]any{"schema_version": 4, "id": v.SandboxID, "generation": v.Generation, "source_private": secret, "processes": []map[string]any{{"id": "", "status": 2, "pending_signal": v.Signal, "pending_signal_operation": v.OperationID, "pending_signal_acknowledgement": ack}}})
	return data
}
func manifest(t *testing.T, v intent) *os.File {
	data, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return sealed(t, data, true)
}
func assertPrivate(t *testing.T, r report, v intent) {
	t.Helper()
	raw, _ := json.Marshal(r)
	for _, value := range []string{secret, v.OperationID, v.RecordSHA256, v.RecordDirectory, "01234567890123456789012345678901"} {
		if strings.Contains(string(raw), value) {
			t.Fatal("private value escaped report")
		}
	}
	if strings.Contains(string(raw), `"`+v.ProcessID+`"`) {
		t.Fatal("private target escaped report")
	}
	if r.OriginalIntentModified || r.SignalAcknowledgementSent || r.QuiesceOrShutdownAttempted || r.HostResourceMutationAttempted || r.NormalCleanupQualified {
		t.Fatal("replay gained extra authority")
	}
}
func TestReplayWireBudgetIdentityAndPrivateFailures(t *testing.T) {
	for _, mode := range []string{"stopped", "still-running", "already-stopped", "wrong-pid", "changed-final-pid", "wrong-release", "missing-feature", "remote-refusal", "malformed-refusal", "signal-timeout", "publish-uncertain", "cancel-after-publication", "directory-replaced", "wrong-peer", "unsealed-key", "wrong-id", "created", "bad-exit", "missing-root", "source-changed", "source-directory-replaced", "source-changed-after-publication"} {
		t.Run(mode, func(t *testing.T) {
			o, v, root := inputs(t)
			key := []byte("01234567890123456789012345678901")
			if mode == "wrong-peer" {
				o.peerPID += 100000
			}
			listener, e := net.Listen("unix", o.socket)
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			os.Chmod(o.socket, 0600)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			original := publishAttempt
			defer func() { publishAttempt = original }()
			if mode == "publish-uncertain" {
				publishAttempt = func(d *safefile.Directory, data []byte) (bool, error) {
					created, e := original(d, data)
					if e != nil {
						return created, e
					}
					return created, errors.New(secret)
				}
			}
			if mode == "cancel-after-publication" {
				publishAttempt = func(d *safefile.Directory, data []byte) (bool, error) {
					created, e := original(d, data)
					cancel()
					return created, e
				}
			}
			if mode == "source-changed-after-publication" {
				publishAttempt = func(d *safefile.Directory, data []byte) (bool, error) {
					created, e := original(d, data)
					os.WriteFile(filepath.Join(v.RecordDirectory, "sandbox.json"), sourceRecord(v, true), 0600)
					return created, e
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
				c.SetDeadline(time.Now().Add(2 * time.Second))
				var last uint64
				for {
					var header [4]byte
					if _, e = io.ReadFull(c, header[:]); e != nil {
						return
					}
					size := binary.BigEndian.Uint32(header[:])
					if size > 8192 {
						methods = append(methods, "INVALID")
						return
					}
					raw := make([]byte, size)
					if _, e = io.ReadFull(c, raw); e != nil {
						return
					}
					var request agent.Envelope
					if json.Unmarshal(raw, &request) != nil {
						methods = append(methods, "INVALID")
						return
					}
					signed := request
					agent.Sign(&signed, key)
					if signed.MAC != request.MAC || request.Sequence <= last || request.SandboxID != o.id || request.Generation != o.generation || request.Endpoint != 7200 {
						methods = append(methods, "INVALID")
						return
					}
					last = request.Sequence
					methods = append(methods, request.Method)
					var body any
					switch request.Method {
					case "Capabilities":
						features := []string{"signal-operation-id-v1"}
						release := o.release
						if mode == "wrong-release" {
							release = "wrong"
						}
						if mode == "missing-feature" {
							features = nil
						}
						body = map[string]any{"protocol": 1, "protocol_features": features, "oci_features": []string{}, "kernel": map[string]any{"architecture": "amd64", "release": release, "secret": secret}, "agent": map[string]any{"uid": 0, "gid": 0}}
						if mode == "missing-root" {
							body.(map[string]any)["agent"] = map[string]any{"uid": nil, "gid": 0}
						}

					case "StateProcess":
						var target map[string]string
						if json.Unmarshal(request.Body, &target) != nil || len(target) != 1 || target["id"] != v.ProcessID {
							methods = append(methods, "INVALID")
							return
						}
						state := agent.ProcessState{ID: v.ProcessID, Status: "RUNNING", PID: 613, Stdout: secret, Stderr: secret}
						if mode == "already-stopped" || len(methods) == 4 && mode != "still-running" {
							state.Status = "STOPPED"
							state.ExitCode = 137
						}
						if mode == "wrong-pid" || mode == "changed-final-pid" && len(methods) == 4 {
							state.PID = 614
						}
						if mode == "wrong-id" {
							state.ID = "private-wrong-target"
						}
						if mode == "created" {
							state.Status = "CREATED"
							state.PID = 0
						}
						if mode == "bad-exit" {
							state.ExitCode = 256
						}
						if mode == "source-changed" && len(methods) == 2 {
							os.WriteFile(filepath.Join(v.RecordDirectory, "sandbox.json"), sourceRecord(v, true), 0600)
						}
						if mode == "source-directory-replaced" && len(methods) == 2 {
							os.Rename(v.RecordDirectory, filepath.Join(root, "source-held"))
							os.Mkdir(v.RecordDirectory, 0700)
						}
						if mode == "directory-replaced" && len(methods) == 2 {
							os.Rename(o.attemptDir, filepath.Join(root, "held"))
							os.Mkdir(o.attemptDir, 0700)
						}
						body = state
					case "SignalProcess":
						var signal map[string]any
						if json.Unmarshal(request.Body, &signal) != nil || len(signal) != 3 || signal["ID"] != v.ProcessID || signal["Signal"] != "9" || signal["operation_id"] != v.OperationID {
							methods = append(methods, "INVALID")
							return
						}
						// Publication precedes even the first observed mutation frame. No new
						// operation identifier or result acknowledgement is permitted.
						data, e := os.ReadFile(filepath.Join(o.attemptDir, attemptName))
						var stored intent
						if e != nil || json.Unmarshal(data, &stored) != nil || stored.OperationID != v.OperationID || stored.RecordSHA256 != v.RecordSHA256 {
							methods = append(methods, "NO_MARKER")
							return
						}
						if mode == "signal-timeout" {
							var one [1]byte
							c.Read(one[:])
							return
						}
						if mode == "remote-refusal" || mode == "malformed-refusal" {
							frame, _ := json.Marshal(map[string]any{"version": 1, "sequence": request.Sequence, "error": map[string]any{"code": "FAILED_PRECONDITION", "message": secret}})
							if mode == "malformed-refusal" {
								frame, _ = json.Marshal(map[string]any{"version": 1, "sequence": request.Sequence, "error": secret})
							}
							binary.BigEndian.PutUint32(header[:], uint32(len(frame)))
							c.Write(append(header[:], frame...))
							continue
						}
					default:
						methods = append(methods, "EXTRA_AUTHORITY")
						return
					}
					frame, _ := json.Marshal(agent.Reply{Version: 1, Sequence: request.Sequence, Body: body})
					binary.BigEndian.PutUint32(header[:], uint32(len(frame)))
					if _, e = c.Write(append(header[:], frame...)); e != nil {
						return
					}
				}
			}()
			r := replay(ctx, o, sealed(t, key, mode != "unsealed-key"), manifest(t, v))
			listener.Close()
			methods := <-served
			assertPrivate(t, r, v)
			expected := []string{"Capabilities", "StateProcess", "SignalProcess", "StateProcess"}
			switch mode {
			case "wrong-release", "missing-feature", "missing-root":
				expected = expected[:1]
			case "already-stopped", "wrong-pid", "publish-uncertain", "cancel-after-publication", "directory-replaced", "wrong-id", "created", "bad-exit", "source-changed", "source-directory-replaced", "source-changed-after-publication":
				expected = expected[:2]
			case "wrong-peer", "unsealed-key":
				expected = nil
			case "remote-refusal", "malformed-refusal", "signal-timeout":
				expected = expected[:3]
			}
			if strings.Join(methods, ",") != strings.Join(expected, ",") {
				t.Fatal("unexpected RPC budget or authority", methods)
			}
			switch mode {
			case "stopped", "still-running":
				if r.Status != "observed" || r.Code != "SIGNAL_REPLAY_RESULT_OBSERVED" || !r.SignalCallSucceeded || !r.SignalResultKnown || !r.AttemptMarkerPublished || !r.OriginalRecordVerified || !r.OriginalRecordRechecked || r.After == nil {
					t.Fatalf("missing replay observation: %+v", r)
				}
				if (r.After.Status == "STOPPED") != (mode == "stopped") {
					t.Fatal("process observation fabricated completion")
				}
			case "already-stopped":
				if r.Code != "ALREADY_STOPPED_NO_SIGNAL_REPLAYED" || r.Status != "observed" || r.SignalRequestAttempted || r.AttemptMarkerMayExist {
					t.Fatal("stopped task mutated")
				}
			case "signal-timeout":
				if r.Code != "DEADLINE_OR_CANCELED" || !r.SignalRequestAttempted || r.SignalResultKnown || r.After != nil || !r.AttemptMarkerPublished {
					t.Fatal("unknown signal outcome upgraded")
				}
			case "malformed-refusal":
				if r.Code != "SIGNAL_RESULT_UNAVAILABLE" || !r.SignalRequestAttempted || r.SignalResultKnown || r.After != nil || !r.AttemptMarkerPublished {
					t.Fatal("malformed reply upgraded unknown mutation")
				}
			case "remote-refusal":
				if r.Code != "SIGNAL_REPLAY_REFUSED" || !r.SignalResultKnown || r.SignalCallSucceeded || r.After != nil {
					t.Fatal("private remote refusal misclassified")
				}
			case "publish-uncertain":
				if r.Code != "ATTEMPT_PUBLICATION_UNCERTAIN_OR_OCCUPIED" || r.SignalRequestAttempted || !r.AttemptMarkerMayExist {
					t.Fatal("uncertain publication allowed signal")
				}
			case "cancel-after-publication":
				if r.Code != "DEADLINE_OR_CANCELED" || r.SignalRequestAttempted || !r.AttemptMarkerPublished {
					t.Fatal("cancelled publication allowed signal")
				}
			case "source-changed", "source-directory-replaced", "source-changed-after-publication":
				if r.Code != "ORIGINAL_RECORD_CHANGED_OR_UNAVAILABLE" || r.SignalRequestAttempted || r.OriginalRecordRechecked {
					t.Fatal("changed original record permitted replay")
				}
				if r.AttemptMarkerPublished != (mode == "source-changed-after-publication") {
					t.Fatal("changed-record marker preservation wrong")
				}
			case "directory-replaced":
				if r.Code != "ATTEMPT_DIRECTORY_CHANGED" || r.SignalRequestAttempted || r.AttemptMarkerMayExist {
					t.Fatal("replaced directory allowed signal")
				}
			default:
				if r.Status != "inconclusive" || r.After != nil {
					t.Fatal("mismatch accepted")
				}
			}
			if r.AttemptMarkerMayExist {
				data, e := os.ReadFile(filepath.Join(o.attemptDir, attemptName))
				if e != nil || !strings.Contains(string(data), v.OperationID) {
					t.Fatal("attempt marker lost")
				}
				st, _ := os.Stat(filepath.Join(o.attemptDir, attemptName))
				if st.Mode().Perm() != 0600 {
					t.Fatal("attempt marker exposed")
				}
				// A fresh invocation with the same directory/intent refuses before key read
				// or dial, even when the first response was lost or publication uncertain.
				again := replay(t.Context(), o, sealed(t, key, true), manifest(t, v))
				if again.Code != "ATTEMPT_ALREADY_RECORDED_OR_UNAVAILABLE" || again.CredentialDescriptorVerified || again.MayAdvanceAuthenticationSequence {
					t.Fatal("recorded attempt repeated")
				}
			}
		})
	}
}
func TestReplayRejectsUntrustedIntentBeforeCredentialOrDial(t *testing.T) {
	for _, mode := range []string{"unsealed", "unknown-field", "duplicate-field", "acknowledged", "missing-ack", "wrong-boot", "wrong-generation", "wrong-endpoint", "wrong-scope", "operation", "record-hash", "signal", "pid", "directory-identity", "shared-directory", "occupied", "cancelled", "source-record-missing", "source-record-symlink", "source-record-hardlink", "source-shared", "source-identity", "source-acked", "source-signal", "source-operation", "source-target", "source-duplicate-target", "source-schema", "source-record-identical-replacement"} {
		t.Run(mode, func(t *testing.T) {
			o, v, _ := inputs(t)
			switch mode {
			case "acknowledged":
				yes := true
				v.Acknowledged = &yes
			case "missing-ack":
				v.Acknowledged = nil
			case "wrong-boot":
				v.BootID = strings.Repeat("0", 36)
			case "wrong-generation":
				v.Generation = strings.Repeat("d", 32)
			case "wrong-endpoint":
				v.Endpoint++
			case "wrong-scope":
				v.Scope = "new-signal"
			case "operation":
				v.OperationID = "private-operation"
			case "record-hash":
				v.RecordSHA256 = "private-hash"
			case "signal":
				v.Signal = 65
			case "pid":
				v.GuestPID = 0
			case "directory-identity":
				v.AttemptDirectory.Inode++
			case "shared-directory":
				os.Chmod(o.attemptDir, 0755)
			case "occupied":
				os.WriteFile(filepath.Join(o.attemptDir, attemptName), []byte(secret), 0600)
			}
			if strings.HasPrefix(mode, "source-") {
				path := filepath.Join(v.RecordDirectory, "sandbox.json")
				switch mode {
				case "source-record-missing":
					os.Remove(path)
				case "source-record-symlink":
					os.Remove(path)
					os.Symlink("missing-private-target", path)
				case "source-record-identical-replacement":
					data, _ := os.ReadFile(path)
					os.Rename(path, path+".original")
					os.WriteFile(path, data, 0600)
				case "source-record-hardlink":
					os.Link(path, filepath.Join(v.RecordDirectory, "alias"))
				case "source-shared":
					os.Chmod(v.RecordDirectory, 0755)
				case "source-identity":
					v.RecordDirectoryIdentity.Inode++
				default:
					data := sourceRecord(v, mode == "source-acked")
					var object map[string]any
					json.Unmarshal(data, &object)
					processes := object["processes"].([]any)
					entry := processes[0].(map[string]any)
					switch mode {
					case "source-signal":
						entry["pending_signal"] = 15
					case "source-operation":
						entry["pending_signal_operation"] = strings.Repeat("d", 32)
					case "source-target":
						object["id"] = "foreign"
					case "source-duplicate-target":
						object["processes"] = append(processes, entry)
					case "source-schema":
						object["schema_version"] = 5
					}
					data, _ = json.Marshal(object)
					os.WriteFile(path, data, 0600)
					digest := sha256.Sum256(data)
					v.RecordSHA256 = hex.EncodeToString(digest[:])
					v.RecordIdentity = captureRecord(t, v.RecordDirectory)
				}
			}
			raw, _ := json.Marshal(v)
			if mode == "unknown-field" {
				raw = append(raw[:len(raw)-1], []byte(`,"private":"`+secret+`"}`)...)
			}
			if mode == "duplicate-field" {
				raw = append(raw[:len(raw)-1], []byte(`,"pending_signal":9}`)...)
			}
			ctx, cancel := context.WithCancel(t.Context())
			if mode == "cancelled" {
				cancel()
			}
			defer cancel()
			r := replay(ctx, o, nil, sealed(t, raw, mode != "unsealed"))
			if mode == "cancelled" && r.Code != "DEADLINE_OR_CANCELED" {
				t.Fatal("pre-cancelled action misclassified")
			}

			assertPrivate(t, r, v)
			if r.CredentialDescriptorVerified || r.LocalPeerVerified || r.MayAdvanceAuthenticationSequence || r.SignalRequestAttempted || r.AttemptMarkerMayExist {
				t.Fatal("refusal read key/dialed/mutated")
			}
		})
	}
}
