package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const maxSnapshotBytes = 16 << 20
const maxJournalBytes = 64 << 20
const maxJournalEntryBytes = 1 << 20

type JournalEntry struct {
	Sequence       uint64            `json:"sequence"`
	At             time.Time         `json:"at"`
	OperationID    string            `json:"operation_id"`
	IdempotencyKey string            `json:"idempotency_key"`
	Fingerprint    string            `json:"fingerprint"`
	SandboxID      string            `json:"sandbox_id"`
	Generation     string            `json:"generation,omitempty"`
	Method         string            `json:"method"`
	Phase          string            `json:"phase"`
	State          string            `json:"state,omitempty"`
	Error          *protocol.Error   `json:"error,omitempty"`
	Sandbox        *protocol.Sandbox `json:"sandbox,omitempty"`
}
type IdempotentResult struct {
	Fingerprint string                  `json:"fingerprint"`
	Result      protocol.MutationResult `json:"result"`
	Error       *protocol.Error         `json:"error,omitempty"`
}
type Snapshot struct {
	Version   int                         `json:"version"`
	Sequence  uint64                      `json:"sequence"`
	Sandboxes map[string]protocol.Sandbox `json:"sandboxes"`
	Results   map[string]IdempotentResult `json:"results"`
}
type Store struct {
	mu              sync.Mutex
	dir             string
	dirIdentity     safefile.Identity
	journal         *os.File
	journalIdentity safefile.Identity
	journalFailed   bool
	data            Snapshot
	syncDir         func(string) error
}

func (s *Store) JournalEntries() ([]JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journalFailed {
		return nil, ErrClosed
	}
	if err := s.validateJournalPath(); err != nil {
		return nil, err
	}
	data, identity, err := safefile.ReadOpened(s.journal, maxJournalBytes)
	if err != nil || !safefile.SameObject(identity, s.journalIdentity) {
		return nil, errors.New("journal identity changed while reading")
	}
	return parseJournal(data)
}

func Open(dir string) (*Store, error) {
	directory, err := safefile.OpenDirectory(dir, true)
	if err != nil {
		return nil, fmt.Errorf("open lifecycle state directory: %w", err)
	}
	defer directory.Close()
	s := &Store{dir: dir, dirIdentity: directory.Identity(),
		data:    Snapshot{Version: 1, Sandboxes: map[string]protocol.Sandbox{}, Results: map[string]IdempotentResult{}},
		syncDir: func(string) error { return nil }}
	if data, found, readErr := directory.ReadPrivate("state.json", maxSnapshotBytes); readErr != nil {
		return nil, fmt.Errorf("read lifecycle snapshot: %w", readErr)
	} else if found {
		if err = protocol.StrictDecode(data, &s.data); err != nil {
			return nil, fmt.Errorf("decode lifecycle snapshot: %w", err)
		}
		if s.data.Version != 1 || s.data.Sandboxes == nil || s.data.Results == nil {
			return nil, errors.New("lifecycle snapshot is malformed or unsupported")
		}
	}
	s.journal, s.journalIdentity, err = directory.OpenAppend("journal.jsonl", maxJournalBytes, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lifecycle journal: %w", err)
	}
	data, identity, err := safefile.ReadOpened(s.journal, maxJournalBytes)
	if err != nil || !safefile.SameObject(identity, s.journalIdentity) {
		_ = s.journal.Close()
		return nil, errors.New("lifecycle journal identity changed while opening")
	}
	entries, err := parseJournal(data)
	if err != nil {
		_ = s.journal.Close()
		return nil, fmt.Errorf("decode lifecycle journal: %w", err)
	}
	if len(entries) == 0 && s.data.Sequence != 0 || len(entries) != 0 && s.data.Sequence > entries[len(entries)-1].Sequence {
		_ = s.journal.Close()
		return nil, errors.New("lifecycle snapshot sequence is ahead of its journal")
	}
	for _, entry := range entries {
		if entry.Sequence > s.data.Sequence {
			s.data.Sequence = entry.Sequence
		}
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journalFailed = true
	return s.journal.Close()
}
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.data)
	var d Snapshot
	json.Unmarshal(b, &d)
	return d
}
func (s *Store) Append(e JournalEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journalFailed {
		return ErrClosed
	}
	if err := s.validateJournalPath(); err != nil {
		s.journalFailed = true
		return err
	}
	identity, err := safefile.InspectOpened(s.journal, maxJournalBytes)
	if err != nil || !safefile.SameObject(identity, s.journalIdentity) {
		s.journalFailed = true
		return errors.New("journal identity changed before append")
	}
	e.Sequence = s.data.Sequence + 1
	e.At = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if len(b) > maxJournalEntryBytes || identity.Size > maxJournalBytes-int64(len(b)) {
		return errors.New("lifecycle journal capacity exceeded")
	}
	written, err := s.journal.Write(b)
	if err != nil || written != len(b) {
		s.journalFailed = true
		return errors.Join(err, io.ErrShortWrite)
	}
	if err = s.journal.Sync(); err != nil {
		s.journalFailed = true
		return err
	}
	s.data.Sequence = e.Sequence
	return nil
}
func (s *Store) Commit(sb protocol.Sandbox, key, fingerprint string, result protocol.MutationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousSandbox, hadSandbox := s.data.Sandboxes[sb.ID]
	previousResult, hadResult := s.data.Results[key]
	if sb.State == "ABSENT" {
		delete(s.data.Sandboxes, sb.ID)
	} else {
		s.data.Sandboxes[sb.ID] = sb
	}
	if key != "" {
		s.data.Results[key] = IdempotentResult{Fingerprint: fingerprint, Result: result}
	}
	if err := s.persistLocked(); err != nil {
		if hadSandbox {
			s.data.Sandboxes[sb.ID] = previousSandbox
		} else {
			delete(s.data.Sandboxes, sb.ID)
		}
		if key != "" {
			if hadResult {
				s.data.Results[key] = previousResult
			} else {
				delete(s.data.Results, key)
			}
		}
		return err
	}
	return nil
}

// Tombstone records terminal failure replay while retaining sandbox ownership
// for a cancellation worker that has not yet proven external cleanup.
func (s *Store) Tombstone(key, fingerprint string, failure *protocol.Error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data.Results[key]
	s.data.Results[key] = IdempotentResult{Fingerprint: fingerprint, Error: failure}
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Results[key] = previous
		} else {
			delete(s.data.Results, key)
		}
		return err
	}
	return nil
}

// AbortCreate atomically removes create-only sandbox ownership and records a
// durable error replay for the original idempotency key. A delayed caller can
// therefore never recreate an allocation after cancellation was acknowledged.
func (s *Store) AbortCreate(id, key, fingerprint string, failure *protocol.Error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previousSandbox, hadSandbox := s.data.Sandboxes[id]
	previousResult, hadResult := s.data.Results[key]
	delete(s.data.Sandboxes, id)
	s.data.Results[key] = IdempotentResult{Fingerprint: fingerprint, Error: failure}
	if err := s.persistLocked(); err != nil {
		if hadSandbox {
			s.data.Sandboxes[id] = previousSandbox
		} else {
			delete(s.data.Sandboxes, id)
		}
		if hadResult {
			s.data.Results[key] = previousResult
		} else {
			delete(s.data.Results, key)
		}
		return err
	}
	return nil
}
func (s *Store) SetSandbox(sb protocol.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data.Sandboxes[sb.ID]
	s.data.Sandboxes[sb.ID] = sb
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Sandboxes[sb.ID] = previous
		} else {
			delete(s.data.Sandboxes, sb.ID)
		}
		return err
	}
	return nil
}

func (s *Store) RemoveSandbox(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data.Sandboxes[id]
	delete(s.data.Sandboxes, id)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Sandboxes[id] = previous
		}
		return err
	}
	return nil
}
func (s *Store) Result(key string) (IdempotentResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.data.Results[key]
	return r, ok
}
func (s *Store) Sandbox(id string) (protocol.Sandbox, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Sandboxes[id]
	return v, ok
}

func (s *Store) openDirectory() (*safefile.Directory, error) {
	directory, err := safefile.OpenDirectory(s.dir, false)
	if err != nil {
		return nil, err
	}
	if directory.Identity() != s.dirIdentity {
		_ = directory.Close()
		return nil, errors.New("lifecycle state directory identity changed")
	}
	return directory, nil
}

func (s *Store) validateJournalPath() error {
	directory, err := s.openDirectory()
	if err != nil {
		return err
	}
	defer directory.Close()
	identity, found, err := directory.InspectPrivate("journal.jsonl", maxJournalBytes)
	if err != nil || !found || !safefile.SameObject(identity, s.journalIdentity) {
		return errors.New("lifecycle journal pathname identity changed")
	}
	return nil
}

func (s *Store) persistLocked() error {
	b, e := json.MarshalIndent(s.data, "", "  ")
	if e != nil {
		return e
	}
	if len(b)+1 > maxSnapshotBytes {
		return errors.New("lifecycle snapshot capacity exceeded")
	}
	directory, e := s.openDirectory()
	if e != nil {
		return e
	}
	defer directory.Close()
	if e = directory.Replace("state.json", append(b, '\n'), 0600); e != nil {
		return e
	}
	return s.syncDir(s.dir)
}

func ReadJournal(dir string) ([]JournalEntry, error) {
	directory, err := safefile.OpenDirectory(dir, false)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	data, found, err := directory.ReadPrivate("journal.jsonl", maxJournalBytes)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, os.ErrNotExist
	}
	return parseJournal(data)
}

func parseJournal(data []byte) ([]JournalEntry, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, errors.New("lifecycle journal has a truncated final entry")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	entries := make([]JournalEntry, 0, len(lines))
	var previous uint64
	for index, line := range lines {
		if len(line) == 0 || len(line) > maxJournalEntryBytes {
			return nil, fmt.Errorf("lifecycle journal entry %d is empty or oversized", index+1)
		}
		var entry JournalEntry
		if err := protocol.StrictDecode(line, &entry); err != nil {
			return nil, fmt.Errorf("lifecycle journal entry %d: %w", index+1, err)
		}
		if entry.Sequence != previous+1 {
			return nil, fmt.Errorf("lifecycle journal entry %d has a non-contiguous sequence", index+1)
		}
		previous = entry.Sequence
		entries = append(entries, entry)
	}
	return entries, nil
}
func Incomplete(es []JournalEntry) []JournalEntry {
	done := map[string]bool{}
	for _, e := range es {
		if e.Phase == "complete" {
			done[e.OperationID] = true
		}
	}
	var r []JournalEntry
	for _, e := range es {
		if e.Phase == "intent" && !done[e.OperationID] {
			r = append(r, e)
		}
	}
	return r
}

var ErrClosed = errors.New("store closed")
