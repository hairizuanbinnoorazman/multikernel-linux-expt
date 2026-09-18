package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	statepkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	storagepkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var manifestRE = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,62}$`)
var labelKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,62}$`)

type Artifacts struct {
	Kernel, Initrd, Cmdline string
	KernelFile, InitrdFile  *os.File
}

func (a Artifacts) Close() error {
	var closeErrors []error
	if a.KernelFile != nil {
		closeErrors = append(closeErrors, a.KernelFile.Close())
	}
	if a.InitrdFile != nil {
		closeErrors = append(closeErrors, a.InitrdFile.Close())
	}
	return errors.Join(closeErrors...)
}

type ArtifactResolver interface {
	Resolve(string) (Artifacts, error)
}
type PreparedBootResolver interface {
	OpenPreparedBoot(context.Context, string, *protocol.StorageConfig) (*os.File, *os.File, error)
}
type Service struct {
	store             *statepkg.Store
	backend           kerf.Backend
	artifacts         Artifacts
	resolver          ArtifactResolver
	poolCPUs          map[int]bool
	poolMemoryBytes   uint64
	poolMemoryReserve uint64
	storage           *storagepkg.Service
	preparedBoot      PreparedBootResolver
	global            sync.Mutex
	locks             sync.Map
	fault             func(string) error
}

func (s *Service) SetArtifactResolver(resolver ArtifactResolver) { s.resolver = resolver }
func (s *Service) SetStorage(service *storagepkg.Service)        { s.storage = service }
func (s *Service) SetPreparedBootResolver(resolver PreparedBootResolver) {
	s.preparedBoot = resolver
}
func (s *Service) SetFaultInjector(injector func(string) error) { s.fault = injector }
func (s *Service) checkpoint(point string) error {
	if s.fault == nil {
		return nil
	}
	return s.fault(point)
}
func (s *Service) artifactsFor(manifest string) (Artifacts, error) {
	artifacts := s.artifacts
	if s.resolver == nil {
		return artifacts, nil
	}
	resolved, err := s.resolver.Resolve(manifest)
	if err != nil {
		return Artifacts{}, err
	}
	resolved.Cmdline = artifacts.Cmdline
	return resolved, nil
}

func New(st *statepkg.Store, b kerf.Backend, a Artifacts) *Service {
	return &Service{store: st, backend: b, artifacts: a}
}

func preparedStorage(value *protocol.StorageConfig) storagepkg.PreparedImage {
	return storagepkg.PreparedImage{Path: value.Path, ImageID: value.ImageID, FilesystemUUID: value.FilesystemUUID,
		SizeBytes: value.SizeBytes, QuotaBytes: value.QuotaBytes, InodeLimit: value.InodeLimit, Port: value.Port, SHA256: value.SHA256}
}

func storageStatus(value storagepkg.Export) *protocol.StorageStatus {
	return &protocol.StorageStatus{ExportGeneration: value.ExportGeneration, State: value.State,
		OfflineCheck: value.OfflineCheck, Reads: value.Counters.Reads, ReadBytes: value.Counters.ReadBytes,
		Writes: value.Counters.Writes, WrittenBytes: value.Counters.WrittenBytes, Flushes: value.Counters.Flushes,
		ReleasedAt: value.ReleasedAt}
}

func (s *Service) provisionStorage(ctx context.Context, sandbox *protocol.Sandbox) error {
	if s.storage == nil {
		return nil
	}
	if sandbox.Config.Storage == nil {
		return errors.New("prepared storage identity is required")
	}
	value, err := s.storage.Provision(ctx, sandbox.ID, sandbox.Generation, preparedStorage(sandbox.Config.Storage))
	if err != nil {
		return err
	}
	if sandbox.Storage != nil && sandbox.Storage.ExportGeneration != value.ExportGeneration {
		return errors.New("sandbox storage generation differs from durable export")
	}
	sandbox.Storage = storageStatus(value)
	return nil
}

func (s *Service) releaseStorage(ctx context.Context, sandbox *protocol.Sandbox) error {
	if s.storage == nil {
		return nil
	}
	if sandbox.Storage == nil || sandbox.Storage.ExportGeneration == "" {
		return errors.New("sandbox has no generation-bound storage export")
	}
	value, err := s.storage.Release(ctx, sandbox.ID, sandbox.Generation, sandbox.Storage.ExportGeneration)
	if err != nil {
		return err
	}
	sandbox.Storage = storageStatus(value)
	return nil
}
func (s *Service) SetPoolCPUs(cpus []int) {
	s.poolCPUs = make(map[int]bool, len(cpus))
	for _, cpu := range cpus {
		s.poolCPUs[cpu] = true
	}
}
func (s *Service) SetPoolMemory(total, reserve uint64) {
	s.poolMemoryBytes = total
	s.poolMemoryReserve = reserve
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
func operationError(code, msg string, retry bool, operationID string) *protocol.Error {
	err := apierr(code, msg, retry)
	err.OperationID = operationID
	return err
}
func (s *Service) replay(key, fp string) (protocol.MutationResult, *protocol.Error, bool) {
	if !printableASCII(key, 1, 128) {
		return protocol.MutationResult{}, apierr("INVALID_ARGUMENT", "idempotency key must be 1-128 printable ASCII bytes", false), true
	}
	if old, ok := s.store.Result(key); ok {
		if old.Fingerprint != fp {
			return protocol.MutationResult{}, apierr("IDEMPOTENCY_CONFLICT", "idempotency key has different input", false), true
		}
		if old.Error != nil {
			failure := *old.Error
			return protocol.MutationResult{}, &failure, true
		}
		r := old.Result
		r.Replayed = true
		return r, nil, true
	}
	return protocol.MutationResult{}, nil, false
}

// CancelCreate makes an ambiguous CreateSandbox failure terminal. It is bound
// to the original idempotency key and config fingerprint. The original intent
// is completed durably before external resources are removed, preventing
// restart reconciliation from creating a sandbox after cancellation.
func (s *Service) CancelCreate(ctx context.Context, c protocol.SandboxConfig, key string) (bool, *protocol.Error) {
	if err := validateConfig(c); err != nil || !printableASCII(key, 1, 128) {
		return false, apierr("INVALID_ARGUMENT", "invalid create cancellation identity", false)
	}
	fp := fingerprint("CreateSandbox", c)
	s.global.Lock()
	defer s.global.Unlock()
	defer s.lock(c.ID)()

	if previous, ok := s.store.Result(key); ok {
		if previous.Fingerprint != fp {
			return false, apierr("IDEMPOTENCY_CONFLICT", "create cancellation fingerprint differs", false)
		}
	}
	entries, err := s.store.JournalEntries()
	if err != nil {
		return false, apierr("INTERNAL", "create journal is unavailable", true)
	}
	var intent *statepkg.JournalEntry
	completed := false
	for i := range entries {
		entry := entries[i]
		if entry.Method != "CreateSandbox" || entry.IdempotencyKey != key {
			continue
		}
		if entry.Fingerprint != fp || entry.SandboxID != c.ID || entry.Sandbox == nil || !reflect.DeepEqual(entry.Sandbox.Config, c) {
			return false, apierr("IDEMPOTENCY_CONFLICT", "create cancellation does not own journaled input", false)
		}
		if entry.Phase == "intent" {
			copy := entry
			intent = &copy
		} else if intent != nil && entry.OperationID == intent.OperationID && entry.Phase == "complete" {
			completed = true
		}
	}
	sandbox, exists := s.store.Sandbox(c.ID)
	if intent == nil && !exists {
		failure := apierr("ABORTED", "create canceled before allocation", false)
		if err = s.store.Tombstone(key, fp, failure); err != nil {
			return false, apierr("INTERNAL", "persist create cancellation", true)
		}
		return true, nil
	}
	if intent == nil || (exists && !reflect.DeepEqual(sandbox.Config, c)) {
		return false, apierr("IDEMPOTENCY_CONFLICT", "sandbox ID is not owned by canceled create", false)
	}
	if !exists {
		sandbox = *intent.Sandbox
	}
	if sandbox.State != "ALLOCATING" && sandbox.State != "CREATED" {
		return false, apierr("FAILED_PRECONDITION", "sandbox progressed beyond cancellable create state", false)
	}
	failure := apierr("ABORTED", "create canceled by runtime shim", false)
	failure.OperationID = intent.OperationID
	if !completed {
		completion := *intent
		completion.Phase = "complete"
		completion.Error = failure
		completion.State = sandbox.State
		if err = s.store.Append(completion); err != nil {
			return false, apierr("INTERNAL", "persist create cancellation intent", true)
		}
	}
	if err = s.store.Tombstone(key, fp, failure); err != nil {
		return false, apierr("INTERNAL", "persist create cancellation tombstone", true)
	}
	actual, err := s.backend.Observe(ctx, sandbox.ID)
	if err != nil {
		return false, apierr("BACKEND_FAILURE", "observe canceled create", true)
	}
	if actual != "ABSENT" && actual != "CREATED" {
		return false, apierr("FAILED_PRECONDITION", "backend progressed beyond cancellable create state", false)
	}
	if sandbox.Storage != nil && sandbox.Storage.State != "RELEASED" {
		if err = s.releaseStorage(ctx, &sandbox); err != nil {
			return false, apierr("BACKEND_FAILURE", "release canceled create storage", true)
		}
	}
	if actual == "CREATED" {
		if err = s.backend.Delete(ctx, sandbox); err != nil {
			return false, apierr("BACKEND_FAILURE", "delete canceled create sandbox", true)
		}
		if actual, err = s.backend.Observe(ctx, sandbox.ID); err != nil || actual != "ABSENT" {
			return false, apierr("BACKEND_FAILURE", "canceled create deletion was not observed", true)
		}
	}
	// The canceled create can be absent from the snapshot after a backend
	// failure even though first-sandbox pool creation committed. With the
	// global allocation lock held, zero or one owner both mean no other
	// sandbox can depend on the pool.
	if len(s.store.Snapshot().Sandboxes) <= 1 {
		if err = s.backend.ReleasePool(ctx); err != nil {
			return false, apierr("BACKEND_FAILURE", "release canceled create pool", true)
		}
	}
	if err = s.store.AbortCreate(c.ID, key, fp, failure); err != nil {
		return false, apierr("INTERNAL", "persist canceled create completion", true)
	}
	return true, nil
}

func printableASCII(value string, min, max int) bool {
	if len(value) < min || len(value) > max {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return false
		}
	}
	return true
}
func (s *Service) mutate(ctx context.Context, method, key, fp string, sb protocol.Sandbox, fn func(*protocol.Sandbox) error) (protocol.MutationResult, *protocol.Error) {
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	op, _ := generation()
	intent := statepkg.JournalEntry{OperationID: op, IdempotencyKey: key, Fingerprint: fp, SandboxID: sb.ID, Generation: sb.Generation, Method: method, Phase: "intent", State: sb.State, Sandbox: &sb}
	if e := s.checkpoint("before-intent"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash before intent", true, op)
	}
	if e := s.store.Append(intent); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
	}
	if e := s.checkpoint("after-intent"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash after intent", true, op)
	}
	if e := s.checkpoint("before-external-mutation"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash before external mutation", true, op)
	}
	if e := fn(&sb); e != nil {
		code := "BACKEND_FAILURE"
		if errors.Is(e, context.DeadlineExceeded) {
			code = "BACKEND_TIMEOUT"
		}
		message := "backend operation failed"
		if code == "BACKEND_TIMEOUT" {
			message = "backend operation timed out"
		}
		pe := operationError(code, message, true, op)
		actual := ""
		if observed, observeErr := s.backend.Observe(ctx, sb.ID); observeErr == nil {
			actual = observed
			sb.State = failureState(method, actual, sb.State)
		}
		intent.Phase = "complete"
		intent.Error = pe
		intent.State = sb.State
		if appendErr := s.store.Append(intent); appendErr != nil {
			return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
		}
		sb.Error = pe
		sb.UpdatedAt = time.Now().UTC()
		var stateErr error
		if method == "CreateSandbox" && actual == "ABSENT" {
			stateErr = s.store.RemoveSandbox(sb.ID)
		} else {
			stateErr = s.store.SetSandbox(sb)
		}
		if stateErr != nil {
			return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
		}
		return protocol.MutationResult{}, pe
	}
	if e := s.checkpoint("after-external-mutation"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash after external mutation", true, op)
	}
	actual, observeErr := s.backend.Observe(ctx, sb.ID)
	if observeErr != nil || !stateMatches(sb.State, actual) {
		pe := operationError("BACKEND_FAILURE", "backend state did not confirm operation", true, op)
		intent.Phase = "complete"
		intent.Error = pe
		if appendErr := s.store.Append(intent); appendErr != nil {
			return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
		}
		sb.Error = pe
		if stateErr := s.store.SetSandbox(sb); stateErr != nil {
			return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
		}
		return protocol.MutationResult{}, pe
	}
	if e := s.checkpoint("after-observation"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash after observation", true, op)
	}
	sb.Error = nil
	sb.UpdatedAt = time.Now().UTC()
	result := protocol.MutationResult{Sandbox: sb}
	if e := s.checkpoint("before-snapshot"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash before snapshot", true, op)
	}
	if e := s.store.Commit(sb, key, fp, result); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
	}
	if e := s.checkpoint("after-snapshot"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash after snapshot", true, op)
	}
	intent.Phase = "complete"
	intent.State = sb.State
	if e := s.checkpoint("before-completion"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash before completion", true, op)
	}
	if e := s.store.Append(intent); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "state persistence failed", true, op)
	}
	if e := s.checkpoint("after-completion"); e != nil {
		return protocol.MutationResult{}, operationError("INTERNAL", "injected crash after completion", true, op)
	}
	return result, nil
}

func failureState(method, actual, fallback string) string {
	switch actual {
	case "ABSENT", "CREATED", "RUNNING":
		return actual
	case "LOADED":
		if method == "StopSandbox" || method == "DeleteSandbox" {
			return "STOPPED"
		}
		return "LOADED"
	default:
		return fallback
	}
}

func stateMatches(durable, actual string) bool {
	return durable == actual || durable == "STOPPED" && actual == "LOADED"
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
	if !manifestRE.MatchString(c.KernelManifest) {
		return errors.New("invalid kernel manifest name")
	}
	if !filepath.IsAbs(c.Bundle) {
		return errors.New("bundle path must be absolute")
	}
	if len(c.Labels) > 32 {
		return errors.New("at most 32 labels are allowed")
	}
	for key, value := range c.Labels {
		if !labelKeyRE.MatchString(key) || len(value) > 256 {
			return errors.New("invalid label key or value")
		}
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
	if s.storage != nil && c.Storage == nil {
		return protocol.MutationResult{}, apierr("INVALID_ARGUMENT", "prepared storage identity is required", false)
	}
	if len(s.poolCPUs) != 0 {
		for _, cpu := range c.CPUs {
			if !s.poolCPUs[cpu] {
				return protocol.MutationResult{}, apierr("INVALID_ARGUMENT", fmt.Sprintf("APIC ID %d is outside the configured Kerf pool", cpu), false)
			}
		}
	}
	if s.resolver != nil {
		artifacts, err := s.artifactsFor(c.KernelManifest)
		if err != nil {
			return protocol.MutationResult{}, apierr("FAILED_PRECONDITION", "approved kernel manifest validation failed", false)
		}
		if err = artifacts.Close(); err != nil {
			return protocol.MutationResult{}, apierr("FAILED_PRECONDITION", "approved kernel artifact close failed", false)
		}
	}
	fp := fingerprint("CreateSandbox", c)
	s.global.Lock()
	defer s.global.Unlock()
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	defer s.lock(c.ID)()
	if _, ok := s.store.Sandbox(c.ID); ok {
		return protocol.MutationResult{}, apierr("ALREADY_EXISTS", "sandbox ID exists", false)
	}
	for _, x := range s.store.Snapshot().Sandboxes {
		if overlap(c, x.Config) {
			return protocol.MutationResult{}, apierr("RESOURCE_EXHAUSTED", "CPU, port, or writable bundle overlaps another sandbox", false)
		}
	}
	if s.poolMemoryBytes != 0 {
		var usable uint64
		if s.poolMemoryReserve < s.poolMemoryBytes {
			usable = s.poolMemoryBytes - s.poolMemoryReserve
		}
		var allocated uint64
		for _, x := range s.store.Snapshot().Sandboxes {
			if x.State == "ABSENT" {
				continue
			}
			if x.Config.MemoryBytes > usable-allocated {
				allocated = usable
				break
			}
			allocated += x.Config.MemoryBytes
		}
		if c.MemoryBytes > usable-allocated {
			return protocol.MutationResult{}, apierr("RESOURCE_EXHAUSTED", "sandbox memory exceeds configured usable Kerf pool memory", false)
		}
	}
	g, e := generation()
	if e != nil {
		return protocol.MutationResult{}, apierr("INTERNAL", e.Error(), true)
	}
	now := time.Now().UTC()
	sb := protocol.Sandbox{ID: c.ID, Generation: g, State: "ALLOCATING", Config: c, CreatedAt: now, UpdatedAt: now}
	firstSandbox := len(s.store.Snapshot().Sandboxes) == 0
	return s.mutate(ctx, "CreateSandbox", key, fp, sb, func(current *protocol.Sandbox) error {
		if e := s.store.SetSandbox(*current); e != nil {
			return e
		}
		if firstSandbox {
			if e := s.backend.EnsurePool(ctx); e != nil {
				return e
			}
		}
		if e := s.backend.Create(ctx, *current); e != nil {
			var poolErr error
			if firstSandbox {
				poolErr = s.backend.ReleasePool(context.WithoutCancel(ctx))
			}
			return errors.Join(e, poolErr)
		}
		if e := s.provisionStorage(ctx, current); e != nil {
			deleteErr := s.backend.Delete(context.WithoutCancel(ctx), *current)
			var poolErr error
			if firstSandbox {
				poolErr = s.backend.ReleasePool(context.WithoutCancel(ctx))
			}
			return errors.Join(e, deleteErr, poolErr)
		}
		if e := s.store.SetSandbox(*current); e != nil {
			storageErr := s.releaseStorage(context.WithoutCancel(ctx), current)
			deleteErr := s.backend.Delete(context.WithoutCancel(ctx), *current)
			return errors.Join(e, storageErr, deleteErr)
		}
		current.State = "CREATED"
		return nil
	})
}
func (s *Service) transition(ctx context.Context, id, gen, key, method string, from []string, intermediate, to string, fn func(protocol.Sandbox) error) (protocol.MutationResult, *protocol.Error) {
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
		if intermediate != "" {
			current.State = intermediate
			if e := s.store.SetSandbox(*current); e != nil {
				return e
			}
		}
		if e := fn(*current); e != nil {
			return e
		}
		current.State = to
		return nil
	})
}
func (s *Service) Load(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "LoadSandbox", []string{"CREATED"}, "", "LOADED", func(x protocol.Sandbox) error {
		if err := s.provisionStorage(ctx, &x); err != nil {
			return err
		}
		artifacts, err := s.artifactsFor(x.Config.KernelManifest)
		if err != nil {
			return err
		}
		defer artifacts.Close()
		bootFiles := kerf.LoadFiles{Kernel: artifacts.KernelFile, Initramfs: artifacts.InitrdFile}
		if s.preparedBoot != nil {
			runtimeDir, initramfs, openErr := s.preparedBoot.OpenPreparedBoot(ctx, x.Config.Bundle, x.Config.Storage)
			if openErr != nil {
				return openErr
			}
			defer runtimeDir.Close()
			defer initramfs.Close()
			bootFiles.RuntimeDir, bootFiles.Initramfs = runtimeDir, initramfs
		}
		return s.backend.Load(ctx, x, artifacts.Kernel, artifacts.Initrd, artifacts.Cmdline, bootFiles)
	})
}
func (s *Service) Start(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "StartSandbox", []string{"LOADED", "STOPPED"}, "", "RUNNING", func(x protocol.Sandbox) error { return s.backend.Start(ctx, x) })
}
func (s *Service) Stop(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	return s.transition(ctx, id, gen, key, "StopSandbox", []string{"RUNNING"}, "STOPPING", "STOPPED", func(x protocol.Sandbox) error { return s.backend.Stop(ctx, x) })
}
func (s *Service) Delete(ctx context.Context, id, gen, key string) (protocol.MutationResult, *protocol.Error) {
	s.global.Lock()
	defer s.global.Unlock()
	defer s.lock(id)()
	fp := fingerprint("DeleteSandbox", struct{ ID, Gen string }{id, gen})
	if r, e, ok := s.replay(key, fp); ok {
		return r, e
	}
	sb, ok := s.store.Sandbox(id)
	if !ok {
		return protocol.MutationResult{}, apierr("NOT_FOUND", "sandbox not found", false)
	}
	if sb.Generation != gen {
		return protocol.MutationResult{}, apierr("STALE_GENERATION", "generation does not match", false)
	}
	return s.mutate(ctx, "DeleteSandbox", key, fp, sb, func(current *protocol.Sandbox) error {
		wasRunning := current.State == "RUNNING"
		current.State = "RELEASING"
		if e := s.store.SetSandbox(*current); e != nil {
			return e
		}
		if wasRunning {
			if e := s.backend.Stop(ctx, *current); e != nil {
				return e
			}
		}
		if e := s.releaseStorage(ctx, current); e != nil {
			return e
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
func (s *Service) Events(after uint64, limit uint32) ([]protocol.Event, *protocol.Error) {
	if limit == 0 {
		limit = 128
	}
	if limit > 1024 {
		return nil, apierr("INVALID_ARGUMENT", "event limit must be at most 1024", false)
	}
	entries, err := s.store.JournalEntries()
	if err != nil {
		return nil, apierr("INTERNAL", "event journal is unavailable", true)
	}
	events := make([]protocol.Event, 0)
	for _, entry := range entries {
		if entry.Phase != "complete" || entry.Sequence <= after {
			continue
		}
		events = append(events, protocol.Event{Version: 1, Sequence: entry.Sequence, At: entry.At, SandboxID: entry.SandboxID, Generation: entry.Generation, Method: entry.Method, State: entry.State, Error: entry.Error})
		if uint32(len(events)) == limit {
			break
		}
	}
	return events, nil
}
func (s *Service) Reconcile(ctx context.Context) error {
	if s.storage != nil {
		if err := s.storage.Reconcile(ctx); err != nil {
			return fmt.Errorf("storage reconcile: %w", err)
		}
	}
	es, e := s.store.JournalEntries()
	if e != nil {
		return e
	}
	incomplete := statepkg.Incomplete(es)
	pending := map[string]bool{}
	for _, x := range incomplete {
		pending[x.SandboxID] = true
		if e = s.reconcileIncomplete(ctx, x); e != nil {
			return e
		}
	}
	if inventory, ok := s.backend.(kerf.InventoryBackend); ok {
		instances, inventoryErr := inventory.ListInstances(ctx)
		if inventoryErr != nil {
			return inventoryErr
		}
		known := map[string]bool{}
		for _, sandbox := range s.List() {
			known[sandbox.ID] = true
		}
		var unknown []string
		for _, instance := range instances {
			if !known[instance] {
				unknown = append(unknown, instance)
			}
		}
		if len(unknown) != 0 {
			return fmt.Errorf("OPERATOR_ACTION: unknown backend instances: %s", strings.Join(unknown, ","))
		}
	}
	for _, sb := range s.List() {
		actual, e := s.backend.Observe(ctx, sb.ID)
		if e != nil {
			return e
		}
		if sb.State == "STOPPED" && actual == "LOADED" {
			continue
		}
		if actual != sb.State && !pending[sb.ID] {
			sb.Error = apierr("OPERATOR_ACTION", fmt.Sprintf("journal=%s backend=%s incomplete=%t", sb.State, actual, pending[sb.ID]), false)
			if e = s.store.SetSandbox(sb); e != nil {
				return e
			}
		}
	}
	return nil
}

func (s *Service) reconcileIncomplete(ctx context.Context, intent statepkg.JournalEntry) error {
	if intent.Sandbox == nil {
		return fmt.Errorf("OPERATOR_ACTION: incomplete %s intent %s has no recoverable sandbox", intent.Method, intent.OperationID)
	}
	sandbox := *intent.Sandbox
	actual, err := s.backend.Observe(ctx, sandbox.ID)
	if err != nil {
		return err
	}
	complete := intent
	complete.Phase = "complete"
	finish := func(state string) error {
		sandbox.State = state
		sandbox.Error = nil
		result := protocol.MutationResult{Sandbox: sandbox}
		if err := s.store.Commit(sandbox, intent.IdempotencyKey, intent.Fingerprint, result); err != nil {
			return err
		}
		complete.State = state
		return s.store.Append(complete)
	}
	fail := func(cause error) error {
		sandbox.Error = apierr("OPERATOR_ACTION", "incomplete operation could not be reconciled safely", false)
		if setErr := s.store.SetSandbox(sandbox); setErr != nil {
			return setErr
		}
		complete.Error = sandbox.Error
		complete.State = sandbox.State
		if appendErr := s.store.Append(complete); appendErr != nil {
			return appendErr
		}
		return fmt.Errorf("OPERATOR_ACTION: reconcile %s: %w", intent.Method, cause)
	}
	switch intent.Method {
	case "CreateSandbox":
		if actual == "ABSENT" {
			if err = s.backend.EnsurePool(ctx); err == nil {
				err = s.backend.Create(ctx, sandbox)
			}
		}
		if err == nil {
			actual, err = s.backend.Observe(ctx, sandbox.ID)
		}
		if err == nil && actual == "CREATED" {
			err = s.provisionStorage(ctx, &sandbox)
		}
		if err == nil && actual == "CREATED" {
			return finish("CREATED")
		}
	case "LoadSandbox":
		if actual == "CREATED" {
			var artifacts Artifacts
			err = s.provisionStorage(ctx, &sandbox)
			if err == nil {
				artifacts, err = s.artifactsFor(sandbox.Config.KernelManifest)
			}
			if err == nil {
				defer artifacts.Close()
				bootFiles := kerf.LoadFiles{Kernel: artifacts.KernelFile, Initramfs: artifacts.InitrdFile}
				if s.preparedBoot != nil {
					var runtimeDir, initramfs *os.File
					runtimeDir, initramfs, err = s.preparedBoot.OpenPreparedBoot(ctx, sandbox.Config.Bundle, sandbox.Config.Storage)
					if err == nil {
						defer runtimeDir.Close()
						defer initramfs.Close()
						bootFiles.RuntimeDir, bootFiles.Initramfs = runtimeDir, initramfs
					}
				}
				if err == nil {
					err = s.backend.Load(ctx, sandbox, artifacts.Kernel, artifacts.Initrd, artifacts.Cmdline, bootFiles)
				}
			}
		}
		if err == nil {
			actual, err = s.backend.Observe(ctx, sandbox.ID)
		}
		if err == nil && actual == "LOADED" {
			return finish("LOADED")
		}
	case "StartSandbox":
		if actual == "LOADED" || actual == "STOPPED" {
			err = s.backend.Start(ctx, sandbox)
		}
		if err == nil {
			actual, err = s.backend.Observe(ctx, sandbox.ID)
		}
		if err == nil && actual == "RUNNING" {
			return finish("RUNNING")
		}
	case "StopSandbox":
		if actual == "RUNNING" {
			err = s.backend.Stop(ctx, sandbox)
		}
		if err == nil {
			actual, err = s.backend.Observe(ctx, sandbox.ID)
		}
		if err == nil && (actual == "LOADED" || actual == "STOPPED") {
			return finish("STOPPED")
		}
	case "DeleteSandbox":
		if actual == "RUNNING" {
			err = s.backend.Stop(ctx, sandbox)
		}
		if err == nil && actual != "ABSENT" {
			err = s.releaseStorage(ctx, &sandbox)
		}
		if err == nil && actual != "ABSENT" {
			err = s.backend.Delete(ctx, sandbox)
		}
		if err == nil {
			actual, err = s.backend.Observe(ctx, sandbox.ID)
		}
		if err == nil && actual == "ABSENT" {
			return finish("ABSENT")
		}
	default:
		err = fmt.Errorf("unknown method %s", intent.Method)
	}
	if err == nil {
		err = fmt.Errorf("unexpected backend state %s", actual)
	}
	return fail(err)
}
