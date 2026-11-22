package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func requireOutputConservation(t *testing.T, b *lockedBuffer) protocol.OutputBufferAccounting {
	t.Helper()
	r := b.accounting()
	if err := r.Validate(); err != nil {
		t.Fatalf("invalid coherent buffer counters: %v", err)
	}
	return r
}

func TestOutputAccountingTimeoutDropReadReplayAndHighWater(t *testing.T) {
	var b lockedBuffer
	payload := bytes.Repeat([]byte("s"), maxOutputBytes+17)
	if n, err := b.Write(payload); err != nil || n != len(payload) {
		t.Fatal("writer did not consume its bounded input", n, err)
	}
	before := requireOutputConservation(t, &b)
	if before.ObservedBytes != uint64(len(payload)) || before.DroppedBytes != 17 || before.BufferedBytes != maxOutputBytes || before.BufferedPeakBytes != maxOutputBytes || before.PendingBytes != 0 || before.AcknowledgedBytes != 0 || !before.Truncated {
		t.Fatal("timeout loss or retained peak differs", before)
	}
	chunk, next := b.slice(0, 4096)
	if len(chunk) != 4096 || next != 4096 {
		t.Fatal("bounded read changed")
	}
	if after := requireOutputConservation(t, &b); after != before {
		t.Fatal("read or counter observation acknowledged output")
	}
	replay, again := b.slice(0, 4096)
	if !bytes.Equal(chunk, replay) || again != next {
		t.Fatal("counter sampling changed replay")
	}
	b.slice(next, 4096)
	after := requireOutputConservation(t, &b)
	if after.AcknowledgedBytes != 4096 || after.BufferedBytes != maxOutputBytes-4096 || after.BufferedPeakBytes != maxOutputBytes || after.DroppedBytes != 17 {
		t.Fatal("acknowledgement or high-water reset", after)
	}
	t.Logf("observed=%d dropped=%d retained_peak=%d acknowledged=%d", after.ObservedBytes, after.DroppedBytes, after.BufferedPeakBytes, after.AcknowledgedBytes)
}

func TestOutputAccountingPendingAndConcurrentSnapshots(t *testing.T) {
	var b lockedBuffer
	if _, err := b.Write(make([]byte, maxOutputBytes)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := b.Write([]byte("pending")); done <- err }()
	deadline := time.Now().Add(time.Second)
	for {
		r := requireOutputConservation(t, &b)
		if r.PendingBytes == 7 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending write not observed")
		}
		time.Sleep(time.Microsecond)
	}
	// Acknowledge the full buffer. The pending writer can fill its released space.
	b.slice(maxOutputBytes, 1)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r := requireOutputConservation(t, &b)
	if r.PendingBytes != 0 || r.DroppedBytes != 0 || r.BufferedBytes != 7 || r.AcknowledgedBytes != maxOutputBytes {
		t.Fatal("pending write accounting differs", r)
	}
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				b.Write([]byte("bounded"))
			}
		}()
	}
	for i := 0; i < 500; i++ {
		requireOutputConservation(t, &b)
	}
	workers.Wait()
	r = requireOutputConservation(t, &b)
	if r.ObservedBytes != maxOutputBytes+7+8*100*7 {
		t.Fatal("concurrent input disappeared", r)
	}
}

func TestOutputAccountingOverflowNeverRegainsAllowance(t *testing.T) {
	b := lockedBuffer{observed: ^uint64(0), base: ^uint64(0)}
	if n, err := b.Write([]byte("x")); n != 0 || err == nil {
		t.Fatal("overflow accepted input")
	}
	r := b.accounting()
	if r.ObservedBytes != ^uint64(0) || r.PendingBytes != 0 || r.BufferedBytes != 0 || !r.AccountingExhausted || r.Validate() == nil {
		t.Fatal("overflow wrapped or claimed qualified counters", r)
	}
}

func TestOutputAccountingAuthenticatedRPCContainsNoOutput(t *testing.T) {
	manager := NewManager(true)
	p := &process{state: ProcessState{ID: "bounded", PID: 41, Status: "STOPPED"}}
	secret := strings.Repeat("SYNTHETIC_OUTPUT_NOT_FOR_ACCOUNTING_RPC", 4096)
	p.stdout.Write([]byte(secret))
	p.stderr.Write([]byte("private stderr payload"))
	manager.processes["bounded"] = p
	server := &Server{Manager: manager, SandboxID: "box", Generation: strings.Repeat("a", 32), Endpoint: 7001, Token: bytes.Repeat([]byte("t"), 32)}
	request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "ProcessOutputAccounting", Body: json.RawMessage(`{"id":"bounded"}`)}
	Sign(&request, bytes.Repeat([]byte("x"), 32))
	if r := server.Dispatch(request); r.Error == "" {
		t.Fatal("untrusted caller obtained counters")
	}
	Sign(&request, server.Token)
	reply := server.Dispatch(request)
	if reply.Error != "" {
		t.Fatal("authenticated accounting refused", reply.Error)
	}
	report, ok := reply.Body.(protocol.ProcessOutputAccounting)
	if !ok || report.Validate("bounded", 41) != nil {
		t.Fatal("invalid process accounting reply")
	}
	wire, err := json.Marshal(reply)
	if err != nil || len(wire) > 2048 || bytes.Contains(wire, []byte("SYNTHETIC_OUTPUT")) || bytes.Contains(wire, []byte("private stderr")) {
		t.Fatal("accounting exposed or retained output")
	}
	if report.Stdout.ObservedBytes != uint64(len(secret)) {
		t.Fatal("wire counters differ")
	}
	if r := server.Dispatch(request); r.Error == "" {
		t.Fatal("replay accepted")
	}
	request.Sequence = 2
	request.Generation = strings.Repeat("b", 32)
	Sign(&request, server.Token)
	if r := server.Dispatch(request); r.Error == "" {
		t.Fatal("wrong generation accepted")
	}
	request.Sequence = 2
	request.Generation = server.Generation
	request.Body = json.RawMessage(`{"id":"missing"}`)
	Sign(&request, server.Token)
	if r := server.Dispatch(request); r.Error == "" {
		t.Fatal("missing process fabricated zero counters")
	}
	if manager.processes["bounded"].stdout.accounting() != report.Stdout {
		t.Fatal("observation consumed output")
	}
	t.Logf("accounting_reply_bytes=%d payload_bytes_excluded=%d", len(wire), len(secret))
}
