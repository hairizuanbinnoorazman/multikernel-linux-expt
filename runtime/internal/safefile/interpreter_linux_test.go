package safefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptInterpreterPolicy(t *testing.T) {
	for _, test := range []struct {
		name, header string
		environment  []string
		accept       bool
	}{
		{"absolute-shell", "#!/bin/sh\n", nil, true},
		{"shell-option", "#!/bin/sh -eu\n", nil, true},
		{"env-shell", "#!/usr/bin/env sh\n", []string{"PATH=/usr/bin:/bin"}, true},
		{"env-absolute", "#!/usr/bin/env /bin/sh\n", nil, true},
		{"env-last-path", "#!/usr/bin/env sh\n", []string{"PATH=relative", "PATH=/bin"}, true},
		{"relative-interpreter", "#!bin/sh\n", nil, false},
		{"env-no-path", "#!/usr/bin/env sh\n", nil, false},
		{"env-relative-path", "#!/usr/bin/env sh\n", []string{"PATH=relative:/bin"}, false},
		{"env-empty-path-entry", "#!/usr/bin/env sh\n", []string{"PATH=/bin:"}, false},
		{"env-split-string", "#!/usr/bin/env -S sh -eu\n", []string{"PATH=/bin"}, false},
		{"env-assignment", "#!/usr/bin/env PATH=/tmp/sh\n", []string{"PATH=/bin"}, false},
		{"empty-shebang", "#! \t\n", nil, false},
		{"nul-shebang", "#!/bin/sh\x00\n", nil, false},
		{"cr-shebang", "#!/bin/sh\r\n", nil, false},
		{"overlong", "#!/bin/sh " + strings.Repeat("x", 256) + "\n", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "script")
			if err := os.WriteFile(path, []byte(test.header+"exit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			file, err := OpenTrustedExecutable(path, 1024)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = ValidateScriptInterpreter(file, test.environment)
			if (err == nil) != test.accept {
				t.Fatalf("accept=%v error=%v", test.accept, err)
			}
			buffer := make([]byte, 2)
			n, readErr := file.Read(buffer)
			if readErr != nil || n != 2 || string(buffer) != "#!" {
				t.Fatalf("inspection changed inherited offset: %q %v", buffer, readErr)
			}
		})
	}
}

func TestScriptInterpreterRejectsUnsafePathAndCycles(t *testing.T) {
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
	interpreter := filepath.Join(unsafe, "sh")
	if err := os.WriteFile(interpreter, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, env := range []bool{false, true} {
		path := filepath.Join(root, "script")
		header := "#!" + interpreter + "\n"
		if env {
			header = "#!/usr/bin/env sh\n"
		}
		if err := os.WriteFile(path, []byte(header), 0700); err != nil {
			t.Fatal(err)
		}
		file, err := OpenTrustedExecutable(path, 1024)
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateScriptInterpreter(file, []string{"PATH=" + unsafe + ":/bin"})
		file.Close()
		if err == nil {
			t.Fatal("unsafe interpreter selected")
		}
	}
	cycle := filepath.Join(root, "cycle")
	if err := os.WriteFile(cycle, []byte("#!"+cycle+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := OpenTrustedExecutable(cycle, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := ValidateScriptInterpreter(file, nil); err == nil {
		t.Fatal("recursive interpreter cycle accepted")
	}
}

func TestInterpreterLinkCountPolicy(t *testing.T) {
	for _, test := range []struct {
		uid                 uint32
		links               uint64
		interpreter, accept bool
	}{
		{0, 1, false, true}, {0, 115, false, false}, {0, 115, true, true},
		{1000, 2, true, false}, {0, 0, true, false}, {1000, 1, true, true},
	} {
		if got := trustedExecutableLinkCount(Identity{UID: test.uid, Links: test.links}, test.interpreter); got != test.accept {
			t.Fatalf("link policy %+v result=%v", test, got)
		}
	}
}

func TestMultilinkInterpreterRootFixture(t *testing.T) {
	if os.Getenv("MK_ROOT_INTERPRETER_LINKS") != "1" {
		t.Skip("explicit root-owned interpreter alias fixture")
	}
	if os.Geteuid() != 0 {
		t.Fatal("root-owned alias fixture requires root")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	native, err := os.ReadFile("/bin/sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		data   []byte
		accept bool
	}{{"native", native, true}, {"script", []byte("#!/bin/sh\nexit 0\n"), false}} {
		path := filepath.Join(root, test.name)
		if err := os.WriteFile(path, test.data, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, path+".alias"); err != nil {
			t.Fatal(err)
		}
		file, err := openTrustedInterpreter(path, 256<<20)
		if file != nil {
			file.Close()
		}
		if (err == nil) != test.accept {
			t.Fatalf("%s interpreter result=%v", test.name, err)
		}
		file, err = OpenTrustedExecutable(path, 256<<20)
		if file != nil {
			file.Close()
		}
		if err == nil {
			t.Fatal("configured artifact accepted aliases")
		}
	}
	t.Log("PASS: root-owned ELF aliases accepted only as interpreters; script aliases/configured artifacts rejected")
}
