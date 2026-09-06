package rootfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var identityRE = regexp.MustCompile(`^task-[a-f0-9]{32}$`)

type Backend interface {
	Mount(context.Context, []Mount, string) error
	Unmount(context.Context, string) error
	Build(context.Context, PrepareRequest, string, string) (PrepareResult, error)
	VerifyPrepared(context.Context, Record) error
}

type Service struct {
	store       *Store
	backend     Backend
	storageRoot string
	mu          sync.Mutex
}

func NewService(store *Store, backend Backend, storageRoot string) (*Service, error) {
	if store == nil || backend == nil {
		return nil, errors.New("rootfs store and backend are required")
	}
	storageRoot = filepath.Clean(storageRoot)
	if !filepath.IsAbs(storageRoot) {
		return nil, errors.New("rootfs storage root must be absolute")
	}
	if err := os.MkdirAll(storageRoot, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(storageRoot)
	if err != nil || resolved != storageRoot {
		return nil, errors.New("rootfs storage root may not contain symlinks")
	}
	if err := os.Chmod(storageRoot, 0700); err != nil {
		return nil, err
	}
	return &Service{store: store, backend: backend, storageRoot: storageRoot}, nil
}

func validateRequest(request PrepareRequest) error {
	if request.Version != Version || !identityRE.MatchString(request.TaskIdentity) || request.StoragePort < 1024 {
		return errors.New("invalid rootfs request identity, version, or port")
	}
	if !filepath.IsAbs(request.Bundle) || filepath.Clean(request.Bundle) != request.Bundle {
		return errors.New("bundle must be an absolute canonical path")
	}
	resolved, err := filepath.EvalSymlinks(request.Bundle)
	if err != nil || resolved != request.Bundle {
		return errors.New("bundle may not contain symlinks")
	}
	info, err := os.Stat(request.Bundle)
	if err != nil || !info.IsDir() {
		return errors.New("bundle is not a directory")
	}
	if len(request.Mounts) > 8 {
		return errors.New("at most eight rootfs mounts are supported")
	}
	for _, mount := range request.Mounts {
		if mount.Type != "overlay" && mount.Type != "bind" && mount.Type != "none" {
			return fmt.Errorf("unsupported rootfs mount type %q", mount.Type)
		}
		if (mount.Type == "bind" || mount.Type == "none") && (!filepath.IsAbs(mount.Source) || filepath.Clean(mount.Source) != mount.Source) {
			return errors.New("bind rootfs source must be absolute and canonical")
		}
		if mount.Type == "bind" || mount.Type == "none" {
			resolved, err := filepath.EvalSymlinks(mount.Source)
			if err != nil || resolved != mount.Source {
				return errors.New("bind rootfs source may not contain symlinks")
			}
		}
		if len(mount.Options) > 64 {
			return errors.New("too many rootfs mount options")
		}
		for _, option := range mount.Options {
			if option == "" || len(option) > 4096 || strings.ContainsAny(option, "\x00\n\r") {
				return errors.New("rootfs mount option is empty, oversized, or unsafe")
			}
			switch option {
			case "shared", "rshared", "slave", "rslave", "private", "rprivate", "unbindable", "runbindable":
				return errors.New("rootfs propagation changes are unsupported")
			}
			if err := validateMountOption(mount.Type, option); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMountOption(mountType, option string) error {
	if mountType == "bind" || mountType == "none" {
		if slices.Contains([]string{"bind", "rbind", "ro", "rw", "nosuid", "nodev", "noexec", "relatime", "noatime", "strictatime"}, option) {
			return nil
		}
		return fmt.Errorf("unsupported bind rootfs option %q", option)
	}
	if slices.Contains([]string{"ro", "rw", "nosuid", "nodev", "noexec", "index=off", "index=on", "userxattr", "volatile", "metacopy=on", "metacopy=off", "redirect_dir=on", "redirect_dir=off", "xino=on", "xino=off", "xino=auto"}, option) {
		return nil
	}
	for _, prefix := range []string{"lowerdir=", "upperdir=", "workdir="} {
		if !strings.HasPrefix(option, prefix) {
			continue
		}
		paths := strings.Split(strings.TrimPrefix(option, prefix), ":")
		if prefix != "lowerdir=" && len(paths) != 1 {
			return errors.New("overlay writable path option must contain exactly one path")
		}
		for _, path := range paths {
			if !filepath.IsAbs(path) || filepath.Clean(path) != path {
				return errors.New("overlay rootfs paths must be absolute and canonical")
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || resolved != path {
				return errors.New("overlay rootfs paths may not contain symlinks")
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported overlay rootfs option %q", option)
}

func (s *Service) Prepare(ctx context.Context, request PrepareRequest) (PrepareResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateRequest(request); err != nil {
		return PrepareResult{}, err
	}
	if existing, ok := s.store.Get(request.TaskIdentity); ok {
		if !reflect.DeepEqual(existing.Request, request) || existing.Phase != "PREPARED" || existing.Storage == nil {
			return PrepareResult{}, errors.New("task identity has conflicting or incomplete rootfs state")
		}
		if err := s.backend.VerifyPrepared(ctx, existing); err != nil {
			return PrepareResult{}, err
		}
		return PrepareResult{Storage: *existing.Storage, BuildResult: existing.BuildResult}, nil
	}
	for _, existing := range s.store.List() {
		if existing.Request.Bundle == request.Bundle || existing.Request.StoragePort == request.StoragePort {
			return PrepareResult{}, errors.New("bundle or storage port is already prepared for another task")
		}
	}
	root := filepath.Join(request.Bundle, "rootfs")
	runtimeDir := filepath.Join(request.Bundle, ".multikernel")
	storageDir := filepath.Join(s.storageRoot, request.TaskIdentity)
	if err := os.MkdirAll(root, 0711); err != nil {
		return PrepareResult{}, err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return PrepareResult{}, errors.New("bundle rootfs must be a real directory")
	}
	if _, err := os.Lstat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		return PrepareResult{}, errors.New("runtime artifact directory already exists")
	}
	if _, err := os.Lstat(storageDir); !errors.Is(err, os.ErrNotExist) {
		return PrepareResult{}, errors.New("storage artifact directory already exists")
	}
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return PrepareResult{}, err
	}
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		_ = os.Remove(runtimeDir)
		return PrepareResult{}, err
	}
	record := Record{Version: Version, Request: request, Root: root, RuntimeDir: runtimeDir, StorageDir: storageDir, Phase: "MOUNTING"}
	if err := s.store.Put(record); err != nil {
		_ = os.Remove(runtimeDir)
		_ = os.Remove(storageDir)
		return PrepareResult{}, err
	}
	if err := s.backend.Mount(ctx, request.Mounts, root); err != nil {
		_ = s.store.Delete(request.TaskIdentity)
		_ = os.RemoveAll(runtimeDir)
		_ = os.RemoveAll(storageDir)
		return PrepareResult{}, err
	}
	record.Phase = "MOUNTED"
	if err := s.store.Put(record); err != nil {
		unmountErr := s.backend.Unmount(context.WithoutCancel(ctx), root)
		if unmountErr == nil {
			_ = s.store.Delete(request.TaskIdentity)
			_ = os.RemoveAll(runtimeDir)
			_ = os.RemoveAll(storageDir)
		}
		return PrepareResult{}, errors.Join(err, unmountErr)
	}
	result, buildErr := s.backend.Build(ctx, request, runtimeDir, storageDir)
	unmountErr := s.backend.Unmount(context.WithoutCancel(ctx), root)
	if unmountErr != nil {
		return PrepareResult{}, errors.Join(buildErr, fmt.Errorf("rootfs unmount failed: %w", unmountErr))
	}
	if buildErr != nil {
		_ = s.store.Delete(request.TaskIdentity)
		_ = os.RemoveAll(runtimeDir)
		_ = os.RemoveAll(storageDir)
		return PrepareResult{}, buildErr
	}
	record.Phase = "PREPARED"
	record.Storage = &result.Storage
	record.BuildResult = append([]byte(nil), result.BuildResult...)
	if err := s.store.Put(record); err != nil {
		return PrepareResult{}, err
	}
	return result, nil
}

func (s *Service) Cleanup(ctx context.Context, request CleanupRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request.Version != Version || !identityRE.MatchString(request.TaskIdentity) || !sha256RE.MatchString(request.StorageSHA256) {
		return errors.New("invalid rootfs cleanup identity")
	}
	record, ok := s.store.Get(request.TaskIdentity)
	if !ok {
		return nil
	}
	if record.Request.Bundle != request.Bundle || record.Storage == nil || record.Storage.SHA256 != request.StorageSHA256 || record.Phase != "PREPARED" {
		return errors.New("rootfs cleanup identity is stale or conflicting")
	}
	if err := s.backend.Unmount(ctx, record.Root); err != nil {
		return err
	}
	if err := os.RemoveAll(record.RuntimeDir); err != nil {
		return err
	}
	if err := os.RemoveAll(record.StorageDir); err != nil {
		return err
	}
	return s.store.Delete(request.TaskIdentity)
}

// Reconcile repairs interrupted preparations and removes prepared images that
// are not owned by a live lifecycle record. The map binds an exact backing
// path to its content digest, so a reused path cannot inherit stale ownership.
func (s *Service) Reconcile(ctx context.Context, storageOwners map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.store.List() {
		switch record.Phase {
		case "MOUNTING", "MOUNTED":
			if err := s.backend.Unmount(ctx, record.Root); err != nil {
				return fmt.Errorf("recover partial rootfs mount: %w", err)
			}
			if err := os.RemoveAll(record.RuntimeDir); err != nil {
				return err
			}
			if err := os.RemoveAll(record.StorageDir); err != nil {
				return err
			}
			if err := s.store.Delete(record.Request.TaskIdentity); err != nil {
				return err
			}
		case "PREPARED":
			if record.Storage == nil {
				return errors.New("prepared rootfs has no storage identity")
			}
			if digest, owned := storageOwners[record.Storage.Path]; !owned || digest != record.Storage.SHA256 {
				if err := s.backend.Unmount(ctx, record.Root); err != nil {
					return fmt.Errorf("unmount orphaned rootfs: %w", err)
				}
				if err := os.RemoveAll(record.RuntimeDir); err != nil {
					return err
				}
				if err := os.RemoveAll(record.StorageDir); err != nil {
					return err
				}
				if err := s.store.Delete(record.Request.TaskIdentity); err != nil {
					return err
				}
				continue
			}
			if err := s.backend.VerifyPrepared(ctx, record); err != nil {
				return fmt.Errorf("verify prepared rootfs: %w", err)
			}
		default:
			return fmt.Errorf("unsupported rootfs recovery phase %q", record.Phase)
		}
	}
	return nil
}
