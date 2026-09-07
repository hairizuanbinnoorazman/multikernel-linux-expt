//go:build linux

package rootfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/containerd/containerd/mount"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

var sha256RE = regexp.MustCompile(`^[a-f0-9]{64}$`)

type LinuxBackend struct {
	Builder string
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

func (b *LinuxBackend) Mount(ctx context.Context, input []Mount, target string) error {
	mounts := make([]mount.Mount, len(input))
	for i, item := range input {
		options := make([]string, 0, len(item.Options)+3)
		for _, option := range item.Options {
			if option != "rw" && option != "dev" && option != "suid" {
				options = append(options, option)
			}
		}
		for _, required := range []string{"ro", "nodev", "nosuid", "noexec"} {
			if !slices.Contains(options, required) {
				options = append(options, required)
			}
		}
		mounts[i] = mount.Mount{Type: item.Type, Source: item.Source, Options: options}
	}
	if err := mount.All(mounts, target); err != nil {
		return fmt.Errorf("mount read-only source root: %w", err)
	}
	return nil
}

func (b *LinuxBackend) Unmount(_ context.Context, target string) error {
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

func (b *LinuxBackend) Build(ctx context.Context, request PrepareRequest, runtimeDir, storageDir string) (PrepareResult, error) {
	if b.Builder == "" || !filepath.IsAbs(b.Builder) {
		return PrepareResult{}, errors.New("rootfs builder must be an absolute path")
	}
	initrd := filepath.Join(runtimeDir, "initramfs.cpio.gz")
	storagePath := filepath.Join(storageDir, "root.ext4")
	cmd := exec.CommandContext(ctx, b.Builder, request.Bundle, initrd)
	cmd.Env = append(os.Environ(), "MK_TASK_IDENTITY="+request.TaskIdentity,
		"MK_STORAGE_PORT="+strconv.FormatUint(uint64(request.StoragePort), 10), "MK_STORAGE_OUTPUT="+storagePath)
	output, err := cmd.CombinedOutput()
	if len(output) > 1<<20 {
		return PrepareResult{}, errors.New("rootfs builder output exceeds limit")
	}
	if err != nil {
		return PrepareResult{}, fmt.Errorf("build child root: %w: %s", err, strings.TrimSpace(string(output)))
	}
	for required, maximum := range map[string]int64{
		"initramfs.cpio.gz": 16 << 30, "initramfs.manifest.json": 16 << 20,
		"initramfs.source-manifest.json": 16 << 20, "storage.json": 1 << 20,
	} {
		file, _, openErr := openTrustedArtifact(filepath.Join(runtimeDir, required), maximum, required == "storage.json")
		if openErr != nil {
			return PrepareResult{}, fmt.Errorf("builder did not produce safe required %s", required)
		}
		if closeErr := file.Close(); closeErr != nil {
			return PrepareResult{}, fmt.Errorf("close required artifact %s: %w", required, closeErr)
		}
	}
	storage, err := loadStorageBuild(filepath.Join(runtimeDir, "storage.json"), storagePath, request.StoragePort)
	if err != nil {
		return PrepareResult{}, err
	}
	storageDigest, err := fileSHA256(storagePath, storage.SizeBytes)
	if err != nil || storageDigest != storage.SHA256 {
		return PrepareResult{}, errors.New("builder storage image differs from its declared identity")
	}
	if !json.Valid(output) {
		return PrepareResult{}, errors.New("rootfs builder result is not JSON")
	}
	if err = writePrivateExclusive(filepath.Join(runtimeDir, "build-result.json"), output); err != nil {
		return PrepareResult{}, err
	}
	if err = writePrivateExclusive(filepath.Join(runtimeDir, "initramfs.path"), []byte(initrd+"\n")); err != nil {
		return PrepareResult{}, err
	}
	return PrepareResult{Storage: storage, BuildResult: append(json.RawMessage(nil), output...)}, nil
}

func fileSHA256(path string, expectedSize uint64) (string, error) {
	file, before, err := openTrustedArtifact(path, 16<<30, true)
	if err != nil {
		return "", err
	}
	if before.Size < 0 || uint64(before.Size) != expectedSize || uint64(before.Blocks)*512 < expectedSize {
		_ = file.Close()
		return "", errors.New("storage artifact size or allocation differs from its declared quota")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	stableErr := verifyStableArtifact(file, before)
	return hex.EncodeToString(hash.Sum(nil)), errors.Join(copyErr, stableErr, file.Close())
}

func (b *LinuxBackend) VerifyPrepared(_ context.Context, record Record) error {
	if record.Storage == nil || record.Storage.Path != filepath.Join(record.StorageDir, "root.ext4") {
		return errors.New("prepared storage path differs from journal")
	}
	for path, maximum := range map[string]int64{
		record.Storage.Path: 16 << 30, filepath.Join(record.RuntimeDir, "initramfs.cpio.gz"): 16 << 30,
		filepath.Join(record.RuntimeDir, "storage.json"): 1 << 20,
	} {
		file, _, err := openTrustedArtifact(path, maximum, true)
		if err != nil {
			return fmt.Errorf("prepared artifact is missing or unsafe: %s", path)
		}
		if err = file.Close(); err != nil {
			return fmt.Errorf("close prepared artifact %s: %w", path, err)
		}
	}
	digest, err := fileSHA256(record.Storage.Path, record.Storage.SizeBytes)
	if err != nil || digest != record.Storage.SHA256 {
		return errors.New("prepared storage content differs from journal")
	}
	metadata, err := loadStorageBuild(filepath.Join(record.RuntimeDir, "storage.json"), record.Storage.Path, record.Storage.Port)
	if err != nil || metadata != *record.Storage {
		return errors.New("prepared storage metadata differs from journal")
	}
	return nil
}
