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

	"github.com/containerd/containerd/mount"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
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
	} `json:"determinism"`
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
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return protocol.StorageConfig{}, errors.New("builder storage identity must be a private bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return protocol.StorageConfig{}, err
	}
	var value storageBuildMetadata
	if err = protocol.StrictDecode(data, &value); err != nil {
		return protocol.StorageConfig{}, err
	}
	if value.SchemaVersion != 1 || value.Path != expectedPath || value.Port != expectedPort ||
		value.Format != "ext4" || value.Allocation != "posix_fallocate" || value.Determinism.FakeTime != 0 ||
		value.Determinism.HashSeed != value.FilesystemUUID || value.Determinism.LazyInitialization ||
		!sha256RE.MatchString(value.SHA256) || value.SizeBytes == 0 || value.QuotaBytes != value.SizeBytes || value.InodeLimit == 0 {
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
	for _, required := range []string{"initramfs.cpio.gz", "initramfs.manifest.json", "initramfs.source-manifest.json", "storage.json"} {
		info, statErr := os.Lstat(filepath.Join(runtimeDir, required))
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() == 0 {
			return PrepareResult{}, fmt.Errorf("builder did not produce safe required %s", required)
		}
	}
	storage, err := loadStorageBuild(filepath.Join(runtimeDir, "storage.json"), storagePath, request.StoragePort)
	if err != nil {
		return PrepareResult{}, err
	}
	if !json.Valid(output) {
		return PrepareResult{}, errors.New("rootfs builder result is not JSON")
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "build-result.json"), output, 0600); err != nil {
		return PrepareResult{}, err
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "initramfs.path"), []byte(initrd+"\n"), 0600); err != nil {
		return PrepareResult{}, err
	}
	return PrepareResult{Storage: storage, BuildResult: append(json.RawMessage(nil), output...)}, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	return hex.EncodeToString(hash.Sum(nil)), errors.Join(copyErr, file.Close())
}

func (b *LinuxBackend) VerifyPrepared(_ context.Context, record Record) error {
	if record.Storage == nil || record.Storage.Path != filepath.Join(record.StorageDir, "root.ext4") {
		return errors.New("prepared storage path differs from journal")
	}
	for _, path := range []string{record.Storage.Path, filepath.Join(record.RuntimeDir, "initramfs.cpio.gz"), filepath.Join(record.RuntimeDir, "storage.json")} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() == 0 {
			return fmt.Errorf("prepared artifact is missing or unsafe: %s", path)
		}
	}
	digest, err := fileSHA256(record.Storage.Path)
	if err != nil || digest != record.Storage.SHA256 {
		return errors.New("prepared storage content differs from journal")
	}
	metadata, err := loadStorageBuild(filepath.Join(record.RuntimeDir, "storage.json"), record.Storage.Path, record.Storage.Port)
	if err != nil || metadata != *record.Storage {
		return errors.New("prepared storage metadata differs from journal")
	}
	return nil
}
