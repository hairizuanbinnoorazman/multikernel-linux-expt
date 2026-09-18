//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/namespaces"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const maxPendingEvents = 4096

var supportedEventTopics = map[string]bool{
	ctruntime.TaskCreateEventTopic: true, ctruntime.TaskStartEventTopic: true,
	ctruntime.TaskExecAddedEventTopic: true, ctruntime.TaskExecStartedEventTopic: true,
	ctruntime.TaskExitEventTopic: true, ctruntime.TaskDeleteEventTopic: true,
	ctruntime.TaskPausedEventTopic: true, ctruntime.TaskResumedEventTopic: true,
}

func eventMatchesTopic(topic string, event any) bool {
	switch topic {
	case ctruntime.TaskCreateEventTopic:
		_, ok := event.(*eventstypes.TaskCreate)
		return ok
	case ctruntime.TaskStartEventTopic:
		_, ok := event.(*eventstypes.TaskStart)
		return ok
	case ctruntime.TaskExecAddedEventTopic:
		_, ok := event.(*eventstypes.TaskExecAdded)
		return ok
	case ctruntime.TaskExecStartedEventTopic:
		_, ok := event.(*eventstypes.TaskExecStarted)
		return ok
	case ctruntime.TaskExitEventTopic:
		_, ok := event.(*eventstypes.TaskExit)
		return ok
	case ctruntime.TaskDeleteEventTopic:
		_, ok := event.(*eventstypes.TaskDelete)
		return ok
	case ctruntime.TaskPausedEventTopic:
		_, ok := event.(*eventstypes.TaskPaused)
		return ok
	case ctruntime.TaskResumedEventTopic:
		_, ok := event.(*eventstypes.TaskResumed)
		return ok
	default:
		return false
	}
}

type persistedEvent struct {
	Sequence uint64 `json:"sequence"`
	Topic    string `json:"topic"`
	TypeURL  string `json:"type_url"`
	Value    []byte `json:"value"`
}

type eventJournal struct {
	SchemaVersion int              `json:"schema_version"`
	NextSequence  uint64           `json:"next_sequence"`
	Pending       []persistedEvent `json:"pending"`
}

func (s *service) eventJournalPath() string {
	return filepath.Join(s.bundle, ".multikernel-events.json")
}

func validateEventJournal(value eventJournal) error {
	if value.SchemaVersion != 1 || value.NextSequence == 0 || len(value.Pending) > maxPendingEvents {
		return errors.New("event journal identity or bounds are invalid")
	}
	var previous uint64
	for _, event := range value.Pending {
		if event.Sequence == 0 || event.Sequence >= value.NextSequence || event.Sequence <= previous ||
			!supportedEventTopics[event.Topic] ||
			event.TypeURL == "" || len(event.TypeURL) > 256 || len(event.Value) > 1<<20 {
			return errors.New("event journal contains an invalid pending event")
		}
		decoded, err := typeurl.UnmarshalAny(&anypb.Any{TypeUrl: event.TypeURL, Value: event.Value})
		if err != nil {
			return fmt.Errorf("event journal contains an undecodable event: %w", err)
		}
		if !eventMatchesTopic(event.Topic, decoded) {
			return errors.New("event journal topic and payload type differ")
		}
		previous = event.Sequence
	}
	return nil
}

func (s *service) loadEventJournal() error {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	if err := s.ensureBundleIdentity(); err != nil {
		return fmt.Errorf("verify event journal bundle identity: %w", err)
	}
	data, present, loadedIdentity, err := s.bundleDirectory.ReadPrivateIdentity(".multikernel-events.json", 8<<20)
	if err != nil {
		return fmt.Errorf("read event journal: %w", err)
	}
	if !present {
		s.eventJournalIdentity = nil
		if s.events.NextSequence == 0 {
			s.events = eventJournal{SchemaVersion: 1, NextSequence: 1}
		}
		return nil
	}
	if os.FileMode(loadedIdentity.Mode).Perm() != 0600 || len(data) == 0 {
		return errors.New("event journal must have exact private mode and nonempty bounded contents")
	}
	var value eventJournal
	if err = protocol.StrictDecode(data, &value); err != nil {
		return fmt.Errorf("decode event journal: %w", err)
	}
	if err = validateEventJournal(value); err != nil {
		return err
	}
	s.events = value
	s.eventJournalIdentity = &loadedIdentity
	return nil
}

func (s *service) persistEventJournalLocked() error {
	if err := validateEventJournal(s.events); err != nil {
		return err
	}
	if err := s.ensureBundleIdentity(); err != nil {
		return fmt.Errorf("verify event journal bundle identity: %w", err)
	}
	if len(s.events.Pending) == 0 {
		if s.eventJournalIdentity == nil {
			if _, present, err := s.bundleDirectory.EntryIdentity(".multikernel-events.json"); err != nil || present {
				return errors.Join(errors.New("refusing to remove an unowned event journal"), err)
			}
			return nil
		}
		removed, err := s.bundleDirectory.RemoveIfIdentity(".multikernel-events.json", *s.eventJournalIdentity)
		if err != nil || !removed {
			return errors.Join(errors.New("owned event journal disappeared or changed before removal"), err)
		}
		s.eventJournalIdentity = nil
		return nil
	}
	data, err := jsonMarshal(s.events)
	if err != nil {
		return err
	}
	published, err := s.bundleDirectory.ReplaceIdentity(".multikernel-events.json", data, 0600, s.eventJournalIdentity)
	if err != nil {
		return err
	}
	s.eventJournalIdentity = &published
	return nil
}

// jsonMarshal is a variable only to make the pre-publication persistence
// failure boundary deterministic in focused tests.
var jsonMarshal = func(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (s *service) flushEvents(ctx context.Context) error {
	if err := lockContext(ctx, &s.eventMu); err != nil {
		return err
	}
	defer s.eventMu.Unlock()
	for len(s.events.Pending) != 0 {
		next := s.events.Pending[0]
		decoded, err := typeurl.UnmarshalAny(&anypb.Any{TypeUrl: next.TypeURL, Value: next.Value})
		if err != nil {
			return err
		}
		if err = s.publisher.Publish(namespaces.WithNamespace(ctx, s.namespace), next.Topic, decoded); err != nil {
			return err
		}
		s.events.Pending = append([]persistedEvent(nil), s.events.Pending[1:]...)
		if err = s.persistEventJournalLocked(); err != nil {
			s.events.Pending = append([]persistedEvent{next}, s.events.Pending...)
			return fmt.Errorf("acknowledge published event %d: %w", next.Sequence, err)
		}
	}
	return nil
}

func (s *service) publish(ctx context.Context, topic string, event any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded, err := typeurl.MarshalAny(event)
	if err != nil {
		return err
	}
	if !eventMatchesTopic(topic, event) {
		return errors.New("event topic and payload type differ")
	}
	if err = lockContext(ctx, &s.eventMu); err != nil {
		return err
	}
	if len(s.events.Pending) >= maxPendingEvents {
		s.eventMu.Unlock()
		return errors.New("event journal pending limit reached")
	}
	if s.events.NextSequence == 0 {
		s.events = eventJournal{SchemaVersion: 1, NextSequence: 1}
	}
	s.events.Pending = append(s.events.Pending, persistedEvent{Sequence: s.events.NextSequence, Topic: topic,
		TypeURL: encoded.GetTypeUrl(), Value: append([]byte(nil), encoded.GetValue()...)})
	s.events.NextSequence++
	err = s.persistEventJournalLocked()
	if err != nil {
		s.events.Pending = s.events.Pending[:len(s.events.Pending)-1]
		s.events.NextSequence--
	}
	s.eventMu.Unlock()
	if err != nil {
		return fmt.Errorf("persist event before publication: %w", err)
	}
	// Once queued durably, a transient broker error must not roll back the
	// lifecycle mutation. The next event or worker reconstruction retries the
	// ordered journal. Remote acceptance followed by a crash before local ack
	// can replay a duplicate; consumers must treat the Task event tuple as
	// idempotent.
	if err = s.flushEvents(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "multikernel event publication deferred: %v\n", err)
	}
	return nil
}

// startEventRetry guarantees that a transiently disconnected containerd does
// not require another lifecycle request or a shim restart to receive durable
// events. Each attempt is bounded; sequence serialization remains in
// flushEvents, and cancellation is joined before the shim exits.
func (s *service) startEventRetry(interval time.Duration) {
	if interval <= 0 || s.eventRetryCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.eventRetryCancel = cancel
	s.eventRetryDone = make(chan struct{})
	go func() {
		defer close(s.eventRetryDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, attemptCancel := context.WithTimeout(ctx, 5*time.Second)
				_ = s.flushEvents(attempt)
				attemptCancel()
			}
		}
	}()
}

func (s *service) stopEventRetry() {
	s.eventRetryStop.Do(func() {
		if s.eventRetryCancel == nil {
			return
		}
		s.eventRetryCancel()
		<-s.eventRetryDone
	})
}
