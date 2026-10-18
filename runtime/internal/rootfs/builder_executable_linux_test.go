package rootfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
)

func TestBuilderExecutesHeldScriptAfterGenerationReplacement(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(root, "generation")
	if err := os.Mkdir(generation, 0700); err != nil {
		t.Fatal(err)
	}
	builder := filepath.Join(generation, "builder.sh")
	script := `#!/bin/bash
set -eu
test "$1" = /proc/self/fd/3
test "$(cat "$1")" = held-input
test "$0" = /proc/self/fd/4
script_path=$(readlink -f /proc/$$/fd/255)
script_dir=$(dirname "$script_path")
test "$(cat "$script_dir/support")" = original-support
echo held-builder
`
	if err := os.WriteFile(builder, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generation, "support"), []byte("original-support"), 0600); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "current")
	if err := os.Symlink("generation", current); err != nil {
		t.Fatal(err)
	}
	held, err := safefile.OpenTrustedExecutable(filepath.Join(current, "builder.sh"), 256<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := os.Rename(generation, generation+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(generation, 0700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(root, "replacement-executed")
	if err := os.WriteFile(builder, []byte("#!/bin/bash\ntouch '"+canary+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(root, "input")
	if err := os.WriteFile(inputPath, []byte("held-input"), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := runOpenedBuilder(context.Background(), time.Second, held, []string{"/proc/self/fd/3"}, builderEnvironment(), 1024, input)
	if err != nil || strings.TrimSpace(string(output)) != "held-builder" {
		t.Fatalf("output=%q error=%v", output, err)
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("replacement builder executed: %v", err)
	}
}

func TestBuilderRejectsWritableExecutableParentBeforeLaunch(t *testing.T) {
	root := t.TempDir()
	builder := filepath.Join(root, "builder.sh")
	canary := filepath.Join(t.TempDir(), "executed")
	if err := os.WriteFile(builder, []byte("#!/bin/sh\ntouch '"+canary+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0777); err != nil {
		t.Fatal(err)
	}
	output, err := runBoundedBuilder(context.Background(), time.Second, builder, nil, builderEnvironment(), 1024)
	_, canaryErr := os.Stat(canary)
	if err == nil || len(output) != 0 || !os.IsNotExist(canaryErr) {
		t.Fatalf("unsafe builder launched: output=%q error=%v canary_present=%t canary_error=%v", output, err, canaryErr == nil, canaryErr)
	}
}

func TestBuilderRejectsUnsafeInterpreterBeforeLaunch(t *testing.T) {
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
	builder := filepath.Join(root, "builder.sh")
	if err := os.WriteFile(builder, []byte("#!"+interpreter+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	output, err := runBoundedBuilder(context.Background(), time.Second, builder, nil, builderEnvironment(), 1024)
	_, canaryErr := os.Stat(canary)
	if err == nil || !strings.Contains(err.Error(), "interpreter rejected") || len(output) != 0 || !os.IsNotExist(canaryErr) {
		t.Fatalf("unsafe interpreter launched: error=%v output=%q canary_present=%t", err, output, canaryErr == nil)
	}
}
