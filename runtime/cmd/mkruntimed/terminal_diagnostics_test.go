//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemonTerminalDiagnosticsRealStderr(t *testing.T) {
	if os.Getenv("MK_G8_DAEMON_TERMINAL_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("mkruntimed", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"mkruntimed"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		os.Exit(0)
	}

	for _, mode := range []string{"capture", "blocked", "closed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			args := []string{"--config=" + filepath.Join(t.TempDir(), "SYNTHETIC_DAEMON_SECRET", "missing-config.json")}
			cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDaemonTerminalDiagnosticsRealStderr$", "--"}, args...)...)
			cmd.Env = append(os.Environ(), "MK_G8_DAEMON_TERMINAL_CHILD=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			if mode == "capture" {
				cmd.Stderr = &stderr
			} else {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				if mode == "closed" {
					reader.Close()
				} else {
					fd := int(writer.Fd())
					if err = unix.SetNonblock(fd, true); err != nil {
						t.Fatal(err)
					}
					padding := bytes.Repeat([]byte("x"), 4096)
					for {
						_, err = unix.Write(fd, padding)
						if errors.Is(err, unix.EAGAIN) {
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
				cmd.Stderr = writer
			}
			started := time.Now()
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 2 || stdout.Len() != 0 || ctx.Err() != nil || time.Since(started) > 3*time.Second {
				t.Fatal("terminal daemon failed to exit with original status", err)
			}
			if mode == "capture" {
				var record map[string]json.RawMessage
				if json.Unmarshal(stderr.Bytes(), &record) != nil || len(record) != 8 {
					t.Fatal("wrong terminal record schema")
				}
				var code, component string
				_ = json.Unmarshal(record["code"], &code)
				_ = json.Unmarshal(record["component"], &component)
				if code != "RUNTIME_HOST_CONFIG_FAILED" || component != "mkruntimed" || bytes.Contains(stderr.Bytes(), []byte("SYNTHETIC_DAEMON_SECRET")) || bytes.Count(stderr.Bytes(), []byte("\n")) != 1 {
					t.Fatal("private terminal error escaped or fixed code changed")
				}
			}
		})
	}
}
