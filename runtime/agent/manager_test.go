package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func bundle(t *testing.T, args []string, extra string) string {
	t.Helper()
	d := t.TempDir()
	root := filepath.Join(d, "rootfs")
	os.Mkdir(root, 0755)
	bin, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(root, "probe")
	b, e := os.ReadFile(bin)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(target, b, 0755); e != nil {
		t.Fatal(e)
	}
	cfg := `{"ociVersion":"1.1.0","process":{"user":{"uid":0,"gid":0},"args":` + mustJSON(args) + `,"env":[],"cwd":"/"},"root":{"path":"rootfs"}` + extra + `}`
	os.WriteFile(filepath.Join(d, "config.json"), []byte(cfg), 0644)
	return d
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func TestHelperProcess(t *testing.T) {
	if os.Getenv("MK_AGENT_HELPER") != "1" {
		return
	}
	if os.Getenv("MK_AGENT_TERMINAL_HELPER") == "1" {
		time.Sleep(100 * time.Millisecond)
		ws, err := unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
		if err != nil {
			os.Exit(99)
		}
		fmt.Printf("terminal-size=%dx%d", ws.Col, ws.Row)
		return
	}
	os.Stdout.WriteString("stdout-ok")
	os.Stderr.WriteString("stderr-ok")
	os.Exit(17)
}

func TestTerminalAndResize(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	c, _, err := LoadBundle(b)
	if err != nil {
		t.Fatal(err)
	}
	c.Process.Terminal = true
	c.Process.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_TERMINAL_HELPER=1"}
	raw, _ := json.Marshal(c)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = m.Create("tty", b); err != nil {
		t.Fatal(err)
	}
	if err = m.StartWithSize("tty", 80, 24, true); err != nil {
		t.Fatal(err)
	}
	if err = m.Resize("tty", 91, 37); err != nil {
		t.Fatal(err)
	}
	state, err := m.Wait("tty")
	if err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 0 || !strings.Contains(state.Stdout, "terminal-size=91x37") || state.Stderr != "" {
		t.Fatalf("terminal state: %+v", state)
	}
}
func TestLifecycle(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	c, _, e := LoadBundle(b)
	if e != nil {
		t.Fatal(e)
	}
	c.Process.Env = []string{"MK_AGENT_HELPER=1"}
	c.Process.User.UID = uint32(os.Getuid())
	c.Process.User.GID = uint32(os.Getgid())
	raw, _ := json.Marshal(c)
	os.WriteFile(filepath.Join(b, "config.json"), raw, 0644)
	if e = m.Create("p1", b); e != nil {
		t.Fatal(e)
	}
	if e = m.Start("p1"); e != nil {
		t.Fatal(e)
	}
	s, e := m.Wait("p1")
	if e != nil {
		t.Fatal(e)
	}
	if s.ExitCode != 17 || s.Stdout != "stdout-ok" || s.Stderr != "stderr-ok" {
		t.Fatalf("state: %+v", s)
	}
	if e = m.Delete("p1"); e != nil {
		t.Fatal(e)
	}
}
func TestUnsupportedFailsClosed(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe"}, `,"mounts":[{}]`)
	if e := m.Create("p1", b); e == nil {
		t.Fatal("unsupported mount accepted")
	}
}

func TestWaitRejectsUnstartedProcess(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe"}, "")
	if err := m.Create("p1", b); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Wait("p1"); err == nil || !strings.Contains(err.Error(), "not started") {
		t.Fatalf("Wait() error = %v, want not-started error", err)
	}
}
func TestAuthenticationAndReplay(t *testing.T) {
	s := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	e := Envelope{Version: 1, SandboxID: s.SandboxID, Generation: s.Generation, Endpoint: s.Endpoint, Sequence: 1, Method: "Capabilities"}
	Sign(&e, s.Token)
	r := s.Dispatch(e)
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	capabilities, ok := r.Body.(map[string]any)
	if !ok {
		t.Fatalf("capabilities body type = %T", r.Body)
	}
	features, ok := capabilities["oci_features"].([]string)
	if !ok {
		t.Fatalf("oci_features type = %T", capabilities["oci_features"])
	}
	for _, feature := range features {
		if feature == "uid" || feature == "gid" || feature == "supplementary-groups" || feature == "signals" {
			t.Fatalf("unproved feature %q advertised", feature)
		}
	}
	if r := s.Dispatch(e); r.Error != "replayed sequence" {
		t.Fatalf("replay result: %+v", r)
	}
	e.Sequence = 2
	e.MAC = "00"
	if r := s.Dispatch(e); r.Error != "invalid authentication" {
		t.Fatalf("auth result: %+v", r)
	}
}
