//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/namespaces"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/typeurl/v2"
	"golang.org/x/sys/unix"
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
	path := s.eventJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if s.events.NextSequence == 0 {
			s.events = eventJournal{SchemaVersion: 1, NextSequence: 1}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect event journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 8<<20 {
		return errors.New("event journal must be a private bounded regular file")
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open event journal: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("event journal identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		return errors.New("event journal changed or exceeded its read bound")
	}
	var value eventJournal
	if err = protocol.StrictDecode(data, &value); err != nil {
		return fmt.Errorf("decode event journal: %w", err)
	}
	if err = validateEventJournal(value); err != nil {
		return err
	}
	s.events = value
	return nil
}

func (s *service) persistEventJournalLocked() error {
	if err := validateEventJournal(s.events); err != nil {
		return err
	}
	path := s.eventJournalPath()
	if len(s.events.Pending) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		directory, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		err = directory.Sync()
		return errors.Join(err, directory.Close())
	}
	data, err := jsonMarshal(s.events)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0600)
}

// jsonMarshal is a variable only to make the pre-publication persistence
// failure boundary deterministic in focused tests.
var jsonMarshal = func(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (s *service) flushEvents(ctx context.Context) error {
	s.eventMu.Lock()
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
	encoded, err := typeurl.MarshalAny(event)
	if err != nil {
		return err
	}
	if !eventMatchesTopic(topic, event) {
		return errors.New("event topic and payload type differ")
	}
	s.eventMu.Lock()
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
