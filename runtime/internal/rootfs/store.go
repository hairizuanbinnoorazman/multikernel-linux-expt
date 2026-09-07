package rootfs

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type diskState struct {
	Version int               `json:"version"`
	Records map[string]Record `json:"records"`
}

type Store struct {
	mu           sync.Mutex
	dir          string
	data         diskState
	persistFault func() error
}

func OpenStore(dir string) (*Store, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("rootfs state directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return nil, errors.New("rootfs state directory may not contain symlinks")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	store := &Store{dir: dir, data: diskState{Version: Version, Records: map[string]Record{}}}
	path := filepath.Join(dir, "state.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return nil, errors.New("rootfs state must be a private bounded regular file")
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
	if err = protocol.StrictDecode(data, &store.data); err != nil || store.data.Version != Version || store.data.Records == nil {
		return nil, errors.New("rootfs state is malformed or unsupported")
	}
	return store, nil
}

func (s *Store) Get(identity string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data.Records[identity]
	return value, ok
}
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Record, 0, len(s.data.Records))
	for _, value := range s.data.Records {
		result = append(result, value)
	}
	return result
}
func (s *Store) Put(value Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data.Records[value.Request.TaskIdentity]
	s.data.Records[value.Request.TaskIdentity] = value
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Records[value.Request.TaskIdentity] = previous
		} else {
			delete(s.data.Records, value.Request.TaskIdentity)
		}
		return err
	}
	return nil
}
func (s *Store) Delete(identity string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, existed := s.data.Records[identity]
	delete(s.data.Records, identity)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Records[identity] = previous
		}
		return err
	}
	return nil
}
func (s *Store) persistLocked() error {
	if s.persistFault != nil {
		if err := s.persistFault(); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.dir, ".state.*")
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
