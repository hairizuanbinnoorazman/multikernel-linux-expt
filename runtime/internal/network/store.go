package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type diskState struct {
	Version   int                 `json:"version"`
	Endpoints map[string]Endpoint `json:"endpoints"`
}

type Store struct {
	mu           sync.Mutex
	dir          string
	data         diskState
	persistFault func() error
}

func OpenStore(dir string) (*Store, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, errors.New("mknetd state directory must be absolute and canonical")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	identity, identityOK := dirInfo.Sys().(*syscall.Stat_t)
	if !identityOK || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || identity.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("mknetd state directory must be a caller-owned real directory")
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
	descriptor, err := unix.Open(statePath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), statePath)
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.New("mknetd state identity changed while opening")
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
	for key, endpoint := range store.data.Endpoints {
		if err = validateStoredEndpoint(key, endpoint); err != nil {
			return nil, fmt.Errorf("invalid durable endpoint %q: %w", key, err)
		}
	}
	return store, nil
}

func endpointKey(networkName, containerID, ifName string) string {
	return networkName + "\x00" + containerID + "\x00" + ifName
}

func validateStoredEndpoint(key string, endpoint Endpoint) error {
	if key != endpointKey(endpoint.NetworkName, endpoint.ContainerID, endpoint.IfName) ||
		!identifier.MatchString(endpoint.NetworkName) || !identifier.MatchString(endpoint.ContainerID) ||
		!identifier.MatchString(endpoint.IfName) || len(endpoint.IfName) > 15 {
		return errors.New("endpoint key or workload identity is invalid")
	}
	if endpoint.Owner != "cni" && endpoint.Owner != "runtime" {
		return errors.New("endpoint owner is invalid")
	}
	if endpoint.ManagedNamespace != (endpoint.Owner == "runtime") {
		return errors.New("managed namespace ownership is inconsistent")
	}
	if _, _, _, _, _, err := endpointTopology(endpoint); err != nil {
		return err
	}
	if endpoint.MTU < 576 || endpoint.MTU > 65515 {
		return errors.New("endpoint MTU is invalid")
	}
	if endpoint.ManagedNamespace {
		if endpoint.NetNS != managedNamespacePath(endpoint.Generation) {
			return errors.New("managed namespace path differs from its generation")
		}
	} else if _, err := netnsTarget(endpoint.NetNS); err != nil {
		return err
	}
	if endpoint.State != "ALLOCATING" && endpoint.State != "DELETING" && endpoint.State != "READY" && endpoint.State != "DEGRADED" && endpoint.State != "DISCONNECTED" {
		return errors.New("endpoint state is invalid")
	}
	if (endpoint.SandboxID == "") != (endpoint.SandboxGeneration == "") ||
		endpoint.SandboxID != "" && (!identifier.MatchString(endpoint.SandboxID) || !endpointGeneration.MatchString(endpoint.SandboxGeneration)) {
		return errors.New("endpoint sandbox binding is invalid")
	}
	if (endpoint.State == "ALLOCATING" || endpoint.State == "DELETING") && endpoint.SandboxID != "" {
		return errors.New("transitional endpoint may not be sandbox-bound")
	}
	if len(endpoint.DNS.Nameservers) > 8 || len(endpoint.DNS.Search) > 8 || len(endpoint.DNS.Options) > 8 {
		return errors.New("endpoint DNS policy is oversized")
	}
	for _, server := range endpoint.DNS.Nameservers {
		if net.ParseIP(server) == nil {
			return errors.New("endpoint DNS server is invalid")
		}
	}
	return nil
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
	if err := validateStoredEndpoint(key, endpoint); err != nil {
		return err
	}
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
	if s.persistFault != nil {
		if err := s.persistFault(); err != nil {
			return err
		}
	}
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
