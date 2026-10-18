package rootfs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func requireSnapshotMountFixture(t *testing.T) {
	if os.Getenv("MK_ROOTFS_SNAPSHOT_PRIVILEGED") != "1" {
		t.Skip("explicit private-namespace snapshot fixture required")
	}
	if os.Geteuid() != 0 {
		t.Fatal("fixture requires root in a private mount namespace")
	}
	current, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := os.Readlink("/proc/1/ns/mnt")
	if err != nil || current == initial {
		t.Fatal("fixture refuses the primary mount namespace")
	}
}

// Inspect with NOATIME so proof collection does not alter its own canaries.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	var walk func(string)
	walk = func(path string) {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		stat := info.Sys().(*syscall.Stat_t)
		var dataHash string
		if info.Mode().IsRegular() {
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOATIME|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			file := os.NewFile(uintptr(fd), path)
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			dataHash = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		var xattrs = map[string]string{}
		names := make([]byte, 65536)
		n, err := unix.Listxattr(path, names)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range splitXattrNames(names[:n]) {
			value := make([]byte, 65536)
			count, err := unix.Getxattr(path, name, value)
			if err != nil {
				t.Fatal(err)
			}
			xattrs[name] = fmt.Sprintf("%x", value[:count])
		}
		encoded, err := json.Marshal(struct {
			Stat  *syscall.Stat_t
			SHA   string
			Attrs map[string]string
		}{stat, dataHash, xattrs})
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		result[relative] = string(encoded)
		if info.IsDir() {
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOATIME|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			file := os.NewFile(uintptr(fd), path)
			entries, err := file.ReadDir(-1)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, entry := range entries {
				walk(filepath.Join(path, entry.Name()))
			}
		}
	}
	walk(root)
	return result
}

func splitXattrNames(buffer []byte) []string {
	var result []string
	start := 0
	for index, value := range buffer {
		if value == 0 {
			result = append(result, string(buffer[start:index]))
			start = index + 1
		}
	}
	return result
}

func TestSnapshotOverlayPreservesCallerMetadataPrivileged(t *testing.T) {
	requireSnapshotMountFixture(t)
	root := t.TempDir()
	paths := map[string]string{}
	for _, name := range []string{"lower-a", "lower-b", "upper", "work", "target"} {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		paths[name] = path
	}
	write := func(base, name, value string) {
		t.Helper()
		path := filepath.Join(paths[base], name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("lower-b", "shared", "lower-b")
	write("lower-a", "shared", "lower-a")
	write("upper", "shared", "upper")
	write("lower-a", "removed", "lower-visible")
	write("lower-a", "opaque/old", "hidden")
	write("upper", "opaque/new", "visible")
	if err := unix.Mknod(filepath.Join(paths["upper"], "removed"), unix.S_IFCHR|0600, 0); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(filepath.Join(paths["upper"], "opaque"), "trusted.overlay.opaque", []byte("y"), 0); err != nil {
		t.Fatal(err)
	}
	before := map[string]map[string]string{}
	for _, name := range []string{"lower-a", "lower-b", "upper", "work"} {
		before[name] = snapshotTree(t, paths[name])
	}
	info, err := os.Stat(paths["target"])
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := openedDirectoryIdentity(info)
	backend := &LinuxBackend{}
	defer unix.Unmount(paths["target"], unix.MNT_DETACH)
	err = backend.Mount(context.Background(), []Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=" + paths["lower-a"] + ":" + paths["lower-b"], "upperdir=" + paths["upper"], "workdir=" + paths["work"], "rw", "index=on"}}}, paths["target"], identity)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ name, value string }{{"shared", "upper"}, {"opaque/new", "visible"}} {
		data, err := os.ReadFile(filepath.Join(paths["target"], item.name))
		if err != nil || string(data) != item.value {
			t.Fatalf("view %s=%q error=%v", item.name, data, err)
		}
	}
	for _, name := range []string{"removed", "opaque/old"} {
		if _, err := os.Stat(filepath.Join(paths["target"], name)); !os.IsNotExist(err) {
			t.Fatalf("hidden entry %s remained visible: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths["target"], "new"), []byte("mutation"), 0600); !errors.Is(err, syscall.EROFS) {
		t.Fatalf("view write=%v", err)
	}
	if err := unix.Unmount(paths["target"], 0); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lower-a", "lower-b", "upper", "work"} {
		after := snapshotTree(t, paths[name])
		if !reflect.DeepEqual(before[name], after) {
			t.Fatalf("caller snapshot %s metadata/content changed: before=%v after=%v", name, before[name], after)
		}
	}
	if err := os.WriteFile(filepath.Join(paths["lower-a"], "caller-still-writable"), []byte("owned"), 0600); err != nil {
		t.Fatalf("caller mount was altered: %v", err)
	}
	t.Log("PASS: precedence, whiteout, opaque directory, EROFS and complete caller stat/xattr/content preservation including atime; caller mount remains writable")
}

func TestSnapshotRecursiveBindSealsSubmountsPrivileged(t *testing.T) {
	requireSnapshotMountFixture(t)
	root := t.TempDir()
	source := filepath.Join(root, "source")
	nested := filepath.Join(source, "nested")
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("tmpfs", nested, "tmpfs", unix.MS_NODEV|unix.MS_NOSUID, "size=1m"); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(nested, unix.MNT_DETACH)
	if err := os.WriteFile(filepath.Join(nested, "canary"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, source)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := openedDirectoryIdentity(info)
	defer unix.Unmount(target, unix.MNT_DETACH)
	if err := (&LinuxBackend{}).Mount(context.Background(), []Mount{{Type: "bind", Source: source, Options: []string{"rbind", "rw", "relatime"}}}, target, identity); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "nested/canary")); err != nil || string(data) != "original" {
		t.Fatalf("submount view=%q error=%v", data, err)
	}
	if err := os.WriteFile(filepath.Join(target, "nested/canary"), []byte("mutation"), 0600); !errors.Is(err, syscall.EROFS) {
		t.Fatalf("submount write=%v", err)
	}
	if err := unix.Unmount(target, unix.MNT_DETACH); err != nil {
		t.Fatal(err)
	}
	if after := snapshotTree(t, source); !reflect.DeepEqual(before, after) {
		t.Fatalf("caller bind tree changed: before=%v after=%v", before, after)
	}
	if err := os.WriteFile(filepath.Join(nested, "canary"), []byte("caller-write"), 0600); err != nil {
		t.Fatalf("original nested mount altered: %v", err)
	}
	t.Log("PASS: recursive submount EROFS, caller stat/xattr/content preservation and original nested mount writable")
}
