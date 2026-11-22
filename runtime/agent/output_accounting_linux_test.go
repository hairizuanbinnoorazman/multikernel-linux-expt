//go:build linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestOutputAccountingNativeProducer(t *testing.T) {
	if os.Getenv("MK_AGENT_OUTPUT_ACCOUNTING_HELPER") != "1" {
		return
	}
	if _, err := os.Stdout.Write(bytes.Repeat([]byte("p"), maxOutputBytes+17)); err != nil {
		os.Exit(96)
	}
	if _, err := os.Stderr.Write([]byte("bounded-stderr")); err != nil {
		os.Exit(95)
	}
	os.Exit(37)
}

func TestOutputAccountingNativeProcessCheckedUnixRPC(t *testing.T) {
	manager := NewManager(true)
	path := bundle(t, []string{"/probe", "-test.run=^TestOutputAccountingNativeProducer$"}, "")
	config, _, err := LoadBundle(path)
	if err != nil {
		t.Fatal("native fixture config unavailable")
	}
	config.Process.Env = []string{"MK_AGENT_OUTPUT_ACCOUNTING_HELPER=1"}
	config.Process.User.UID = uint32(os.Geteuid())
	config.Process.User.GID = uint32(os.Getegid())
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(path+"/config.json", raw, 0600) != nil {
		t.Fatal("native config publication failed")
	}
	if err = manager.Create("native", path); err != nil {
		t.Fatal("native create failed", err)
	}
	if err = manager.Start("native"); err != nil {
		t.Fatal("native start failed", err)
	}
	state, err := manager.Wait("native")
	if err != nil || state.ExitCode != 37 {
		t.Fatal("native exit differs")
	}
	listener, socket := ownedAgentListener(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	server := &Server{Manager: manager, SandboxID: "native-output", Generation: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Endpoint: 7001, Token: bytes.Repeat([]byte("t"), 32)}
	go server.Serve(ctx, listener)
	client, err := DialPeerContext(ctx, socket, server.SandboxID, server.Generation, server.Endpoint, server.Token, os.Getpid())
	if err != nil {
		t.Fatal("checked native peer unavailable", err)
	}
	defer client.Close()
	var report protocol.ProcessOutputAccounting
	if err = client.CallContext(ctx, "ProcessOutputAccounting", map[string]string{"id": "native"}, &report); err != nil {
		t.Fatal("native accounting RPC failed", err)
	}
	if err = report.Validate("native", state.PID); err != nil {
		t.Fatal(err)
	}
	if report.Status != "STOPPED" || report.Stdout.ObservedBytes != maxOutputBytes+17 || report.Stdout.DroppedBytes != 17 || report.Stdout.BufferedBytes != maxOutputBytes || report.Stdout.PendingBytes != 0 || report.Stdout.AcknowledgedBytes != 0 || report.Stdout.BufferedPeakBytes != maxOutputBytes || report.Stderr.ObservedBytes != 14 {
		t.Fatal("native stream conservation differs")
	}
	var again protocol.ProcessOutputAccounting
	if err = client.CallContext(ctx, "ProcessOutputAccounting", map[string]string{"id": "native"}, &again); err != nil || again != report {
		t.Fatal("native observation mutated counters")
	}
	if err = manager.Delete("native"); err != nil {
		t.Fatal("native delete failed")
	}
	if err = client.CallContext(ctx, "ProcessOutputAccounting", map[string]string{"id": "native"}, &again); err == nil {
		t.Fatal("deleted process fabricated a report")
	}
	t.Logf("native_exit=37 stdout_observed=%d stdout_dropped=%d stdout_buffered=%d retained_peak=%d stderr_observed=%d checked_unix_peer=true", report.Stdout.ObservedBytes, report.Stdout.DroppedBytes, report.Stdout.BufferedBytes, report.Stdout.BufferedPeakBytes, report.Stderr.ObservedBytes)
}
