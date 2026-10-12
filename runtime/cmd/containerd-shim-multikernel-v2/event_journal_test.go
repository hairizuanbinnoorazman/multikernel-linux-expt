//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	ctruntime "github.com/containerd/containerd/runtime"
)

func TestEventJournalPreservesOrderAndReplaysAfterFailure(t *testing.T) {
	bundle := privateTestDirectory(t)
	failed := &fakePublisher{failures: 2}
	s := &service{bundle: bundle, namespace: "default", publisher: failed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	if len(failed.topics) != 0 {
		t.Fatalf("later event bypassed failed predecessor: %v", failed.topics)
	}
	replayed := &fakePublisher{}
	recovered := &service{bundle: bundle, namespace: "default", publisher: replayed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := recovered.loadEventJournal(); err != nil {
		t.Fatal(err)
	}
	if err := recovered.flushEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskCreateEventTopic, ctruntime.TaskStartEventTopic}
	if fmt.Sprint(replayed.topics) != fmt.Sprint(want) {
		t.Fatalf("replayed topics = %v, want %v", replayed.topics, want)
	}
	if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged journal remains: %v", err)
	}
}

func TestEventJournalCompleteLifecycleOrderAndPersistenceFailureMatrix(t *testing.T) {
	items := []struct {
		name  string
		topic string
		event any
	}{
		{"create", ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task", Pid: 7}},
		{"start", ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}},
		{"exec-added", ctruntime.TaskExecAddedEventTopic, &eventstypes.TaskExecAdded{ContainerID: "task", ExecID: "exec"}},
		{"exec-started", ctruntime.TaskExecStartedEventTopic, &eventstypes.TaskExecStarted{ContainerID: "task", ExecID: "exec", Pid: 8}},
		{"exit", ctruntime.TaskExitEventTopic, &eventstypes.TaskExit{ContainerID: "task", ID: "exec", Pid: 8, ExitStatus: 17}},
		{"delete", ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: "task", ID: "exec", Pid: 8, ExitStatus: 17}},
	}

	// Each queue attempt reaches a disconnected broker, so no later event can
	// bypass its durable predecessor. Reconstruction must replay all six in the
	// original sequence and remove the fully acknowledged journal.
	bundle := privateTestDirectory(t)
	disconnected := &fakePublisher{failures: len(items)}
	s := &service{bundle: bundle, namespace: "default", publisher: disconnected,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	for _, item := range items {
		if err := s.publish(t.Context(), item.topic, item.event); err != nil {
			t.Fatalf("queue %s: %v", item.name, err)
		}
	}
	if len(disconnected.topics) != 0 || len(s.events.Pending) != len(items) || s.events.NextSequence != uint64(len(items)+1) {
		t.Fatalf("disconnected journal: published=%v events=%+v", disconnected.topics, s.events)
	}
	for index, pending := range s.events.Pending {
		if pending.Sequence != uint64(index+1) || pending.Topic != items[index].topic {
			t.Fatalf("pending[%d] = sequence %d topic %q", index, pending.Sequence, pending.Topic)
		}
	}

	replayed := &fakePublisher{}
	recovered := &service{bundle: bundle, namespace: "default", publisher: replayed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := recovered.loadEventJournal(); err != nil {
		t.Fatal(err)
	}
	if err := recovered.flushEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
	wantTopics := make([]string, 0, len(items))
	for _, item := range items {
		wantTopics = append(wantTopics, item.topic)
	}
	if fmt.Sprint(replayed.topics) != fmt.Sprint(wantTopics) {
		t.Fatalf("replayed topics = %v, want %v", replayed.topics, wantTopics)
	}
	if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged journal remains: %v", err)
	}

	// Persistence is the pre-publication boundary for every supported lifecycle
	// event. A failure must publish nothing and roll back both queue and sequence.
	for _, item := range items {
		t.Run(item.name+"-persistence", func(t *testing.T) {
			originalMarshal := jsonMarshal
			jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected journal failure") }
			t.Cleanup(func() { jsonMarshal = originalMarshal })
			publisher := &fakePublisher{}
			candidate := &service{bundle: privateTestDirectory(t), namespace: "default", publisher: publisher,
				events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := candidate.publish(t.Context(), item.topic, item.event); err == nil {
				t.Fatal("journal failure was ignored")
			}
			if len(publisher.topics) != 0 || len(candidate.events.Pending) != 0 || candidate.events.NextSequence != 1 {
				t.Fatalf("failed persistence changed state: topics=%v events=%+v", publisher.topics, candidate.events)
			}
		})
	}
}

func TestEventJournalRefusesReplacedPathForWriteAndAcknowledgement(t *testing.T) {
	publisher := &fakePublisher{failures: 1}
	s := &service{bundle: privateTestDirectory(t), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(t.Context(), ctruntime.TaskCreateEventTopic,
		&eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	path := s.eventJournalPath()
	if err := os.Rename(path, path+".owned"); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("caller replacement")
	if err := os.WriteFile(path, replacement, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(t.Context(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err == nil {
		t.Fatal("event journal write replaced an unowned inode")
	}
	if len(s.events.Pending) != 1 || s.events.NextSequence != 2 {
		t.Fatalf("failed replacement write changed event ownership: %+v", s.events)
	}
	if err := s.flushEvents(t.Context()); err == nil {
		t.Fatal("event acknowledgement removed an unowned inode")
	}
	if len(s.events.Pending) != 1 || len(publisher.topics) != 1 {
		t.Fatalf("failed acknowledgement lost replay state: pending=%+v topics=%v", s.events.Pending, publisher.topics)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, replacement) {
		t.Fatalf("replacement journal changed: %q, %v", data, err)
	}
}

func TestEventJournalRejectsPublicBundleReplacement(t *testing.T) {
	base := privateTestDirectory(t)
	bundle := filepath.Join(base, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{failures: 2}
	s := &service{bundle: bundle, namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(t.Context(), ctruntime.TaskCreateEventTopic,
		&eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(base, "bundle-held")
	if err := os.Rename(bundle, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(t.Context(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err == nil || !strings.Contains(err.Error(), "bundle directory identity changed") {
		t.Fatalf("event publication after bundle replacement = %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundle, ".multikernel-events.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement bundle received event journal: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(moved, ".multikernel-events.json"))
	if err != nil || !bytes.Contains(data, []byte(ctruntime.TaskCreateEventTopic)) {
		t.Fatalf("held bundle journal = %q, %v", data, err)
	}
}

func TestEventJournalLockWaitHonorsCancellationWithoutMutation(t *testing.T) {
	s := &service{id: "task", namespace: "default", bundle: privateTestDirectory(t), publisher: &fakePublisher{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	s.eventMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- s.publish(ctx, ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 41})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.eventMu.Unlock()
			t.Fatalf("contended publication error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.eventMu.Unlock()
		t.Fatal("contended event publication ignored cancellation")
	}
	if len(s.events.Pending) != 0 || s.events.NextSequence != 1 {
		s.eventMu.Unlock()
		t.Fatalf("cancelled publication mutated journal: %+v", s.events)
	}
	s.eventMu.Unlock()

	s.eventMu.Lock()
	ctx, cancel = context.WithCancel(context.Background())
	result = make(chan error, 1)
	go func() { result <- s.flushEvents(ctx) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.eventMu.Unlock()
			t.Fatalf("contended flush error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		s.eventMu.Unlock()
		t.Fatal("contended event flush ignored cancellation")
	}
	s.eventMu.Unlock()
}

func TestEventJournalRetriesWithoutAnotherLifecycleRequest(t *testing.T) {
	publisher := &retryPublisher{published: make(chan string, 1)}
	s := &service{bundle: privateTestDirectory(t), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	s.startEventRetry(5 * time.Millisecond)
	defer s.stopEventRetry()
	select {
	case topic := <-publisher.published:
		if topic != ctruntime.TaskStartEventTopic {
			t.Fatalf("retried topic = %q", topic)
		}
	case <-time.After(time.Second):
		t.Fatal("durable event was not retried while the shim remained running")
	}
	s.stopEventRetry()
	if len(s.events.Pending) != 0 {
		t.Fatalf("acknowledged retry remains pending: %+v", s.events.Pending)
	}
	if _, err := os.Stat(s.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged retry journal remains: %v", err)
	}
}

func TestEventJournalPersistenceFailurePrecedesPublication(t *testing.T) {
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected journal failure") }
	defer func() { jsonMarshal = originalMarshal }()
	publisher := &fakePublisher{}
	s := &service{bundle: privateTestDirectory(t), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err == nil {
		t.Fatal("journal failure was ignored")
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("event published before durable queue: %v", publisher.topics)
	}
	if len(s.events.Pending) != 0 || s.events.NextSequence != 1 {
		t.Fatalf("failed queue mutated memory: %+v", s.events)
	}
}

func TestEventJournalAckFailureRetainsPublishedEventForReplay(t *testing.T) {
	publisher := &fakePublisher{failures: 2}
	s := &service{bundle: privateTestDirectory(t), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	for _, item := range []struct {
		topic string
		event any
	}{{ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}},
		{ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}}} {
		if err := s.publish(context.Background(), item.topic, item.event); err != nil {
			t.Fatal(err)
		}
	}
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected acknowledgement failure") }
	defer func() { jsonMarshal = originalMarshal }()
	if err := s.flushEvents(context.Background()); err == nil {
		t.Fatal("acknowledgement failure was ignored")
	}
	if len(publisher.topics) != 1 || len(s.events.Pending) != 2 || s.events.Pending[0].Sequence != 1 {
		t.Fatalf("at-least-once replay state was lost: topics=%v events=%+v", publisher.topics, s.events)
	}
}

func TestEventJournalRejectsSymlinkAndPermissiveState(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{"symlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
				return err
			}
			return os.Symlink(target, path)
		}},
		{"permissive", func(path string) error {
			return os.WriteFile(path, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0644)
		}},
		{"hardlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
				return err
			}
			return os.Link(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &service{bundle: privateTestDirectory(t), events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := test.make(s.eventJournalPath()); err != nil {
				t.Fatal(err)
			}
			if err := s.loadEventJournal(); err == nil {
				t.Fatal("unsafe event journal accepted")
			}
		})
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		directory := t.TempDir()
		realBundle := filepath.Join(directory, "real")
		if err := os.Mkdir(realBundle, 0700); err != nil {
			t.Fatal(err)
		}
		realJournal := filepath.Join(realBundle, ".multikernel-events.json")
		if err := os.WriteFile(realJournal, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		linkedBundle := filepath.Join(directory, "linked")
		if err := os.Symlink(realBundle, linkedBundle); err != nil {
			t.Fatal(err)
		}
		s := &service{bundle: linkedBundle, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
		if err := s.loadEventJournal(); err == nil {
			t.Fatal("event journal beneath a symlinked ancestor was accepted")
		}
	})
}
