package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	statepkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Artifacts struct{ Kernel, Initrd, Cmdline string }
type Service struct {
	store     *statepkg.Store
	backend   kerf.Backend
	artifacts Artifacts
	global    sync.Mutex
	locks     sync.Map
}

func New(st *statepkg.Store, b kerf.Backend, a Artifacts) *Service {
	return &Service{store: st, backend: b, artifacts: a}
}
func (s *Service) lock(id string) func() {
	v, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}
func generation() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func fingerprint(method string, v any) string {
	b, _ := json.Marshal(struct {
		M string `json:"method"`
		V any    `json:"value"`
	}{method, v})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func apierr(code, msg string, retry bool) *protocol.Error {
	return &protocol.Error{Code: code, Message: msg, Retryable: retry}
}
func (s *Service) replay(key, fp string) (protocol.MutationResult, *protocol.Error, bool) {
	if key == "" {
		return protocol.MutationResult{}, apierr("INVALID_ARGUMENT", "idempotency key is required", false), true
	}
	if old, ok := s.store.Result(key); ok {
		if old.Fingerprint != fp {
			return protocol.MutationResult{}, apierr("IDEMPOTENCY_CONFLICT", "idempotency key has different input", false), true
		}
		r := old.Result
		r.Replayed = true
		return r, nil, true
	}
	return protocol.MutationResult{}, nil, false
}
func (s *Service) mutate(ctx context.Context, method, key, fp string, sb protocol.Sandbox, fn func(*protocol.Sandbox) error) (protocol.MutationResult, *protocol.Error) {
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	op, _ := generation()
	intent := statepkg.JournalEntry{OperationID: op, IdempotencyKey: key, Fingerprint: fp, SandboxID: sb.ID, Generation: sb.Generation, Method: method, Phase: "intent", State: sb.State}
	if e := s.store.Append(intent); e != nil {
		return protocol.MutationResult{}, apierr("INTERNAL", e.Error(), true)
	}
	if e := fn(&sb); e != nil {
		pe := apierr("BACKEND_FAILURE", e.Error(), true)
		intent.Phase = "complete"
		intent.Error = pe
		s.store.Append(intent)
		sb.Error = pe
		sb.UpdatedAt = time.Now().UTC()
		s.store.SetSandbox(sb)
		return protocol.MutationResult{}, pe
	}
	sb.Error = nil
	sb.UpdatedAt = time.Now().UTC()
	result := protocol.MutationResult{Sandbox: sb}
	if e := s.store.Commit(sb, key, fp, result); e != nil {
		return protocol.MutationResult{}, apierr("INTERNAL", e.Error(), true)
	}
	intent.Phase = "complete"
	intent.State = sb.State
	if e := s.store.Append(intent); e != nil {
		return protocol.MutationResult{}, apierr("INTERNAL", e.Error(), true)
	}
	return result, nil
}

func validateConfig(c protocol.SandboxConfig) error {
	if c.SchemaVersion != 1 {
		return errors.New("schema_version must be 1")
	}
	if !idRE.MatchString(c.ID) {
		return errors.New("invalid sandbox ID")
	}
	if len(c.CPUs) == 0 {
		return errors.New("at least one CPU is required")
	}
	seen := map[int]bool{}
	for _, n := range c.CPUs {
		if n == 0 {
			return errors.New("APIC ID 0 is forbidden")
		}
		if n < 0 || seen[n] {
			return errors.New("invalid or duplicate CPU")
		}
		seen[n] = true
	}
	if c.MemoryBytes < 512<<20 {
		return errors.New("memory must be at least 512 MiB")
	}
	if c.AgentPort < 1024 {
		return errors.New("agent port must be at least 1024")
	}
	if c.ChildCID == 0 || c.ChildCID == ^uint32(0) {
		return errors.New("child CID must be a concrete nonzero value")
	}
	return nil
}
func overlap(a, b protocol.SandboxConfig) bool {
	m := map[int]bool{}
	for _, n := range a.CPUs {
		m[n] = true
	}
	for _, n := range b.CPUs {
		if m[n] {
			return true
		}
	}
	return a.AgentPort == b.AgentPort || a.ChildCID == b.ChildCID || a.Bundle == b.Bundle
}

func (s *Service) Create(ctx context.Context, c protocol.SandboxConfig, key string) (protocol.MutationResult, *protocol.Error) {
	if e := validateConfig(c); e != nil {
		return protocol.MutationResult{}, apierr("INVALID_ARGUMENT", e.Error(), false)
	}
	fp := fingerprint("CreateSandbox", c)
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	s.global.Lock()
	defer s.global.Unlock()
	defer s.lock(c.ID)()
	if _, ok := s.store.Sandbox(c.ID); ok {
		return protocol.MutationResult{}, apierr("ALREADY_EXISTS", "sandbox ID exists", false)
	}
	for _, x := range s.store.Snapshot().Sandboxes {
		if overlap(c, x.Config) {
			return protocol.MutationResult{}, apierr("RESOURCE_EXHAUSTED", "CPU, port, or writable bundle overlaps another sandbox", false)
		}
	}
	g, e := generation()
	if e != nil {
		return protocol.MutationResult{}, apierr("INTERNAL", e.Error(), true)
	}
	now := time.Now().UTC()
	sb := protocol.Sandbox{ID: c.ID, Generation: g, State: "ALLOCATING", Config: c, CreatedAt: now, UpdatedAt: now}
	return s.mutate(ctx, "CreateSandbox", key, fp, sb, func(current *protocol.Sandbox) error {
		if len(s.store.Snapshot().Sandboxes) == 0 {
			if e := s.backend.EnsurePool(ctx); e != nil {
				return e
			}
		}
		if e := s.backend.Create(ctx, *current); e != nil {
			return e
		}
		current.State = "CREATED"
		return nil
	})
}
func (s *Service) transition(ctx context.Context, id, gen, key, method string, from []string, to string, fn func(protocol.Sandbox) error) (protocol.MutationResult, *protocol.Error) {
	defer s.lock(id)()
	sb, ok := s.store.Sandbox(id)
	if !ok {
		return protocol.MutationResult{}, apierr("NOT_FOUND", "sandbox not found", false)
	}
	if sb.Generation != gen {
		return protocol.MutationResult{}, apierr("STALE_GENERATION", "generation does not match", false)
	}
	fp := fingerprint(method, struct{ ID, Gen string }{id, gen})
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	if sb.State == to {
		return s.mutate(ctx, method, key, fp, sb, func(*protocol.Sandbox) error { return nil })
	}
	allowed := false
	for _, x := range from {
		if sb.State == x {
			allowed = true
		}
	}
	if !allowed {
		return protocol.MutationResult{}, apierr("FAILED_PRECONDITION", "invalid state "+sb.State, false)
	}
	return s.mutate(ctx, method, key, fp, sb, func(current *protocol.Sandbox) error {
		if e := fn(*current); e != nil {
			return e
		}
		current.State = to
		return nil
	})
}
func (s *Service) Load(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "LoadSandbox", []string{"CREATED"}, "LOADED", func(x protocol.Sandbox) error {
		return s.backend.Load(ctx, x, s.artifacts.Kernel, s.artifacts.Initrd, s.artifacts.Cmdline)
	})
}
func (s *Service) Start(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "StartSandbox", []string{"LOADED", "STOPPED"}, "RUNNING", func(x protocol.Sandbox) error { return s.backend.Start(ctx, x) })
}
func (s *Service) Stop(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "StopSandbox", []string{"RUNNING"}, "STOPPED", func(x protocol.Sandbox) error { return s.backend.Stop(ctx, x) })
}
func (s *Service) Delete(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	s.global.Lock()
	defer s.global.Unlock()
	defer s.lock(id)()
	sb, ok := s.store.Sandbox(id)
	if !ok {
		return protocol.MutationResult{}, apierr("NOT_FOUND", "sandbox not found", false)
	}
	if sb.Generation != gen {
		return protocol.MutationResult{}, apierr("STALE_GENERATION", "generation does not match", false)
	}
	fp := fingerprint("DeleteSandbox", struct{ ID, Gen string }{id, gen})
	return s.mutate(ctx, "DeleteSandbox", key, fp, sb, func(current *protocol.Sandbox) error {
		if current.State == "RUNNING" {
			if e := s.backend.Stop(ctx, *current); e != nil {
				return e
			}
		}
		if e := s.backend.Delete(ctx, *current); e != nil {
			return e
		}
		current.State = "ABSENT"
		if len(s.store.Snapshot().Sandboxes) == 1 {
			return s.backend.ReleasePool(ctx)
		}
		return nil
	})
}
func (s *Service) Get(id string) (protocol.Sandbox, bool) { return s.store.Sandbox(id) }
func (s *Service) List() []protocol.Sandbox {
	d := s.store.Snapshot()
	r := make([]protocol.Sandbox, 0, len(d.Sandboxes))
	for _, x := range d.Sandboxes {
		r = append(r, x)
	}
	sort.Slice(r, func(i, j int) bool { return r[i].ID < r[j].ID })
	return r
}
func (s *Service) Reconcile(ctx context.Context) error {
	es, e := s.store.JournalEntries()
	if e != nil {
		return e
	}
	incomplete := statepkg.Incomplete(es)
	pending := map[string]bool{}
	for _, x := range incomplete {
		pending[x.SandboxID] = true
	}
	for _, sb := range s.List() {
		actual, e := s.backend.Observe(ctx, sb.ID)
		if e != nil {
			return e
		}
		if actual != sb.State || pending[sb.ID] {
			sb.Error = apierr("OPERATOR_ACTION", fmt.Sprintf("journal=%s backend=%s incomplete=%t", sb.State, actual, pending[sb.ID]), false)
			if e = s.store.SetSandbox(sb); e != nil {
				return e
			}
		}
	}
	return nil
}
