package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type User struct {
	UID            uint32   `json:"uid"`
	GID            uint32   `json:"gid"`
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`
}
type Rlimit struct {
	Type string `json:"type"`
	Hard uint64 `json:"hard"`
	Soft uint64 `json:"soft"`
}
type ProcessSpec struct {
	Terminal        bool                `json:"terminal,omitempty"`
	User            User                `json:"user"`
	Args            []string            `json:"args"`
	Env             []string            `json:"env,omitempty"`
	Cwd             string              `json:"cwd"`
	NoNewPrivileges bool                `json:"noNewPrivileges,omitempty"`
	Rlimits         []Rlimit            `json:"rlimits,omitempty"`
	Capabilities    map[string][]string `json:"capabilities,omitempty"`
}
type Root struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly,omitempty"`
}
type OCIConfig struct {
	OCIVersion string                     `json:"ociVersion"`
	Process    ProcessSpec                `json:"process"`
	Root       Root                       `json:"root"`
	Hostname   string                     `json:"hostname,omitempty"`
	Mounts     []json.RawMessage          `json:"mounts,omitempty"`
	Hooks      map[string]json.RawMessage `json:"hooks,omitempty"`
	Linux      *struct {
		Namespaces    []json.RawMessage `json:"namespaces,omitempty"`
		Resources     json.RawMessage   `json:"resources,omitempty"`
		Seccomp       json.RawMessage   `json:"seccomp,omitempty"`
		MaskedPaths   []string          `json:"maskedPaths,omitempty"`
		ReadonlyPaths []string          `json:"readonlyPaths,omitempty"`
	} `json:"linux,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}
type ProcessState struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	PID      int    `json:"pid,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}
type process struct {
	spec           ProcessSpec
	root           string
	cmd            *exec.Cmd
	stdout, stderr bytes.Buffer
	done           chan struct{}
	state          ProcessState
}
type Manager struct {
	mu        sync.Mutex
	processes map[string]*process
	NoChroot  bool
}

func NewManager(noChroot bool) *Manager {
	return &Manager{processes: map[string]*process{}, NoChroot: noChroot}
}

func strictJSON(path string, v any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return e
	}
	var x any
	if e = d.Decode(&x); e != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}
func LoadBundle(bundle string) (OCIConfig, string, error) {
	if !filepath.IsAbs(bundle) {
		return OCIConfig{}, "", errors.New("bundle must be absolute")
	}
	var c OCIConfig
	if e := strictJSON(filepath.Join(bundle, "config.json"), &c); e != nil {
		return c, "", e
	}
	if len(c.Process.Args) == 0 {
		return c, "", errors.New("process.args is required")
	}
	if c.Process.Terminal {
		return c, "", errors.New("terminal is unsupported")
	}
	if c.Process.NoNewPrivileges {
		return c, "", errors.New("noNewPrivileges is not implemented")
	}
	if len(c.Process.Rlimits) > 0 {
		return c, "", errors.New("rlimits are not implemented")
	}
	if c.Root.Readonly {
		return c, "", errors.New("read-only root is not implemented")
	}
	if c.Hostname != "" {
		return c, "", errors.New("hostname is not implemented")
	}
	if len(c.Process.Capabilities) > 0 {
		return c, "", errors.New("capabilities are not implemented")
	}
	if len(c.Mounts) > 0 || len(c.Hooks) > 0 {
		return c, "", errors.New("mounts and hooks are not implemented")
	}
	if c.Linux != nil && (len(c.Linux.Namespaces) > 0 || len(c.Linux.Resources) > 0 || len(c.Linux.Seccomp) > 0 || len(c.Linux.MaskedPaths) > 0 || len(c.Linux.ReadonlyPaths) > 0) {
		return c, "", errors.New("Linux namespaces/resources/seccomp/path controls are not implemented")
	}
	root := c.Root.Path
	if !filepath.IsAbs(root) {
		root = filepath.Join(bundle, root)
	}
	root, e := filepath.Abs(root)
	if e != nil {
		return c, "", e
	}
	st, e := os.Stat(root)
	if e != nil || !st.IsDir() {
		return c, "", errors.New("root.path is not a directory")
	}
	return c, root, nil
}
func validProcessID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func (m *Manager) Create(id, bundle string) error {
	if !validProcessID(id) {
		return errors.New("invalid process ID")
	}
	c, root, e := LoadBundle(bundle)
	if e != nil {
		return e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.processes[id]; ok {
		return errors.New("process exists")
	}
	m.processes[id] = &process{spec: c.Process, root: root, done: make(chan struct{}), state: ProcessState{ID: id, Status: "CREATED"}}
	return nil
}
func envList(v []string) error {
	for _, x := range v {
		p := strings.IndexByte(x, '=')
		if p < 1 || strings.IndexByte(x[:p], 0) >= 0 {
			return fmt.Errorf("invalid environment entry")
		}
	}
	return nil
}
func gids(v []uint32) []uint32 { r := make([]uint32, len(v)); copy(r, v); return r }
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("process not found")
	}
	if p.state.Status == "RUNNING" {
		m.mu.Unlock()
		return nil
	}
	if p.state.Status != "CREATED" {
		m.mu.Unlock()
		return errors.New("process is not created")
	}
	if e := envList(p.spec.Env); e != nil {
		m.mu.Unlock()
		return e
	}
	exe := p.spec.Args[0]
	if !strings.HasPrefix(exe, "/") {
		m.mu.Unlock()
		return errors.New("argv[0] must be absolute")
	}
	cmd := exec.Command(exe, p.spec.Args[1:]...)
	cmd.Env = append([]string(nil), p.spec.Env...)
	cmd.Dir = p.spec.Cwd
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if !m.NoChroot {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: p.spec.User.UID, Gid: p.spec.User.GID, Groups: gids(p.spec.User.AdditionalGids), NoSetGroups: false}
		cmd.SysProcAttr.Chroot = p.root
	} else {
		cmd.Path = filepath.Join(p.root, exe)
		if p.spec.Cwd != "" {
			cmd.Dir = filepath.Join(p.root, p.spec.Cwd)
		}
	}
	if e := cmd.Start(); e != nil {
		m.mu.Unlock()
		return e
	}
	p.cmd = cmd
	p.state.Status = "RUNNING"
	p.state.PID = cmd.Process.Pid
	m.mu.Unlock()
	go m.wait(p)
	return nil
}
func (m *Manager) wait(p *process) {
	e := p.cmd.Wait()
	code := 0
	if e != nil {
		var x *exec.ExitError
		if errors.As(e, &x) {
			code = x.ExitCode()
		} else {
			code = 255
		}
	}
	m.mu.Lock()
	p.state.Status = "STOPPED"
	p.state.ExitCode = code
	p.state.Stdout = p.stdout.String()
	p.state.Stderr = p.stderr.String()
	close(p.done)
	m.mu.Unlock()
}
func (m *Manager) Signal(id string, sig syscall.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok || p.cmd == nil || p.state.Status != "RUNNING" {
		return errors.New("process is not running")
	}
	return p.cmd.Process.Signal(sig)
}
func (m *Manager) Wait(id string) (ProcessState, error) {
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return ProcessState{}, errors.New("process not found")
	}
	done := p.done
	m.mu.Unlock()
	<-done
	m.mu.Lock()
	defer m.mu.Unlock()
	return p.state, nil
}
func (m *Manager) State(id string) (ProcessState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok {
		return ProcessState{}, errors.New("process not found")
	}
	x := p.state
	x.Stdout = p.stdout.String()
	x.Stderr = p.stderr.String()
	return x, nil
}
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok {
		return nil
	}
	if p.state.Status == "RUNNING" {
		return errors.New("process is running")
	}
	delete(m.processes, id)
	return nil
}
func SignalNumber(s string) (syscall.Signal, error) {
	if n, e := strconv.Atoi(s); e == nil && n > 0 && n < 65 {
		return syscall.Signal(n), nil
	}
	m := map[string]syscall.Signal{"SIGTERM": syscall.SIGTERM, "SIGKILL": syscall.SIGKILL, "SIGINT": syscall.SIGINT, "SIGHUP": syscall.SIGHUP}
	v, ok := m[s]
	if !ok {
		return 0, errors.New("unsupported signal")
	}
	return v, nil
}
