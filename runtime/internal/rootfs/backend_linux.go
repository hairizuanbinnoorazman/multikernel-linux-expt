//go:build linux

package rootfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd/mount"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

var sha256RE = regexp.MustCompile(`^[a-f0-9]{64}$`)

type LinuxBackend struct {
	Builder      string
	BuildTimeout time.Duration
	mountAll     func([]mount.Mount, string) error
}

const maximumBuilderOutput = 1 << 20

func runBoundedBuilder(ctx context.Context, timeout time.Duration, binary string, arguments, environment []string, maximum int, files ...*os.File) ([]byte, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	output, err := boundedexec.RunWithFiles(ctx, timeout, binary, arguments, environment, maximum, files)
	if errors.Is(err, boundedexec.ErrOutputLimit) {
		return output, errors.Join(err, errors.New("rootfs builder output exceeds limit"))
	}
	return output, err
}

func builderDiagnostic(output []byte) string {
	const maximum = 16 << 10
	if len(output) <= maximum {
		return strings.TrimSpace(string(output))
	}
	return strings.TrimSpace(string(output[:maximum])) + " [truncated]"
}

type storageBuildMetadata struct {
	SchemaVersion      int    `json:"schema_version"`
	Path               string `json:"path"`
	ImageID            string `json:"image_id"`
	FilesystemUUID     string `json:"filesystem_uuid"`
	SizeBytes          uint64 `json:"size_bytes"`
	QuotaBytes         uint64 `json:"quota_bytes"`
	InodeLimit         uint64 `json:"inode_limit"`
	Port               uint32 `json:"port"`
	SHA256             string `json:"sha256"`
	OfflineCheckSHA256 string `json:"offline_check_sha256"`
	Allocation         string `json:"allocation"`
	Format             string `json:"format"`
	Determinism        struct {
		FakeTime           int    `json:"fake_time"`
		HashSeed           string `json:"hash_seed"`
		LazyInitialization bool   `json:"lazy_initialization"`
		SourceMetadataTime int    `json:"source_metadata_time"`
	} `json:"determinism"`
}

type readonlyBindSummary struct {
	Destination    string `json:"destination"`
	Source         string `json:"source"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Ownership      string `json:"ownership"`
	Propagation    string `json:"propagation"`
	GuestPolicy    string `json:"guest_policy"`
}

type readonlyBindManifest struct {
	Destination    string          `json:"destination"`
	Source         string          `json:"source"`
	Manifest       json.RawMessage `json:"manifest"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	Ownership      string          `json:"ownership"`
	Propagation    string          `json:"propagation"`
	GuestPolicy    string          `json:"guest_policy"`
}

type readonlyBindManifestSet struct {
	SchemaVersion int                    `json:"schema_version"`
	ReadonlyBinds []readonlyBindManifest `json:"readonly_binds"`
}

func verifyReadonlyBindManifests(path string, buildResult json.RawMessage) error {
	data, err := readTrustedArtifact(path, 128<<20, true)
	if err != nil {
		return err
	}
	var manifests readonlyBindManifestSet
	if err = protocol.StrictDecode(data, &manifests); err != nil || manifests.SchemaVersion != 1 || len(manifests.ReadonlyBinds) > 8 {
		return errors.New("invalid read-only bind manifest set")
	}
	var result map[string]json.RawMessage
	if err = protocol.StrictDecode(buildResult, &result); err != nil {
		return errors.New("invalid rootfs builder result")
	}
	encodedSummaries, ok := result["readonly_bind_inputs"]
	if !ok {
		return errors.New("rootfs builder result omits read-only bind provenance")
	}
	var summaries []readonlyBindSummary
	if err = protocol.StrictDecode(encodedSummaries, &summaries); err != nil || len(summaries) != len(manifests.ReadonlyBinds) {
		return errors.New("read-only bind summary differs from manifest set")
	}
	seenDestinations := make([]string, 0, len(manifests.ReadonlyBinds))
	for index, manifest := range manifests.ReadonlyBinds {
		summary := readonlyBindSummary{Destination: manifest.Destination, Source: manifest.Source,
			ManifestSHA256: manifest.ManifestSHA256, Ownership: manifest.Ownership,
			Propagation: manifest.Propagation, GuestPolicy: manifest.GuestPolicy}
		if summary != summaries[index] || !filepath.IsAbs(manifest.Source) || filepath.Clean(manifest.Source) != manifest.Source ||
			!filepath.IsAbs(manifest.Destination) || filepath.Clean(manifest.Destination) != manifest.Destination || manifest.Destination == "/" ||
			!sha256RE.MatchString(manifest.ManifestSHA256) || manifest.Ownership != "numeric-uid-gid-preserved" ||
			manifest.Propagation != "none-materialized-copy" || manifest.GuestPolicy != "bind-remount-ro-nodev-nosuid-noexec" {
			return errors.New("read-only bind identity differs from the enforced contract")
		}
		for _, protected := range []string{"/dev", "/proc", "/run", "/sys"} {
			if manifest.Destination == protected || strings.HasPrefix(manifest.Destination, protected+"/") {
				return errors.New("read-only bind destination overlaps a runtime-owned path")
			}
		}
		for _, prior := range seenDestinations {
			if manifest.Destination == prior || strings.HasPrefix(manifest.Destination, prior+"/") || strings.HasPrefix(prior, manifest.Destination+"/") {
				return errors.New("read-only bind destinations overlap")
			}
		}
		seenDestinations = append(seenDestinations, manifest.Destination)
		canonical := append(append([]byte(nil), manifest.Manifest...), '\n')
		digest := sha256.Sum256(canonical)
		if hex.EncodeToString(digest[:]) != manifest.ManifestSHA256 {
			return errors.New("read-only bind manifest digest mismatch")
		}
		var normalized struct {
			SchemaVersion int             `json:"schema_version"`
			Normalization json.RawMessage `json:"normalization"`
			Entries       json.RawMessage `json:"entries"`
		}
		if err = protocol.StrictDecode(manifest.Manifest, &normalized); err != nil || normalized.SchemaVersion != 1 ||
			len(normalized.Normalization) == 0 || len(normalized.Entries) == 0 {
			return errors.New("invalid normalized read-only bind manifest")
		}
	}
	return nil
}

func openTrustedArtifact(path string, maximum int64, private bool) (*os.File, *syscall.Stat_t, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	identity, identityOK := info.Sys().(*syscall.Stat_t)
	unsafeMode := info.Mode().Perm()&0022 != 0
	if private {
		unsafeMode = info.Mode().Perm()&0077 != 0
	}
	if !identityOK || !info.Mode().IsRegular() || unsafeMode || info.Size() <= 0 || info.Size() > maximum ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 {
		return nil, nil, errors.New("artifact must be a bounded caller-owned single-link regular file with safe mode")
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, nil, errors.New("artifact identity changed while opening")
	}
	openedIdentity, ok := opened.Sys().(*syscall.Stat_t)
	if !ok {
		_ = file.Close()
		return nil, nil, errors.New("artifact identity is unavailable")
	}
	return file, openedIdentity, nil
}

func verifyStableArtifact(file *os.File, before *syscall.Stat_t) error {
	afterInfo, err := file.Stat()
	if err != nil {
		return err
	}
	after, ok := afterInfo.Sys().(*syscall.Stat_t)
	if !ok || after.Dev != before.Dev || after.Ino != before.Ino || after.Size != before.Size ||
		after.Mtim != before.Mtim || after.Ctim != before.Ctim {
		return errors.New("artifact changed while reading")
	}
	return nil
}

func readTrustedArtifact(path string, maximum int64, private bool) ([]byte, error) {
	file, before, err := openTrustedArtifact(path, maximum, private)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	stableErr := verifyStableArtifact(file, before)
	closeErr := file.Close()
	if err = errors.Join(readErr, stableErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errors.New("artifact exceeds bound")
	}
	return data, nil
}

func writePrivateExclusive(path string, data []byte) error {
	descriptor, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), path)
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}

func pinRootfsDirectory(path string) (*os.File, string, error) {
	descriptor, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", err
	}
	file := os.NewFile(uintptr(descriptor), path)
	return file, fmt.Sprintf("/proc/self/fd/%d", descriptor), nil
}

func pinRootfsMounts(ctx context.Context, input []Mount) ([]mount.Mount, []*os.File, error) {
	mounts := make([]mount.Mount, len(input))
	files := make([]*os.File, 0, len(input)*2)
	closeFiles := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	pin := func(path string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		file, anchored, err := pinRootfsDirectory(path)
		if err != nil {
			return "", err
		}
		files = append(files, file)
		return anchored, nil
	}
	for i, item := range input {
		if err := ctx.Err(); err != nil {
			closeFiles()
			return nil, nil, err
		}
		source := item.Source
		if item.Type == "bind" || item.Type == "none" {
			var err error
			if source, err = pin(source); err != nil {
				closeFiles()
				return nil, nil, fmt.Errorf("pin rootfs mount source: %w", err)
			}
		}
		options := make([]string, 0, len(item.Options)+3)
		for _, option := range item.Options {
			if option == "rw" || option == "dev" || option == "suid" {
				continue
			}
			rewritten := option
			for _, prefix := range []string{"lowerdir=", "upperdir=", "workdir="} {
				if item.Type != "overlay" || !strings.HasPrefix(option, prefix) {
					continue
				}
				paths := strings.Split(strings.TrimPrefix(option, prefix), ":")
				anchored := make([]string, len(paths))
				for index, path := range paths {
					var err error
					if anchored[index], err = pin(path); err != nil {
						closeFiles()
						return nil, nil, fmt.Errorf("pin overlay %s path: %w", strings.TrimSuffix(prefix, "="), err)
					}
				}
				rewritten = prefix + strings.Join(anchored, ":")
				break
			}
			options = append(options, rewritten)
		}
		for _, required := range []string{"ro", "nodev", "nosuid", "noexec"} {
			if !slices.Contains(options, required) {
				options = append(options, required)
			}
		}
		mounts[i] = mount.Mount{Type: item.Type, Source: source, Options: options}
	}
	return mounts, files, nil
}

func (b *LinuxBackend) Mount(ctx context.Context, input []Mount, target string, expectedTarget DirectoryIdentity) (result error) {
	if err := ctx.Err(); err != nil {
		return errors.Join(errMountNotAttempted, err)
	}
	mounts, files, err := pinRootfsMounts(ctx, input)
	if err != nil {
		return errors.Join(errMountNotAttempted, err)
	}
	targetFile, anchoredTarget, err := pinRootfsDirectory(target)
	if err != nil {
		for _, file := range files {
			_ = file.Close()
		}
		return fmt.Errorf("pin rootfs mount target: %w", errors.Join(errMountNotAttempted, err))
	}
	files = append(files, targetFile)
	defer func() {
		for _, file := range files {
			result = errors.Join(result, file.Close())
		}
	}()
	targetInfo, err := targetFile.Stat()
	if err != nil {
		return fmt.Errorf("inspect rootfs mount target: %w", errors.Join(errMountNotAttempted, err))
	}
	targetIdentity, ok := openedDirectoryIdentity(targetInfo)
	if !ok || targetIdentity != expectedTarget {
		return errors.Join(errMountNotAttempted, errors.New("rootfs mount target identity changed"))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(errMountNotAttempted, err)
	}
	mountAll := b.mountAll
	if mountAll == nil {
		mountAll = mount.All
	}
	if err := mountAll(mounts, anchoredTarget); err != nil {
		return fmt.Errorf("mount read-only source root: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (b *LinuxBackend) Unmount(ctx context.Context, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mount.UnmountAll(target, 0); err != nil {
		return err
	}
	return nil
}

func loadStorageBuild(path, expectedPath string, expectedPort uint32) (protocol.StorageConfig, error) {
	data, err := readTrustedArtifact(path, 1<<20, true)
	if err != nil {
		return protocol.StorageConfig{}, err
	}
	var value storageBuildMetadata
	if err = protocol.StrictDecode(data, &value); err != nil {
		return protocol.StorageConfig{}, err
	}
	if value.SchemaVersion != 1 || value.Path != expectedPath || value.Port != expectedPort ||
		value.Format != "ext4" || value.Allocation != "posix_fallocate" || value.Determinism.FakeTime != 1 ||
		value.Determinism.HashSeed != value.FilesystemUUID || value.Determinism.LazyInitialization ||
		value.Determinism.SourceMetadataTime != 1 ||
		!rootfsImageIDRE.MatchString(value.ImageID) || !rootfsUUIDRE.MatchString(value.FilesystemUUID) ||
		!sha256RE.MatchString(value.SHA256) || !sha256RE.MatchString(value.OfflineCheckSHA256) ||
		value.SizeBytes < 64<<20 || value.SizeBytes > 16<<30 || value.SizeBytes%4096 != 0 ||
		value.QuotaBytes != value.SizeBytes || value.InodeLimit < 128 || value.InodeLimit > 2_097_152 {
		return protocol.StorageConfig{}, errors.New("builder storage identity differs from the enforced v1 contract")
	}
	return protocol.StorageConfig{Path: value.Path, ImageID: value.ImageID, FilesystemUUID: value.FilesystemUUID,
		SizeBytes: value.SizeBytes, QuotaBytes: value.QuotaBytes, InodeLimit: value.InodeLimit, Port: value.Port, SHA256: value.SHA256}, nil
}

func (b *LinuxBackend) Build(ctx context.Context, request PrepareRequest, roots BuildRoots) (PrepareResult, error) {
	if b.Builder == "" || !filepath.IsAbs(b.Builder) {
		return PrepareResult{}, errors.New("rootfs builder must be an absolute path")
	}
	if roots.Bundle == nil || roots.RuntimeDir == nil || roots.StorageDir == nil || roots.BundlePath != request.Bundle ||
		roots.RuntimeDirPath != filepath.Join(request.Bundle, ".multikernel") ||
		!filepath.IsAbs(roots.StorageDirPath) || filepath.Clean(roots.StorageDirPath) != roots.StorageDirPath ||
		filepath.Base(roots.StorageDirPath) != request.TaskIdentity {
		return PrepareResult{}, errors.New("rootfs build roots do not match the request")
	}
	for label, file := range map[string]*os.File{"bundle": roots.Bundle, "runtime": roots.RuntimeDir, "storage": roots.StorageDir} {
		info, err := file.Stat()
		if err != nil {
			return PrepareResult{}, fmt.Errorf("inspect %s build root: %w", label, err)
		}
		identity, ok := openedDirectoryIdentity(info)
		private := label != "bundle" && info.Mode().Perm() != 0700
		if !ok || !info.IsDir() || identity.UID != uint32(os.Geteuid()) || private {
			return PrepareResult{}, fmt.Errorf("%s build root is not a caller-owned directory with safe mode", label)
		}
	}
	files := []*os.File{roots.Bundle, roots.RuntimeDir, roots.StorageDir}
	childBundle := "/proc/self/fd/3"
	childRuntimeDir := "/proc/self/fd/4"
	childStorageDir := "/proc/self/fd/5"
	runtimeDir := fmt.Sprintf("/proc/self/fd/%d", roots.RuntimeDir.Fd())
	storageDir := fmt.Sprintf("/proc/self/fd/%d", roots.StorageDir.Fd())
	childInitrd := filepath.Join(childRuntimeDir, "initramfs.cpio.gz")
	storagePath := filepath.Join(storageDir, "root.ext4")
	logicalStoragePath := filepath.Join(roots.StorageDirPath, "root.ext4")
	environment := append(os.Environ(), "MK_TASK_IDENTITY="+request.TaskIdentity,
		"MK_STORAGE_PORT="+strconv.FormatUint(uint64(request.StoragePort), 10),
		"MK_STORAGE_OUTPUT="+filepath.Join(childStorageDir, "root.ext4"),
		"MK_STORAGE_LOGICAL_OUTPUT="+logicalStoragePath, "MK_LOGICAL_BUNDLE="+request.Bundle)
	output, err := runBoundedBuilder(ctx, b.BuildTimeout, b.Builder, []string{childBundle, childInitrd}, environment, maximumBuilderOutput, files...)
	if err != nil {
		return PrepareResult{}, fmt.Errorf("build child root: %w: %s", err, builderDiagnostic(output))
	}
	for required, maximum := range map[string]int64{
		"initramfs.cpio.gz": 16 << 30, "initramfs.manifest.json": 16 << 20,
		"initramfs.source-manifest.json": 16 << 20, "initramfs.readonly-binds.manifest.json": 128 << 20,
		"storage.json": 1 << 20,
	} {
		private := required == "storage.json" || required == "initramfs.readonly-binds.manifest.json"
		file, _, openErr := openTrustedArtifact(filepath.Join(runtimeDir, required), maximum, private)
		if openErr != nil {
			return PrepareResult{}, fmt.Errorf("builder did not produce safe required %s: %w", required, openErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return PrepareResult{}, fmt.Errorf("close required artifact %s: %w", required, closeErr)
		}
	}
	storage, err := loadStorageBuild(filepath.Join(runtimeDir, "storage.json"), logicalStoragePath, request.StoragePort)
	if err != nil {
		return PrepareResult{}, err
	}
	storageDigest, err := fileSHA256(ctx, storagePath, storage.SizeBytes)
	if err != nil {
		return PrepareResult{}, fmt.Errorf("verify builder storage image: %w", err)
	}
	if storageDigest != storage.SHA256 {
		return PrepareResult{}, errors.New("builder storage image differs from its declared identity")
	}
	if !json.Valid(output) {
		return PrepareResult{}, errors.New("rootfs builder result is not JSON")
	}
	if err = verifyReadonlyBindManifests(filepath.Join(runtimeDir, "initramfs.readonly-binds.manifest.json"), output); err != nil {
		return PrepareResult{}, fmt.Errorf("verify read-only bind manifests: %w", err)
	}
	if err = writePrivateExclusive(filepath.Join(runtimeDir, "build-result.json"), output); err != nil {
		return PrepareResult{}, err
	}
	if err = writePrivateExclusive(filepath.Join(runtimeDir, "initramfs.path"), []byte(filepath.Join(roots.RuntimeDirPath, "initramfs.cpio.gz")+"\n")); err != nil {
		return PrepareResult{}, err
	}
	return PrepareResult{Storage: storage, BuildResult: append(json.RawMessage(nil), output...)}, nil
}

func fileSHA256(ctx context.Context, path string, expectedSize uint64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, before, err := openTrustedArtifact(path, 16<<30, true)
	if err != nil {
		return "", err
	}
	if before.Size < 0 || uint64(before.Size) != expectedSize || uint64(before.Blocks)*512 < expectedSize {
		_ = file.Close()
		return "", errors.New("storage artifact size or allocation differs from its declared quota")
	}
	hash := sha256.New()
	buffer := make([]byte, 1<<20)
	var copyErr error
	for {
		if copyErr = ctx.Err(); copyErr != nil {
			break
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			if _, copyErr = hash.Write(buffer[:n]); copyErr != nil {
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				copyErr = readErr
			}
			break
		}
	}
	stableErr := verifyStableArtifact(file, before)
	return hex.EncodeToString(hash.Sum(nil)), errors.Join(copyErr, stableErr, file.Close())
}

func buildResultDigest(buildResult json.RawMessage, section, field string) (string, error) {
	var result map[string]json.RawMessage
	if err := protocol.StrictDecode(buildResult, &result); err != nil {
		return "", errors.New("invalid durable rootfs build result")
	}
	encodedSection, ok := result[section]
	if !ok {
		return "", fmt.Errorf("rootfs build result omits %s", section)
	}
	var values map[string]json.RawMessage
	if err := protocol.StrictDecode(encodedSection, &values); err != nil {
		return "", fmt.Errorf("invalid rootfs build result section %s", section)
	}
	encodedDigest, ok := values[field]
	if !ok {
		return "", fmt.Errorf("rootfs build result omits %s.%s", section, field)
	}
	var digest string
	if err := protocol.StrictDecode(encodedDigest, &digest); err != nil || !sha256RE.MatchString(digest) {
		return "", fmt.Errorf("invalid rootfs build result digest %s.%s", section, field)
	}
	return digest, nil
}

func (b *LinuxBackend) VerifyPrepared(ctx context.Context, record Record, roots PreparedRoots) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.Storage == nil || record.Storage.Path != filepath.Join(record.StorageDir, "root.ext4") {
		return errors.New("prepared storage path differs from journal")
	}
	if roots.RuntimeDir == nil || roots.StorageDir == nil {
		return errors.New("prepared artifact roots are unavailable")
	}
	runtimeDir := fmt.Sprintf("/proc/self/fd/%d", roots.RuntimeDir.Fd())
	storageDir := fmt.Sprintf("/proc/self/fd/%d", roots.StorageDir.Fd())
	storagePath := filepath.Join(storageDir, "root.ext4")
	for path, maximum := range map[string]int64{
		storagePath: 16 << 30, filepath.Join(runtimeDir, "initramfs.cpio.gz"): 16 << 30,
		filepath.Join(runtimeDir, "initramfs.manifest.json"):                16 << 20,
		filepath.Join(runtimeDir, "initramfs.source-manifest.json"):         16 << 20,
		filepath.Join(runtimeDir, "storage.json"):                           1 << 20,
		filepath.Join(runtimeDir, "initramfs.readonly-binds.manifest.json"): 128 << 20,
		filepath.Join(runtimeDir, "build-result.json"):                      1 << 20,
		filepath.Join(runtimeDir, "initramfs.path"):                         1 << 20,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, _, err := openTrustedArtifact(path, maximum, true)
		if err != nil {
			return fmt.Errorf("prepared artifact is missing or unsafe: %s", path)
		}
		if err = file.Close(); err != nil {
			return fmt.Errorf("close prepared artifact %s: %w", path, err)
		}
	}
	digest, err := fileSHA256(ctx, storagePath, record.Storage.SizeBytes)
	if err != nil {
		return fmt.Errorf("verify prepared storage content: %w", err)
	}
	if digest != record.Storage.SHA256 {
		return errors.New("prepared storage content differs from journal")
	}
	buildResult, err := readTrustedArtifact(filepath.Join(runtimeDir, "build-result.json"), 1<<20, true)
	if err != nil || !bytes.Equal(buildResult, record.BuildResult) {
		return errors.New("prepared build result differs from journal")
	}
	initrdPath, err := readTrustedArtifact(filepath.Join(runtimeDir, "initramfs.path"), 1<<20, true)
	if err != nil || string(initrdPath) != filepath.Join(record.RuntimeDir, "initramfs.cpio.gz")+"\n" {
		return errors.New("prepared initramfs path differs from journal")
	}
	initrd := filepath.Join(runtimeDir, "initramfs.cpio.gz")
	initrdFile, initrdIdentity, err := openTrustedArtifact(initrd, 16<<30, true)
	if err != nil {
		return errors.New("prepared initramfs is missing or unsafe")
	}
	if err = initrdFile.Close(); err != nil {
		return err
	}
	initrdDigest, err := fileSHA256(ctx, initrd, uint64(initrdIdentity.Size))
	if err != nil {
		return fmt.Errorf("verify prepared initramfs content: %w", err)
	}
	generatedInitrd, err := buildResultDigest(record.BuildResult, "generated_bootstrap", "initramfs_sha256")
	if err != nil || initrdDigest != generatedInitrd {
		return errors.New("prepared initramfs digest differs from build result")
	}
	verifiedInitrd, err := buildResultDigest(record.BuildResult, "verified_bootstrap", "archive_sha256")
	if err != nil || initrdDigest != verifiedInitrd {
		return errors.New("prepared initramfs digest differs from verification result")
	}
	manifest, err := readTrustedArtifact(filepath.Join(runtimeDir, "initramfs.manifest.json"), 16<<20, true)
	if err != nil {
		return errors.New("prepared initramfs manifest is missing or unsafe")
	}
	manifestSum := sha256.Sum256(manifest)
	manifestDigest := hex.EncodeToString(manifestSum[:])
	generatedManifest, err := buildResultDigest(record.BuildResult, "generated_bootstrap", "manifest_sha256")
	if err != nil || manifestDigest != generatedManifest {
		return errors.New("prepared initramfs manifest differs from build result")
	}
	verifiedManifest, err := buildResultDigest(record.BuildResult, "verified_bootstrap", "manifest_sha256")
	if err != nil || manifestDigest != verifiedManifest {
		return errors.New("prepared initramfs manifest differs from verification result")
	}
	sourceManifest, err := readTrustedArtifact(filepath.Join(runtimeDir, "initramfs.source-manifest.json"), 16<<20, true)
	if err != nil {
		return errors.New("prepared source manifest is missing or unsafe")
	}
	sourceSum := sha256.Sum256(sourceManifest)
	sourceDigest := hex.EncodeToString(sourceSum[:])
	for _, section := range []string{"source_scan_before", "source_scan_after"} {
		expected, digestErr := buildResultDigest(record.BuildResult, section, "manifest_sha256")
		if digestErr != nil || sourceDigest != expected {
			return fmt.Errorf("prepared source manifest differs from %s", section)
		}
	}
	metadata, err := loadStorageBuild(filepath.Join(runtimeDir, "storage.json"), record.Storage.Path, record.Storage.Port)
	if err != nil || metadata != *record.Storage {
		return errors.New("prepared storage metadata differs from journal")
	}
	if err = verifyReadonlyBindManifests(filepath.Join(runtimeDir, "initramfs.readonly-binds.manifest.json"), record.BuildResult); err != nil {
		return fmt.Errorf("verify prepared read-only bind manifests: %w", err)
	}
	return nil
}
