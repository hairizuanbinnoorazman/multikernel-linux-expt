//go:build linux

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

// This opt-in one-generation operator action only uses the production graceful
// shutdown method. Its caller must first preserve private output/state/cloud
// evidence and verify native backing identity. It never kills a child/NBD PID,
// edits failed ledgers, writes raw sysfs, unloads a kernel or releases a pool.
// Independent kernel/native-close observations remain mandatory afterwards.
func TestLiveGracefulQuarantine0c50de9(t *testing.T) {
	if os.Getenv("MK_LIVE_GRACEFUL_CASE") != "0c50de9" {
		t.Skip("explicit one-generation operator quarantine only")
	}
	const boot = "3ae4f0d8-4b8f-4c68-a5d1-b43c90d878a6"
	const sid = "mk-fc7012dd2d64b03023855fe12ba09039-979d3d345f108048"
	const generation = "a49b785d3e4b944f8c33237120797712"
	name, _ := os.Hostname()
	observedBoot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || name != "mklinux-g7-storage-20261012" || string(observedBoot) != boot+"\n" || os.Geteuid() != 0 || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		t.Fatal("exact protected test host/boot required")
	}
	if os.Getenv("MK_LIVE_GRACEFUL_EVIDENCE_READY") != "1" {
		t.Fatal("caller evidence/ownership preflight required")
	}
	dir := os.Getenv("MK_LIVE_RELAY_PROBE_DIR")
	target, err := safefile.OpenDirectory(dir, false)
	if err != nil {
		t.Fatal("private operator directory required")
	}
	defer target.Close()
	fd, err := strconv.Atoi(os.Getenv("MK_LIVE_RELAY_TOKEN_FD"))
	if err != nil || fd < 3 || fd > 1024 {
		t.Fatal("sealed credential required")
	}
	credential := os.NewFile(uintptr(fd), "private-inherited-credential")
	defer credential.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 || st.Uid != 0 || st.Nlink != 0 || st.Size != 32 {
		t.Fatal("credential descriptor refused")
	}
	seals, err := unix.FcntlInt(uintptr(fd), unix.F_GET_SEALS, 0)
	if err != nil || seals&15 != 15 {
		t.Fatal("credential seals required")
	}
	key, err := io.ReadAll(io.LimitReader(credential, 33))
	if err != nil || len(key) != 32 {
		t.Fatal("credential unavailable")
	}
	defer clear(key)
	s := &service{sandbox: protocol.Sandbox{ID: sid, Generation: generation, Config: protocol.SandboxConfig{ChildCID: 40, AgentPort: 7200}}, token: key, relaySocket: filepath.Join(dir, "relay.sock"), ioCallTimeout: 90 * time.Second}
	s.relay = s.relayCommand(7200, s.relaySocket)
	if s.relay.Start() != nil {
		t.Fatal("owned CID relay launch failed")
	}
	defer func() {
		if s.stopRelay() != nil {
			t.Error("owned relay cleanup failed")
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 110*time.Second)
	defer cancel()
	deadline := time.Now().Add(12 * time.Second)
	for {
		err = s.acquireRelaySocketOwner(s.relaySocket)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) || time.Now().After(deadline) {
			t.Fatal("protected owned relay did not publish")
		}
		time.Sleep(50 * time.Millisecond)
	}
	client, err := s.dialAgent(ctx, s.relaySocket)
	if err != nil {
		t.Fatal("owned peer dial refused")
	}
	defer client.Close()
	s.agent = client
	checked, ok := client.(*agent.Client)
	if !ok {
		t.Fatal("production checked client required")
	}
	checked.Timeout = 90 * time.Second
	var caps guestCapabilities
	if client.CallContext(ctx, "Capabilities", map[string]any{}, &caps) != nil || !liveRelayCapabilitiesMatch(caps, "7.0.0-mk2-gce-lab-g7-pool-rb11") || requireCapabilitySet(caps.ProtocolFeatures, requiredAgentProtocolFeatures, "protocol") != nil {
		t.Fatal("approved guest capabilities required before graceful stop")
	}
	var state agent.ProcessState
	if client.CallContext(ctx, "StateProcess", map[string]string{"id": "init"}, &state) != nil || state.ID != "init" || state.Status != "STOPPED" || state.PID != 616 || state.ExitCode != 137 {
		t.Fatal("exact retained stopped task required")
	}
	// A case-wide durable marker refuses duplicate/ambiguous operator attempts,
	// even if a later caller chooses another work directory or unit name.
	original, err := safefile.OpenDirectory("/opt/mkruntime/g8-host-0c50de9/failed-retention", false)
	if err != nil {
		t.Fatal("original private retention directory required")
	}
	defer original.Close()
	marker, _ := json.Marshal(map[string]any{"version": 1, "scope": "one-generation-graceful-stop-attempt", "boot_id": boot, "sandbox_id": sid, "generation": generation, "invocation_id": os.Getenv("INVOCATION_ID"), "mutation_may_have_started": true, "forced_stop_allowed": false})
	created, err := original.PublishExclusive("graceful-stop-requested.json", marker, 0600)
	if err != nil || !created {
		t.Fatal("existing or uncertain operator attempt must remain retained")
	}
	t.Log("EXACT_RETAINED_STOPPED_TASK_MATCHED_GRACEFUL_STOP_ATTEMPT_BEGIN")
	if s.quiesceAndShutdownGuest(ctx) != nil {
		t.Fatal("graceful shutdown method failed; retain all resources and inspect independent state")
	}
	complete, _ := json.Marshal(map[string]any{"version": 1, "scope": "production-graceful-method-returned", "quiesce_acknowledged": true, "shutdown_attempted": true, "kernel_stop_independently_verified": false, "native_synced_close_independently_verified": false, "normal_cleanup_qualified": false})
	created, err = target.PublishExclusive("graceful-method-result.json", complete, 0600)
	if err != nil || !created {
		t.Fatal("graceful method result persistence uncertain; retain independent observations")
	}
	t.Log("PRODUCTION_QUIESCE_ACKNOWLEDGED_SHUTDOWN_ATTEMPTED_INDEPENDENT_KERNEL_AND_NATIVE_OBSERVATION_REQUIRED")
}
