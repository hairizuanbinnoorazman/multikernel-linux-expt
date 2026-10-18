package rootfs

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/containerd/containerd/mount"
)

func repeatedLayers(count int) string {
	return "lowerdir=" + strings.TrimSuffix(strings.Repeat("/a:", count), ":")
}

func TestRootfsMountBudgetBeforeFilesystemAccess(t *testing.T) {
	for _, item := range []struct {
		name  string
		input []Mount
	}{
		{"layer-count", []Mount{{Type: "overlay", Source: "overlay", Options: []string{repeatedLayers(129)}}}},
		{"upper-counted", []Mount{{Type: "overlay", Source: "overlay", Options: []string{repeatedLayers(128), "upperdir=/u", "workdir=/w"}}}},
		{"aggregate", []Mount{{Type: "overlay", Source: "overlay", Options: []string{repeatedLayers(128)}}, {Type: "overlay", Source: "overlay", Options: []string{repeatedLayers(128)}}, {Type: "bind", Source: "/absent"}}},
		{"duplicate-lower", []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/first", "lowerdir=/second"}}}},
		{"duplicate-upper", []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/l", "upperdir=/u1", "upperdir=/u2", "workdir=/w"}}}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if err := ValidateMounts(item.input); !errors.Is(err, errMountBudget) {
				t.Fatalf("budget did not precede filesystem access: %v", err)
			}
			mounts, files, err := pinRootfsMounts(context.Background(), item.input)
			for _, file := range files {
				file.Close()
			}
			if !errors.Is(err, errMountBudget) || mounts != nil || len(files) != 0 {
				t.Fatalf("oversized request pinned filesystem inputs: mounts=%v files=%d err=%v", mounts, len(files), err)
			}
		})
	}
	if err := validateMountBudget([]Mount{{Type: "overlay", Options: []string{repeatedLayers(127), "upperdir=/u", "workdir=/w"}}, {Type: "overlay", Options: []string{repeatedLayers(128)}}}); err != nil {
		t.Fatal("boundary request rejected:", err)
	}
}

func TestRootfsMountExecCannotOverrideNoexec(t *testing.T) {
	root := t.TempDir()
	mounts, files, err := pinRootfsMounts(context.Background(), []Mount{{Type: "bind", Source: root, Options: []string{"bind", "noexec", "exec", "rw", "dev", "suid"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, file := range files {
			file.Close()
		}
	}()
	for _, flag := range []string{"exec", "rw", "dev", "suid"} {
		if slices.Contains(mounts[0].Options, flag) {
			t.Fatalf("loosening flag retained: %s", flag)
		}
	}
	for _, flag := range []string{"ro", "nodev", "nosuid", "noexec"} {
		if !slices.Contains(mounts[0].Options, flag) {
			t.Fatalf("forced flag absent: %s", flag)
		}
	}
}

func TestRootfsSealCancellationDoesNotTouchFilesystem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	file, path, err := readonlyRootfsClone(ctx, filepath.Join(t.TempDir(), "absent"))
	if file != nil || path != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled clone: file=%v path=%q err=%v", file, path, err)
	}
	mounts, files, err := sealRootfsMountInputs(ctx, []mount.Mount{{Type: "bind", Source: "/absent"}})
	if mounts != nil || len(files) != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sealing: mounts=%v files=%d err=%v", mounts, len(files), err)
	}
}
