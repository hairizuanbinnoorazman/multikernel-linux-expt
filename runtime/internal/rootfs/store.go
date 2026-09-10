package rootfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

var rootfsDigestRE = regexp.MustCompile(`^[a-f0-9]{64}$`)
var rootfsUUIDRE = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var rootfsImageIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type diskState struct {
	Version int               `json:"version"`
	Records map[string]Record `json:"records"`
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
		return nil, fmt.Errorf("open rootfs state directory: %w", err)
	}
	defer directory.Close()
	store := &Store{dir: dir, dirIdentity: directory.Identity(), data: diskState{Version: Version, Records: map[string]Record{}}}
	data, found, err := directory.ReadPrivate("state.json", 16<<20)
	if err != nil {
		return nil, fmt.Errorf("read rootfs state: %w", err)
	}
	if !found {
		return store, nil
	}
	if err = protocol.StrictDecode(data, &store.data); err != nil || store.data.Version != Version || store.data.Records == nil {
		return nil, errors.New("rootfs state is malformed or unsupported")
	}
	if err = validateRootfsDiskState(store.data); err != nil {
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
		return nil, errors.New("rootfs state directory identity changed")
	}
	return directory, nil
}

func validateStoredRequest(request PrepareRequest) error {
	if request.Version != Version || !identityRE.MatchString(request.TaskIdentity) || request.StoragePort < 1024 || request.StoragePort > 65535 ||
		!filepath.IsAbs(request.Bundle) || filepath.Clean(request.Bundle) != request.Bundle || len(request.Mounts) > 8 {
		return errors.New("rootfs request identity, version, bundle, mounts, or port is invalid")
	}
	for _, mount := range request.Mounts {
		if mount.Type != "overlay" && mount.Type != "bind" && mount.Type != "none" {
			return errors.New("rootfs mount type is invalid")
		}
		if (mount.Type == "bind" || mount.Type == "none") && (!filepath.IsAbs(mount.Source) || filepath.Clean(mount.Source) != mount.Source) {
			return errors.New("rootfs bind source is invalid")
		}
		if len(mount.Options) > 64 {
			return errors.New("rootfs mount option count is invalid")
		}
		for _, option := range mount.Options {
			if option == "" || len(option) > 4096 || strings.ContainsAny(option, "\x00\n\r") {
				return errors.New("rootfs mount option is invalid")
			}
		}
	}
	return nil
}

func validateStoredStorage(value protocol.StorageConfig, record Record) error {
	if value.Path != filepath.Join(record.StorageDir, "root.ext4") || !rootfsImageIDRE.MatchString(value.ImageID) ||
		!rootfsUUIDRE.MatchString(value.FilesystemUUID) || !rootfsDigestRE.MatchString(value.SHA256) ||
		value.SizeBytes < 64<<20 || value.SizeBytes > 16<<30 || value.SizeBytes%4096 != 0 ||
		value.QuotaBytes != value.SizeBytes || value.InodeLimit < 128 || value.InodeLimit > 2_097_152 ||
		value.Port != record.Request.StoragePort {
		return errors.New("prepared rootfs storage identity is invalid")
	}
	return nil
}

func validateRootfsRecord(key string, record Record) error {
	if record.Version != Version || key != record.Request.TaskIdentity {
		return errors.New("rootfs record key or version is invalid")
	}
	if err := validateStoredRequest(record.Request); err != nil {
		return err
	}
	if record.Root != filepath.Join(record.Request.Bundle, "rootfs") ||
		record.RuntimeDir != filepath.Join(record.Request.Bundle, ".multikernel") ||
		!filepath.IsAbs(record.StorageDir) || filepath.Clean(record.StorageDir) != record.StorageDir ||
		filepath.Base(record.StorageDir) != record.Request.TaskIdentity {
		return errors.New("rootfs artifact paths are not bound to the request")
	}
	switch record.Phase {
	case "MOUNTING", "MOUNTED":
		if record.Storage != nil || len(record.BuildResult) != 0 {
			return errors.New("incomplete rootfs record contains prepared results")
		}
	case "PREPARED":
		if record.Storage == nil || len(record.BuildResult) == 0 || len(record.BuildResult) > 1<<20 || !json.Valid(record.BuildResult) {
			return errors.New("prepared rootfs record has invalid build results")
		}
		if err := validateStoredStorage(*record.Storage, record); err != nil {
			return err
		}
	default:
		return errors.New("rootfs preparation phase is invalid")
	}
	return nil
}

func validateRootfsDiskState(state diskState) error {
	bundles := map[string]string{}
	ports := map[uint32]string{}
	for key, record := range state.Records {
		if err := validateRootfsRecord(key, record); err != nil {
			return fmt.Errorf("invalid durable rootfs record %q: %w", key, err)
		}
		for label, previous := range map[string]string{
			"bundle": bundles[record.Request.Bundle], "storage port": ports[record.Request.StoragePort],
		} {
			if previous != "" && previous != key {
				return fmt.Errorf("rootfs %s is allocated by multiple records", label)
			}
		}
		bundles[record.Request.Bundle] = key
		ports[record.Request.StoragePort] = key
	}
	return nil
}

func cloneRootfsRecord(value Record) Record {
	result := value
	if value.Request.Mounts != nil {
		result.Request.Mounts = make([]Mount, len(value.Request.Mounts))
		for index, mount := range value.Request.Mounts {
			result.Request.Mounts[index] = mount
			if mount.Options != nil {
				result.Request.Mounts[index].Options = append([]string(nil), mount.Options...)
			}
		}
	}
	if value.Storage != nil {
		storage := *value.Storage
		result.Storage = &storage
	}
	result.BuildResult = append(json.RawMessage(nil), value.BuildResult...)
	return result
}

func validRootfsTransition(previous, next Record) bool {
	if previous.Version != next.Version || !reflect.DeepEqual(previous.Request, next.Request) || previous.Root != next.Root ||
		previous.RuntimeDir != next.RuntimeDir || previous.StorageDir != next.StorageDir {
		return false
	}
	if previous.Phase == next.Phase {
		return reflect.DeepEqual(previous, next)
	}
	return previous.Phase == "MOUNTING" && next.Phase == "MOUNTED" ||
		previous.Phase == "MOUNTED" && next.Phase == "PREPARED"
}

func (s *Store) Get(identity string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data.Records[identity]
	return cloneRootfsRecord(value), ok
}
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Record, 0, len(s.data.Records))
	for _, value := range s.data.Records {
		result = append(result, cloneRootfsRecord(value))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Request.TaskIdentity < result[j].Request.TaskIdentity
	})
	return result
}
func (s *Store) Put(value Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value = cloneRootfsRecord(value)
	key := value.Request.TaskIdentity
	if err := validateRootfsRecord(key, value); err != nil {
		return err
	}
	previous, existed := s.data.Records[key]
	if !existed && value.Phase != "MOUNTING" {
		return errors.New("new rootfs ownership must begin in MOUNTING")
	}
	if existed && !validRootfsTransition(previous, value) {
		return errors.New("rootfs identity or phase transition is invalid")
	}
	s.data.Records[key] = value
	if err := validateRootfsDiskState(s.data); err != nil {
		if existed {
			s.data.Records[key] = previous
		} else {
			delete(s.data.Records, key)
		}
		return err
	}
	if err := s.persistLocked(); err != nil {
		if existed {
			s.data.Records[key] = previous
		} else {
			delete(s.data.Records, key)
		}
		return err
	}
	return nil
}
func (s *Store) Delete(identity string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !identityRE.MatchString(identity) {
		return errors.New("rootfs deletion identity is invalid")
	}
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
	directory, err := s.openDirectory()
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Replace("state.json", append(data, '\n'), 0600)
}
