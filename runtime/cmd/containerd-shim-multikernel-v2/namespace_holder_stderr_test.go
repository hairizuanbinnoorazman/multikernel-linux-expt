//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// This exercises the actual absolute nsenter binary and the production holder
// startup, without entering a namespace or starting any child kernel.
func TestNamespaceHolderStderrBoundary(t *testing.T) {
	if os.Getenv("MK_NAMESPACE_STDERR_CHILD") == "1" {
		start := time.Now()
		holder, err := startNamespaceHolder(os.Getenv("MK_NAMESPACE_STDERR_PATH"))
		if holder != nil {
			_ = holder.Stop()
			os.Exit(30)
		}
		if err == nil || time.Since(start) >= 2*time.Second {
			os.Exit(31)
		}
		_, _ = os.Stdout.WriteString("NAMESPACE_REFUSAL_PROGRESS_PASS\n")
		os.Exit(23)
	}
	if _, err := os.Stat("/usr/bin/nsenter"); err != nil {
		t.Fatal("actual nsenter is required")
	}
	for _, transport := range []string{"capture", "closed-reader", "full"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			scratch := privateTestDirectory(t)
			privatePath := filepath.Join(scratch, "SYNTHETIC_PRIVATE_NAMESPACE_TOKEN")
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNamespaceHolderStderrBoundary$")
			command.Env = append(os.Environ(), "MK_NAMESPACE_STDERR_CHILD=1", "MK_NAMESPACE_STDERR_PATH="+privatePath)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			var writer *os.File
			flags := 0
			if transport == "capture" {
				command.Stderr = &stderr
			} else {
				reader, output, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				writer = output
				defer writer.Close()
				if transport == "closed-reader" {
					reader.Close()
				} else {
					defer reader.Close()
					fd := int(writer.Fd())
					if err = unix.SetNonblock(fd, true); err != nil {
						t.Fatal(err)
					}
					for {
						_, err = unix.Write(fd, bytes.Repeat([]byte("x"), 4096))
						if errors.Is(err, syscall.EAGAIN) {
							break
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					if err = unix.SetNonblock(fd, false); err != nil {
						t.Fatal(err)
					}
				}
				flags, err = unix.FcntlInt(writer.Fd(), unix.F_GETFL, 0)
				if err != nil {
					t.Fatal(err)
				}
				command.Stderr = writer
			}
			err := command.Run()
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatal("namespace refusal blocked or lost intended status")
			}
			if stdout.String() != "NAMESPACE_REFUSAL_PROGRESS_PASS\n" || stderr.Len() != 0 {
				t.Fatal("namespace helper emitted raw stderr or lost progress")
			}
			if writer != nil {
				after, err := unix.FcntlInt(writer.Fd(), unix.F_GETFL, 0)
				if err != nil || after != flags {
					t.Fatal("inherited stderr flags changed")
				}
			}
			if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
				t.Fatal("namespace refusal left synthetic scratch")
			}
		})
	}
}
