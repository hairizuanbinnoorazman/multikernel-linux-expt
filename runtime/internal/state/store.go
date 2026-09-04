package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

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
}
type Snapshot struct {
	Version   int                         `json:"version"`
	Sequence  uint64                      `json:"sequence"`
	Sandboxes map[string]protocol.Sandbox `json:"sandboxes"`
	Results   map[string]IdempotentResult `json:"results"`
}
type Store struct {
	mu      sync.Mutex
	dir     string
	journal *os.File
	data    Snapshot
	syncDir func(string) error
}

func (s *Store) JournalEntries() ([]JournalEntry, error) { return ReadJournal(s.dir) }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, data: Snapshot{Version: 1, Sandboxes: map[string]protocol.Sandbox{}, Results: map[string]IdempotentResult{}}, syncDir: syncDirectory}
	if b, e := os.ReadFile(filepath.Join(dir, "state.json")); e == nil {
		if e = json.Unmarshal(b, &s.data); e != nil {
			return nil, fmt.Errorf("decode state: %w", e)
		}
	}
	f, e := os.OpenFile(filepath.Join(dir, "journal.jsonl"), os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	s.journal = f
	if entries, readErr := ReadJournal(dir); readErr == nil {
		for _, entry := range entries {
			if entry.Sequence > s.data.Sequence {
				s.data.Sequence = entry.Sequence
			}
		}
	}
	return s, nil
}
func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.journal.Close() }
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
	s.data.Sequence++
	e.Sequence = s.data.Sequence
	e.At = time.Now().UTC()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = s.journal.Write(append(b, '\n')); err != nil {
		return err
	}
	return s.journal.Sync()
}
func (s *Store) Commit(sb protocol.Sandbox, key, fingerprint string, result protocol.MutationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sb.State == "ABSENT" {
		delete(s.data.Sandboxes, sb.ID)
	} else {
		s.data.Sandboxes[sb.ID] = sb
	}
	if key != "" {
		s.data.Results[key] = IdempotentResult{fingerprint, result}
	}
	return s.persistLocked()
}
func (s *Store) SetSandbox(sb protocol.Sandbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Sandboxes[sb.ID] = sb
	return s.persistLocked()
}

func (s *Store) RemoveSandbox(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data.Sandboxes, id)
	return s.persistLocked()
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
func (s *Store) persistLocked() error {
	b, e := json.MarshalIndent(s.data, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.dir, "state.json.tmp")
	if e = os.WriteFile(tmp, append(b, '\n'), 0600); e != nil {
		return e
	}
	f, e := os.Open(tmp)
	if e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	f.Close()
	if e = os.Rename(tmp, filepath.Join(s.dir, "state.json")); e != nil {
		return e
	}
	return s.syncDir(s.dir)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func ReadJournal(dir string) ([]JournalEntry, error) {
	f, e := os.Open(filepath.Join(dir, "journal.jsonl"))
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out []JournalEntry
	s := bufio.NewScanner(f)
	for s.Scan() {
		var x JournalEntry
		if e = json.Unmarshal(s.Bytes(), &x); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, s.Err()
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
