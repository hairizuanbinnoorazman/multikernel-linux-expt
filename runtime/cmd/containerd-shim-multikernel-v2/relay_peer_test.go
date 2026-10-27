//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func TestDefaultRelayBindsDeclaredChildPeer(t *testing.T) {
	t.Setenv("MK_RELAY", "/opt/approved/mkvsock-relay")
	for _, cid := range []uint32{40, 41} {
		s := &service{sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{ChildCID: cid}}}
		c := s.relayCommand(7200, "/run/test-relay.sock")
		want := []string{"/opt/approved/mkvsock-relay", "server-peer", strconv.FormatUint(uint64(cid), 10), "7200", "/run/test-relay.sock"}
		if !reflect.DeepEqual(c.Args, want) || c.SysProcAttr == nil || !c.SysProcAttr.Setpgid {
			t.Fatalf("declared peer/process-group policy missing: %v", c.Args)
		}
	}
}
func TestDefaultRelayCannotFallBackToLegacyHelper(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "old-helper")
	attempts := filepath.Join(dir, "attempts")
	// The old mode would succeed. The CID mode must fail once, without retry.
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"" + attempts + "\"\n[ \"$1\" = server ]\n"
	if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MK_RELAY", helper)
	s := &service{sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{ChildCID: 40}}}
	if err := s.relayCommand(7200, filepath.Join(dir, "unused.sock")).Run(); err == nil {
		t.Fatal("old helper silently accepted")
	}
	got, err := os.ReadFile(attempts)
	if err != nil || string(got) != "server-peer\n" {
		t.Fatal("legacy fallback attempted")
	}
}

// This opt-in qualification exercises the production command factory against
// a retained actual child, never a guest process mutation. The launcher must
// pin the boot/resources and bound this test's containing systemd unit.
func TestLiveDefaultRelayDeclaredPeer(t *testing.T) {
	dir := os.Getenv("MK_LIVE_RELAY_PROBE_DIR")
	if dir == "" {
		t.Skip("requires explicitly staged retained-child qualification")
	}
	if os.Geteuid() != 0 || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil {
		t.Fatal("protected root diagnostic required")
	}
	fd, err := strconv.Atoi(os.Getenv("MK_LIVE_RELAY_TOKEN_FD"))
	if err != nil || fd < 3 || fd > 1024 {
		t.Fatal("sealed credential descriptor required")
	}
	credential := os.NewFile(uintptr(fd), "private-credential")
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
	id, gen := os.Getenv("MK_LIVE_RELAY_SANDBOX_ID"), os.Getenv("MK_LIVE_RELAY_GENERATION")
	if id == "" || len(gen) != 32 {
		t.Fatal("expected sandbox identity required")
	}
	run := func(cid uint32, matched bool) {
		t.Helper()
		path := filepath.Join(dir, "relay-"+strconv.Itoa(int(cid))+".sock")
		s := &service{sandbox: protocol.Sandbox{ID: id, Generation: gen, Config: protocol.SandboxConfig{ChildCID: cid, AgentPort: 7200}}}
		command := s.relayCommand(7200, path)
		// Native diagnostics contain only fixed messages; read after Wait to avoid
		// races with exec's stderr-copy goroutine.
		var stderr bytes.Buffer
		command.Stderr = &stderr
		if err = command.Start(); err != nil {
			t.Fatal("staged relay launch failed")
		}
		defer func() {
			if terminateRelay(command) != nil {
				t.Error("owned relay cleanup failed")
			}
		}()
		deadline := time.Now().Add(4 * time.Second)
		published := false
		for time.Now().Before(deadline) {
			info, inspectErr := os.Lstat(path)
			if inspectErr == nil {
				if !matched || info.Mode()&os.ModeSocket == 0 {
					t.Fatal("wrong peer published bridge")
				}
				published = true
				break
			}
			if !os.IsNotExist(inspectErr) {
				t.Fatal("relay path inspection failed")
			}
			time.Sleep(25 * time.Millisecond)
		}
		if matched {
			if !published {
				t.Fatal("selected peer did not publish bridge")
			}
			owner, err := s.captureRelaySocket(path)
			if err != nil {
				t.Fatal("protected bridge capture failed")
			}
			defer owner.Remove()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			client, err := agent.DialPeerContext(ctx, path, id, gen, 7200, key, command.Process.Pid)
			if err != nil {
				t.Fatal("local relay peer verification failed")
			}
			defer client.Close()
			var c guestCapabilities
			if client.CallContext(ctx, "Capabilities", map[string]any{}, &c) != nil {
				t.Fatal("bounded capabilities request failed")
			}
			if !liveRelayCapabilitiesMatch(c, os.Getenv("MK_LIVE_RELAY_RELEASE")) {
				t.Fatal("capabilities mismatch")
			}
		}
		if terminateRelay(command) != nil {
			t.Fatal("relay termination failed")
		}
		if !matched && !strings.Contains(stderr.String(), "relay peer rejected before Unix bridge publication") {
			t.Fatal("no actual peer refusal observed")
		}
		outcome := map[string]any{"declared_cid": cid, "bridge_published": published, "capabilities_matched": matched, "actual_peer_refusal_observed": !matched, "mutating_guest_method_attempted": false}
		b, _ := json.Marshal(outcome)
		t.Log(string(b))
	}
	run(41, false)
	run(40, true)
}

func liveRelayCapabilitiesMatch(c guestCapabilities, release string) bool {
	uid, uidPresent := c.Agent["uid"].(float64)
	gid, gidPresent := c.Agent["gid"].(float64)
	return c.Protocol == 1 && c.Kernel["architecture"] == "amd64" && c.Kernel["release"] == release && uidPresent && gidPresent && uid == 0 && gid == 0
}

func TestLiveRelayCapabilityProjection(t *testing.T) {
	// The partial live fixture omitted protocol_features/oci_features and the
	// kernel/agent details. Strict decoding must remain strict; use the same
	// complete projection as production negotiation instead.
	body := []byte(`{"protocol":1,"protocol_features":["signal-operation-id-v1"],"oci_features":["argv"],"kernel":{"architecture":"amd64","release":"test-release","multikernel":true,"cgroup_v2":true,"mk_transport":true,"namespaces":["mnt"]},"agent":{"uid":0,"gid":0,"effective_capabilities":"0","bounding_capabilities":"0","no_new_privileges":"1"}}`)
	var partial struct {
		Protocol int `json:"protocol"`
	}
	if protocol.StrictDecode(body, &partial) == nil {
		t.Fatal("partial fixture unexpectedly accepted the full response")
	}
	var c guestCapabilities
	if protocol.StrictDecode(body, &c) != nil || !liveRelayCapabilitiesMatch(c, "test-release") {
		t.Fatal("complete production capability projection refused")
	}
	if liveRelayCapabilitiesMatch(c, "other-release") {
		t.Fatal("wrong release accepted")
	}
	delete(c.Agent, "uid")
	if liveRelayCapabilitiesMatch(c, "test-release") {
		t.Fatal("omitted root identity accepted")
	}
	c.Agent["uid"] = "0"
	if liveRelayCapabilitiesMatch(c, "test-release") {
		t.Fatal("string root identity accepted")
	}
}
