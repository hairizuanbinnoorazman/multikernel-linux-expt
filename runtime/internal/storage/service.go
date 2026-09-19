package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var identityRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
var generationRE = regexp.MustCompile(`^[a-f0-9]{32}$`)
var uuidRE = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var digestRE = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Backend interface {
	Inspect(context.Context, PreparedImage) (ImageIdentity, error)
	Start(context.Context, Export) error
	Observe(context.Context, Export) (Observation, error)
	Stop(context.Context, Export) (Counters, error)
	OfflineCheck(context.Context, Export) (string, error)
}

type Service struct {
	store   *Store
	backend Backend
	mu      sync.Mutex
	now     func() time.Time
	random  func([]byte) error
}

func NewService(store *Store, backend Backend) *Service {
	return &Service{store: store, backend: backend, now: time.Now, random: func(value []byte) error {
		_, err := rand.Read(value)
		return err
	}}
}

func (s *Service) generation() (string, error) {
	value := make([]byte, 16)
	if err := s.random(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func validatePrepared(value PreparedImage) error {
	if !filepath.IsAbs(value.Path) || filepath.Clean(value.Path) != value.Path || strings.ContainsAny(value.Path, "\x00\n\r") {
		return errors.New("storage image path must be absolute and canonical")
	}
	if !identityRE.MatchString(value.ImageID) || !uuidRE.MatchString(value.FilesystemUUID) {
		return errors.New("invalid image identity or filesystem UUID")
	}
	if !digestRE.MatchString(value.SHA256) {
		return errors.New("storage image digest is malformed")
	}
	if value.SizeBytes < 64<<20 || value.SizeBytes > 16<<30 || value.SizeBytes%4096 != 0 || value.QuotaBytes != value.SizeBytes {
		return errors.New("image size must be aligned, bounded by, and equal to its enforced quota")
	}
	if value.InodeLimit < 128 || value.InodeLimit > 2_097_152 || value.Port < 1024 || value.Port > 65535 {
		return errors.New("inode limit and non-privileged export port are required")
	}
	return nil
}

// recoverPreparation repairs a journaled start whose external outcome was
// ambiguous. The caller holds s.mu. An exact live process is stopped first so
// only a Start call that returns success can transition the lease to ACTIVE.
func (s *Service) recoverPreparation(ctx context.Context, value Export) (Export, error) {
	if err := ctx.Err(); err != nil {
		return Export{}, err
	}
	observation, err := s.backend.Observe(ctx, value)
	if err != nil {
		return Export{}, fmt.Errorf("observe retained storage preparation: %w", err)
	}
	if observation.Active && observation.Generation != value.ExportGeneration {
		return Export{}, errors.New("preparing storage process has a conflicting generation")
	}
	if observation.Active {
		if _, err = s.backend.Stop(ctx, value); err != nil {
			return Export{}, fmt.Errorf("stop ambiguous storage preparation: %w", err)
		}
	}
	identity, err := s.backend.Inspect(ctx, value.PreparedImage)
	if err != nil {
		return Export{}, fmt.Errorf("reinspect retained storage preparation: %w", err)
	}
	if identity != value.ImageIdentity {
		return Export{}, errors.New("retained storage image identity changed")
	}
	if err = s.backend.Start(ctx, value); err != nil {
		return Export{}, fmt.Errorf("restart retained storage preparation: %w", err)
	}
	value.State = "ACTIVE"
	value.UpdatedAt = s.now().UTC()
	if err = s.store.Put(value); err != nil {
		return Export{}, fmt.Errorf("publish recovered storage export with PREPARING ownership retained: %w", err)
	}
	return value, nil
}

func (s *Service) Provision(ctx context.Context, sandboxID, sandboxGeneration string, image PreparedImage) (Export, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Export{}, err
	}
	if !identityRE.MatchString(sandboxID) || !generationRE.MatchString(sandboxGeneration) {
		return Export{}, errors.New("invalid sandbox identity")
	}
	if err := validatePrepared(image); err != nil {
		return Export{}, err
	}
	if existing, ok := s.store.Get(sandboxID, sandboxGeneration); ok {
		if existing.PreparedImage != image {
			return Export{}, errors.New("sandbox generation has conflicting or incomplete storage state")
		}
		if existing.State == "PREPARING" {
			return s.recoverPreparation(ctx, existing)
		}
		if existing.State != "ACTIVE" {
			return Export{}, errors.New("sandbox generation has conflicting or incomplete storage state")
		}
		observation, err := s.backend.Observe(ctx, existing)
		if err != nil || !observation.Active || observation.Generation != existing.ExportGeneration {
			return Export{}, errors.New("durable export is not active with its recorded generation")
		}
		existing.Counters = observation.Counters
		return existing, nil
	}
	for _, existing := range s.store.List() {
		if existing.State == "RELEASED" {
			continue
		}
		if existing.SandboxID == sandboxID || existing.Path == image.Path || existing.Port == image.Port || existing.FilesystemUUID == image.FilesystemUUID {
			return Export{}, errors.New("storage owner, path, port, or filesystem UUID is already allocated")
		}
	}
	imageIdentity, err := s.backend.Inspect(ctx, image)
	if err != nil {
		return Export{}, fmt.Errorf("inspect prepared image: %w", err)
	}
	generation, err := s.generation()
	if err != nil {
		return Export{}, err
	}
	now := s.now().UTC()
	value := Export{SandboxID: sandboxID, SandboxGeneration: sandboxGeneration, ExportGeneration: generation,
		PreparedImage: image, ImageIdentity: imageIdentity, State: "PREPARING", CreatedAt: now, UpdatedAt: now}
	if err = s.store.Put(value); err != nil {
		return Export{}, err
	}
	if err = s.backend.Start(ctx, value); err != nil {
		return Export{}, fmt.Errorf("start storage export with PREPARING ownership retained: %w", err)
	}
	value.State = "ACTIVE"
	value.UpdatedAt = s.now().UTC()
	if err = s.store.Put(value); err != nil {
		return Export{}, fmt.Errorf("publish active storage export with PREPARING ownership retained: %w", err)
	}
	return value, nil
}

func (s *Service) Release(ctx context.Context, sandboxID, sandboxGeneration, exportGeneration string) (Export, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Export{}, err
	}
	value, ok := s.store.Get(sandboxID, sandboxGeneration)
	if !ok {
		return Export{}, errors.New("storage export not found")
	}
	if value.ExportGeneration != exportGeneration {
		return Export{}, errors.New("stale storage export generation")
	}
	if value.State == "RELEASED" {
		return value, nil
	}
	if value.State != "ACTIVE" && value.State != "QUIESCING" {
		return Export{}, errors.New("storage export is not safely releasable")
	}
	value.State = "QUIESCING"
	value.UpdatedAt = s.now().UTC()
	if err := s.store.Put(value); err != nil {
		return Export{}, err
	}
	counters, err := s.backend.Stop(ctx, value)
	if err != nil {
		return Export{}, fmt.Errorf("stop storage export: %w", err)
	}
	result, err := s.backend.OfflineCheck(ctx, value)
	if err != nil {
		return Export{}, fmt.Errorf("offline filesystem check: %w", err)
	}
	value.State = "RELEASED"
	value.Counters = counters
	value.OfflineCheck = result
	value.ReleasedAt = s.now().UTC()
	value.UpdatedAt = value.ReleasedAt
	if err = s.store.Put(value); err != nil {
		return Export{}, err
	}
	return value, nil
}

func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, value := range s.store.List() {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch value.State {
		case "RELEASED":
			continue
		case "ACTIVE":
			observation, err := s.backend.Observe(ctx, value)
			if err != nil {
				return err
			}
			if observation.Active && observation.Generation == value.ExportGeneration {
				continue
			}
			if observation.Active {
				return errors.New("active storage process has a conflicting generation")
			}
			identity, inspectErr := s.backend.Inspect(ctx, value.PreparedImage)
			if inspectErr == nil && identity != value.ImageIdentity {
				inspectErr = errors.New("durable storage image identity changed")
			}
			if inspectErr == nil {
				err = s.backend.Start(ctx, value)
			} else {
				err = inspectErr
			}
			if err != nil {
				return fmt.Errorf("restart durable storage export: %w", err)
			}
		case "PREPARING":
			if _, err := s.recoverPreparation(ctx, value); err != nil {
				return err
			}
		case "QUIESCING":
			observation, err := s.backend.Observe(ctx, value)
			if err != nil {
				return fmt.Errorf("observe quiescing storage export: %w", err)
			}
			if observation.Active && observation.Generation != value.ExportGeneration {
				return errors.New("quiescing storage process has a conflicting generation")
			}
			counters := observation.Counters
			if observation.Active {
				counters, err = s.backend.Stop(ctx, value)
				if err != nil {
					return fmt.Errorf("resume storage quiescence: %w", err)
				}
			} else if !observation.Closed {
				return errors.New("quiescing storage server is absent without a graceful close record")
			}
			result, err := s.backend.OfflineCheck(ctx, value)
			if err != nil {
				return fmt.Errorf("recover offline filesystem check: %w", err)
			}
			value.State = "RELEASED"
			value.Counters = counters
			value.OfflineCheck = result
			value.ReleasedAt = s.now().UTC()
			value.UpdatedAt = value.ReleasedAt
			if err = s.store.Put(value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported storage state %q", value.State)
		}
	}
	return nil
}
