//go:build linux

package boundedexec

import (
	"bytes"
	"context"
	"errors"
	"os"
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
