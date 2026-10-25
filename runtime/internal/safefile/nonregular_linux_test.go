package safefile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A subprocess contains regressions that block in open(2), before the regular
// file check can run. The parent never opens a FIFO peer to make a bad call pass.
func TestPrivateReadersRejectUnpairedFIFO(t *testing.T) {
	if method := os.Getenv("MK_SAFEFILE_FIFO_METHOD"); method != "" {
		directory, err := OpenDirectory(os.Getenv("MK_SAFEFILE_FIFO_DIRECTORY"), false)
		if err != nil {
			t.Fatal(err)
		}
		defer directory.Close()
		switch method {
		case "read":
			_, _, err = directory.ReadPrivate("entry", 4096)
		case "identity":
			_, _, _, err = directory.ReadPrivateIdentity("entry", 4096)
		case "quarantine":
			_, _, _, _, err = directory.ReadPrivateOrQuarantineIdentity("entry", 4096)
		case "snapshot":
			_, _, err = directory.ReadPrivateSnapshot("entry", 4096)
		case "inspect":
			_, _, err = directory.InspectPrivate("entry", 4096)
		case "open-read":
			file, _, failure := directory.OpenPrivateFile("entry", 4096, false)
			err = failure
			if file != nil {
				file.Close()
			}
		case "open-write":
			file, _, failure := directory.OpenPrivateFile("entry", 4096, true)
			err = failure
			if file != nil {
				file.Close()
			}
		case "executable":
			file, _, failure := directory.OpenTrustedExecutable("entry", 4096)
			err = failure
			if file != nil {
				file.Close()
			}
		default:
			t.Fatal("unknown private-reader test method")
		}
		if err == nil {
			t.Fatal("non-regular file was accepted")
		}
		return
	}
	for _, method := range []string{"read", "identity", "quarantine", "snapshot", "inspect", "open-read", "open-write", "executable"} {
		t.Run(method, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(root, "entry")
			if err := unix.Mkfifo(name, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(name)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPrivateReadersRejectUnpairedFIFO$", "-test.v")
			command.Env = append(os.Environ(), "MK_SAFEFILE_FIFO_METHOD="+method, "MK_SAFEFILE_FIFO_DIRECTORY="+root, "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
			output, err := command.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("private reader waited for a FIFO peer: %s", output)
			}
			if err != nil {
				t.Fatalf("FIFO refusal failed: %v: %s", err, output)
			}
			after, err := os.Lstat(name)
			if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() {
				t.Fatal("FIFO evidence changed")
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 {
				t.Fatal("reader created or removed files")
			}
		})
	}
}
