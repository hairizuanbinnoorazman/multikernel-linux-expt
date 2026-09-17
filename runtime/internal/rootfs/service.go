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
	"syscall"
)

var identityRE = regexp.MustCompile(`^task-[a-f0-9]{32}$`)
var errMountNotAttempted = errors.New("rootfs mount was not attempted")

type Backend interface {
	Mount(context.Context, []Mount, string, DirectoryIdentity) error
	Unmount(context.Context, string) error
	Build(context.Context, PrepareRequest, BuildRoots) (PrepareResult, error)
	VerifyPrepared(context.Context, Record) error
}

type BuildRoots struct {
	Bundle         *os.File
	BundlePath     string
	RuntimeDir     *os.File
	RuntimeDirPath string
	StorageDir     *os.File
	StorageDirPath string
}

type Service struct {
	store       *Store
	backend     Backend
	storageRoot string
	storageID   DirectoryIdentity
	mu          sync.Mutex
}

func openedDirectoryIdentity(info os.FileInfo) (DirectoryIdentity, bool) {
	value, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return DirectoryIdentity{}, false
	}
	return DirectoryIdentity{Device: uint64(value.Dev), Inode: value.Ino, UID: value.Uid}, true
}

func inspectStableRoot(path string) (*os.Root, DirectoryIdentity, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, DirectoryIdentity{}, errors.New("cleanup root must be a real directory")
	}
	identity, identityOK := openedDirectoryIdentity(before)
	if !identityOK || identity.UID != uint32(os.Geteuid()) {
		return nil, DirectoryIdentity{}, errors.New("cleanup root must be owned by the caller")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, DirectoryIdentity{}, err
	}
	after, err := root.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = root.Close()
		return nil, DirectoryIdentity{}, errors.New("cleanup root identity changed while opening")
	}
	return root, identity, nil
}

func openStableRoot(path string) (*os.Root, error) {
	root, _, err := inspectStableRoot(path)
	return root, err
}

func openRelativeDirectory(root *os.Root, name string, private bool) (*os.File, DirectoryIdentity, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, DirectoryIdentity{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, DirectoryIdentity{}, err
	}
	identity, ok := openedDirectoryIdentity(info)
	unsafeMode := private && info.Mode().Perm() != 0700
	if !ok || !info.IsDir() || identity.UID != uint32(os.Geteuid()) || unsafeMode {
		_ = file.Close()
		return nil, DirectoryIdentity{}, errors.New("artifact directory must be a caller-owned real directory with safe mode")
	}
	return file, identity, nil
}

func verifyRootIdentity(path string, expected DirectoryIdentity) error {
	root, observed, err := inspectStableRoot(path)
	if err != nil {
		return err
	}
	err = root.Close()
	if observed != expected {
		return errors.Join(errors.New("cleanup root no longer has its recorded identity"), err)
	}
	return err
}

func removeRelativeTree(rootPath, name string, expected ...DirectoryIdentity) error {
	root, observed, err := inspectStableRoot(rootPath)
	if err != nil {
		return err
	}
	if len(expected) > 0 && observed != expected[0] {
		return errors.Join(errors.New("cleanup root no longer has its recorded identity"), root.Close())
	}
	if len(expected) > 1 {
		info, inspectErr := root.Lstat(name)
		if errors.Is(inspectErr, os.ErrNotExist) {
			return root.Close()
		}
		child, ok := openedDirectoryIdentity(info)
		if inspectErr != nil || !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || child != expected[1] {
			return errors.Join(errors.New("cleanup artifact no longer has its recorded identity"), inspectErr, root.Close())
		}
	}
	err = root.RemoveAll(name)
	return errors.Join(err, root.Close())
}

func (s *Service) removePreparedArtifacts(record Record) error {
	if err := s.validateArtifactPaths(record); err != nil {
		return err
	}
	return errors.Join(
		removeRelativeTree(record.Request.Bundle, ".multikernel", record.BundleID, record.RuntimeID),
		removeRelativeTree(s.storageRoot, record.Request.TaskIdentity, record.StorageID, record.StorageDirID),
	)
}

func (s *Service) cleanupFailedPreparation(record Record) error {
	var failures []error
	if err := s.removePreparedArtifacts(record); err != nil {
		failures = append(failures, fmt.Errorf("remove prepared artifacts: %w", err))
	}
	if err := errors.Join(failures...); err != nil {
		// Keep the durable record so reconciliation can retry any cleanup that
		// could not be proven complete.
		return err
	}
	if err := s.store.Delete(record.Request.TaskIdentity); err != nil {
		failures = append(failures, fmt.Errorf("remove rootfs recovery record: %w", err))
	}
	return errors.Join(failures...)
}

func (s *Service) validateArtifactPaths(record Record) error {
	if record.Root != filepath.Join(record.Request.Bundle, "rootfs") ||
		record.RuntimeDir != filepath.Join(record.Request.Bundle, ".multikernel") ||
		record.StorageDir != filepath.Join(s.storageRoot, record.Request.TaskIdentity) || record.StorageID != s.storageID {
		return errors.New("rootfs recovery paths are not bound to the configured roots")
	}
	return nil
}

func (s *Service) verifyArtifactRootIdentities(record Record) error {
	if err := verifyRootIdentity(record.Request.Bundle, record.BundleID); err != nil {
		return fmt.Errorf("bundle identity: %w", err)
	}
	if err := verifyRootIdentity(s.storageRoot, record.StorageID); err != nil {
		return fmt.Errorf("storage-root identity: %w", err)
	}
	return nil
}

func verifyRelativeDirectoryIdentity(rootPath string, rootID DirectoryIdentity, name string, expected DirectoryIdentity) error {
	root, observed, err := inspectStableRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	if observed != rootID {
		return errors.New("artifact root no longer has its recorded identity")
	}
	info, err := root.Lstat(name)
	if err != nil {
		return errors.New("artifact directory no longer has its recorded identity")
	}
	identity, ok := openedDirectoryIdentity(info)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || identity != expected {
		return errors.New("artifact directory no longer has its recorded identity")
	}
	return nil
}

func (s *Service) verifyArtifactDirectoryIdentities(record Record) error {
	return errors.Join(
		verifyRelativeDirectoryIdentity(record.Request.Bundle, record.BundleID, ".multikernel", record.RuntimeID),
		verifyRelativeDirectoryIdentity(s.storageRoot, record.StorageID, record.Request.TaskIdentity, record.StorageDirID),
	)
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
	info, err := os.Lstat(storageRoot)
	if err != nil {
		return nil, err
	}
	identity, identityOK := info.Sys().(*syscall.Stat_t)
	if !identityOK || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || identity.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("rootfs storage root must be a caller-owned real directory")
	}
	if err := os.Chmod(storageRoot, 0700); err != nil {
		return nil, err
	}
	storageHandle, storageID, err := inspectStableRoot(storageRoot)
	if err != nil {
		return nil, err
	}
	if err = storageHandle.Close(); err != nil {
		return nil, err
	}
	service := &Service{store: store, backend: backend, storageRoot: storageRoot, storageID: storageID}
	for _, record := range store.List() {
		if err := service.validateArtifactPaths(record); err != nil {
			return nil, fmt.Errorf("reject unsafe rootfs recovery record %s: %w", record.Request.TaskIdentity, err)
		}
		if err := validateRequest(record.Request); err != nil {
			return nil, fmt.Errorf("reject invalid rootfs recovery request %s: %w", record.Request.TaskIdentity, err)
		}
		if err := service.verifyArtifactRootIdentities(record); err != nil {
			return nil, fmt.Errorf("reject replaced rootfs bundle %s: %w", record.Request.TaskIdentity, err)
		}
		if record.Phase == "PREPARED" {
			if err := service.verifyArtifactDirectoryIdentities(record); err != nil {
				return nil, fmt.Errorf("reject replaced prepared artifact directory %s: %w", record.Request.TaskIdentity, err)
			}
		}
	}
	return service, nil
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
	return ValidateMounts(request.Mounts)
}

// ValidateMounts applies the complete rootfs mount contract without allocating
// any sandbox or storage state. Callers at an earlier trust boundary can use it
// to reject hostile containerd input before performing external mutations.
func ValidateMounts(mounts []Mount) error {
	if len(mounts) > 8 {
		return errors.New("at most eight rootfs mounts are supported")
	}
	for _, mount := range mounts {
		if mount.Type != "overlay" && mount.Type != "bind" && mount.Type != "none" {
			return fmt.Errorf("unsupported rootfs mount type %q", mount.Type)
		}
		if mount.Source == "" || len(mount.Source) > 4096 || strings.ContainsAny(mount.Source, "\x00\n\r") {
			return errors.New("rootfs mount source is empty, oversized, or unsafe")
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
		seenOptions := make(map[string]struct{}, len(mount.Options))
		for _, option := range mount.Options {
			if option == "" || len(option) > 4096 || strings.ContainsAny(option, "\x00\n\r") {
				return errors.New("rootfs mount option is empty, oversized, or unsafe")
			}
			if _, duplicate := seenOptions[option]; duplicate {
				return fmt.Errorf("duplicate rootfs mount option %q", option)
			}
			seenOptions[option] = struct{}{}
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
	if err := ctx.Err(); err != nil {
		return PrepareResult{}, err
	}
	if err := validateRequest(request); err != nil {
		return PrepareResult{}, err
	}
	if existing, ok := s.store.Get(request.TaskIdentity); ok {
		if !reflect.DeepEqual(existing.Request, request) || existing.Phase != "PREPARED" || existing.Storage == nil {
			return PrepareResult{}, errors.New("task identity has conflicting or incomplete rootfs state")
		}
		if err := s.verifyArtifactRootIdentities(existing); err != nil {
			return PrepareResult{}, err
		}
		if err := s.verifyArtifactDirectoryIdentities(existing); err != nil {
			return PrepareResult{}, err
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
	bundleHandle, bundleID, err := inspectStableRoot(request.Bundle)
	if err != nil {
		return PrepareResult{}, err
	}
	defer bundleHandle.Close()
	storageHandle, storageID, err := inspectStableRoot(s.storageRoot)
	if err != nil {
		return PrepareResult{}, err
	}
	defer storageHandle.Close()
	if storageID != s.storageID {
		return PrepareResult{}, errors.New("rootfs storage root no longer has its configured identity")
	}
	if err := bundleHandle.MkdirAll("rootfs", 0711); err != nil {
		return PrepareResult{}, err
	}
	rootInfo, err := bundleHandle.Lstat("rootfs")
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return PrepareResult{}, errors.New("bundle rootfs must be a real directory")
	}
	rootID, ok := openedDirectoryIdentity(rootInfo)
	if !ok || rootID.UID != uint32(os.Geteuid()) {
		return PrepareResult{}, errors.New("bundle rootfs must be owned by the caller")
	}
	if _, err := bundleHandle.Lstat(".multikernel"); !errors.Is(err, os.ErrNotExist) {
		return PrepareResult{}, errors.New("runtime artifact directory already exists")
	}
	if _, err := storageHandle.Lstat(request.TaskIdentity); !errors.Is(err, os.ErrNotExist) {
		return PrepareResult{}, errors.New("storage artifact directory already exists")
	}
	if err := bundleHandle.Mkdir(".multikernel", 0700); err != nil {
		return PrepareResult{}, err
	}
	if err := storageHandle.Mkdir(request.TaskIdentity, 0700); err != nil {
		_ = bundleHandle.Remove(".multikernel")
		return PrepareResult{}, err
	}
	runtimeInfo, err := bundleHandle.Lstat(".multikernel")
	var runtimeID DirectoryIdentity
	var runtimeOK bool
	if err == nil {
		runtimeID, runtimeOK = openedDirectoryIdentity(runtimeInfo)
	}
	storageDirInfo, storageDirErr := storageHandle.Lstat(request.TaskIdentity)
	var storageDirID DirectoryIdentity
	var storageDirOK bool
	if storageDirErr == nil {
		storageDirID, storageDirOK = openedDirectoryIdentity(storageDirInfo)
	}
	if err != nil || !runtimeOK || !runtimeInfo.IsDir() || runtimeInfo.Mode().Perm() != 0700 ||
		storageDirErr != nil || !storageDirOK || !storageDirInfo.IsDir() || storageDirInfo.Mode().Perm() != 0700 {
		return PrepareResult{}, errors.New("new artifact directory identity is unavailable or unsafe")
	}
	record := Record{Version: Version, Request: request, Root: root, RuntimeDir: runtimeDir, StorageDir: storageDir,
		BundleID: bundleID, RootID: rootID, RuntimeID: runtimeID, StorageID: s.storageID, StorageDirID: storageDirID, Phase: "MOUNTING"}
	bundleBuild, openedBundleID, err := openRelativeDirectory(bundleHandle, ".", false)
	if err != nil || openedBundleID != bundleID {
		return PrepareResult{}, errors.Join(errors.New("bundle identity changed before build"), err, s.removePreparedArtifacts(record))
	}
	defer bundleBuild.Close()
	runtimeBuild, openedRuntimeID, err := openRelativeDirectory(bundleHandle, ".multikernel", true)
	if err != nil || openedRuntimeID != runtimeID {
		return PrepareResult{}, errors.Join(errors.New("runtime directory identity changed before build"), err, s.removePreparedArtifacts(record))
	}
	defer runtimeBuild.Close()
	storageBuild, openedStorageDirID, err := openRelativeDirectory(storageHandle, request.TaskIdentity, true)
	if err != nil || openedStorageDirID != storageDirID {
		return PrepareResult{}, errors.Join(errors.New("storage directory identity changed before build"), err, s.removePreparedArtifacts(record))
	}
	defer storageBuild.Close()
	if err := s.verifyArtifactRootIdentities(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.removePreparedArtifacts(record))
	}
	if err := s.verifyArtifactDirectoryIdentities(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.removePreparedArtifacts(record))
	}
	if err := s.store.Put(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.removePreparedArtifacts(record))
	}
	if err := s.verifyArtifactRootIdentities(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
	}
	if err := s.verifyArtifactDirectoryIdentities(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
	}
	if err := s.backend.Mount(ctx, request.Mounts, root, rootID); err != nil {
		if errors.Is(err, errMountNotAttempted) {
			return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
		}
		unmountErr := s.backend.Unmount(context.WithoutCancel(ctx), root)
		if unmountErr != nil {
			return PrepareResult{}, errors.Join(err, fmt.Errorf("rollback uncertain rootfs mount: %w", unmountErr))
		}
		return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
	}
	record.Phase = "MOUNTED"
	if err := s.store.Put(record); err != nil {
		unmountErr := s.backend.Unmount(context.WithoutCancel(ctx), root)
		if unmountErr == nil {
			return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
		}
		return PrepareResult{}, errors.Join(err, unmountErr)
	}
	result, buildErr := s.backend.Build(ctx, request, BuildRoots{
		Bundle: bundleBuild, BundlePath: request.Bundle,
		RuntimeDir: runtimeBuild, RuntimeDirPath: runtimeDir,
		StorageDir: storageBuild, StorageDirPath: storageDir,
	})
	unmountErr := s.backend.Unmount(context.WithoutCancel(ctx), root)
	if unmountErr != nil {
		return PrepareResult{}, errors.Join(buildErr, fmt.Errorf("rootfs unmount failed: %w", unmountErr))
	}
	if buildErr != nil {
		return PrepareResult{}, errors.Join(buildErr, s.cleanupFailedPreparation(record))
	}
	if err := s.verifyArtifactDirectoryIdentities(record); err != nil {
		return PrepareResult{}, err
	}
	record.Phase = "PREPARED"
	record.Storage = &result.Storage
	record.BuildResult = append([]byte(nil), result.BuildResult...)
	if err := s.store.Put(record); err != nil {
		return PrepareResult{}, errors.Join(err, s.cleanupFailedPreparation(record))
	}
	return result, nil
}

func (s *Service) Cleanup(ctx context.Context, request CleanupRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if err := s.validateArtifactPaths(record); err != nil {
		return err
	}
	if err := s.verifyArtifactRootIdentities(record); err != nil {
		return err
	}
	if err := s.verifyArtifactDirectoryIdentities(record); err != nil {
		return err
	}
	if err := s.backend.Unmount(ctx, record.Root); err != nil {
		return err
	}
	if err := s.removePreparedArtifacts(record); err != nil {
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
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, record := range s.store.List() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.validateArtifactPaths(record); err != nil {
			return err
		}
		if err := s.verifyArtifactRootIdentities(record); err != nil {
			return err
		}
		switch record.Phase {
		case "MOUNTING", "MOUNTED":
			if err := s.backend.Unmount(ctx, record.Root); err != nil {
				return fmt.Errorf("recover partial rootfs mount: %w", err)
			}
			if err := s.removePreparedArtifacts(record); err != nil {
				return err
			}
			if err := s.store.Delete(record.Request.TaskIdentity); err != nil {
				return err
			}
		case "PREPARED":
			if record.Storage == nil {
				return errors.New("prepared rootfs has no storage identity")
			}
			if err := s.verifyArtifactDirectoryIdentities(record); err != nil {
				return err
			}
			if digest, owned := storageOwners[record.Storage.Path]; !owned || digest != record.Storage.SHA256 {
				if err := s.backend.Unmount(ctx, record.Root); err != nil {
					return fmt.Errorf("unmount orphaned rootfs: %w", err)
				}
				if err := s.removePreparedArtifacts(record); err != nil {
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
