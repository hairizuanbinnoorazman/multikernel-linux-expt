//go:build linux

package boundedexec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunDrainsButRejectsOverflow(t *testing.T) {
	output, err := Run(context.Background(), time.Second, "/bin/sh",
		[]string{"-c", "head -c 4097 /dev/zero"}, os.Environ(), 4096)
	if !errors.Is(err, ErrOutputLimit) || len(output) != 4096 {
		t.Fatalf("overflow result = len:%d err:%v", len(output), err)
	}
}

func TestRunKillsProcessGroupAtDeadline(t *testing.T) {
	started := time.Now()
	_, err := Run(context.Background(), 50*time.Millisecond, "/bin/sh",
		[]string{"-c", "sleep 60 & wait"}, os.Environ(), 4096)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("command cancellation took %s", elapsed)
	}
}

func TestRunCapturesSuccessfulCombinedOutput(t *testing.T) {
	output, err := Run(context.Background(), time.Second, "/bin/sh",
		[]string{"-c", "printf stdout; printf stderr >&2"}, os.Environ(), 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("stdout")) || !bytes.Contains(output, []byte("stderr")) {
		t.Fatalf("combined output = %q", output)
	}
}

func TestRunRejectsUnboundedConfiguration(t *testing.T) {
	for _, options := range []struct {
		timeout time.Duration
		maximum int
	}{{0, 1}, {time.Second, 0}} {
		if _, err := Run(context.Background(), options.timeout, "/bin/true", nil, nil, options.maximum); err == nil {
			t.Fatalf("unbounded options accepted: %+v", options)
		}
	}
}

func TestRunWithFilesPinsRenamedDirectory(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "source")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "marker"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = os.Rename(directory, directory+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "marker"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := RunWithFiles(context.Background(), time.Second, "/bin/sh",
		[]string{"-c", "read value </proc/self/fd/3/marker; printf %s \"$value\""}, os.Environ(), 4096, []*os.File{file})
	if err != nil || string(output) != "original" {
		t.Fatalf("descriptor-pinned output = %q, %v", output, err)
	}
}

func TestRunWithFilesRejectsNilDescriptor(t *testing.T) {
	if _, err := RunWithFiles(context.Background(), time.Second, "/bin/true", nil, nil, 4096, []*os.File{nil}); err == nil {
		t.Fatal("nil inherited descriptor accepted")
	}
}
