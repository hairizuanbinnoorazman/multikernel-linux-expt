package hostconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func fixture(t *testing.T) (string, uint32) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	uid := os.Getuid()
	for _, directory := range []string{"etc", "var/lib", "run", "opt/kerf/bin", "sys/fs/multikernel", "etc/kernels"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0755); err != nil {
			t.Fatal(err)
		}
	}
	kerf := filepath.Join(root, "opt/kerf/bin/kerf")
	if err := os.WriteFile(kerf, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(`{"schema_version":1,"state_directory":%q,"socket_path":%q,"kerf_executable":%q,"multikernel_sysfs_root":%q,"kernel_manifest_directory":%q,"min_primary_cpus":4,"min_primary_memory_bytes":8589934592,"forbidden_apic_ids":[0],"backend_timeout_seconds":30,"max_frame_size_bytes":1048576}`,
		filepath.Join(root, "var/lib/mkruntime"), filepath.Join(root, "run/mkruntimed.sock"), kerf,
		filepath.Join(root, "sys/fs/multikernel"), filepath.Join(root, "etc/kernels"))
	path := filepath.Join(root, "etc/config.json")
	if err := os.WriteFile(path, []byte(config), 0640); err != nil {
		t.Fatal(err)
	}
	return path, uint32(uid)
}

func TestStrictSafeHostConfiguration(t *testing.T) {
	path, uid := fixture(t)
	root := filepath.Dir(filepath.Dir(path))
	config, err := load(path, uid, root)
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxFrameSizeBytes != 1<<20 || config.BackendTimeoutSeconds != 30 {
		t.Fatalf("config = %+v", config)
	}
}

func TestHostConfigurationAcceptsManagedGenerationSymlink(t *testing.T) {
	path, uid := fixture(t)
	root := filepath.Dir(filepath.Dir(path))
	target := filepath.Join(root, "etc/multikernel/deployments", strings.Repeat("a", 64), "mkruntime/config.json")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("multikernel/deployments", strings.Repeat("a", 64), "mkruntime/config.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path, uid, root); err != nil {
		t.Fatal(err)
	}
}

func TestHostConfigurationRejectsSymlinkIntoUnsafeParent(t *testing.T) {
	path, uid := fixture(t)
	root := filepath.Dir(filepath.Dir(path))
	targetDir := filepath.Join(root, "unsafe")
	if err := os.Mkdir(targetDir, 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(targetDir, 0777); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "config.json")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := load(path, uid, root); err == nil {
		t.Fatal("configuration below unsafe symlink target parent accepted")
	}
}

func TestHostConfigurationRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) error
	}{
		{"mode", func(path string) error { return os.Chmod(path, 0666) }},
		{"unsafe-parent", func(path string) error { return os.Chmod(filepath.Dir(path), 0777) }},
		{"unknown-field", func(path string) error {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(path, []byte(strings.Replace(string(raw), "{", `{"unknown":true,`, 1)), 0640)
		}},
		{"duplicate-field", func(path string) error {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(path, []byte(strings.Replace(string(raw), "{", `{"schema_version":1,`, 1)), 0640)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, uid := fixture(t)
			if err := test.mutate(path); err != nil {
				t.Fatal(err)
			}
			if _, err := load(path, uid, filepath.Dir(filepath.Dir(path))); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestHostConfigurationOwnerIsEnforced(t *testing.T) {
	path, uid := fixture(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := info.Sys().(*syscall.Stat_t); !ok {
		t.Skip("ownership unavailable")
	}
	if _, err = load(path, uid+1, filepath.Dir(filepath.Dir(path))); err == nil {
		t.Fatal("wrong owner accepted")
	}
}

func TestHostConfigurationExecutableTrust(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string, string) error
		accept bool
	}{
		{"writable-parent", func(root, executable string) error {
			return os.Chmod(filepath.Dir(executable), 0777)
		}, false},
		{"writable-ancestor", func(root, executable string) error {
			return os.Chmod(filepath.Join(root, "opt"), 0777)
		}, false},
		{"not-executable", func(root, executable string) error {
			return os.Chmod(executable, 0644)
		}, false},
		{"empty", func(root, executable string) error {
			return os.Truncate(executable, 0)
		}, false},
		{"directory", func(root, executable string) error {
			if err := os.Remove(executable); err != nil {
				return err
			}
			return os.Mkdir(executable, 0755)
		}, false},
		{"fifo", func(root, executable string) error {
			if err := os.Remove(executable); err != nil {
				return err
			}
			return syscall.Mkfifo(executable, 0700)
		}, false},
		{"hardlink", func(root, executable string) error {
			return os.Link(executable, executable+".alias")
		}, false},
		{"managed-relative-link", func(root, executable string) error {
			if err := os.Rename(executable, executable+".generation"); err != nil {
				return err
			}
			return os.Symlink("kerf.generation", executable)
		}, true},
		{"managed-directory-link", func(root, executable string) error {
			parent := filepath.Dir(executable)
			if err := os.Rename(parent, parent+".generation"); err != nil {
				return err
			}
			return os.Symlink("bin.generation", parent)
		}, true},
		{"unsafe-link-target-parent", func(root, executable string) error {
			unsafe := filepath.Join(root, "unsafe")
			if err := os.Mkdir(unsafe, 0755); err != nil {
				return err
			}
			if err := os.Chmod(unsafe, 0777); err != nil {
				return err
			}
			if err := os.Rename(executable, filepath.Join(unsafe, "kerf")); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(unsafe, "kerf"), executable)
		}, false},
		{"unsafe-intermediate-link", func(root, executable string) error {
			unsafe := filepath.Join(root, "unsafe")
			if err := os.Mkdir(unsafe, 0755); err != nil {
				return err
			}
			if err := os.Chmod(unsafe, 0777); err != nil {
				return err
			}
			if err := os.Rename(executable, executable+".generation"); err != nil {
				return err
			}
			if err := os.Symlink(executable+".generation", filepath.Join(unsafe, "redirect")); err != nil {
				return err
			}
			return os.Symlink(filepath.Join(unsafe, "redirect"), executable)
		}, false},
		{"symlink-cycle", func(root, executable string) error {
			if err := os.Remove(executable); err != nil {
				return err
			}
			return os.Symlink("kerf", executable)
		}, false},
		{"symlink-escape", func(root, executable string) error {
			outside := filepath.Join(t.TempDir(), "kerf")
			if err := os.Rename(executable, outside); err != nil {
				return err
			}
			return os.Symlink(outside, executable)
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, uid := fixture(t)
			root := filepath.Dir(filepath.Dir(path))
			executable := filepath.Join(root, "opt/kerf/bin/kerf")
			if err := test.mutate(root, executable); err != nil {
				t.Fatal(err)
			}
			_, err := load(path, uid, root)
			if (err == nil) != test.accept {
				t.Fatalf("accept=%v, error=%v", test.accept, err)
			}
		})
	}
}
