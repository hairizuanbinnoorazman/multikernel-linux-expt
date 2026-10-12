//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

func TestSupervisorRestartsSignaledWorker(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, "worker-signaled")
	if err := os.WriteFile(filepath.Join(directory, ".multikernel-worker.pid"), []byte("999999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := os.CreateTemp(directory, "listener")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_TEST_MARKER="+marker)
	if status := superviseShimWorkerWith(listener, os.Args[0], []string{"-test.run=^$"}, directory, environment); status != 0 {
		t.Fatalf("supervisor status = %d", status)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "first-worker-signaled\n" {
		t.Fatalf("restart marker = %q, error = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker PID file remains after clean exit: %v", err)
	}
}

func TestShimConnectionBridgeSurvivesWorkerReplacement(t *testing.T) {
	client, stable := net.Pipe()
	defer client.Close()
	defer stable.Close()
	bridge := newShimConnectionBridge(stable)
	deadline := time.Now().Add(5 * time.Second)
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}

	for generation, values := range [][2]string{{"request-one", "response-one"}, {"request-two", "response-two"}} {
		proxy, worker := net.Pipe()
		if err := worker.SetDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- bridge.serveWorker(proxy) }()

		streamID := uint32(generation*2 + 1)
		requestFrame := encodeTestTTRPCFrame(streamID, ttrpcRequest, 0, []byte(values[0]))
		if _, err := client.Write(requestFrame); err != nil {
			t.Fatalf("generation %d client write: %v", generation, err)
		}
		request, err := readTTRPCFrame(worker)
		if err != nil || string(request.encoded[ttrpcHeaderLength:]) != values[0] {
			t.Fatalf("generation %d worker request = %q, %v", generation, request.encoded, err)
		}
		responseFrame := encodeTestTTRPCFrame(streamID, ttrpcResponse, 0, []byte(values[1]))
		go func() { _ = writeFull(worker, responseFrame) }()
		response, err := readTTRPCFrame(client)
		if err != nil || string(response.encoded[ttrpcHeaderLength:]) != values[1] {
			t.Fatalf("generation %d client response = %q, %v", generation, response.encoded, err)
		}
		if err := worker.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("generation %d proxy did not observe worker close", generation)
		}
	}
}

func TestProxiedSupervisorReapsWorkerWhenContainerdDisconnects(t *testing.T) {
	probe, file, _, err := privateShimListener()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("abstract Unix listeners unavailable in this environment: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = probe.Close()
	previousGrace := shimWorkerDisconnectGrace
	shimWorkerDisconnectGrace = 100 * time.Millisecond
	t.Cleanup(func() { shimWorkerDisconnectGrace = previousGrace })
	bundle := privateTestDirectory(t)
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	client, stable := net.Pipe()
	marker := filepath.Join(bundle, "blocked-worker-pid")
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_BLOCK_WORKER="+marker)
	done := make(chan int, 1)
	go func() {
		done <- superviseProxiedShimWorkerWithDirectory(stable, os.Args[0], []string{"-test.run=^$"},
			directory.ProcPath(), environment, directory)
	}()
	var pid int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		data, readErr := os.ReadFile(marker)
		if readErr == nil {
			if value := strings.TrimSpace(string(data)); value != "" {
				pid, err = strconv.Atoi(value)
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || pid <= 0 {
		t.Fatalf("blocked worker PID = %d, %v", pid, err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-done:
		if status != 0 {
			t.Fatalf("supervisor status = %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not exit after containerd disconnect")
	}
	if killErr := syscall.Kill(pid, 0); !errors.Is(killErr, syscall.ESRCH) {
		t.Fatalf("worker process survived containerd disconnect: %v", killErr)
	}
	if _, err = os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker PID file remains after disconnect: %v", err)
	}
}

func TestProxiedSupervisorAcceptsContainerdReconnectBeforeReapingWorker(t *testing.T) {
	probe, file, _, err := privateShimListener()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("abstract Unix listeners unavailable in this environment: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = probe.Close()
	previousGrace := shimWorkerDisconnectGrace
	shimWorkerDisconnectGrace = 100 * time.Millisecond
	t.Cleanup(func() { shimWorkerDisconnectGrace = previousGrace })
	bundle := privateTestDirectory(t)
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	firstClient, firstStable := net.Pipe()
	secondClient, secondStable := net.Pipe()
	defer firstClient.Close()
	defer secondClient.Close()
	reconnections := make(chan net.Conn, 1)
	reconnections <- secondStable
	reconnect := func(timeout time.Duration) (net.Conn, error) {
		select {
		case connection := <-reconnections:
			return connection, nil
		case <-time.After(timeout):
			return nil, os.ErrDeadlineExceeded
		}
	}
	marker := filepath.Join(bundle, "blocked-worker-pid")
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_BLOCK_WORKER="+marker)
	done := make(chan int, 1)
	go func() {
		done <- superviseReconnectableShimWorkerWithDirectory(firstStable, reconnect, os.Args[0], []string{"-test.run=^$"},
			directory.ProcPath(), environment, directory)
	}()
	var pid int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		data, readErr := os.ReadFile(marker)
		if readErr == nil {
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			break
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || pid <= 0 {
		t.Fatalf("blocked worker PID = %d, %v", pid, err)
	}
	if err = firstClient.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	select {
	case status := <-done:
		t.Fatalf("supervisor exited during containerd reconnect with status %d", status)
	default:
	}
	if killErr := syscall.Kill(pid, 0); killErr != nil {
		t.Fatalf("worker did not survive containerd reconnect: %v", killErr)
	}
	if err = secondClient.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-done:
		if status != 0 {
			t.Fatalf("supervisor status after reconnect grace = %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not apply bounded orphan reap after reconnect")
	}
	if killErr := syscall.Kill(pid, 0); !errors.Is(killErr, syscall.ESRCH) {
		t.Fatalf("worker survived exhausted reconnect grace: %v", killErr)
	}
}

func TestProxiedSupervisorRestartsSignaledWorkerAfterProxyEOF(t *testing.T) {
	probe, file, _, err := privateShimListener()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("abstract Unix listeners unavailable in this environment: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	_ = probe.Close()
	bundle := privateTestDirectory(t)
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	client, stable := net.Pipe()
	defer client.Close()
	marker := filepath.Join(bundle, "proxy-first-worker")
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_PROXY_FIRST_MARKER="+marker)
	done := make(chan int, 1)
	go func() {
		done <- superviseProxiedShimWorkerWithDirectory(stable, os.Args[0], []string{"-test.run=^$"},
			directory.ProcPath(), environment, directory)
	}()
	select {
	case status := <-done:
		if status != 0 {
			t.Fatalf("supervisor status = %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("supervisor did not recover proxy-first worker death")
	}
	value, err := os.ReadFile(marker)
	if err != nil || string(value) != "first-worker-proxy-first\nsecond-worker-clean\n" {
		t.Fatalf("worker generations = %q, %v", value, err)
	}
	if _, err = os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker PID file remains after recovery: %v", err)
	}
}

func encodeTestTTRPCFrame(streamID uint32, typeID, flags byte, payload []byte) []byte {
	encoded := make([]byte, ttrpcHeaderLength+len(payload))
	binary.BigEndian.PutUint32(encoded[:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(encoded[4:8], streamID)
	encoded[8], encoded[9] = typeID, flags
	copy(encoded[ttrpcHeaderLength:], payload)
	return encoded
}

func TestShimConnectionBridgeReplaysInFlightRequest(t *testing.T) {
	client, stable := net.Pipe()
	defer client.Close()
	defer stable.Close()
	bridge := newShimConnectionBridge(stable)
	deadline := time.Now().Add(5 * time.Second)
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	request := encodeTestTTRPCFrame(1, ttrpcRequest, 0, []byte("wait-request"))

	firstProxy, firstWorker := net.Pipe()
	firstDone := make(chan error, 1)
	go func() { firstDone <- bridge.serveWorker(firstProxy) }()
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	observed, err := readTTRPCFrame(firstWorker)
	if err != nil || !bytes.Equal(observed.encoded, request) {
		t.Fatalf("first worker request = %x, %v", observed.encoded, err)
	}
	if err = firstWorker.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("first worker proxy did not close")
	}

	secondProxy, secondWorker := net.Pipe()
	secondDone := make(chan error, 1)
	go func() { secondDone <- bridge.serveWorker(secondProxy) }()
	replayed, err := readTTRPCFrame(secondWorker)
	if err != nil || !bytes.Equal(replayed.encoded, request) {
		t.Fatalf("replayed request = %x, %v", replayed.encoded, err)
	}
	response := encodeTestTTRPCFrame(1, ttrpcResponse, 0, []byte("wait-response"))
	go func() { _ = writeFull(secondWorker, response) }()
	forwarded, err := readTTRPCFrame(client)
	if err != nil || !bytes.Equal(forwarded.encoded, response) {
		t.Fatalf("forwarded response = %x, %v", forwarded.encoded, err)
	}
	if err = secondWorker.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondDone:
	case <-time.After(5 * time.Second):
		t.Fatal("second worker proxy did not close")
	}
}

func TestInitialSupervisorCommandUsesHeldBundleAfterPublicReplacement(t *testing.T) {
	base := privateTestDirectory(t)
	bundle := filepath.Join(base, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	moved := filepath.Join(base, "bundle-held")
	if err = os.Rename(bundle, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", "-c", "printf held > initial-supervisor-marker")
	if err = bindCommandToBundle(command, directory); err != nil {
		t.Fatal(err)
	}
	if err = command.Run(); err != nil {
		t.Fatal(err)
	}
	if value, err := os.ReadFile(filepath.Join(moved, "initial-supervisor-marker")); err != nil || string(value) != "held" {
		t.Fatalf("held supervisor marker = %q, %v", value, err)
	}
	if _, err = os.Stat(filepath.Join(bundle, "initial-supervisor-marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement bundle received supervisor marker: %v", err)
	}
}

func TestSupervisorWorkerUsesHeldBundleAfterPublicReplacement(t *testing.T) {
	base := privateTestDirectory(t)
	bundle := filepath.Join(base, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	expected := directory.Identity()
	moved := filepath.Join(base, "bundle-held")
	if err = os.Rename(bundle, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := os.CreateTemp(base, "listener")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	output := filepath.Join(base, "worker-identity")
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_IDENTITY_OUTPUT="+output)
	if status := superviseShimWorkerWithDirectory(listener, os.Args[0], []string{"-test.run=^$"}, bundle, environment, directory); status != 0 {
		t.Fatalf("supervisor status = %d", status)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%d %d %d %d %d %d\n", expected.Device, expected.Inode, expected.UID,
		expected.Device, expected.Inode, expected.UID)
	if string(data) != want {
		t.Fatalf("worker identity = %q, want %q", data, want)
	}
	if _, err = os.Stat(filepath.Join(bundle, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement bundle received PID marker: %v", err)
	}
	if _, err = os.Stat(filepath.Join(moved, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("held bundle retained PID marker: %v", err)
	}
}

func TestOpenServiceIdentityRejectsSupervisorHandoffMismatch(t *testing.T) {
	bundle := privateTestDirectory(t)
	identity := testBundleIdentity(t, bundle)
	t.Setenv("MK_SHIM_BUNDLE_DEVICE", strconv.FormatUint(identity.Device, 10))
	t.Setenv("MK_SHIM_BUNDLE_INODE", strconv.FormatUint(identity.Inode+1, 10))
	t.Setenv("MK_SHIM_BUNDLE_UID", strconv.FormatUint(uint64(identity.UID), 10))
	if directory, _, err := openServiceIdentity("task", "default", bundle); err == nil || directory != nil {
		t.Fatalf("mismatched supervisor handoff = directory:%v error:%v", directory, err)
	}
	t.Setenv("MK_SHIM_BUNDLE_INODE", "")
	if directory, _, err := openServiceIdentity("task", "default", bundle); err == nil || directory != nil {
		t.Fatalf("incomplete supervisor handoff = directory:%v error:%v", directory, err)
	}
}
