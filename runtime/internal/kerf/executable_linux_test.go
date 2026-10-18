package kerf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestKerfRefusesUnsafeExecutableDespiteMatchingObservation(t *testing.T) {
	for _, operation := range []string{"create", "load", "start"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			instance := filepath.Join(root, "instances", "box")
			if err := os.MkdirAll(instance, 0700); err != nil {
				t.Fatal(err)
			}
			status := map[string]string{"create": "created", "load": "loaded", "start": "active"}[operation]
			if err := os.WriteFile(filepath.Join(instance, "status"), []byte(status), 0600); err != nil {
				t.Fatal(err)
			}
			script := privateKerfScriptPath(t)
			canary := filepath.Join(root, "executed")
			if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+canary+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Dir(script), 0777); err != nil {
				t.Fatal(err)
			}
			client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
			sandbox := protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{Bundle: t.TempDir(), CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 2}}
			var err error
			switch operation {
			case "create":
				err = client.Create(context.Background(), sandbox)
			case "load":
				err = client.Load(context.Background(), sandbox, "/kernel", "/initrd", "cmdline", LoadFiles{})
			case "start":
				err = client.Start(context.Background(), sandbox)
			}
			_, canaryErr := os.Stat(canary)
			if err == nil || !os.IsNotExist(canaryErr) {
				t.Fatalf("unsafe Kerf accepted: error=%v canary_present=%t", err, canaryErr == nil)
			}
		})
	}
}

func TestKerfPinnedVirtualenvEntryPointPreservesImportsAndDescriptors(t *testing.T) {
	t.Setenv("G7_SYNTHETIC_PARENT_SECRET", "fixture-secret")
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("Python 3 is required for virtualenv launch qualification:", err)
	}
	venv := filepath.Join(root, "venv")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, python, "-m", "venv", "--without-pip", venv).CombinedOutput(); err != nil {
		t.Fatalf("create virtualenv: %v: %s", err, output)
	}
	if err := filepath.WalkDir(venv, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	interpreter := filepath.Join(venv, "bin/python")
	output, err := exec.CommandContext(ctx, interpreter, "-c", "import sysconfig; print(sysconfig.get_path('purelib'))").CombinedOutput()
	if err != nil {
		t.Fatalf("virtualenv purelib: %v: %s", err, output)
	}
	marker := filepath.Join(root, "original-entrypoint")
	module := fmt.Sprintf("import os, sys, site\ndef main():\n assert sys.flags.safe_path\n assert sys.flags.no_user_site\n assert site.ENABLE_USER_SITE is False\n assert sys.argv[0] == '/proc/self/fd/5'\n assert os.read(3,100) == b'original-initramfs'\n assert os.read(4,100) == b'original-kernel'\n assert 'G7_SYNTHETIC_PARENT_SECRET' not in os.environ\n with open(%q,'w') as marker: marker.write('imports-and-fds-preserved')\n", marker)
	if err := os.WriteFile(filepath.Join(strings.TrimSpace(string(output)), "g7_fixture.py"), []byte(module), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(venv, "bin/kerf")
	if err := os.WriteFile(script, []byte("#!"+interpreter+"\nfrom g7_fixture import main\nmain()\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// Python resolves the held /proc/self/fd entry point to this script's
	// directory. Its implicit search path previously shadowed the installed
	// virtualenv module with this synthetic adjacent module.
	shadowCanary := filepath.Join(root, "adjacent-module-executed")
	shadow := fmt.Sprintf("def main():\n with open(%q,'w') as marker: marker.write('adjacent-module-executed')\n", shadowCanary)
	if err := os.WriteFile(filepath.Join(venv, "bin/g7_fixture.py"), []byte(shadow), 0600); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(root, "current")
	if err := os.Symlink(filepath.Join(venv, "bin"), selected); err != nil {
		t.Fatal(err)
	}
	var files []*os.File
	for _, name := range []string{"initramfs", "kernel"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("original-"+name), 0600); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		files = append(files, file)
	}
	canary := filepath.Join(root, "replacement-executed")
	client := &CLI{Path: filepath.Join(selected, "kerf"), Timeout: 5 * time.Second, afterExecutableOpen: func() {
		if err := os.Rename(script, script+".original"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+canary+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}}
	if err := client.runWithFiles(ctx, files, "load", "box"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shadowCanary); !os.IsNotExist(err) {
		t.Fatalf("adjacent Python module ran instead of virtualenv package: %v", err)
	}
	if value, err := os.ReadFile(marker); err != nil || string(value) != "imports-and-fds-preserved" {
		t.Fatalf("entrypoint marker=%q error=%v", value, err)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("replacement executable ran: %v", err)
	}
}

func TestKerfRejectsUnsafeInterpreterDespiteMatchingObservation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	unsafe := filepath.Join(root, "unsafe")
	if err := os.Mkdir(unsafe, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(root, "interpreter-executed")
	interpreter := filepath.Join(unsafe, "interpreter")
	if err := os.WriteFile(interpreter, []byte("#!/bin/sh\ntouch '"+canary+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "kerf")
	if err := os.WriteFile(script, []byte("#!"+interpreter+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	instance := filepath.Join(root, "instances/box")
	if err := os.MkdirAll(instance, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instance, "status"), []byte("created"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &CLI{Path: script, Sysfs: root, Timeout: time.Second}
	err := client.Create(context.Background(), protocol.Sandbox{ID: "box", Config: protocol.SandboxConfig{CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 2}})
	_, canaryErr := os.Stat(canary)
	if err == nil || !strings.Contains(err.Error(), "interpreter rejected") || !os.IsNotExist(canaryErr) {
		t.Fatalf("unsafe interpreter accepted: error=%v canary_present=%t", err, canaryErr == nil)
	}
}
