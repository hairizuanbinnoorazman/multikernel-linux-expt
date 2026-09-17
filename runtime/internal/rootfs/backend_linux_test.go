//go:build linux

package rootfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	containermount "github.com/containerd/containerd/mount"
)

func TestMountPinsFilesystemInputsAcrossPathReplacement(t *testing.T) {
	base := t.TempDir()
	makeSource := func(name, marker string) string {
		t.Helper()
		path := filepath.Join(base, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "marker"), []byte(marker), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	bindSource := makeSource("bind", "bind-original")
	lowerA := makeSource("lower-a", "lower-a-original")
	lowerB := makeSource("lower-b", "lower-b-original")
	upper := makeSource("upper", "upper-original")
	work := makeSource("work", "work-original")
	originals := []string{bindSource, lowerA, lowerB, upper, work}
	wantMarkers := map[string]string{
		"bind-original": "", "lower-a-original": "", "lower-b-original": "",
		"upper-original": "", "work-original": "",
	}
	var anchoredPaths []string
	backend := &LinuxBackend{mountAll: func(mounts []containermount.Mount, target string) error {
		if target != filepath.Join(base, "target") || len(mounts) != 2 {
			return errors.New("unexpected mount request")
		}
		for _, path := range originals {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			if err := os.Mkdir(path, 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(path, "marker"), []byte("replacement"), 0600); err != nil {
				return err
			}
		}
		anchoredPaths = append(anchoredPaths, mounts[0].Source)
		for _, option := range mounts[1].Options {
			for _, prefix := range []string{"lowerdir=", "upperdir=", "workdir="} {
				if strings.HasPrefix(option, prefix) {
					anchoredPaths = append(anchoredPaths, strings.Split(strings.TrimPrefix(option, prefix), ":")...)
				}
			}
		}
		if len(anchoredPaths) != 5 {
			return errors.New("not every mount input was descriptor anchored")
		}
		for _, path := range anchoredPaths {
			if !strings.HasPrefix(path, "/proc/self/fd/") {
				return errors.New("mount input retained its caller pathname")
			}
			marker, err := os.ReadFile(filepath.Join(path, "marker"))
			if err != nil {
				return err
			}
			if _, ok := wantMarkers[string(marker)]; !ok {
				return errors.New("descriptor resolved to replacement content")
			}
			delete(wantMarkers, string(marker))
		}
		if len(wantMarkers) != 0 {
			return errors.New("descriptor identities were not unique and complete")
		}
		for _, required := range []string{"ro", "nodev", "nosuid", "noexec"} {
			if !slices.Contains(mounts[0].Options, required) || !slices.Contains(mounts[1].Options, required) {
				return errors.New("required read-only mount policy missing")
			}
		}
		return nil
	}}
	input := []Mount{
		{Type: "bind", Source: bindSource, Options: []string{"bind", "rw", "dev", "suid"}},
		{Type: "overlay", Source: "overlay", Options: []string{
			"lowerdir=" + lowerA + ":" + lowerB, "upperdir=" + upper, "workdir=" + work,
		}},
	}
	if err := os.Mkdir(filepath.Join(base, "target"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := backend.Mount(context.Background(), input, filepath.Join(base, "target")); err != nil {
		t.Fatal(err)
	}
	for _, path := range anchoredPaths {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("descriptor path remained usable after mount: %s: %v", path, err)
		}
	}
}

func TestMountRejectsSymlinkSubstitutionAtDescriptorOpen(t *testing.T) {
	base := t.TempDir()
	realSource := filepath.Join(base, "real")
	if err := os.Mkdir(realSource, 0700); err != nil {
		t.Fatal(err)
	}
	linkedSource := filepath.Join(base, "linked")
	if err := os.Symlink(realSource, linkedSource); err != nil {
		t.Fatal(err)
	}
	called := false
	backend := &LinuxBackend{mountAll: func([]containermount.Mount, string) error {
		called = true
		return nil
	}}
	err := backend.Mount(context.Background(), []Mount{{Type: "bind", Source: linkedSource}}, filepath.Join(base, "target"))
	if err == nil || called {
		t.Fatalf("symlink source reached mount call: err=%v called=%t", err, called)
	}
}

func TestLoadStorageBuildRequiresExactDeterministicIdentity(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "storage.json")
	image := filepath.Join(directory, "root.ext4")
	value := storageBuildMetadata{SchemaVersion: 1, Path: image, ImageID: "root-abc",
		FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
		QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("a", 64),
		OfflineCheckSHA256: strings.Repeat("b", 64), Allocation: "posix_fallocate", Format: "ext4"}
	value.Determinism.FakeTime = 1
	value.Determinism.HashSeed = value.FilesystemUUID
	value.Determinism.SourceMetadataTime = 1
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := loadStorageBuild(path, image, 4061)
	if err != nil || config.SHA256 != value.SHA256 || config.FilesystemUUID != value.FilesystemUUID {
		t.Fatalf("storage config = %+v, %v", config, err)
	}
	value.Determinism.FakeTime = 0
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("wall-clock fake time accepted")
	}
	value.Determinism.FakeTime = 1
	value.Determinism.SourceMetadataTime = 0
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("unnormalized source metadata accepted")
	}
	value.Determinism.SourceMetadataTime = 1
	value.Determinism.LazyInitialization = true
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("lazy initialization accepted")
	}
	if err = os.WriteFile(path, []byte(`{"schema_version":1,"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
}

func validStorageBuildMetadata(image string) storageBuildMetadata {
	value := storageBuildMetadata{SchemaVersion: 1, Path: image, ImageID: "root-abc",
		FilesystemUUID: "11111111-2222-4333-8444-555555555555", SizeBytes: 64 << 20,
		QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: strings.Repeat("a", 64),
		OfflineCheckSHA256: strings.Repeat("b", 64), Allocation: "posix_fallocate", Format: "ext4"}
	value.Determinism.FakeTime = 1
	value.Determinism.HashSeed = value.FilesystemUUID
	value.Determinism.SourceMetadataTime = 1
	return value
}

func TestLoadStorageBuildRejectsUnsafeFieldsAndFileIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*storageBuildMetadata){
		"image ID":             func(value *storageBuildMetadata) { value.ImageID = "../image" },
		"filesystem UUID":      func(value *storageBuildMetadata) { value.FilesystemUUID = "bad" },
		"offline check digest": func(value *storageBuildMetadata) { value.OfflineCheckSHA256 = "bad" },
		"oversized image": func(value *storageBuildMetadata) {
			value.SizeBytes = 17 << 30
			value.QuotaBytes = value.SizeBytes
		},
		"inode limit": func(value *storageBuildMetadata) { value.InodeLimit = 2_097_153 },
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "storage.json")
			image := filepath.Join(directory, "root.ext4")
			value := validStorageBuildMetadata(image)
			mutate(&value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = loadStorageBuild(path, image, 4061); err == nil {
				t.Fatal("unsafe storage metadata was accepted")
			}
		})
	}

	directory := t.TempDir()
	path := filepath.Join(directory, "storage.json")
	image := filepath.Join(directory, "root.ext4")
	data, err := json.Marshal(validStorageBuildMetadata(image))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(path, path+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err = loadStorageBuild(path, image, 4061); err == nil {
		t.Fatal("hard-linked storage metadata was accepted")
	}
}

func TestExclusiveArtifactWriteRefusesExistingFileAndSymlink(t *testing.T) {
	directory := t.TempDir()
	victim := filepath.Join(directory, "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "result.json")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateExclusive(path, []byte("replacement")); err == nil {
		t.Fatal("exclusive artifact write followed an existing symlink")
	}
	if data, err := os.ReadFile(victim); err != nil || string(data) != "unchanged" {
		t.Fatalf("exclusive write changed victim: %q, %v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateExclusive(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateExclusive(path, []byte("second")); err == nil {
		t.Fatal("exclusive artifact write replaced an existing file")
	}
}

func TestFileSHA256RequiresExactAllocatedPrivateArtifact(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "root.ext4")
	payload := bytes.Repeat([]byte("x"), 4096)
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(payload)
	digest, err := fileSHA256(context.Background(), path, uint64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if digest != hex.EncodeToString(want[:]) {
		t.Fatalf("digest = %s, want %x", digest, want)
	}
	if _, err = fileSHA256(context.Background(), path, uint64(len(payload))+4096); err == nil {
		t.Fatal("artifact with a size different from its declared quota was accepted")
	}

	sparse := filepath.Join(directory, "sparse.ext4")
	file, err := os.OpenFile(sparse, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(64 << 20); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = fileSHA256(context.Background(), sparse, 64<<20); err == nil {
		t.Fatal("sparse artifact without its declared allocation was accepted")
	}

	symlink := filepath.Join(directory, "linked.ext4")
	if err = os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err = fileSHA256(context.Background(), symlink, uint64(len(payload))); err == nil {
		t.Fatal("symlinked artifact was accepted")
	}
}

func TestFileSHA256HonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root.ext4")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 4096), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fileSHA256(ctx, path, 4096); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled hash error = %v", err)
	}
}

func TestVerifyReadonlyBindManifestsBindsBuilderSummaryAndContent(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "initramfs.readonly-binds.manifest.json")
	normalized, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"normalization":  map[string]any{"mtime": 0},
		"entries":        []any{map[string]any{"path": ".", "type": "directory"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(append(append([]byte(nil), normalized...), '\n'))
	summary := readonlyBindSummary{Destination: "/opt/input", Source: "/srv/input",
		ManifestSHA256: hex.EncodeToString(digest[:]), Ownership: "numeric-uid-gid-preserved",
		Propagation: "none-materialized-copy", GuestPolicy: "bind-remount-ro-nodev-nosuid-noexec"}
	manifest := readonlyBindManifest{Destination: summary.Destination, Source: summary.Source,
		Manifest: normalized, ManifestSHA256: summary.ManifestSHA256, Ownership: summary.Ownership,
		Propagation: summary.Propagation, GuestPolicy: summary.GuestPolicy}
	encoded, err := json.Marshal(readonlyBindManifestSet{SchemaVersion: 1, ReadonlyBinds: []readonlyBindManifest{manifest}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	buildResult, err := json.Marshal(map[string]any{"readonly_bind_inputs": []readonlyBindSummary{summary}, "other": true})
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyReadonlyBindManifests(path, buildResult); err != nil {
		t.Fatal(err)
	}

	manifest.ManifestSHA256 = strings.Repeat("0", 64)
	encoded, _ = json.Marshal(readonlyBindManifestSet{SchemaVersion: 1, ReadonlyBinds: []readonlyBindManifest{manifest}})
	if err = os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err = verifyReadonlyBindManifests(path, buildResult); err == nil {
		t.Fatal("changed bind manifest identity was accepted")
	}
}

func TestRunBoundedBuilderDrainsButRejectsOverflow(t *testing.T) {
	output, err := runBoundedBuilder(context.Background(), time.Second, "/bin/sh",
		[]string{"-c", "head -c 4097 /dev/zero"}, os.Environ(), 4096)
	if err == nil || !strings.Contains(err.Error(), "output exceeds limit") {
		t.Fatalf("overflow error = %v", err)
	}
	if len(output) != 4096 {
		t.Fatalf("captured output length = %d", len(output))
	}
}

func TestRunBoundedBuilderKillsProcessGroupAtDeadline(t *testing.T) {
	started := time.Now()
	_, err := runBoundedBuilder(context.Background(), 50*time.Millisecond, "/bin/sh",
		[]string{"-c", "sleep 60 & wait"}, os.Environ(), 4096)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("builder cancellation took %s", elapsed)
	}
}

func TestRunBoundedBuilderCapturesSuccessfulOutput(t *testing.T) {
	output, err := runBoundedBuilder(context.Background(), time.Second, "/bin/sh",
		[]string{"-c", "printf stdout; printf stderr >&2"}, os.Environ(), 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("stdout")) || !bytes.Contains(output, []byte("stderr")) {
		t.Fatalf("combined output = %q", output)
	}
}

func TestBuilderDiagnosticIsBounded(t *testing.T) {
	diagnostic := builderDiagnostic(bytes.Repeat([]byte("x"), 32<<10))
	if len(diagnostic) > 17<<10 || !strings.HasSuffix(diagnostic, "[truncated]") {
		t.Fatalf("diagnostic length/suffix = %d, %q", len(diagnostic), diagnostic[len(diagnostic)-16:])
	}
}
