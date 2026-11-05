//go:build linux

// An explicit operator action replays one caller-certified persisted signal.
// It never invents an operation ID, acknowledges/discards the result, executes
// a command, quiesces/shuts down a guest, or changes host resource ownership.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/sealedinput"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

const attemptName = "pending-signal-replay.json"

type options struct {
	socket, id, generation, release, attemptDir string
	endpoint                                    uint
	peerPID, tokenFD, intentFD                  int
	timeout                                     time.Duration
}
type recordIdentity struct {
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	UID     uint32 `json:"uid"`
	MTimeNS int64  `json:"mtime_ns"`
	CTimeNS int64  `json:"ctime_ns"`
}

func bindRecord(x safefile.Identity) recordIdentity {
	return recordIdentity{Device: x.Device, Inode: x.Inode, UID: x.UID, MTimeNS: x.MTime.Sec*1_000_000_000 + x.MTime.Nsec, CTimeNS: x.CTime.Sec*1_000_000_000 + x.CTime.Nsec}
}

type intent struct {
	Version                 int                        `json:"version"`
	Scope                   string                     `json:"scope"`
	BootID                  string                     `json:"boot_id"`
	SandboxID               string                     `json:"sandbox_id"`
	Generation              string                     `json:"generation"`
	Endpoint                uint32                     `json:"endpoint"`
	Release                 string                     `json:"expected_release"`
	ProcessID               string                     `json:"process_id"`
	GuestPID                int                        `json:"guest_pid"`
	Signal                  uint32                     `json:"pending_signal"`
	OperationID             string                     `json:"pending_signal_operation"`
	Acknowledged            *bool                      `json:"pending_signal_acknowledgement"`
	RecordSHA256            string                     `json:"original_record_sha256"`
	RecordIdentity          recordIdentity             `json:"original_record_identity"`
	RecordDirectory         string                     `json:"original_record_directory"`
	RecordDirectoryIdentity protocol.DirectoryIdentity `json:"original_record_directory_identity"`
	AttemptDirectory        protocol.DirectoryIdentity `json:"attempt_directory_identity"`
}
type processReport struct {
	Status   string `json:"status"`
	GuestPID int    `json:"guest_pid"`
	ExitCode *int   `json:"exit_code,omitempty"`
}
type report struct {
	Version                          int            `json:"version"`
	Scope                            string         `json:"scope"`
	Status                           string         `json:"status"`
	Code                             string         `json:"code"`
	IntentDescriptorVerified         bool           `json:"intent_descriptor_verified"`
	TargetMatchesIntent              bool           `json:"target_matches_intent"`
	OriginalRecordVerified           bool           `json:"original_record_verified"`
	OriginalRecordRechecked          bool           `json:"original_record_rechecked"`
	CredentialDescriptorVerified     bool           `json:"credential_descriptor_verified"`
	LocalPeerVerified                bool           `json:"local_peer_verified"`
	CapabilitiesMatch                bool           `json:"capabilities_match"`
	InitialProcessMatched            bool           `json:"initial_process_matched"`
	AttemptMarkerPublished           bool           `json:"attempt_marker_published"`
	AttemptMarkerMayExist            bool           `json:"attempt_marker_may_exist"`
	MayAdvanceAuthenticationSequence bool           `json:"may_advance_authentication_sequence"`
	SignalRequestAttempted           bool           `json:"signal_request_attempted"`
	SignalResultKnown                bool           `json:"signal_result_known"`
	SignalCallSucceeded              bool           `json:"signal_call_succeeded"`
	Signal                           uint32         `json:"signal,omitempty"`
	Before                           *processReport `json:"process_before,omitempty"`
	After                            *processReport `json:"process_after,omitempty"`
	OriginalIntentModified           bool           `json:"original_intent_modified"`
	SignalAcknowledgementSent        bool           `json:"signal_acknowledgement_sent"`
	QuiesceOrShutdownAttempted       bool           `json:"quiesce_or_shutdown_attempted"`
	HostResourceMutationAttempted    bool           `json:"host_resource_mutation_attempted"`
	NormalCleanupQualified           bool           `json:"normal_cleanup_qualified"`
	Unobserved                       []string       `json:"unobserved"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var processPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var generationPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var bootPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var releasePattern = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,128}$`)

func valid(o options) bool {
	return filepath.IsAbs(o.socket) && filepath.Clean(o.socket) == o.socket && filepath.IsAbs(o.attemptDir) && filepath.Clean(o.attemptDir) == o.attemptDir && idPattern.MatchString(o.id) && generationPattern.MatchString(o.generation) && releasePattern.MatchString(o.release) && o.endpoint >= 1024 && o.endpoint <= 65535 && o.peerPID > 1 && o.peerPID <= math.MaxInt32 && o.tokenFD >= 3 && o.tokenFD <= 1024 && o.intentFD >= 3 && o.intentFD <= 1024 && o.intentFD != o.tokenFD && o.timeout > 0 && o.timeout <= 30*time.Second
}
func validIntent(v intent, o options, boot string) bool {
	return v.Version == 1 && v.Scope == "persisted-shim-pending-signal" && bootPattern.MatchString(boot) && v.BootID == boot && v.SandboxID == o.id && v.Generation == o.generation && v.Endpoint == uint32(o.endpoint) && v.Release == o.release && processPattern.MatchString(v.ProcessID) && v.GuestPID > 0 && v.GuestPID <= math.MaxInt32 && v.Signal >= 1 && v.Signal <= 64 && generationPattern.MatchString(v.OperationID) && v.Acknowledged != nil && !*v.Acknowledged && hashPattern.MatchString(v.RecordSHA256) && v.RecordIdentity.Device > 0 && v.RecordIdentity.Inode > 0 && v.RecordIdentity.UID == uint32(os.Geteuid()) && v.RecordIdentity.MTimeNS > 0 && v.RecordIdentity.CTimeNS > 0 && filepath.IsAbs(v.RecordDirectory) && filepath.Clean(v.RecordDirectory) == v.RecordDirectory && len(v.RecordDirectory) <= 4096 && strings.IndexByte(v.RecordDirectory, 0) < 0 && v.RecordDirectoryIdentity.Device > 0 && v.RecordDirectoryIdentity.Inode > 0 && v.RecordDirectoryIdentity.UID == uint32(os.Geteuid()) && v.AttemptDirectory.Device > 0 && v.AttemptDirectory.Inode > 0 && v.AttemptDirectory.UID == uint32(os.Geteuid())
}

// Source verification is a current protected-file observation, not an
// attestation of host history. Caller coordination still excludes competing
// controllers that can acknowledge/retire the signal's replay result.
func sourceMatches(directory *safefile.Directory, v intent) bool {
	expected := v.RecordDirectoryIdentity
	x := directory.Identity()
	if x.Device != expected.Device || x.Inode != expected.Inode || x.UID != expected.UID {
		return false
	}
	current, err := safefile.OpenDirectory(v.RecordDirectory, false)
	if err != nil {
		return false
	}
	named := current.Identity() == directory.Identity()
	current.Close()
	if !named {
		return false
	}
	data, found, identity, err := directory.ReadPrivateIdentity("sandbox.json", 8<<20)
	if err != nil || !found || identity.MTime.Sec < 0 || identity.MTime.Sec > math.MaxInt64/1_000_000_000 || identity.CTime.Sec < 0 || identity.CTime.Sec > math.MaxInt64/1_000_000_000 || bindRecord(identity) != v.RecordIdentity {
		return false
	}
	defer clear(data)
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != v.RecordSHA256 {
		return false
	}
	var object map[string]json.RawMessage
	if protocol.StrictDecode(data, &object) != nil {
		return false
	}
	var schema int
	var id, generation string
	var processes []struct {
		ID           string `json:"id"`
		Status       uint32 `json:"status"`
		Signal       uint32 `json:"pending_signal"`
		Operation    string `json:"pending_signal_operation"`
		Acknowledged bool   `json:"pending_signal_acknowledgement"`
	}
	if json.Unmarshal(object["schema_version"], &schema) != nil || schema != 4 || json.Unmarshal(object["id"], &id) != nil || id != v.SandboxID || json.Unmarshal(object["generation"], &generation) != nil || generation != v.Generation || json.Unmarshal(object["processes"], &processes) != nil {
		return false
	}
	matched := 0
	for _, p := range processes {
		name := p.ID
		if name == "" {
			name = "init"
		}
		if name != v.ProcessID {
			continue
		}
		matched++
		if (p.Status != 2 && p.Status != 3) || p.Signal != v.Signal || p.Operation != v.OperationID || p.Acknowledged {
			return false
		}
	}
	return matched == 1
}
func projected(p agent.ProcessState, v intent) *processReport {
	if p.ID != v.ProcessID || p.PID != v.GuestPID || p.ExitCode < 0 || p.ExitCode > 255 {
		return nil
	}
	switch p.Status {
	case "RUNNING":
		if p.ExitCode != 0 {
			return nil
		}
		return &processReport{Status: p.Status, GuestPID: p.PID}
	case "STOPPED":
		return &processReport{Status: p.Status, GuestPID: p.PID, ExitCode: &p.ExitCode}
	}
	return nil
}
func sameDirectory(d *safefile.Directory, v intent) bool {
	x := d.Identity()
	return x.Device == v.AttemptDirectory.Device && x.Inode == v.AttemptDirectory.Inode && x.UID == v.AttemptDirectory.UID
}
func namedDirectory(o options, v intent) bool {
	d, e := safefile.OpenDirectory(o.attemptDir, false)
	if e != nil {
		return false
	}
	defer d.Close()
	return sameDirectory(d, v)
}

// Tests inject a publication uncertainty; production always uses the synced,
// exclusive descriptor-relative primitive and never removes an uncertain marker.
var publishAttempt = func(d *safefile.Directory, data []byte) (bool, error) {
	return d.PublishExclusive(attemptName, data, 0600)
}

func replay(parent context.Context, o options, credential, manifest *os.File) report {
	r := report{Version: 1, Scope: "one-persisted-agent-signal-replay", Status: "inconclusive", Code: "INVALID_REPLAY_INPUT", Unobserved: []string{"host-persistence-of-result", "guest-process-incarnation", "storage-quiescence", "kernel-stop", "native-synced-close", "resource-release", "containment"}}
	if !valid(o) {
		return r
	}
	if parent.Err() != nil {
		r.Code = "DEADLINE_OR_CANCELED"
		return r
	}
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()
	raw, err := sealedinput.Read(manifest, 2, 8192)
	if err != nil {
		r.Code = "INTENT_DESCRIPTOR_REJECTED"
		return r
	}
	defer clear(raw)
	r.IntentDescriptorVerified = true
	var v intent
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil || protocol.StrictDecode(raw, &v) != nil || !validIntent(v, o, strings.TrimSpace(string(boot))) {
		r.Code = "INTENT_TARGET_MISMATCH"
		return r
	}
	r.TargetMatchesIntent = true
	r.Signal = v.Signal
	directory, err := safefile.OpenDirectory(o.attemptDir, false)
	if err != nil {
		r.Code = "ATTEMPT_DIRECTORY_UNAVAILABLE"
		return r
	}
	defer directory.Close()
	if !sameDirectory(directory, v) {
		r.Code = "ATTEMPT_DIRECTORY_CHANGED"
		return r
	}
	if _, exists, e := directory.EntryIdentity(attemptName); e != nil || exists {
		r.Code = "ATTEMPT_ALREADY_RECORDED_OR_UNAVAILABLE"
		return r
	}
	source, err := safefile.OpenDirectory(v.RecordDirectory, false)
	if err != nil {
		r.Code = "ORIGINAL_RECORD_UNAVAILABLE"
		return r
	}
	defer source.Close()
	if !sourceMatches(source, v) {
		r.Code = "ORIGINAL_RECORD_CHANGED_OR_UNAVAILABLE"
		return r
	}
	r.OriginalRecordVerified = true
	if ctx.Err() != nil {
		r.Code = "DEADLINE_OR_CANCELED"
		return r
	}
	key, err := sealedinput.Read(credential, 32, 32)
	if err != nil {
		r.Code = "CREDENTIAL_DESCRIPTOR_REJECTED"
		return r
	}
	defer clear(key)
	r.CredentialDescriptorVerified = true
	client, err := agent.DialPeerContext(ctx, o.socket, o.id, o.generation, uint32(o.endpoint), key, o.peerPID)
	failure := func(code string, err error) report {
		r.Code = code
		var timed net.Error
		if ctx.Err() != nil || errors.As(err, &timed) && timed.Timeout() {
			r.Code = "DEADLINE_OR_CANCELED"
		}
		return r
	}
	if err != nil {
		return failure("LOCAL_RELAY_UNAVAILABLE", err)
	}
	defer client.Close()
	r.LocalPeerVerified = true
	var caps struct {
		Protocol         int            `json:"protocol"`
		ProtocolFeatures []string       `json:"protocol_features"`
		OCIFeatures      []string       `json:"oci_features"`
		Kernel           map[string]any `json:"kernel"`
		Agent            map[string]any `json:"agent"`
	}
	r.MayAdvanceAuthenticationSequence = true
	if err = client.CallContext(ctx, "Capabilities", map[string]any{}, &caps); err != nil {
		return failure("CAPABILITIES_UNAVAILABLE", err)
	}
	uid, uidOK := caps.Agent["uid"].(float64)
	gid, gidOK := caps.Agent["gid"].(float64)
	feature := false
	if len(caps.ProtocolFeatures) <= 128 {
		for _, name := range caps.ProtocolFeatures {
			feature = feature || name == "signal-operation-id-v1"
		}
	}
	if caps.Protocol != 1 || caps.Kernel["architecture"] != "amd64" || caps.Kernel["release"] != o.release || !uidOK || !gidOK || uid != 0 || gid != 0 || !feature {
		r.Code = "CAPABILITIES_MISMATCH"
		return r
	}
	r.CapabilitiesMatch = true
	var before agent.ProcessState
	if err = client.CallContext(ctx, "StateProcess", map[string]string{"id": v.ProcessID}, &before); err != nil {
		return failure("INITIAL_PROCESS_UNAVAILABLE", err)
	}
	r.Before = projected(before, v)
	if r.Before == nil {
		r.Code = "INITIAL_PROCESS_MISMATCH"
		return r
	}
	r.InitialProcessMatched = true
	if before.Status == "STOPPED" {
		r.Status = "observed"
		r.Code = "ALREADY_STOPPED_NO_SIGNAL_REPLAYED"
		return r
	}
	if !sourceMatches(source, v) {
		r.Code = "ORIGINAL_RECORD_CHANGED_OR_UNAVAILABLE"
		return r
	}
	if !namedDirectory(o, v) {
		r.Code = "ATTEMPT_DIRECTORY_CHANGED"
		return r
	}
	marker, _ := json.Marshal(v) // Private metadata only, never included in the report.
	r.AttemptMarkerMayExist = true
	created, err := publishAttempt(directory, marker)
	clear(marker)
	if err != nil || !created {
		r.Code = "ATTEMPT_PUBLICATION_UNCERTAIN_OR_OCCUPIED"
		return r
	}
	r.AttemptMarkerPublished = true
	if !namedDirectory(o, v) {
		r.Code = "ATTEMPT_DIRECTORY_CHANGED_AFTER_PUBLICATION"
		return r
	}
	if !sourceMatches(source, v) {
		r.Code = "ORIGINAL_RECORD_CHANGED_OR_UNAVAILABLE"
		return r
	}
	r.OriginalRecordRechecked = true
	if ctx.Err() != nil {
		r.Code = "DEADLINE_OR_CANCELED"
		return r
	}
	r.SignalRequestAttempted = true
	err = client.CallContext(ctx, "SignalProcess", map[string]any{"ID": v.ProcessID, "Signal": strconv.FormatUint(uint64(v.Signal), 10), "operation_id": v.OperationID}, nil)
	if err != nil {
		var remote *agent.RemoteError
		if errors.As(err, &remote) {
			r.SignalResultKnown = true
			r.Code = "SIGNAL_REPLAY_REFUSED"
			return r
		}
		return failure("SIGNAL_RESULT_UNAVAILABLE", err)
	}
	r.SignalResultKnown = true
	r.SignalCallSucceeded = true
	var after agent.ProcessState
	if err = client.CallContext(ctx, "StateProcess", map[string]string{"id": v.ProcessID}, &after); err != nil {
		return failure("FINAL_PROCESS_UNAVAILABLE", err)
	}
	r.After = projected(after, v)
	if r.After == nil {
		r.Code = "FINAL_PROCESS_MISMATCH"
		return r
	}
	r.Status = "observed"
	r.Code = "SIGNAL_REPLAY_RESULT_OBSERVED"
	return r
}
func main() {
	var o options
	flag.StringVar(&o.socket, "socket", "", "protected socket of the caller-owned CID relay")
	flag.StringVar(&o.id, "sandbox-id", "", "expected sandbox ID")
	flag.StringVar(&o.generation, "generation", "", "expected sandbox generation")
	flag.StringVar(&o.release, "expected-release", "", "expected child kernel release")
	flag.StringVar(&o.attemptDir, "attempt-dir", "", "existing private generation-bound attempt directory")
	flag.UintVar(&o.endpoint, "endpoint", 0, "expected agent endpoint")
	flag.IntVar(&o.peerPID, "relay-pid", 0, "expected caller-owned local relay PID")
	flag.IntVar(&o.tokenFD, "token-fd", -1, "sealed anonymous 32-byte credential descriptor")
	flag.IntVar(&o.intentFD, "intent-fd", -1, "sealed anonymous caller-certified persisted signal intent")
	flag.DurationVar(&o.timeout, "timeout", 10*time.Second, "shared agent RPC budget, at most 30 seconds")
	flag.Parse()
	if os.Geteuid() != 0 || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil || unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}) != nil {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"version": 1, "status": "inconclusive", "code": "OPERATOR_PROCESS_PROTECTION_REQUIRED"})
		os.Exit(1)
	}
	var key, manifest *os.File
	if o.tokenFD >= 3 && o.tokenFD <= 1024 {
		key = os.NewFile(uintptr(o.tokenFD), "sealed-private-credential")
		defer key.Close()
	}
	if o.intentFD >= 3 && o.intentFD <= 1024 && o.intentFD != o.tokenFD {
		manifest = os.NewFile(uintptr(o.intentFD), "sealed-pending-intent")
		defer manifest.Close()
	}
	r := replay(context.Background(), o, key, manifest)
	json.NewEncoder(os.Stdout).Encode(r)
	if r.Status != "observed" {
		os.Exit(1)
	}
}
