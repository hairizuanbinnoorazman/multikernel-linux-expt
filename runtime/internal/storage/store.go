package storage

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type diskState struct {
	Version int               `json:"version"`
	Exports map[string]Export `json:"exports"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data diskState
}

func OpenStore(dir string) (*Store, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("storage state directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return nil, errors.New("storage state directory may not contain symlinks")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	store := &Store{dir: dir, data: diskState{Version: StateVersion, Exports: map[string]Export{}}}
	path := filepath.Join(dir, "state.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return nil, errors.New("storage state must be a private bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	err = errors.Join(readErr, file.Close())
	if err != nil {
		return nil, err
	}
	if err = protocol.StrictDecode(data, &store.data); err != nil {
		return nil, err
	}
	if store.data.Version != StateVersion || store.data.Exports == nil {
		return nil, errors.New("unsupported storage state")
	}
	return store, nil
}

func exportKey(id, generation string) string { return id + "\x00" + generation }

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
	previous, existed := s.data.Exports[key]
	s.data.Exports[key] = value
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
	temporary, err := os.CreateTemp(s.dir, ".state.json.*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(append(data, '\n'))
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, filepath.Join(s.dir, "state.json")); err != nil {
		return err
	}
	cleanup = false
	directory, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	err = directory.Sync()
	return errors.Join(err, directory.Close())
}
