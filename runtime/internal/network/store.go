package network

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
	Version   int                 `json:"version"`
	Endpoints map[string]Endpoint `json:"endpoints"`
}

type Store struct {
	mu   sync.Mutex
	dir  string
	data diskState
}

func OpenStore(dir string) (*Store, error) {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("mknetd state directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return nil, errors.New("mknetd state directory may not contain symlinks")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	store := &Store{dir: dir, data: diskState{Version: 1, Endpoints: map[string]Endpoint{}}}
	statePath := filepath.Join(dir, "state.json")
	info, err := os.Lstat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return nil, errors.New("mknetd state must be a private bounded regular file")
	}
	file, err := os.Open(statePath)
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
	if store.data.Version != 1 || store.data.Endpoints == nil {
		return nil, errors.New("unsupported mknetd state")
	}
	return store, nil
}

func endpointKey(networkName, containerID, ifName string) string {
	return networkName + "\x00" + containerID + "\x00" + ifName
}

func (s *Store) Get(networkName, containerID, ifName string) (Endpoint, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint, ok := s.data.Endpoints[endpointKey(networkName, containerID, ifName)]
	return endpoint, ok
}

func (s *Store) List() []Endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Endpoint, 0, len(s.data.Endpoints))
	for _, endpoint := range s.data.Endpoints {
		result = append(result, endpoint)
	}
	return result
}

func (s *Store) Put(endpoint Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := endpointKey(endpoint.NetworkName, endpoint.ContainerID, endpoint.IfName)
	previous, existed := s.data.Endpoints[key]
	s.data.Endpoints[key] = endpoint
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Endpoints[key] = previous
		} else {
			delete(s.data.Endpoints, key)
		}
		return err
	}
	return nil
}

func (s *Store) Delete(networkName, containerID, ifName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := endpointKey(networkName, containerID, ifName)
	previous, existed := s.data.Endpoints[key]
	delete(s.data.Endpoints, key)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Endpoints[key] = previous
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
