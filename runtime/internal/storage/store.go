package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var offlineCheckRE = regexp.MustCompile(`^e2fsck-clean-sha256:[a-f0-9]{64}$`)

type diskState struct {
	Version int               `json:"version"`
	Exports map[string]Export `json:"exports"`
}

type Store struct {
	mu          sync.Mutex
	dir         string
	dirIdentity safefile.Identity
	data        diskState
}

func OpenStore(dir string) (*Store, error) {
	directory, err := safefile.OpenDirectory(dir, true)
	if err != nil {
		return nil, fmt.Errorf("open storage state directory: %w", err)
	}
	defer directory.Close()
	store := &Store{dir: dir, dirIdentity: directory.Identity(), data: diskState{Version: StateVersion, Exports: map[string]Export{}}}
	data, found, err := directory.ReadPrivate("state.json", 16<<20)
	if err != nil {
		return nil, fmt.Errorf("read storage state: %w", err)
	}
	if !found {
		return store, nil
	}
	if err = protocol.StrictDecode(data, &store.data); err != nil {
		return nil, err
	}
	if store.data.Exports == nil {
		return nil, errors.New("unsupported storage state")
	}
	if store.data.Version >= 1 && store.data.Version < StateVersion && len(store.data.Exports) == 0 {
		store.data.Version = StateVersion
		upgraded, marshalErr := json.MarshalIndent(store.data, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		if err = directory.Replace("state.json", append(upgraded, '\n'), 0600); err != nil {
			return nil, fmt.Errorf("upgrade empty storage state: %w", err)
		}
	} else if store.data.Version != StateVersion {
		return nil, errors.New("unsupported storage state")
	}
	if err = validateDiskState(store.data); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) openDirectory() (*safefile.Directory, error) {
	directory, err := safefile.OpenDirectory(s.dir, false)
	if err != nil {
		return nil, err
	}
	if directory.Identity() != s.dirIdentity {
		_ = directory.Close()
		return nil, errors.New("storage state directory identity changed")
	}
	return directory, nil
}

func exportKey(id, generation string) string { return id + "\x00" + generation }

func validateExport(key string, value Export) error {
	if key != exportKey(value.SandboxID, value.SandboxGeneration) || !identityRE.MatchString(value.SandboxID) ||
		!generationRE.MatchString(value.SandboxGeneration) || !generationRE.MatchString(value.ExportGeneration) {
		return errors.New("storage key, owner, or generation is invalid")
	}
	if err := validatePrepared(value.PreparedImage); err != nil {
		return err
	}
	if value.ImageIdentity.Device == 0 || value.ImageIdentity.Inode == 0 {
		return errors.New("storage image inode identity is absent")
	}
	if value.CreatedAt.IsZero() || value.UpdatedAt.Before(value.CreatedAt) {
		return errors.New("storage timestamps are invalid")
	}
	_, createdOffset := value.CreatedAt.Zone()
	_, updatedOffset := value.UpdatedAt.Zone()
	if createdOffset != 0 || updatedOffset != 0 {
		return errors.New("storage timestamps must be UTC")
	}
	switch value.State {
	case "PREPARING", "ACTIVE", "QUIESCING":
		if !value.ReleasedAt.IsZero() || value.OfflineCheck != "" || value.Counters != (Counters{}) {
			return errors.New("unreleased storage state contains release results")
		}
	case "RELEASED":
		_, releasedOffset := value.ReleasedAt.Zone()
		if value.ReleasedAt.IsZero() || !value.ReleasedAt.Equal(value.UpdatedAt) || releasedOffset != 0 ||
			!offlineCheckRE.MatchString(value.OfflineCheck) {
			return errors.New("released storage state has invalid completion evidence")
		}
	default:
		return errors.New("storage state is invalid")
	}
	return nil
}

func validateDiskState(state diskState) error {
	claims := map[string]map[string]string{
		"path": {}, "port": {}, "filesystem UUID": {}, "sandbox owner": {},
	}
	for key, value := range state.Exports {
		if err := validateExport(key, value); err != nil {
			return fmt.Errorf("invalid durable storage export %q: %w", key, err)
		}
		if value.State == "RELEASED" {
			continue
		}
		for label, identity := range map[string]string{
			"path": value.Path, "port": fmt.Sprint(value.Port), "filesystem UUID": value.FilesystemUUID,
			"sandbox owner": value.SandboxID,
		} {
			previous := claims[label][identity]
			if previous != "" && previous != key {
				return fmt.Errorf("live storage %s is allocated by multiple records", label)
			}
			claims[label][identity] = key
		}
	}
	return nil
}

func validTransition(previous, next Export) bool {
	if previous.SandboxID != next.SandboxID || previous.SandboxGeneration != next.SandboxGeneration ||
		previous.ExportGeneration != next.ExportGeneration || previous.PreparedImage != next.PreparedImage ||
		!previous.CreatedAt.Equal(next.CreatedAt) || next.UpdatedAt.Before(previous.UpdatedAt) {
		return false
	}
	return previous.State == next.State || previous.State == "PREPARING" && next.State == "ACTIVE" ||
		previous.State == "ACTIVE" && next.State == "QUIESCING" ||
		previous.State == "QUIESCING" && next.State == "RELEASED"
}

func (s *Store) Get(id, generation string) (Export, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data.Exports[exportKey(id, generation)]
	return value, ok
}

func (s *Store) List() []Export {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Export, 0, len(s.data.Exports))
	for _, value := range s.data.Exports {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SandboxID == result[j].SandboxID {
			return result[i].SandboxGeneration < result[j].SandboxGeneration
		}
		return result[i].SandboxID < result[j].SandboxID
	})
	return result
}

func (s *Store) Put(value Export) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := exportKey(value.SandboxID, value.SandboxGeneration)
	if err := validateExport(key, value); err != nil {
		return err
	}
	previous, existed := s.data.Exports[key]
	if !existed && value.State != "PREPARING" {
		return errors.New("new storage ownership must begin in PREPARING")
	}
	if existed && !validTransition(previous, value) {
		return errors.New("storage identity or state transition is invalid")
	}
	s.data.Exports[key] = value
	if err := validateDiskState(s.data); err != nil {
		if existed {
			s.data.Exports[key] = previous
		} else {
			delete(s.data.Exports, key)
		}
		return err
	}
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Exports[key] = previous
		} else {
			delete(s.data.Exports, key)
		}
		return err
	}
	return nil
}

func (s *Store) Delete(id, generation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !identityRE.MatchString(id) || !generationRE.MatchString(generation) {
		return errors.New("storage deletion identity is invalid")
	}
	key := exportKey(id, generation)
	previous, existed := s.data.Exports[key]
	delete(s.data.Exports, key)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Exports[key] = previous
		}
		return err
	}
	return nil
}

func (s *Store) persistLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	directory, err := s.openDirectory()
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Replace("state.json", append(data, '\n'), 0600)
}
