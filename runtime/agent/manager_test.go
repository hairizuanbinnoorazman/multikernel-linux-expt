package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
	"golang.org/x/sys/unix"
)

type bufferWriteCloser struct{ bytes.Buffer }

func (*bufferWriteCloser) Close() error { return nil }

type prefixErrorWriteCloser struct {
	bytes.Buffer
	prefix int
	failed bool
}

func (w *prefixErrorWriteCloser) Write(data []byte) (int, error) {
	if !w.failed {
		w.failed = true
		if w.prefix > len(data) {
			w.prefix = len(data)
		}
		_, _ = w.Buffer.Write(data[:w.prefix])
		return w.prefix, errors.New("injected stdin write failure")
	}
	return w.Buffer.Write(data)
}

func (*prefixErrorWriteCloser) Close() error { return nil }

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
	if os.Getenv("MK_AGENT_SIGNAL_CHILD") == "1" {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		if err := os.WriteFile(os.Getenv("MK_AGENT_SIGNAL_MARKER"), []byte("child-ready\n"), 0644); err != nil {
			os.Exit(97)
		}
		<-ch
		if err := os.WriteFile(os.Getenv("MK_AGENT_SIGNAL_MARKER"), []byte("child-received-sigterm\n"), 0644); err != nil {
			os.Exit(94)
		}
		return
	}
	if os.Getenv("MK_AGENT_SIGNAL_HELPER") == "1" {
		executable, err := os.Executable()
		if err != nil {
			os.Exit(95)
		}
		child := exec.Command(executable, "-test.run=TestHelperProcess")
		child.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_SIGNAL_CHILD=1", "MK_AGENT_SIGNAL_MARKER=" + os.Getenv("MK_AGENT_SIGNAL_MARKER")}
		if err := child.Start(); err != nil {
			os.Exit(96)
		}
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			if data, readErr := os.ReadFile(os.Getenv("MK_AGENT_SIGNAL_MARKER")); readErr == nil && string(data) == "child-ready\n" {
				break
			}
			time.Sleep(time.Millisecond)
		}
		fmt.Printf("signal-ready child=%d\n", child.Process.Pid)
		_ = child.Wait()
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
	if os.Getenv("MK_AGENT_STDIN_HELPER") == "1" {
		fmt.Print("stdin-ready\n")
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(98)
		}
		fmt.Printf("stdin=%s", data)
		return
	}
	if os.Getenv("MK_AGENT_INSPECT_HELPER") == "1" {
		cwd, err := os.Getwd()
		if err != nil {
			os.Exit(93)
		}
		result := struct {
			Args []string `json:"args"`
			Env  string   `json:"env"`
			Cwd  string   `json:"cwd"`
		}{os.Args, os.Getenv("MK_AGENT_EXACT_ENV"), cwd}
		if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
			os.Exit(92)
		}
		return
	}
	if value := os.Getenv("MK_AGENT_EXIT_CODE"); value != "" {
		code, err := strconv.Atoi(value)
		if err != nil {
			os.Exit(91)
		}
		os.Exit(code)
	}
	if os.Getenv("MK_AGENT_IGNORE_TERM_HELPER") == "1" {
		signal.Ignore(syscall.SIGTERM)
		fmt.Print("ignore-term-ready")
		select {}
	}
	os.Stdout.WriteString("stdout-ok")
	os.Stderr.WriteString("stderr-ok")
	os.Exit(17)
}

func TestStdinAndIncrementalOutput(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	c, _, err := LoadBundle(b)
	if err != nil {
		t.Fatal(err)
	}
	c.Process.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_STDIN_HELPER=1"}
	raw, _ := json.Marshal(c)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = m.Create("stdin", b); err != nil {
		t.Fatal(err)
	}
	if err = m.Start("stdin"); err != nil {
		t.Fatal(err)
	}
	var stdout []byte
	var stdoutOffset uint64
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		var chunk []byte
		chunk, _, stdoutOffset, _, _, err = m.ReadOutput("stdin", stdoutOffset, 0, 64<<10)
		if err != nil {
			t.Fatal(err)
		}
		stdout = append(stdout, chunk...)
		if strings.Contains(string(stdout), "stdin-ready") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(string(stdout), "stdin-ready") {
		t.Fatalf("live stdout = %q", stdout)
	}
	if err = m.Write("stdin", []byte("guest-input")); err != nil {
		t.Fatal(err)
	}
	if err = m.CloseStdin("stdin"); err != nil {
		t.Fatal(err)
	}
	state, err := m.Wait("stdin")
	if err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 0 || !strings.Contains(state.Stdout, "stdin=guest-input") {
		t.Fatalf("stdin state: %+v", state)
	}
	if err = m.Write("stdin", []byte("late")); err == nil {
		t.Fatal("write after exit unexpectedly succeeded")
	}
}

func TestStdinWriteOffsetsMakeLostReplyReplayIdempotent(t *testing.T) {
	input := &bufferWriteCloser{}
	m := NewManager(true)
	m.processes["stdin"] = &process{stdin: input, state: ProcessState{ID: "stdin", Status: "RUNNING"}}
	data := []byte("exactly-once")
	next, err := m.WriteAt("stdin", 0, data)
	if err != nil || next != uint64(len(data)) {
		t.Fatalf("initial WriteAt = %d, %v", next, err)
	}
	if next, err = m.WriteAt("stdin", 0, data); err != nil || next != uint64(len(data)) {
		t.Fatalf("lost-reply replay = %d, %v", next, err)
	}
	if input.String() != string(data) {
		t.Fatalf("replayed stdin bytes = %q", input.String())
	}
	if _, err = m.WriteAt("stdin", 0, []byte("different")); err == nil {
		t.Fatal("same offset with different bytes was accepted")
	}
	if _, err = m.WriteAt("stdin", next+1, []byte("gap")); err == nil {
		t.Fatal("gapped stdin offset was accepted")
	}
	if next, err = m.WriteAt("stdin", next, []byte("-next")); err != nil || next != uint64(len("exactly-once-next")) {
		t.Fatalf("next stdin chunk = %d, %v", next, err)
	}
	if input.String() != "exactly-once-next" {
		t.Fatalf("ordered stdin bytes = %q", input.String())
	}
	m.mu.Lock()
	m.processes["stdin"].state.Status = "STOPPED"
	m.mu.Unlock()
	if replayed, replayErr := m.WriteAt("stdin", uint64(len("exactly-once")), []byte("-next")); replayErr != nil || replayed != next {
		t.Fatalf("post-exit lost-reply replay = %d, %v", replayed, replayErr)
	}
	if input.String() != "exactly-once-next" {
		t.Fatalf("post-exit replay duplicated stdin bytes = %q", input.String())
	}
}

func TestStdinWriteOffsetReplayResumesAfterPartialLocalWrite(t *testing.T) {
	input := &prefixErrorWriteCloser{prefix: 5}
	m := NewManager(true)
	m.processes["stdin"] = &process{stdin: input, state: ProcessState{ID: "stdin", Status: "RUNNING"}}
	data := []byte("partial-write-replay")
	if next, err := m.WriteAt("stdin", 0, data); err == nil || next != 0 {
		t.Fatalf("partial WriteAt = %d, %v", next, err)
	}
	if input.String() != string(data[:5]) {
		t.Fatalf("partial bytes = %q", input.String())
	}
	changed := append([]byte(nil), data...)
	changed[len(changed)-1]++
	if _, err := m.WriteAt("stdin", 0, changed); err == nil {
		t.Fatal("changed partial replay was accepted")
	}
	if _, err := m.WriteAt("stdin", 1, data); err == nil {
		t.Fatal("gapped partial replay was accepted")
	}
	if input.String() != string(data[:5]) {
		t.Fatalf("rejected partial replay changed bytes = %q", input.String())
	}
	if next, err := m.WriteAt("stdin", 0, append([]byte(nil), data...)); err != nil || next != uint64(len(data)) {
		t.Fatalf("exact partial replay = %d, %v", next, err)
	}
	if input.String() != string(data) {
		t.Fatalf("replayed partial stdin bytes = %q", input.String())
	}
	if next, err := m.WriteAt("stdin", 0, data); err != nil || next != uint64(len(data)) {
		t.Fatalf("acknowledgement replay = %d, %v", next, err)
	}
	if input.String() != string(data) {
		t.Fatalf("acknowledgement replay duplicated stdin bytes = %q", input.String())
	}
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

func TestInvalidTerminalSizesFailClosed(t *testing.T) {
	m := NewManager(true)
	if err := m.StartWithSize("missing", 65536, 24, true); err == nil || !strings.Contains(err.Error(), "PTY limit") {
		t.Fatalf("oversized initial terminal error = %v", err)
	}
	if err := m.Resize("missing", 80, 65536); err == nil || !strings.Contains(err.Error(), "PTY limit") {
		t.Fatalf("oversized resize error = %v", err)
	}
	b := bundle(t, []string{"/probe"}, "")
	if err := m.Create("notty", b); err != nil {
		t.Fatal(err)
	}
	if err := m.StartWithSize("notty", 80, 24, true); err == nil || !strings.Contains(err.Error(), "without a terminal") {
		t.Fatalf("non-terminal initial size error = %v", err)
	}
}

func TestSignalReachesContainerProcessGroup(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	c, _, err := LoadBundle(b)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "signal-marker")
	c.Process.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_SIGNAL_HELPER=1", "MK_AGENT_SIGNAL_MARKER=" + marker}
	raw, _ := json.Marshal(c)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = m.Create("signal", b); err != nil {
		t.Fatal(err)
	}
	if err = m.Start("signal"); err != nil {
		t.Fatal(err)
	}
	ready := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		state, stateErr := m.State("signal")
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if strings.Contains(state.Stdout, "signal-ready") {
			ready = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ready {
		state, _ := m.State("signal")
		t.Fatalf("signal helper did not become ready: %+v", state)
	}
	if err = m.Signal("signal", syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	state, err := m.Wait("signal")
	if err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 128+int(syscall.SIGTERM) {
		t.Fatalf("exit code = %d, want signal exit", state.ExitCode)
	}
	var observedMarker []byte
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if data, readErr := os.ReadFile(marker); readErr == nil {
			observedMarker = data
			if string(data) == "child-received-sigterm\n" {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("descendant did not observe process-group SIGTERM; marker=%q", observedMarker)
}

func TestSignalOnceCachesExactResultAndRejectsChangedReplay(t *testing.T) {
	m := NewManager(true)
	m.processes["signal"] = &process{cmd: &exec.Cmd{Process: &os.Process{Pid: 41}}, state: ProcessState{ID: "signal", Status: "RUNNING"}}
	calls := 0
	m.signalProcess = func(pid int, signal syscall.Signal) error {
		calls++
		if pid != -41 || signal != syscall.SIGUSR1 {
			return errors.New("unexpected signal target")
		}
		return nil
	}
	operationID := strings.Repeat("a", 32)
	for attempt := 0; attempt < 2; attempt++ {
		if err := m.SignalOnce("signal", syscall.SIGUSR1, operationID); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("signal calls = %d, want 1", calls)
	}
	if err := m.SignalOnce("signal", syscall.SIGUSR2, operationID); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("changed replay error = %v", err)
	}
	delete(m.processes, "signal")
	if err := m.SignalOnce("other", syscall.SIGUSR1, operationID); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("cross-process replay error = %v", err)
	}
	if err := m.SignalOnce("signal", syscall.SIGUSR1, operationID); err != nil {
		t.Fatalf("post-exit exact replay = %v", err)
	}
	if err := m.AcknowledgeSignal("other", operationID); err == nil || !strings.Contains(err.Error(), "target differs") {
		t.Fatalf("wrong-target acknowledgement = %v", err)
	}
	if err := m.AcknowledgeSignal("signal", operationID); err != nil {
		t.Fatal(err)
	}
	if err := m.AcknowledgeSignal("signal", operationID); err != nil {
		t.Fatalf("acknowledgement replay = %v", err)
	}
	if err := m.SignalOnce("signal", syscall.SIGUSR1, operationID); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("retired post-exit operation = %v", err)
	}
}

func TestSignalOnceRefusesFullLedgerBeforeMutation(t *testing.T) {
	m := NewManager(true)
	m.processes["signal"] = &process{cmd: &exec.Cmd{Process: &os.Process{Pid: 41}}, state: ProcessState{ID: "signal", Status: "RUNNING"}}
	m.signalResults = make(map[string]signalResult, maxSignalResults)
	for index := 0; index < maxSignalResults; index++ {
		m.signalResults[fmt.Sprintf("%032x", index)] = signalResult{id: "other", signal: syscall.SIGTERM}
	}
	calls := 0
	m.signalProcess = func(int, syscall.Signal) error { calls++; return nil }
	if err := m.SignalOnce("signal", syscall.SIGTERM, strings.Repeat("f", 32)); err == nil || !strings.Contains(err.Error(), "ledger is full") {
		t.Fatalf("full-ledger error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("full ledger applied %d signals", calls)
	}
}

func TestProcessAndOutputRetentionBounds(t *testing.T) {
	var output lockedBuffer
	payload := make([]byte, maxOutputBytes+1)
	if n, err := output.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	if len(output.String()) != maxOutputBytes || !output.Truncated() {
		t.Fatalf("retained=%d truncated=%v", len(output.String()), output.Truncated())
	}

	m := NewManager(true)
	for i := 0; i < maxProcesses; i++ {
		id := fmt.Sprintf("p-%d", i)
		m.processes[id] = &process{state: ProcessState{ID: id, Status: "STOPPED"}}
	}
	if err := m.Create("over-limit", bundle(t, []string{"/probe"}, "")); err == nil || !strings.Contains(err.Error(), "retention limit") {
		t.Fatalf("Create() error = %v, want retention limit", err)
	}
}

func TestBoundedStreamingBackpressureAndIndependentOffsets(t *testing.T) {
	var stdout, stderr lockedBuffer
	payload := bytes.Repeat([]byte("s"), maxOutputBytes+1)
	written := make(chan error, 1)
	go func() {
		_, err := stdout.Write(payload)
		written <- err
	}()

	var offset uint64
	total := 0
	for total < len(payload) {
		chunk, next := stdout.slice(offset, 64<<10)
		total += len(chunk)
		offset = next
		if len(chunk) == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if stdout.Truncated() || total != len(payload) {
		t.Fatalf("streamed=%d truncated=%v", total, stdout.Truncated())
	}
	if _, err := stderr.Write([]byte("independent-stderr")); err != nil {
		t.Fatal(err)
	}
	chunk, _ := stderr.slice(0, 64<<10)
	if string(chunk) != "independent-stderr" {
		t.Fatalf("stderr stream = %q", chunk)
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
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"mounts", func(c map[string]any) { c["mounts"] = []any{map[string]any{}} }, "sanitized materialized path"},
		{"hooks", func(c map[string]any) { c["hooks"] = map[string]any{"prestart": []any{}} }, "hooks are not implemented"},
		{"namespaces", func(c map[string]any) {
			c["linux"] = map[string]any{"namespaces": []any{map[string]any{"type": "pid"}}}
		}, "namespaces/resources/seccomp"},
		{"resources", func(c map[string]any) { c["linux"] = map[string]any{"resources": map[string]any{}} }, "namespaces/resources/seccomp"},
		{"seccomp", func(c map[string]any) { c["linux"] = map[string]any{"seccomp": map[string]any{}} }, "namespaces/resources/seccomp"},
		{"annotations", func(c map[string]any) { c["annotations"] = map[string]any{} }, "annotations"},
		{"empty hooks", func(c map[string]any) { c["hooks"] = map[string]any{} }, "hooks are not implemented"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := NewManager(true)
			b := bundle(t, []string{"/probe"}, "")
			raw, err := os.ReadFile(filepath.Join(b, "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err = json.Unmarshal(raw, &config); err != nil {
				t.Fatal(err)
			}
			test.mutate(config)
			raw, err = json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
				t.Fatal(err)
			}
			if err = m.Create("p1", b); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Create() error = %v, want rejection containing %q", err, test.want)
			}
		})
	}
}

func TestSanitizedReadonlyBindInputLoadsBeforePrivilegedApplication(t *testing.T) {
	b := bundle(t, []string{"/probe"}, "")
	raw, err := os.ReadFile(filepath.Join(b, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["mounts"] = []any{map[string]any{
		"destination": "/opt/input", "source": "/opt/input", "type": "bind",
		"options": []any{"bind", "nodev", "noexec", "nosuid", "ro"},
	}}
	raw, _ = json.Marshal(config)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadBundle(b)
	if err != nil || len(loaded.Mounts) != 1 || loaded.Mounts[0].Destination != "/opt/input" {
		t.Fatalf("sanitized bind = %+v, %v", loaded.Mounts, err)
	}
	if err = NewManager(true).Create("p1", b); err == nil || !strings.Contains(err.Error(), "root policy cannot be applied") {
		t.Fatalf("no-chroot bind Create() error = %v", err)
	}
}

func TestReadonlyBindInputContractFailsClosed(t *testing.T) {
	valid := MountSpec{Destination: "/opt/input", Source: "/opt/input", Type: "bind",
		Options: []string{"bind", "nodev", "noexec", "nosuid", "ro"}}
	tests := map[string][]MountSpec{
		"host source retained": {{Destination: valid.Destination, Source: "/srv/input", Type: valid.Type, Options: valid.Options}},
		"writable":             {{Destination: valid.Destination, Source: valid.Source, Type: valid.Type, Options: []string{"bind", "nodev", "noexec", "nosuid", "rw"}}},
		"protected":            {{Destination: "/proc/input", Source: "/proc/input", Type: valid.Type, Options: valid.Options}},
		"noncanonical":         {{Destination: "/opt/../input", Source: "/opt/../input", Type: valid.Type, Options: valid.Options}},
		"overlap": {valid, {Destination: "/opt/input/nested", Source: "/opt/input/nested", Type: valid.Type,
			Options: valid.Options}},
	}
	for name, mounts := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateReadonlyBindMounts(mounts); err == nil {
				t.Fatal("hostile read-only bind input was accepted")
			}
		})
	}
}

func TestSupportedRootPolicyLoadsBeforePrivilegedApplication(t *testing.T) {
	b := bundle(t, []string{"/probe"}, "")
	raw, err := os.ReadFile(filepath.Join(b, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["hostname"] = "sandbox-one"
	config["root"].(map[string]any)["readonly"] = false
	config["linux"] = map[string]any{"maskedPaths": []any{"/proc/kcore"}, "readonlyPaths": []any{"/proc/sys"}}
	raw, _ = json.Marshal(config)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadBundle(b)
	if err != nil || loaded.Hostname != "sandbox-one" || loaded.Linux == nil || len(loaded.Linux.MaskedPaths) != 1 {
		t.Fatalf("supported root policy = %+v, %v", loaded, err)
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

func TestUnsupportedOCIVersionFailsBeforeCreate(t *testing.T) {
	b := bundle(t, []string{"/probe"}, "")
	raw, err := os.ReadFile(filepath.Join(b, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["ociVersion"] = "9.9.9"
	raw, _ = json.Marshal(config)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = NewManager(true).Create("p1", b); err == nil || !strings.Contains(err.Error(), "unsupported OCI version") {
		t.Fatalf("Create() error = %v, want unsupported OCI version", err)
	}
}

func TestProcessShapeFailsBeforeGuestStateMutation(t *testing.T) {
	valid := ProcessSpec{User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, Args: []string{"/probe"}, Env: []string{"A=1"}, Cwd: "/"}
	tests := map[string]func(*ProcessSpec){
		"empty argv0":           func(spec *ProcessSpec) { spec.Args = []string{""} },
		"nul argument":          func(spec *ProcessSpec) { spec.Args = []string{"/probe", "bad\x00arg"} },
		"too many arguments":    func(spec *ProcessSpec) { spec.Args = make([]string, maxProcessArgs+1); spec.Args[0] = "/probe" },
		"oversized text":        func(spec *ProcessSpec) { spec.Args = []string{"/probe", strings.Repeat("x", maxProcessText)} },
		"invalid environment":   func(spec *ProcessSpec) { spec.Env = []string{"NOVALUE"} },
		"duplicate environment": func(spec *ProcessSpec) { spec.Env = []string{"A=1", "A=2"} },
		"noncanonical cwd":      func(spec *ProcessSpec) { spec.Cwd = "/work/../escape" },
		"duplicate groups":      func(spec *ProcessSpec) { spec.User.AdditionalGids = []uint32{1, 1} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := valid
			spec.Args = append([]string(nil), valid.Args...)
			spec.Env = append([]string(nil), valid.Env...)
			mutate(&spec)
			manager := NewManager(true)
			manager.processes["parent"] = &process{root: t.TempDir()}
			if err := manager.Exec("candidate", "parent", spec); err == nil {
				t.Fatal("hostile process shape accepted")
			}
			if _, exists := manager.processes["candidate"]; exists {
				t.Fatal("rejected process mutated guest state")
			}
		})
	}
}

func TestBundleRootRejectsSymlinkAndEscape(t *testing.T) {
	t.Run("escape", func(t *testing.T) {
		bundle := bundle(t, []string{"/probe"}, "")
		raw, _ := os.ReadFile(filepath.Join(bundle, "config.json"))
		var config map[string]any
		json.Unmarshal(raw, &config)
		config["root"].(map[string]any)["path"] = "../outside"
		raw, _ = json.Marshal(config)
		os.WriteFile(filepath.Join(bundle, "config.json"), raw, 0644)
		if _, _, err := LoadBundle(bundle); err == nil {
			t.Fatal("escaping root accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		bundle := bundle(t, []string{"/probe"}, "")
		realRoot := filepath.Join(bundle, "rootfs")
		linkedRoot := filepath.Join(bundle, "linked-root")
		if err := os.Symlink(realRoot, linkedRoot); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(bundle, "config.json"))
		var config map[string]any
		json.Unmarshal(raw, &config)
		config["root"].(map[string]any)["path"] = "linked-root"
		raw, _ = json.Marshal(config)
		os.WriteFile(filepath.Join(bundle, "config.json"), raw, 0644)
		if _, _, err := LoadBundle(bundle); err == nil {
			t.Fatal("symlink root accepted")
		}
	})
}

func TestLoadBundleRejectsUnsafeConfigIdentityAndSize(t *testing.T) {
	t.Run("hardlink", func(t *testing.T) {
		bundlePath := bundle(t, []string{"/probe"}, "")
		config := filepath.Join(bundlePath, "config.json")
		linked := filepath.Join(bundlePath, "config.link")
		if err := os.Link(config, linked); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadBundle(bundlePath); err == nil {
			t.Fatal("hard-linked config accepted")
		}
	})
	t.Run("writable", func(t *testing.T) {
		bundlePath := bundle(t, []string{"/probe"}, "")
		if err := os.Chmod(filepath.Join(bundlePath, "config.json"), 0664); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadBundle(bundlePath); err == nil {
			t.Fatal("group-writable config accepted")
		}
	})
	t.Run("oversized valid prefix", func(t *testing.T) {
		bundlePath := bundle(t, []string{"/probe"}, "")
		config := filepath.Join(bundlePath, "config.json")
		data, err := os.ReadFile(config)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, bytes.Repeat([]byte(" "), (1<<20)+1)...)
		if err = os.WriteFile(config, data, 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err = LoadBundle(bundlePath); err == nil {
			t.Fatal("oversized config with valid JSON prefix accepted")
		}
	})
}

func TestExecLifecycleFailuresCleanupAndConcurrency(t *testing.T) {
	manager := NewManager(true)
	bundlePath := bundle(t, []string{"/probe"}, "")
	_, _, err := LoadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Create("init", bundlePath); err != nil {
		t.Fatal(err)
	}
	spec := ProcessSpec{User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, Args: []string{"/probe", "-test.run=TestHelperProcess"}, Env: []string{"MK_AGENT_HELPER=1"}, Cwd: "/"}
	if err = manager.Exec("orphan", "missing-parent", spec); err == nil {
		t.Fatal("exec without a validated parent accepted")
	}
	if err = manager.Exec("exec-one", "init", spec); err != nil {
		t.Fatal(err)
	}
	if err = manager.Exec("exec-one", "init", spec); err == nil {
		t.Fatal("duplicate exec accepted")
	}
	if err = manager.Start("exec-one"); err != nil {
		t.Fatal(err)
	}
	if state, stateErr := manager.Wait("exec-one"); stateErr != nil || state.ExitCode != 17 {
		t.Fatalf("exec state=%+v error=%v", state, stateErr)
	}
	if err = manager.Delete("exec-one"); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.State("exec-one"); err == nil {
		t.Fatal("deleted exec remains")
	}
	if err = manager.Exec("Bad ID", "init", spec); err == nil {
		t.Fatal("invalid exec ID accepted")
	}
	bad := spec
	bad.Args = []string{"/missing"}
	if err = manager.Exec("exec-bad", "init", bad); err != nil {
		t.Fatal(err)
	}
	if err = manager.Start("exec-bad"); err == nil {
		t.Fatal("missing executable started")
	}
	if err = manager.Delete("exec-bad"); err != nil {
		t.Fatalf("delete after failed start: %v", err)
	}

	var group sync.WaitGroup
	errorsOut := make(chan error, 8)
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			id := fmt.Sprintf("exec-%d", index)
			if createErr := manager.Exec(id, "init", spec); createErr != nil {
				errorsOut <- createErr
				return
			}
			if startErr := manager.Start(id); startErr != nil {
				errorsOut <- startErr
				return
			}
			if _, waitErr := manager.Wait(id); waitErr != nil {
				errorsOut <- waitErr
				return
			}
			if deleteErr := manager.Delete(id); deleteErr != nil {
				errorsOut <- deleteErr
			}
		}(index)
	}
	group.Wait()
	close(errorsOut)
	for err := range errorsOut {
		t.Error(err)
	}
}

func TestExactArgvEnvironmentAndWorkingDirectory(t *testing.T) {
	manager := NewManager(true)
	bundlePath := bundle(t, []string{"/probe"}, "")
	_, root, err := LoadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(root, "work"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = manager.Create("init", bundlePath); err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"/probe", "-test.run=TestHelperProcess", "literal;not-a-shell", "$(also-literal)", "two words"}
	spec := ProcessSpec{
		User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())},
		Args: wantArgs,
		Env:  []string{"MK_AGENT_HELPER=1", "MK_AGENT_INSPECT_HELPER=1", "MK_AGENT_EXACT_ENV=exact value;$()"},
		Cwd:  "/work",
	}
	if err = manager.Exec("inspect", "init", spec); err != nil {
		t.Fatal(err)
	}
	if err = manager.Start("inspect"); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Wait("inspect")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Args []string `json:"args"`
		Env  string   `json:"env"`
		Cwd  string   `json:"cwd"`
	}
	if err = json.NewDecoder(strings.NewReader(state.Stdout)).Decode(&got); err != nil {
		t.Fatalf("decode helper output %q: %v", state.Stdout, err)
	}
	if fmt.Sprint(got.Args) != fmt.Sprint(wantArgs) {
		t.Fatalf("argv = %#v, want %#v", got.Args, wantArgs)
	}
	if got.Env != "exact value;$()" {
		t.Fatalf("environment = %q", got.Env)
	}
	if got.Cwd != filepath.Join(root, "work") {
		t.Fatalf("cwd = %q, want %q", got.Cwd, filepath.Join(root, "work"))
	}
}

func TestExitAndForcedKillMatrix(t *testing.T) {
	for _, test := range []struct {
		name string
		env  []string
		want int
	}{
		{name: "zero", env: []string{"MK_AGENT_HELPER=1", "MK_AGENT_EXIT_CODE=0"}, want: 0},
		{name: "nonzero", env: []string{"MK_AGENT_HELPER=1", "MK_AGENT_EXIT_CODE=23"}, want: 23},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager(true)
			bundlePath := bundle(t, []string{"/probe"}, "")
			_, _, err := LoadBundle(bundlePath)
			if err != nil {
				t.Fatal(err)
			}
			spec := ProcessSpec{User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, Args: []string{"/probe", "-test.run=TestHelperProcess"}, Env: test.env, Cwd: "/"}
			if err = manager.Create("init", bundlePath); err != nil {
				t.Fatal(err)
			}
			if err = manager.Exec("exit-test", "init", spec); err != nil {
				t.Fatal(err)
			}
			if err = manager.Start("exit-test"); err != nil {
				t.Fatal(err)
			}
			state, err := manager.Wait("exit-test")
			if err != nil || state.ExitCode != test.want {
				t.Fatalf("state=%+v error=%v, want exit %d", state, err, test.want)
			}
		})
	}
	t.Run("crash", func(t *testing.T) {
		manager := NewManager(true)
		bundlePath := bundle(t, []string{"/probe"}, "")
		_, root, err := LoadBundle(bundlePath)
		if err != nil {
			t.Fatal(err)
		}
		sleepBinary, err := os.ReadFile("/bin/sleep")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "sleep"), sleepBinary, 0755); err != nil {
			t.Fatal(err)
		}
		if err = manager.Create("init", bundlePath); err != nil {
			t.Fatal(err)
		}
		spec := ProcessSpec{User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, Args: []string{"/sleep", "60"}, Cwd: "/"}
		if err = manager.Exec("crash-test", "init", spec); err != nil {
			t.Fatal(err)
		}
		if err = manager.Start("crash-test"); err != nil {
			t.Fatal(err)
		}
		if err = manager.Signal("crash-test", syscall.SIGSEGV); err != nil {
			t.Fatal(err)
		}
		state, err := manager.Wait("crash-test")
		if err != nil || state.ExitCode != 128+int(syscall.SIGSEGV) {
			t.Fatalf("crash state=%+v error=%v", state, err)
		}
	})

	manager := NewManager(true)
	bundlePath := bundle(t, []string{"/probe"}, "")
	_, _, err := LoadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Create("init", bundlePath); err != nil {
		t.Fatal(err)
	}
	spec := ProcessSpec{User: User{UID: uint32(os.Getuid()), GID: uint32(os.Getgid())}, Args: []string{"/probe", "-test.run=TestHelperProcess"}, Env: []string{"MK_AGENT_HELPER=1", "MK_AGENT_IGNORE_TERM_HELPER=1"}, Cwd: "/"}
	if err = manager.Exec("forced-kill", "init", spec); err != nil {
		t.Fatal(err)
	}
	if err = manager.Start("forced-kill"); err != nil {
		t.Fatal(err)
	}
	ready := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		state, stateErr := manager.State("forced-kill")
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		if strings.Contains(state.Stdout, "ignore-term-ready") {
			ready = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ready {
		t.Fatal("ignore-term helper did not become ready")
	}
	if err = manager.Signal("forced-kill", syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if state, stateErr := manager.State("forced-kill"); stateErr != nil || state.Status != "RUNNING" {
		t.Fatalf("SIGTERM state=%+v error=%v", state, stateErr)
	}
	if err = manager.Signal("forced-kill", syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Wait("forced-kill")
	if err != nil || state.ExitCode != 128+int(syscall.SIGKILL) {
		t.Fatalf("forced kill state=%+v error=%v", state, err)
	}
}

func TestDeleteWaitRaceIsDeterministic(t *testing.T) {
	manager := NewManager(true)
	bundlePath := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	config, _, _ := LoadBundle(bundlePath)
	config.Process.Env = []string{"MK_AGENT_HELPER=1"}
	raw, _ := json.Marshal(config)
	os.WriteFile(filepath.Join(bundlePath, "config.json"), raw, 0644)
	if err := manager.Create("race", bundlePath); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start("race"); err != nil {
		t.Fatal(err)
	}
	for {
		state, err := manager.State("race")
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "STOPPED" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err := manager.Delete("race"); err == nil || !strings.Contains(err.Error(), "not been waited") {
		t.Fatalf("delete-before-wait error = %v", err)
	}
	if _, err := manager.Wait("race"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete("race"); err != nil {
		t.Fatal(err)
	}
}

func TestWrongExecutableArchitectureFailsBeforeStart(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe"}, "")
	probe := filepath.Join(b, "rootfs", "probe")
	raw, err := os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 || string(raw[:4]) != "\x7fELF" {
		t.Fatal("test executable is not ELF")
	}
	binary.LittleEndian.PutUint16(raw[18:20], uint16(40)) // EM_ARM
	if err = os.WriteFile(probe, raw, 0755); err != nil {
		t.Fatal(err)
	}
	if err = m.Create("p1", b); err != nil {
		t.Fatal(err)
	}
	if err = m.Start("p1"); err == nil || !strings.Contains(err.Error(), "architecture is not linux/amd64") {
		t.Fatalf("Start() error = %v, want architecture rejection", err)
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
	protocolFeatures, ok := capabilities["protocol_features"].([]string)
	if !ok || !slices.Contains(protocolFeatures, "signal-operation-id-v1") || !slices.Contains(protocolFeatures, "stdin-offset-v1") || !slices.Contains(protocolFeatures, "two-phase-shutdown-v1") {
		t.Fatalf("protocol features = %#v", capabilities["protocol_features"])
	}
	for _, feature := range features {
		if feature == "uid" || feature == "gid" || feature == "supplementary-groups" || feature == "signals" {
			t.Fatalf("unproved feature %q advertised", feature)
		}
	}
	for _, required := range []string{"no-new-privileges", "rlimits", "linux-capabilities", "hostname", "masked-paths", "readonly-paths", "readonly-root", "standard-mounts", "readonly-bind-inputs-v1"} {
		if !slices.Contains(features, required) {
			t.Fatalf("implemented feature %q not advertised: %v", required, features)
		}
	}
	kernel, ok := capabilities["kernel"].(map[string]any)
	if !ok || kernel["architecture"] != "amd64" || kernel["release"] == "" || kernel["release"] == "unknown" {
		t.Fatalf("actual kernel capabilities = %#v", capabilities["kernel"])
	}
	namespaces, ok := kernel["namespaces"].([]string)
	if !ok || len(namespaces) == 0 {
		t.Fatalf("reported namespaces = %#v", kernel["namespaces"])
	}
	agentCapabilities, ok := capabilities["agent"].(map[string]any)
	if !ok || agentCapabilities["effective_capabilities"] == "" || agentCapabilities["effective_capabilities"] == "unknown" {
		t.Fatalf("actual agent capabilities = %#v", capabilities["agent"])
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

func TestShutdownRequiresQuiescence(t *testing.T) {
	m := NewManager(true)
	b := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	c, _, err := LoadBundle(b)
	if err != nil {
		t.Fatal(err)
	}
	c.Process.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_STDIN_HELPER=1"}
	raw, _ := json.Marshal(c)
	if err = os.WriteFile(filepath.Join(b, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = m.Create("running", b); err != nil {
		t.Fatal(err)
	}
	if err = m.Start("running"); err != nil {
		t.Fatal(err)
	}

	server := &Server{Manager: m, SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "Shutdown"}
	Sign(&request, server.Token)
	if reply := server.Dispatch(request); !strings.Contains(reply.Error, "still running") {
		t.Fatalf("running Shutdown reply = %+v", reply)
	}
	if err = m.CloseStdin("running"); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Wait("running"); err != nil {
		t.Fatal(err)
	}
	request.Sequence++
	Sign(&request, server.Token)
	reply := server.Dispatch(request)
	body, ok := reply.Body.(map[string]string)
	if reply.Error != "" || !ok || body["status"] != "quiesced" {
		t.Fatalf("quiescent Shutdown reply = %+v", reply)
	}
}

func TestQuiesceIsIdempotentAndSealsMutations(t *testing.T) {
	calls := 0
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001,
		Token: []byte("01234567890123456789012345678901"), BeforeShutdown: func() error { calls++; return nil }}
	dispatch := func(sequence uint64, method string) Reply {
		request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint,
			Sequence: sequence, Method: method}
		Sign(&request, server.Token)
		return server.Dispatch(request)
	}
	for _, sequence := range []uint64{1, 2} {
		reply := dispatch(sequence, "Quiesce")
		body, ok := reply.Body.(map[string]string)
		if reply.Error != "" || !ok || body["status"] != "quiesced" {
			t.Fatalf("Quiesce reply = %+v", reply)
		}
	}
	if calls != 1 {
		t.Fatalf("storage quiescence calls = %d, want 1", calls)
	}
	if reply := dispatch(3, "DeleteProcess"); reply.Error != "agent is quiesced" {
		t.Fatalf("post-quiesce mutation reply = %+v", reply)
	}
	if reply := dispatch(4, "Shutdown"); reply.Error != "" {
		t.Fatalf("post-quiesce Shutdown reply = %+v", reply)
	}
}

func TestShutdownRequiresStorageQuiescenceBeforeSuccess(t *testing.T) {
	server := &Server{Manager: NewManager(true), SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001,
		Token: []byte("01234567890123456789012345678901"), BeforeShutdown: func() error { return errors.New("injected remount failure") }}
	request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "Shutdown"}
	Sign(&request, server.Token)
	reply := server.Dispatch(request)
	if reply.Error != "storage quiescence failed" {
		t.Fatalf("failed storage quiescence reply = %+v", reply)
	}
	server.BeforeShutdown = func() error { return nil }
	request.Sequence++
	Sign(&request, server.Token)
	reply = server.Dispatch(request)
	if reply.Error != "" {
		t.Fatalf("successful storage quiescence reply = %+v", reply)
	}
}

func TestShutdownRaceRequiresExplicitProcessCompletion(t *testing.T) {
	manager := NewManager(true)
	bundlePath := bundle(t, []string{"/probe", "-test.run=TestHelperProcess"}, "")
	config, _, err := LoadBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	config.Process.Env = []string{"MK_AGENT_HELPER=1", "MK_AGENT_STDIN_HELPER=1"}
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(filepath.Join(bundlePath, "config.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = manager.Create("running", bundlePath); err != nil {
		t.Fatal(err)
	}
	if err = manager.Start("running"); err != nil {
		t.Fatal(err)
	}
	server := &Server{Manager: manager, SandboxID: "box", Generation: "0123456789abcdef0123456789abcdef", Endpoint: 7001, Token: []byte("01234567890123456789012345678901")}
	request := Envelope{Version: 1, SandboxID: server.SandboxID, Generation: server.Generation, Endpoint: server.Endpoint, Sequence: 1, Method: "Shutdown"}
	Sign(&request, server.Token)
	closeDone := make(chan error, 1)
	go func() { closeDone <- manager.CloseStdin("running") }()
	reply := server.Dispatch(request)
	if err = <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Wait("running"); err != nil {
		t.Fatal(err)
	}
	if reply.Error == "" {
		body, ok := reply.Body.(map[string]string)
		if !ok || body["status"] != "quiesced" {
			t.Fatalf("successful racing Shutdown = %+v", reply)
		}
		return
	}
	if !strings.Contains(reply.Error, "still running") {
		t.Fatalf("racing Shutdown error = %q", reply.Error)
	}
	request.Sequence++
	Sign(&request, server.Token)
	if retry := server.Dispatch(request); retry.Error != "" {
		t.Fatalf("Shutdown retry after explicit wait = %+v", retry)
	}
}

func TestProcessGroupStatsAreBoundedToObservedGroup(t *testing.T) {
	manager := NewManager(true)
	manager.processes["self-group"] = &process{state: ProcessState{PID: syscall.Getpgrp(), Status: "RUNNING"}}
	stats, err := manager.Stats("self-group")
	if err != nil {
		t.Fatal(err)
	}
	if stats.PIDs == 0 || stats.RSSBytes == 0 {
		t.Fatalf("stats = %+v, want a nonempty process group", stats)
	}
	if ticks := clockTicks(); ticks == 0 {
		t.Fatal("clock tick discovery returned zero")
	}
}

func TestDNSReplacementRestoresRegularSymlinkAndAbsentState(t *testing.T) {
	for name, setup := range map[string]func(string) error{
		"regular": func(path string) error { return os.WriteFile(path, []byte("nameserver 10.0.0.1\n"), 0600) },
		"symlink": func(path string) error { return os.Symlink("../run/resolv.conf", path) },
		"absent":  func(string) error { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resolv.conf")
			if err := setup(path); err != nil {
				t.Fatal(err)
			}
			original, symlink, mode, existed, err := replaceDNS(path, []string{"192.0.2.53", "2001:db8::53"})
			if err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != "nameserver 192.0.2.53\nnameserver 2001:db8::53\n" {
				t.Fatalf("generated DNS=%q error=%v", data, err)
			}
			if err = restoreDNS(path, original, symlink, mode, existed); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "regular":
				if data, _ := os.ReadFile(path); string(data) != "nameserver 10.0.0.1\n" {
					t.Fatalf("restored DNS=%q", data)
				}
			case "symlink":
				if target, _ := os.Readlink(path); target != "../run/resolv.conf" {
					t.Fatalf("restored symlink=%q", target)
				}
			case "absent":
				if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("generated DNS remains: %v", err)
				}
			}
		})
	}
}

func TestGuestNetworkCommandsBoundOutputCancellationAndDescendants(t *testing.T) {
	command := func(body string) string {
		path := filepath.Join(t.TempDir(), "command")
		if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("caller deadline kills descendants", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		_, err := runBoundedNetworkCommand(ctx, command("sleep 60 & wait"))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("guest network command deadline error = %v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("guest network command cancellation took %s", elapsed)
		}
	})

	t.Run("overflow has bounded diagnostic", func(t *testing.T) {
		output, err := runBoundedNetworkCommand(context.Background(), command("head -c 1048577 /dev/zero"))
		if !errors.Is(err, boundedexec.ErrOutputLimit) {
			t.Fatalf("guest network command overflow error = %v", err)
		}
		if len(output) > 16<<10 {
			t.Fatalf("guest network command diagnostic length = %d", len(output))
		}
	})

	t.Run("captures combined output", func(t *testing.T) {
		output, err := runBoundedNetworkCommand(context.Background(), command("printf stdout; printf stderr >&2"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(output), "stdout") || !strings.Contains(string(output), "stderr") {
			t.Fatalf("guest network combined output = %q", output)
		}
	})
}

func TestGuestNetworkMutationRejectsCancelledContextWithoutLosingState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := NewManager(true)
	config := NetworkConfig{Name: "mk0", Address: "192.0.2.2/30", Gateway: "192.0.2.1", MTU: 1500}
	if err := m.ConfigureNetworkContext(ctx, config); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ConfigureNetwork error = %v", err)
	}
	if m.network != nil || m.networkName != "" || m.networkMTU != 0 {
		t.Fatalf("cancelled configuration mutated network state: name=%q mtu=%d file=%v", m.networkName, m.networkMTU, m.network)
	}
	m.networkName, m.networkMTU = "mk0", 1500
	if err := m.CloseNetworkContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CloseNetwork error = %v", err)
	}
	if m.networkName != "mk0" || m.networkMTU != 1500 {
		t.Fatalf("cancelled close discarded retry identity: name=%q mtu=%d", m.networkName, m.networkMTU)
	}
}

func TestConfigureNetworkAcceptsOnlyExactCompletedReplay(t *testing.T) {
	config := NetworkConfig{Name: "mk0", Address: "192.0.2.2/30", Gateway: "192.0.2.1", MTU: 1500,
		Nameservers: []string{"192.0.2.53"}}
	descriptor, err := os.CreateTemp(t.TempDir(), "tun")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(true)
	m.network, m.networkName, m.networkMTU, m.dnsManaged = descriptor, config.Name, config.MTU, true
	m.networkConfig = config
	m.networkConfig.Nameservers = append([]string(nil), config.Nameservers...)
	if err = m.ConfigureNetworkContext(context.Background(), config); err != nil {
		t.Fatalf("exact network replay failed: %v", err)
	}
	different := config
	different.Gateway = "192.0.2.3"
	if err = m.ConfigureNetworkContext(context.Background(), different); err == nil {
		t.Fatal("different network replay was accepted")
	}
	if m.network != descriptor || m.networkConfig.Gateway != config.Gateway {
		t.Fatalf("rejected replay changed ownership: network=%v config=%+v", m.network, m.networkConfig)
	}
	// Isolate the descriptor close from link/DNS integration while proving a
	// completed close clears the replay identity.
	m.networkName, m.networkMTU, m.dnsManaged = "", 0, false
	if err = m.CloseNetworkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.networkConfig.Name != "" {
		t.Fatalf("completed close retained network replay identity: %+v", m.networkConfig)
	}
}

func TestCloseNetworkRetainsReplayIdentityUntilRetryCompletes(t *testing.T) {
	descriptor, err := os.CreateTemp(t.TempDir(), "closed-tun")
	if err != nil {
		t.Fatal(err)
	}
	if err = descriptor.Close(); err != nil {
		t.Fatal(err)
	}
	config := NetworkConfig{Name: "mk0", Address: "192.0.2.2/30", Gateway: "192.0.2.1", MTU: 1500}
	m := NewManager(true)
	m.network, m.networkConfig = descriptor, config
	if err = m.CloseNetworkContext(context.Background()); err == nil {
		t.Fatal("injected descriptor close failure was ignored")
	}
	if m.networkConfig.Name != config.Name {
		t.Fatal("failed close discarded network replay identity")
	}
	if err = m.CloseNetworkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.networkConfig.Name != "" {
		t.Fatal("successful close retry retained network replay identity")
	}
}

func TestCloseNetworkRestoresDNSIdempotentlyAndRetainsFailedCleanup(t *testing.T) {
	t.Run("failed link deletion remains retryable", func(t *testing.T) {
		m := NewManager(true)
		m.networkName, m.networkMTU = "mk0", 1500
		attempts := 0
		m.networkExec = func(context.Context, ...string) ([]byte, error) {
			attempts++
			if attempts == 1 {
				return []byte("busy"), errors.New("injected delete failure")
			}
			return nil, nil
		}
		if err := m.CloseNetworkContext(context.Background()); err == nil {
			t.Fatal("injected link deletion failure was ignored")
		}
		if m.networkName != "mk0" || m.networkMTU != 1500 {
			t.Fatalf("failed close discarded retry identity: name=%q mtu=%d", m.networkName, m.networkMTU)
		}
		if err := m.CloseNetworkContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if m.networkName != "" || m.networkMTU != 0 || attempts != 2 {
			t.Fatalf("retried close state: name=%q mtu=%d attempts=%d", m.networkName, m.networkMTU, attempts)
		}
	})

	t.Run("repeated close preserves restored file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(path, []byte("nameserver 192.0.2.53\n"), 0644); err != nil {
			t.Fatal(err)
		}
		m := NewManager(true)
		m.dnsPath = path
		m.dnsOriginal = []byte("nameserver 10.0.0.1\n")
		m.dnsMode, m.dnsExisted, m.dnsManaged = 0600, true, true
		if err := m.CloseNetworkContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := m.CloseNetworkContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "nameserver 10.0.0.1\n" {
			t.Fatalf("DNS after repeated close = %q, %v", data, err)
		}
		if m.dnsManaged {
			t.Fatal("successful DNS restoration retained cleanup ownership")
		}
	})

	t.Run("failed restore remains retryable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "occupied"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		m := NewManager(true)
		m.dnsPath, m.dnsManaged = path, true
		if err := m.CloseNetworkContext(context.Background()); err == nil {
			t.Fatal("non-empty DNS replacement directory was removed")
		}
		if !m.dnsManaged {
			t.Fatal("failed DNS restoration discarded retry ownership")
		}
	})
}
