//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"golang.org/x/sys/unix"
)

func credential(t *testing.T, sealed bool) *os.File {
	t.Helper()
	fd, err := unix.MemfdCreate("probe-test", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(fd), "probe-test")
	t.Cleanup(func() { f.Close() })
	if err = f.Chmod(0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("01234567890123456789012345678901")); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if sealed {
		if _, err = unix.FcntlInt(f.Fd(), unix.F_ADD_SEALS, tokenSeals); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func TestCredentialRequiresSealedPrivateDescriptor(t *testing.T) {
	for _, sealed := range []bool{false, true} {
		f := credential(t, sealed)
		key, err := readToken(f)
		if sealed && (err != nil || len(key) != 32) {
			t.Fatalf("sealed descriptor rejected: %v", err)
		}
		if !sealed && (err == nil || key != nil) {
			t.Fatal("unsealed credential accepted")
		}
		clear(key)
	}
	f, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write([]byte("01234567890123456789012345678901"))
	f.Seek(0, 0)
	if _, err = readToken(f); err == nil {
		t.Fatal("named regular file accepted")
	}
}
func TestProbeSingleRequestAndSanitizedOutcomes(t *testing.T) {
	for _, mode := range []string{"matched", "missing-root-claims", "wrong-release", "malformed", "timeout", "wrong-peer", "unsealed"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "mk-caps-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "relay.sock")
			l, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			os.Chmod(path, 0600)
			o := options{socket: path, id: "box", generation: "0123456789abcdef0123456789abcdef", release: "test-release", endpoint: 7200, peerPID: os.Getpid(), tokenFD: 3, timeout: 100 * time.Millisecond}
			if mode == "wrong-peer" {
				o.peerPID += 100000
			}
			served := make(chan int, 1)
			go func() {
				c, err := l.Accept()
				if err != nil {
					served <- 0
					return
				}
				defer c.Close()
				c.SetDeadline(time.Now().Add(time.Second))
				var h [4]byte
				if _, err = io.ReadFull(c, h[:]); err != nil {
					served <- 0
					return
				}
				n := binary.BigEndian.Uint32(h[:])
				if n > 4096 {
					served <- -1
					return
				}
				b := make([]byte, n)
				io.ReadFull(c, b)
				var e agent.Envelope
				if json.Unmarshal(b, &e) != nil {
					served <- -1
					return
				}
				signed := e
				agent.Sign(&signed, []byte("01234567890123456789012345678901"))
				if e.MAC != signed.MAC || e.Method != "Capabilities" || e.SandboxID != o.id || e.Generation != o.generation || e.Endpoint != uint32(o.endpoint) {
					served <- -1
					return
				}
				body := map[string]any{"protocol": 1, "kernel": map[string]any{"architecture": "amd64", "release": "test-release"}, "agent": map[string]any{"uid": 0, "gid": 0}}
				if mode == "missing-root-claims" {
					delete(body, "agent")
				}
				if mode == "wrong-release" {
					body["kernel"] = map[string]any{"architecture": "amd64", "release": "secret-unexpected-release"}
				}
				response, _ := json.Marshal(agent.Reply{Version: 1, Sequence: e.Sequence, Body: body})
				if mode == "malformed" {
					response = []byte("secret-invalid-reply")
				}
				if mode == "timeout" {
					var buf [1]byte
					c.Read(buf[:])
					served <- 1
					return
				}
				binary.BigEndian.PutUint32(h[:], uint32(len(response)))
				c.Write(h[:])
				c.Write(response)
				var extra [1]byte
				nread, _ := c.Read(extra[:])
				if nread != 0 {
					served <- -1
					return
				}
				served <- 1
			}()
			r := inspect(context.Background(), o, credential(t, mode != "unsealed"))
			l.Close()
			count := <-served
			want := 1
			if mode == "wrong-peer" || mode == "unsealed" {
				want = 0
			}
			if count != want || r.RequestAttempted != (want == 1) || r.MutatingMethodAttempted {
				t.Fatalf("request contract: count=%d report=%+v", count, r)
			}
			if (r.Status == "matched") != (mode == "matched") {
				t.Fatalf("unexpected result %+v", r)
			}
			if mode == "missing-root-claims" && r.GuestRootClaimMatches {
				t.Fatal("omitted root identity accepted")
			}
			b, _ := json.Marshal(r)
			if strings.Contains(string(b), "secret") || strings.Contains(string(b), "01234567890123456789012345678901") {
				t.Fatal("unsafe reply or credential appeared in report")
			}
		})
	}
}
