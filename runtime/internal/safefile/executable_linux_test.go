package safefile

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTrustedExecutableManagedLinksAndHeldIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(root, "generation")
	if err := os.Mkdir(generation, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(generation, "builder")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho original\n"), 0700); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "current")
	if err := os.Symlink("generation", current); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "builder")
	if err := os.Symlink(filepath.Join(current, "builder"), link); err != nil {
		t.Fatal(err)
	}
	file, err := OpenTrustedExecutable(link, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(generation, generation+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(generation, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho replacement\n"), 0700); err != nil {
		t.Fatal(err)
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("held identity changed: %v", err)
	}
	value := make([]byte, 128)
	n, err := file.Read(value)
	if err != nil || string(value[:n]) != "#!/bin/sh\necho original\n" {
		t.Fatalf("held contents=%q error=%v", value[:n], err)
	}
}

func TestTrustedExecutableRejectsUnsafeObjects(t *testing.T) {
	for _, name := range []string{"writable-parent", "unsafe-intermediate-link", "writable-file", "non-executable", "empty", "fifo", "directory", "hardlink", "oversized", "cycle"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "builder")
			if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			var err error
			switch name {
			case "writable-parent":
				err = os.Chmod(root, 0777)
			case "writable-file":
				err = os.Chmod(path, 0777)
			case "non-executable":
				err = os.Chmod(path, 0600)
			case "empty":
				err = os.Truncate(path, 0)
			case "oversized":
				err = os.Truncate(path, 2048)
			case "hardlink":
				err = os.Link(path, path+".alias")
			case "fifo", "directory", "cycle":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch name {
				case "fifo":
					err = unix.Mkfifo(path, 0700)
				case "directory":
					err = os.Mkdir(path, 0700)
				case "cycle":
					err = os.Symlink("builder", path)
				}
			case "unsafe-intermediate-link":
				unsafe := filepath.Join(root, "unsafe")
				if err = os.Mkdir(unsafe, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.Chmod(unsafe, 0777); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(path, filepath.Join(unsafe, "redirect")); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(root, "selected")
				err = os.Symlink(filepath.Join(unsafe, "redirect"), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			file, err := OpenTrustedExecutable(path, 1024)
			if file != nil {
				file.Close()
			}
			if err == nil {
				t.Fatal("unsafe executable accepted")
			}
		})
	}
}
