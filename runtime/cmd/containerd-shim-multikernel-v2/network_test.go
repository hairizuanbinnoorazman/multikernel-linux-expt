//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	mknetwork "github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

func networkPumpSocket(t *testing.T) (*os.File, int) {
	t.Helper()
	descriptors, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	device := os.NewFile(uintptr(descriptors[0]), "network-pump-device")
	t.Cleanup(func() {
		_ = device.Close()
		_ = unix.Close(descriptors[1])
	})
	return device, descriptors[1]
}

func waitNetworkCondition(t *testing.T, condition func() bool, description string) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func stopNetworkPumpForTest(s *service) {
	close(s.netDone)
	s.netWG.Wait()
	s.netDone = nil
}

func TestNetworkPumpMTUBoundsCountersAndReconnect(t *testing.T) {
	const mtu = 576
	t.Run("exact MTU round trip and oversized ingress", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		packets := make(chan []byte, 2)
		client := &networkPumpAgent{packets: packets, handler: func(_ int, packet []byte) ([]byte, error) {
			return packet, nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{1}, mtu+1)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return s.netRXDrops.Load() == 1 }, "oversized ingress drop")
		payload := bytes.Repeat([]byte{2}, mtu)
		if _, err := unix.Write(peer, payload); err != nil {
			t.Fatal(err)
		}
		select {
		case observed := <-packets:
			if !bytes.Equal(observed, payload) {
				t.Fatal("exact-MTU ingress changed")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("exact-MTU ingress was not forwarded")
		}
		buffer := make([]byte, mtu+1)
		waitNetworkCondition(t, func() bool {
			n, err := unix.Read(peer, buffer)
			return err == nil && n == len(payload) && bytes.Equal(buffer[:n], payload)
		}, "exact-MTU egress")
		if s.netRXPackets.Load() != 1 || s.netTXPackets.Load() != 1 || s.netRXDrops.Load() != 1 || s.netTXDrops.Load() != 0 || s.netErrors.Load() != 0 {
			t.Fatalf("network counters = rx:%d tx:%d rxdrop:%d txdrop:%d errors:%d", s.netRXPackets.Load(), s.netTXPackets.Load(), s.netRXDrops.Load(), s.netTXDrops.Load(), s.netErrors.Load())
		}
	})

	t.Run("oversized guest egress", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{handler: func(_ int, packet []byte) ([]byte, error) {
			if len(packet) == 0 {
				return nil, nil
			}
			return bytes.Repeat([]byte{3}, mtu+1), nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{2}, mtu)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return s.netTXDrops.Load() == 1 }, "oversized guest egress drop")
		if s.netRXPackets.Load() != 1 || s.netTXPackets.Load() != 0 {
			t.Fatalf("oversized egress counters = rx:%d tx:%d", s.netRXPackets.Load(), s.netTXPackets.Load())
		}
	})

	t.Run("disconnect drops packet and retries reconnect", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		packets := make(chan []byte, 2)
		client := &networkPumpAgent{packets: packets, reconnectFailures: 2}
		client.handler = func(_ int, packet []byte) ([]byte, error) {
			if len(packet) != 0 && client.reconnectCount() == 0 {
				return nil, errors.New("injected exchange disconnect")
			}
			return nil, nil
		}
		s := &service{agent: client, relaySocket: "/run/multikernel/relay.sock", netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		if _, err := unix.Write(peer, bytes.Repeat([]byte{4}, mtu)); err != nil {
			t.Fatal(err)
		}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		waitNetworkCondition(t, func() bool { return client.reconnectCount() >= 3 }, "bounded reconnect retries")
		if _, err := unix.Write(peer, bytes.Repeat([]byte{5}, mtu)); err != nil {
			t.Fatal(err)
		}
		waitNetworkCondition(t, func() bool { return s.netRXPackets.Load() == 1 }, "post-reconnect packet")
		if s.netRXDrops.Load() != 1 || s.netErrors.Load() != 1 {
			t.Fatalf("disconnect counters = drops:%d errors:%d", s.netRXDrops.Load(), s.netErrors.Load())
		}
	})

	t.Run("shared transport recovery avoids a competing redial", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{preserveTransportOnExchangeError: true}
		failed := false
		client.handler = func(_ int, packet []byte) ([]byte, error) {
			if len(packet) != 0 && !failed {
				failed = true
				return nil, errors.New("injected exchange failure after shared recovery")
			}
			return packet, nil
		}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		if _, err := unix.Write(peer, bytes.Repeat([]byte{6}, mtu)); err != nil {
			t.Fatal(err)
		}
		waitNetworkCondition(t, func() bool { return s.netErrors.Load() == 1 }, "shared transport failure")
		if reconnects := client.reconnectCount(); reconnects != 0 {
			t.Fatalf("shared recovered transport triggered %d competing redials", reconnects)
		}
		marker := bytes.Repeat([]byte{7}, mtu)
		if _, err := unix.Write(peer, marker); err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, mtu+1)
		waitNetworkCondition(t, func() bool {
			n, err := unix.Read(peer, buffer)
			return err == nil && n == len(marker) && bytes.Equal(buffer[:n], marker)
		}, "packet after shared transport recovery")
		if s.netRXDrops.Load() != 1 || s.netRXPackets.Load() != 1 || s.netTXPackets.Load() != 1 {
			t.Fatalf("shared recovery counters = rx:%d tx:%d rxdrop:%d", s.netRXPackets.Load(), s.netTXPackets.Load(), s.netRXDrops.Load())
		}
	})
}

func TestNetworkPumpPacketIntegrityOrderingLoadAndSlowReader(t *testing.T) {
	const mtu = 576
	readPacket := func(t *testing.T, descriptor int, size int) []byte {
		t.Helper()
		buffer := make([]byte, size)
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			n, err := unix.Read(descriptor, buffer)
			if err == nil {
				return append([]byte(nil), buffer[:n]...)
			}
			if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("timed out reading network-pump packet")
		return nil
	}
	writePacket := func(t *testing.T, descriptor int, packet []byte) {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if _, err := unix.Write(descriptor, packet); err == nil {
				return
			} else if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("timed out writing network-pump packet")
	}
	checksum := func(header []byte) uint16 {
		var sum uint32
		for index := 0; index+1 < len(header); index += 2 {
			sum += uint32(binary.BigEndian.Uint16(header[index : index+2]))
		}
		for sum > 0xffff {
			sum = (sum & 0xffff) + (sum >> 16)
		}
		return ^uint16(sum)
	}
	ipv4Packet := func(flagsOffset uint16, payload []byte) []byte {
		packet := make([]byte, 20+len(payload))
		packet[0], packet[8], packet[9] = 0x45, 64, 17
		binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
		binary.BigEndian.PutUint16(packet[4:6], 0x1234)
		binary.BigEndian.PutUint16(packet[6:8], flagsOffset)
		copy(packet[12:16], []byte{192, 0, 2, 1})
		copy(packet[16:20], []byte{198, 51, 100, 1})
		binary.BigEndian.PutUint16(packet[10:12], checksum(packet[:20]))
		copy(packet[20:], payload)
		return packet
	}

	t.Run("fragment checksum and malformed bytes remain opaque", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{handler: func(_ int, packet []byte) ([]byte, error) {
			return packet, nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		packets := [][]byte{
			ipv4Packet(0x2000, []byte("first-fragment")),
			ipv4Packet(0x0002, []byte("last-fragment")),
			{0x45, 0x00, 0x00, 0x01, 0xff},
		}
		for index, packet := range packets {
			writePacket(t, peer, packet)
			if observed := readPacket(t, peer, mtu+1); !bytes.Equal(observed, packet) {
				t.Fatalf("packet %d changed: got %x want %x", index, observed, packet)
			}
		}
		if s.netRXPackets.Load() != uint64(len(packets)) || s.netTXPackets.Load() != uint64(len(packets)) ||
			s.netRXDrops.Load() != 0 || s.netTXDrops.Load() != 0 || s.netErrors.Load() != 0 {
			t.Fatalf("opaque-packet counters = rx:%d tx:%d rxdrop:%d txdrop:%d errors:%d",
				s.netRXPackets.Load(), s.netTXPackets.Load(), s.netRXDrops.Load(), s.netTXDrops.Load(), s.netErrors.Load())
		}
	})

	t.Run("ordered burst and sustained traffic", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{handler: func(_ int, packet []byte) ([]byte, error) {
			return packet, nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		const packets = 256
		for index := 0; index < packets; index++ {
			packet := make([]byte, 64)
			binary.BigEndian.PutUint32(packet, uint32(index))
			for offset := 4; offset < len(packet); offset++ {
				packet[offset] = byte(index + offset)
			}
			writePacket(t, peer, packet)
		}
		for index := 0; index < packets; index++ {
			observed := readPacket(t, peer, mtu+1)
			if len(observed) != 64 || binary.BigEndian.Uint32(observed[:4]) != uint32(index) {
				t.Fatalf("packet order at %d = %x", index, observed)
			}
			for offset := 4; offset < len(observed); offset++ {
				if observed[offset] != byte(index+offset) {
					t.Fatalf("packet %d byte %d changed", index, offset)
				}
			}
		}
		waitNetworkCondition(t, func() bool {
			return s.netRXPackets.Load() == packets && s.netTXPackets.Load() == packets
		}, "ordered burst counters")
		if s.netRXDrops.Load() != 0 || s.netTXDrops.Load() != 0 || s.netErrors.Load() != 0 {
			t.Fatalf("ordered-burst counters = rxdrop:%d txdrop:%d errors:%d",
				s.netRXDrops.Load(), s.netTXDrops.Load(), s.netErrors.Load())
		}
	})

	t.Run("slow reader drops without stalling and recovers", func(t *testing.T) {
		device, peer := networkPumpSocket(t)
		client := &networkPumpAgent{handler: func(_ int, packet []byte) ([]byte, error) {
			return packet, nil
		}}
		s := &service{agent: client, netDevice: device, netEndpoint: mknetwork.Endpoint{MTU: mtu}}
		s.startNetworkPump()
		defer stopNetworkPumpForTest(s)
		packet := bytes.Repeat([]byte{0x5a}, 512)
		sent := 0
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline) && s.netTXDrops.Load() == 0; {
			if _, err := unix.Write(peer, packet); err == nil {
				sent++
			} else if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				t.Fatal(err)
			}
			time.Sleep(100 * time.Microsecond)
		}
		if s.netTXDrops.Load() == 0 {
			t.Fatalf("slow reader produced no bounded drop after %d packets", sent)
		}
		waitNetworkCondition(t, func() bool { return s.netRXPackets.Load() == uint64(sent) }, "slow-reader ingress drain")
		buffer := make([]byte, mtu+1)
		for {
			if _, err := unix.Read(peer, buffer); errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				break
			} else if err != nil {
				t.Fatal(err)
			}
		}
		marker := bytes.Repeat([]byte{0xa5}, 512)
		writePacket(t, peer, marker)
		if observed := readPacket(t, peer, mtu+1); !bytes.Equal(observed, marker) {
			t.Fatalf("post-backpressure packet changed: got %x", observed)
		}
		if s.netErrors.Load() != 0 {
			t.Fatalf("slow reader caused %d fatal pump errors", s.netErrors.Load())
		}
	})
}

func TestNetworkReporterRetriesAndCoalescesLatestState(t *testing.T) {
	client := &retryNetworkReportClient{failures: 2, attempts: make(chan mknetwork.Endpoint, 4), success: make(chan mknetwork.Endpoint, 2)}
	endpoint := validShimEndpoint("task", "box", strings.Repeat("b", 32), strings.Repeat("a", 32), "/run/netns/task")
	s := &service{netDone: make(chan struct{}), netReports: make(chan string, 1), netClient: client,
		netEndpoint: endpoint}
	s.netRXPackets.Store(3)
	s.startNetworkReporter()
	s.queueNetworkReport("DISCONNECTED")
	first := <-client.attempts
	if first.State != "DISCONNECTED" || first.RXPackets != 3 {
		t.Fatalf("first network report = %+v", first)
	}
	// Replace the failed state while the reporter is in its bounded retry wait.
	s.netRXPackets.Store(7)
	s.netErrors.Store(1)
	s.queueNetworkReport("READY")
	select {
	case reported := <-client.success:
		if reported.State != "READY" || reported.RXPackets != 7 || reported.Errors != 1 {
			t.Fatalf("successful coalesced report = %+v", reported)
		}
	case <-time.After(time.Second):
		t.Fatal("network reporter did not retry to success")
	}
	second, third := <-client.attempts, <-client.attempts
	if second.State != "READY" || third.State != "READY" {
		t.Fatalf("retried network states = %q, %q", second.State, third.State)
	}
	s.queueNetworkReport("DEGRADED")
	select {
	case reported := <-client.success:
		if reported.State != "DEGRADED" {
			t.Fatalf("later network report state = %q", reported.State)
		}
	case <-time.After(time.Second):
		t.Fatal("network reporter did not accept a later state")
	}
	close(s.netDone)
	s.netWG.Wait()
}

func TestShimRejectsInvalidNetworkEndpointsBeforeUse(t *testing.T) {
	sandboxGeneration := strings.Repeat("b", 32)
	generation := strings.Repeat("a", 32)
	sandbox := protocol.Sandbox{ID: "box", Generation: sandboxGeneration}
	valid := validShimEndpoint("task", sandbox.ID, sandbox.Generation, generation, "/run/netns/task")

	t.Run("provision identity", func(t *testing.T) {
		invalid := valid
		invalid.ContainerID = "other"
		client := &fakeNetworkClient{endpoint: invalid}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if err := s.provisionNetwork(context.Background(), valid.NetNS); err == nil {
			t.Fatal("PROVISION accepted a mismatched endpoint")
		}
		if !reflect.DeepEqual(s.netEndpoint, mknetwork.Endpoint{}) {
			t.Fatalf("invalid PROVISION endpoint was retained: %+v", s.netEndpoint)
		}
	})

	t.Run("provision managed ownership", func(t *testing.T) {
		client := &fakeNetworkClient{endpoint: valid}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if err := s.provisionNetwork(context.Background(), ""); err == nil {
			t.Fatal("managed PROVISION accepted a CNI-owned endpoint")
		}
		if !reflect.DeepEqual(s.netEndpoint, mknetwork.Endpoint{}) {
			t.Fatalf("wrong-ownership PROVISION endpoint was retained: %+v", s.netEndpoint)
		}
	})

	t.Run("attach generation and descriptor", func(t *testing.T) {
		directory := t.TempDir()
		descriptorPath := filepath.Join(directory, "tun")
		if err := os.WriteFile(descriptorPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		invalid := valid
		invalid.Generation = strings.Repeat("c", 32)
		client := &fakeNetworkClient{endpoint: invalid, descriptorPath: descriptorPath}
		s := &service{id: "task", sandbox: sandbox, netClient: client}
		if _, descriptor, err := s.attachNetwork(context.Background(), valid.NetNS, generation); err == nil || descriptor != nil {
			t.Fatalf("ATTACH accepted mismatched generation: descriptor=%v error=%v", descriptor, err)
		}
		if client.descriptor == nil {
			t.Fatal("ATTACH fixture did not transfer a descriptor")
		}
		if _, err := client.descriptor.Stat(); err == nil {
			t.Fatal("invalid ATTACH descriptor remained open")
		}
	})

	t.Run("report and release generation", func(t *testing.T) {
		invalid := valid
		invalid.Generation = "short"
		client := &fakeNetworkClient{endpoint: valid}
		s := &service{netEndpoint: invalid, netClient: client}
		if err := s.reportNetwork("READY"); err == nil {
			t.Fatal("REPORT accepted a malformed generation")
		}
		if err := s.releaseNetwork(context.Background()); err == nil {
			t.Fatal("RELEASE accepted a malformed generation")
		}
		if len(client.calls) != 0 {
			t.Fatalf("malformed endpoint reached mknetd: %v", client.calls)
		}
	})

	t.Run("release already absent", func(t *testing.T) {
		client := &fakeNetworkClient{fail: map[string]error{"RELEASE": &mknetwork.APIError{Code: "NOT_FOUND", Message: "endpoint does not exist"}}}
		s := &service{netEndpoint: valid, netClient: client}
		if err := s.releaseNetwork(context.Background()); err != nil {
			t.Fatalf("already-absent RELEASE failed: %v", err)
		}
		if !reflect.DeepEqual(s.netEndpoint, mknetwork.Endpoint{}) {
			t.Fatalf("already-absent endpoint retained: %+v", s.netEndpoint)
		}
	})
}

func TestStopNetworkBoundsGuestCloseAndContinuesLocalCleanup(t *testing.T) {
	device, peer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	client := &blockedNetworkCloseAgent{calls: make(chan struct{}, 1)}
	s := &service{agent: client, netDevice: device, networkCloseTimeout: 10 * time.Millisecond}
	started := time.Now()
	err = s.stopNetwork()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stopNetwork error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("bounded network close took %s", elapsed)
	}
	select {
	case <-client.calls:
	default:
		t.Fatal("guest CloseNetwork was not attempted")
	}
	if s.netDevice != nil {
		t.Fatal("timed-out guest close retained the TUN owner")
	}
	if _, err := device.Stat(); err == nil {
		t.Fatal("timed-out guest close left the TUN descriptor open")
	}
}

func TestStopNetworkReportsCountersOnlyForLiveNetworkOwner(t *testing.T) {
	endpoint := validShimEndpoint("task", "sandbox", strings.Repeat("a", 32), strings.Repeat("b", 32), "/run/netns/test")

	t.Run("cleanup recovery skips zero counter report", func(t *testing.T) {
		client := &fakeNetworkClient{endpoint: endpoint}
		s := &service{netEndpoint: endpoint, netClient: client}
		if err := s.stopNetwork(); err != nil {
			t.Fatal(err)
		}
		if len(client.calls) != 0 {
			t.Fatalf("cleanup-only recovery reported unowned counters: %v", client.calls)
		}
	})

	t.Run("live TUN owner reports final counters", func(t *testing.T) {
		device, peer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer peer.Close()
		client := &fakeNetworkClient{endpoint: endpoint}
		s := &service{netEndpoint: endpoint, netClient: client, netDevice: device}
		if err = s.stopNetwork(); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(client.calls) != "[REPORT]" {
			t.Fatalf("live network final calls = %v", client.calls)
		}
	})
}
