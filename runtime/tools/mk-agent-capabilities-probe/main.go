//go:build linux

// The diagnostic sends Capabilities and optionally one StateProcess request. It cannot execute,
// signal, quiesce, stop, or repair a guest. Credentials arrive only through a
// sealed inherited descriptor, never an argument or environment value.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"math"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"golang.org/x/sys/unix"
)

type options struct {
	socket, id, generation, release string
	processID                       string
	endpoint                        uint
	peerPID, tokenFD                int
	timeout                         time.Duration
}
type report struct {
	Version                          int                 `json:"version"`
	Scope                            string              `json:"scope"`
	Status                           string              `json:"status"`
	Code                             string              `json:"code"`
	SandboxID                        string              `json:"sandbox_id,omitempty"`
	Generation                       string              `json:"generation,omitempty"`
	Endpoint                         uint                `json:"endpoint,omitempty"`
	CredentialDescriptorVerified     bool                `json:"credential_descriptor_verified"`
	LocalPeerVerified                bool                `json:"local_peer_verified"`
	RequestAttempted                 bool                `json:"request_attempted"`
	KernelReleaseMatches             bool                `json:"kernel_release_matches"`
	ProtocolMatches                  bool                `json:"protocol_matches"`
	GuestRootClaimMatches            bool                `json:"guest_root_claim_matches"`
	MutatingMethodAttempted          bool                `json:"mutating_method_attempted"`
	MayAdvanceAuthenticationSequence bool                `json:"may_advance_authentication_sequence"`
	Unobserved                       []string            `json:"unobserved"`
	ExpectedProcessSHA256            string              `json:"expected_process_id_sha256,omitempty"`
	ProcessQueryAttempted            bool                `json:"process_query_attempted,omitempty"`
	Process                          *processObservation `json:"process,omitempty"`
}
type processObservation struct {
	Status   string `json:"status"`
	GuestPID int    `json:"guest_pid,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}
type capabilities struct {
	Protocol         int      `json:"protocol"`
	ProtocolFeatures []string `json:"protocol_features"`
	OCIFeatures      []string `json:"oci_features"`
	Kernel           struct {
		Architecture string   `json:"architecture"`
		Release      string   `json:"release"`
		Multikernel  bool     `json:"multikernel"`
		CgroupV2     bool     `json:"cgroup_v2"`
		MKTransport  bool     `json:"mk_transport"`
		Namespaces   []string `json:"namespaces"`
	} `json:"kernel"`
	Agent struct {
		UID       *int   `json:"uid"`
		GID       *int   `json:"gid"`
		Effective string `json:"effective_capabilities"`
		Bounding  string `json:"bounding_capabilities"`
		NNP       string `json:"no_new_privileges"`
	} `json:"agent"`
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var genPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var releasePattern = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,128}$`)
var processPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

const tokenSeals = unix.F_SEAL_SEAL | unix.F_SEAL_SHRINK | unix.F_SEAL_GROW | unix.F_SEAL_WRITE

func readToken(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, errors.New("credential unavailable")
	}
	var st unix.Stat_t
	if unix.Fstat(int(file.Fd()), &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 0 || st.Size != 32 {
		return nil, errors.New("credential descriptor rejected")
	}
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil || seals&tokenSeals != tokenSeals {
		return nil, errors.New("credential descriptor rejected")
	}
	data, err := io.ReadAll(io.LimitReader(file, 33))
	if err != nil || len(data) != 32 {
		return nil, errors.New("credential descriptor rejected")
	}
	return data, nil
}
func valid(o options) bool {
	return idPattern.MatchString(o.id) && genPattern.MatchString(o.generation) && releasePattern.MatchString(o.release) && (o.processID == "" || processPattern.MatchString(o.processID)) && o.endpoint >= 1024 && o.endpoint <= 65535 && o.peerPID > 0 && o.tokenFD >= 3 && o.tokenFD <= 1024 && o.timeout > 0 && o.timeout <= 15*time.Second
}
func inspect(parent context.Context, o options, credential *os.File) report {
	r := report{Version: 1, Scope: "one-credential-bound-capabilities-request", Status: "inconclusive", Code: "INVALID_PROBE_INPUT", Unobserved: []string{"remote-process-attestation", "kernel-incarnation", "docker-health", "storage-health", "containment"}}
	if !valid(o) {
		return r
	}
	r.SandboxID, r.Generation, r.Endpoint = o.id, o.generation, o.endpoint
	if o.processID != "" {
		r.Version = 2
		r.Scope = "credential-bound-capabilities-and-process-state"
		hash := sha256.Sum256([]byte(o.processID))
		r.ExpectedProcessSHA256 = hex.EncodeToString(hash[:])
	}
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()
	key, err := readToken(credential)
	if err != nil {
		r.Code = "CREDENTIAL_UNAVAILABLE"
		return r
	}
	defer clear(key)
	r.CredentialDescriptorVerified = true
	client, err := agent.DialPeerContext(ctx, o.socket, o.id, o.generation, uint32(o.endpoint), key, o.peerPID)
	if err != nil {
		r.Code = "LOCAL_RELAY_UNAVAILABLE"
		if ctx.Err() != nil {
			r.Code = "DEADLINE_OR_CANCELED"
		}
		return r
	}
	defer client.Close()
	r.LocalPeerVerified = true
	r.RequestAttempted = true
	r.MayAdvanceAuthenticationSequence = true
	var c capabilities
	if err = client.CallContext(ctx, "Capabilities", map[string]any{}, &c); err != nil {
		r.Code = "CAPABILITIES_UNAVAILABLE"
		var timed net.Error
		if ctx.Err() != nil || errors.As(err, &timed) && timed.Timeout() {
			r.Code = "DEADLINE_OR_CANCELED"
		}
		return r
	}
	r.ProtocolMatches = c.Protocol == 1
	r.KernelReleaseMatches = c.Kernel.Architecture == "amd64" && c.Kernel.Release == o.release
	r.GuestRootClaimMatches = c.Agent.UID != nil && c.Agent.GID != nil && *c.Agent.UID == 0 && *c.Agent.GID == 0
	if !r.ProtocolMatches || !r.KernelReleaseMatches || !r.GuestRootClaimMatches {
		r.Code = "CAPABILITIES_MISMATCH"
		return r
	}
	r.Status = "matched"
	r.Code = "CAPABILITIES_MATCHED"
	if o.processID != "" {
		r.Status = "inconclusive"
		r.ProcessQueryAttempted = true
		var state agent.ProcessState
		err = client.CallContext(ctx, "StateProcess", map[string]string{"id": o.processID}, &state)
		if err != nil {
			r.Code = "PROCESS_STATE_UNAVAILABLE"
			var remote *agent.RemoteError
			var timed net.Error
			if ctx.Err() != nil || errors.As(err, &timed) && timed.Timeout() {
				r.Code = "DEADLINE_OR_CANCELED"
			} else if errors.As(err, &remote) && remote.Failure.Code == "NOT_FOUND" {
				r.Status, r.Code = "observed", "PROCESS_NOT_FOUND"
				r.Process = &processObservation{Status: "NOT_FOUND"}
			}
			return r
		}
		if !validProcessState(state, o.processID) {
			r.Code = "PROCESS_STATE_MISMATCH"
			return r
		}
		r.Process = &processObservation{Status: state.Status, GuestPID: state.PID}
		if state.Status == "STOPPED" {
			r.Process.ExitCode = &state.ExitCode
		}
		r.Status, r.Code = "observed", "PROCESS_STATE_OBSERVED"
	}
	return r
}
func validProcessState(state agent.ProcessState, id string) bool {
	if state.ID != id || state.PID < 0 || state.PID > math.MaxInt32 || state.ExitCode < 0 || state.ExitCode > 255 {
		return false
	}
	switch state.Status {
	case "CREATED":
		return state.PID == 0 && state.ExitCode == 0
	case "RUNNING":
		return state.PID > 0 && state.ExitCode == 0
	case "STOPPED":
		return state.PID > 0
	default:
		return false
	}
}
func main() {
	var o options
	flag.StringVar(&o.socket, "socket", "", "protected Unix socket of explicitly selected native relay")
	flag.StringVar(&o.id, "sandbox-id", "", "expected sandbox ID")
	flag.StringVar(&o.generation, "generation", "", "expected generation")
	flag.StringVar(&o.release, "expected-release", "", "expected child kernel release")
	flag.StringVar(&o.processID, "process-id", "", "optional exact guest process ID for one read-only StateProcess query after capabilities match")
	flag.UintVar(&o.endpoint, "endpoint", 0, "expected agent endpoint")
	flag.IntVar(&o.peerPID, "relay-pid", 0, "expected local relay PID")
	flag.IntVar(&o.tokenFD, "token-fd", -1, "sealed inherited 32-byte credential descriptor")
	flag.DurationVar(&o.timeout, "timeout", 5*time.Second, "whole probe budget, at most 15 seconds")
	flag.Parse()
	if os.Geteuid() != 0 || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"version": 1, "status": "inconclusive", "code": "PROBE_PROCESS_PROTECTION_REQUIRED"})
		os.Exit(1)
	}
	var file *os.File
	if o.tokenFD >= 3 && o.tokenFD <= 1024 {
		file = os.NewFile(uintptr(o.tokenFD), "private-inherited-credential")
		defer file.Close()
	}
	r := inspect(context.Background(), o, file)
	json.NewEncoder(os.Stdout).Encode(r)
	if r.Status != "matched" && r.Status != "observed" {
		os.Exit(1)
	}
}
