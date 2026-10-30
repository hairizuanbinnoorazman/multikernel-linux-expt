//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/events"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const eventDiagnosticSecret = "SYNTHETIC_BROKER_TOKEN /private/event/payload"

type privateFailurePublisher struct{}

func (*privateFailurePublisher) Publish(context.Context, string, events.Event) error {
	return errors.New(eventDiagnosticSecret)
}
func (*privateFailurePublisher) Close() error { return nil }

type eventPayloadPublisher struct {
	topics []string
	pids   []uint32
}

func (p *eventPayloadPublisher) Publish(_ context.Context, topic string, event events.Event) error {
	value, ok := event.(*eventstypes.TaskStart)
	if !ok {
		return errors.New("unexpected replay payload type")
	}
	p.topics = append(p.topics, topic)
	p.pids = append(p.pids, value.Pid)
	return nil
}
func (*eventPayloadPublisher) Close() error { return nil }

func TestEventDiagnosticsDurableOrderSurvivesBlockedWriterAndBrokerFailure(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "capture"
		if blocked {
			name = "blocked"
		}
		t.Run(name, func(t *testing.T) {
			sink, records := diagnosticSinkForTest(t, blocked)
			bundle := privateTestDirectory(t)
			s := &service{bundle: bundle, namespace: "default", publisher: &privateFailurePublisher{}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}, processDiagnosticSink: sink}
			s.setEventDiagnosticOwner(protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)})
			// Real journal persistence and failed publication run with the service lock
			// held. Logging must not re-lock the service or wait for its blocked writer.
			s.mu.Lock()
			done := make(chan error, 1)
			go func() {
				for i := 0; i < 8; i++ {
					if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: uint32(11 + i)}); err != nil {
						done <- err
						return
					}
				}
				done <- nil
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("diagnostics blocked durable queue/publication")
			}
			s.mu.Unlock()
			if len(s.events.Pending) != 8 || s.events.NextSequence != 9 {
				t.Fatal("broker failure changed durable ordering")
			}
			if blocked {
				if len(sink.queue) != 1 {
					t.Fatal("publication logging was repeated")
				}
			} else {
				requireNetworkDiagnostic(t, records, "EVENT_PUBLICATION_DEFERRED")
			}
			// Reconstruct from the real durable file, rather than trusting the old
			// process's in-memory queue. Replay and acknowledgement keep exact order.
			publisher := &eventPayloadPublisher{}
			recovered := &service{bundle: bundle, namespace: "default", publisher: publisher, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := recovered.loadEventJournal(); err != nil {
				t.Fatal(err)
			}
			for i, event := range recovered.events.Pending {
				if event.Sequence != uint64(i+1) || event.Topic != ctruntime.TaskStartEventTopic {
					t.Fatal("wrong durable event order")
				}
			}
			if len(recovered.events.Pending) != 8 || recovered.events.NextSequence != 9 {
				t.Fatal("durable reconstruction lost queued events")
			}
			pendingBefore, err := os.ReadFile(recovered.eventJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			recovered.publisher = &privateFailurePublisher{}
			recovered.processDiagnosticSink = sink
			recovered.setEventDiagnosticOwner(protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)})
			recovered.mu.Lock()
			replayed := make(chan struct{})
			go func() {
				for i := 0; i < 4; i++ {
					recovered.replayEvents(context.Background())
				}
				close(replayed)
			}()
			select {
			case <-replayed:
			case <-time.After(time.Second):
				t.Fatal("logging blocked startup replay")
			}
			recovered.mu.Unlock()
			pendingAfter, err := os.ReadFile(recovered.eventJournalPath())
			if err != nil || !bytes.Equal(pendingBefore, pendingAfter) || len(recovered.events.Pending) != 8 {
				t.Fatal("failed startup replay mutated durable queue", err)
			}
			if blocked {
				if len(sink.queue) != 2 {
					t.Fatal("startup replay diagnostic repeated")
				}
			} else {
				requireNetworkDiagnostic(t, records, "EVENT_REPLAY_DEFERRED")
			}
			recovered.publisher = publisher
			if err := recovered.flushEvents(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(publisher.pids, []uint32{11, 12, 13, 14, 15, 16, 17, 18}) || len(publisher.topics) != 8 || len(recovered.events.Pending) != 0 {
				t.Fatal("wrong replay acknowledgement")
			}
			for _, topic := range publisher.topics {
				if topic != ctruntime.TaskStartEventTopic {
					t.Fatal("wrong replay topic")
				}
			}
			if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("acknowledged journal remains", err)
			}
		})
	}
}

func TestEventDiagnosticsConcurrentAttemptsAndOwnerChanges(t *testing.T) {
	records := make(chan []byte, 64)
	sink := newProcessDiagnosticSink(diagnosticChannelWriter{records})
	defer sink.close()
	s := &service{processDiagnosticSink: sink}
	if s.reportEventDiagnostic(diagnosticEventPublicationDeferred) || s.eventLogAttempts.Load() != 0 {
		t.Fatal("unknown owner consumed quota")
	}
	s.setEventDiagnosticOwner(protocol.Sandbox{ID: "bad\nowner", Generation: strings.Repeat("a", 32)})
	if s.reportEventDiagnostic(diagnosticEventReplayDeferred) {
		t.Fatal("invalid owner emitted diagnostic")
	}
	owner := protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}
	s.setEventDiagnosticOwner(owner)
	var workers sync.WaitGroup
	// Hold both lifecycle locks: the diagnostic interface must use neither.
	s.mu.Lock()
	s.eventMu.Lock()
	for i := 0; i < 64; i++ {
		workers.Go(func() {
			s.reportEventDiagnostic(diagnosticEventPublicationDeferred)
			s.reportEventDiagnostic(diagnosticEventReplayDeferred)
		})
	}
	joined := make(chan struct{})
	go func() { workers.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("diagnostic interface acquired a lifecycle lock")
	}
	s.eventMu.Unlock()
	s.mu.Unlock()
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case raw := <-records:
			var r processDiagnosticRecord
			if bytes.Contains(raw, []byte(eventDiagnosticSecret)) || json.Unmarshal(raw, &r) != nil || !validProcessDiagnosticRecord(r) || r.SandboxID != "box" || r.Generation != owner.Generation || seen[r.Code] {
				t.Fatal("invalid or duplicated fixed event metadata")
			}
			seen[r.Code] = true
		case <-time.After(time.Second):
			t.Fatal("missing concurrent diagnostic")
		}
	}
	if !reflect.DeepEqual(seen, map[string]bool{"EVENT_PUBLICATION_DEFERRED": true, "EVENT_REPLAY_DEFERRED": true}) {
		t.Fatal("wrong fixed event codes")
	}
	s.setEventDiagnosticOwner(protocol.Sandbox{})
	if s.eventDiagnosticOwner.Load() != nil {
		t.Fatal("cleared ownership retained diagnostic context")
	}
	owner.Generation = strings.Repeat("b", 32)
	s.setEventDiagnosticOwner(owner)
	if s.reportEventDiagnostic(diagnosticEventPublicationDeferred) || s.reportEventDiagnostic(diagnosticEventReplayDeferred) || s.reportEventDiagnostic(diagnosticOutputRead) {
		t.Fatal("owner change or unrelated code bypassed worker quota")
	}
	select {
	case <-records:
		t.Fatal("extra event record")
	default:
	}
}

func TestEventDiagnosticsOwnerUpdatesRaceSafelyAndFullQueueConsumesAttempt(t *testing.T) {
	sink, _ := diagnosticSinkForTest(t, true)
	for len(sink.queue) < processDiagnosticQueueSize {
		if !sink.tryRecord(diagnosticTestRecord()) {
			t.Fatal("could not fill queue")
		}
	}
	s := &service{processDiagnosticSink: sink}
	owner := protocol.Sandbox{ID: "box", Generation: strings.Repeat("a", 32)}
	s.setEventDiagnosticOwner(owner)
	if s.reportEventDiagnostic(diagnosticEventReplayDeferred) {
		t.Fatal("full sink accepted event")
	}
	if s.eventLogAttempts.Load()&(uint64(1)<<diagnosticEventReplayDeferred) == 0 {
		t.Fatal("full sink did not consume attempt")
	}
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Go(func() {
			for j := 0; j < 100; j++ {
				s.setEventDiagnosticOwner(owner)
				s.reportEventDiagnostic(diagnosticEventPublicationDeferred)
				s.setEventDiagnosticOwner(protocol.Sandbox{})
			}
		})
	}
	workers.Wait()
	if len(sink.queue) != processDiagnosticQueueSize {
		t.Fatal("event pressure changed bounded queue")
	}
}
