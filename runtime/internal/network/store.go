package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type diskState struct {
	Version   int                 `json:"version"`
	Endpoints map[string]Endpoint `json:"endpoints"`
}

type Store struct {
	mu           sync.Mutex
	dir          string
	dirIdentity  safefile.Identity
	data         diskState
	persistFault func() error
}

func OpenStore(dir string) (*Store, error) {
	directory, err := safefile.OpenDirectory(dir, true)
	if err != nil {
		return nil, fmt.Errorf("open mknetd state directory: %w", err)
	}
	defer directory.Close()
	store := &Store{dir: dir, dirIdentity: directory.Identity(), data: diskState{Version: 1, Endpoints: map[string]Endpoint{}}}
	data, found, err := directory.ReadPrivate("state.json", 16<<20)
	if err != nil {
		return nil, fmt.Errorf("read mknetd state: %w", err)
	}
	if !found {
		return store, nil
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

func (s *Store) openDirectory() (*safefile.Directory, error) {
	directory, err := safefile.OpenDirectory(s.dir, false)
	if err != nil {
		return nil, err
	}
	if directory.Identity() != s.dirIdentity {
		_ = directory.Close()
		return nil, errors.New("mknetd state directory identity changed")
	}
	return directory, nil
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
	if endpoint.State == "ALLOCATING" && endpoint.SandboxID != "" {
		return errors.New("allocating endpoint may not be sandbox-bound")
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
	directory, err := s.openDirectory()
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Replace("state.json", append(data, '\n'), 0600)
}
