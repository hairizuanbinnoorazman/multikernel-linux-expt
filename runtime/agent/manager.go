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
	"time"

	"github.com/containerd/console"
	"golang.org/x/sys/unix"
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
	terminal       console.Console
	stdout, stderr bytes.Buffer
	outputDone     chan struct{}
	done           chan struct{}
	state          ProcessState
}
type Manager struct {
	mu        sync.Mutex
	processes map[string]*process
	network   *os.File
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

// Exec creates an additional process in an existing container root. The caller
// supplies the already validated root because exec requests carry an OCI
// process, not a second OCI bundle.
func (m *Manager) Exec(id, root string, spec ProcessSpec) error {
	if !validProcessID(id) {
		return errors.New("invalid process ID")
	}
	if !filepath.IsAbs(root) {
		return errors.New("root must be absolute")
	}
	if len(spec.Args) == 0 || spec.NoNewPrivileges || len(spec.Rlimits) > 0 || len(spec.Capabilities) > 0 {
		return errors.New("unsupported exec process configuration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.processes[id]; ok {
		return errors.New("process exists")
	}
	m.processes[id] = &process{spec: spec, root: root, done: make(chan struct{}), state: ProcessState{ID: id, Status: "CREATED"}}
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
	return m.StartWithSize(id, 0, 0, false)
}

func (m *Manager) StartWithSize(id string, width, height uint32, sizeSet bool) error {
	if width > 65535 || height > 65535 {
		return errors.New("terminal dimensions exceed the Linux PTY limit")
	}
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
	var slave *os.File
	if p.spec.Terminal {
		var slavePath string
		var err error
		p.terminal, slavePath, err = console.NewPty()
		if err != nil {
			m.mu.Unlock()
			return err
		}
		slave, err = os.OpenFile(slavePath, os.O_RDWR, 0)
		if err != nil {
			p.terminal.Close()
			p.terminal = nil
			m.mu.Unlock()
			return err
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
		cmd.SysProcAttr.Setpgid = false
		cmd.SysProcAttr.Setsid = true
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Ctty = 0
		p.outputDone = make(chan struct{})
		if sizeSet {
			if err = p.terminal.Resize(console.WinSize{Width: uint16(width), Height: uint16(height)}); err != nil {
				slave.Close()
				p.terminal.Close()
				p.terminal = nil
				m.mu.Unlock()
				return err
			}
		}
	} else {
		if sizeSet {
			m.mu.Unlock()
			return errors.New("cannot size a process without a terminal")
		}
		cmd.Stdout = &p.stdout
		cmd.Stderr = &p.stderr
	}
	if e := cmd.Start(); e != nil {
		if slave != nil {
			slave.Close()
		}
		if p.terminal != nil {
			p.terminal.Close()
			p.terminal = nil
		}
		m.mu.Unlock()
		return e
	}
	if slave != nil {
		slave.Close()
		go func() {
			_, _ = io.Copy(&p.stdout, p.terminal)
			close(p.outputDone)
		}()
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
	if p.outputDone != nil {
		<-p.outputDone
	}
	code := 0
	if e != nil {
		var x *exec.ExitError
		if errors.As(e, &x) {
			if status, ok := x.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				code = 128 + int(status.Signal())
			} else {
				code = x.ExitCode()
			}
		} else {
			code = 255
		}
	}
	m.mu.Lock()
	if p.terminal != nil {
		_ = p.terminal.Close()
	}
	p.state.Status = "STOPPED"
	p.state.ExitCode = code
	p.state.Stdout = p.stdout.String()
	p.state.Stderr = p.stderr.String()
	p.terminal = nil
	close(p.done)
	m.mu.Unlock()
}

func (m *Manager) Resize(id string, width, height uint32) error {
	if width > 65535 || height > 65535 {
		return errors.New("terminal dimensions exceed the Linux PTY limit")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok || p.state.Status != "RUNNING" {
		return errors.New("process is not running")
	}
	if p.terminal == nil {
		return errors.New("process has no terminal")
	}
	return p.terminal.Resize(console.WinSize{Width: uint16(width), Height: uint16(height)})
}
func (m *Manager) Signal(id string, sig syscall.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.processes[id]
	if !ok || p.cmd == nil || p.state.Status != "RUNNING" {
		return errors.New("process is not running")
	}
	return syscall.Kill(-p.cmd.Process.Pid, sig)
}

func (m *Manager) Quiescent() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.processes {
		if p.state.Status == "RUNNING" {
			return false
		}
	}
	return true
}
func (m *Manager) Wait(id string) (ProcessState, error) {
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return ProcessState{}, errors.New("process not found")
	}
	if p.state.Status == "CREATED" {
		m.mu.Unlock()
		return ProcessState{}, errors.New("process is not started")
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

const (
	tunSetIFF = 0x400454ca
	iffTun    = 0x0001
	iffNoPI   = 0x1000
)

// ConfigureNetwork creates the child side of the primary-mediated point to
// point link. Packets share the authenticated agent channel because the
// pinned Multikernel VSOCK transport cannot sustain a second stream.
func (m *Manager) ConfigureNetwork(name, address, gateway string) error {
	if name == "" || address == "" || gateway == "" {
		return errors.New("network name, address, and gateway are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.network != nil {
		return errors.New("network is already configured")
	}
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	request, err := unix.NewIfreq(name)
	if err != nil {
		f.Close()
		return err
	}
	request.SetUint16(iffTun | iffNoPI)
	if err = unix.IoctlIfreq(int(f.Fd()), tunSetIFF, request); err != nil {
		f.Close()
		return err
	}
	commands := [][]string{
		{"address", "add", address, "dev", name},
		{"link", "set", name, "mtu", "1400", "up"},
		{"route", "add", "default", "via", gateway, "dev", name},
	}
	for _, args := range commands {
		if output, commandErr := exec.Command("/bin/ip", args...).CombinedOutput(); commandErr != nil {
			f.Close()
			return fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), commandErr, strings.TrimSpace(string(output)))
		}
	}
	if err = os.Remove("/bundle/rootfs/etc/resolv.conf"); err != nil && !errors.Is(err, os.ErrNotExist) {
		f.Close()
		return err
	}
	if err = os.WriteFile("/bundle/rootfs/etc/resolv.conf", []byte("nameserver 8.8.8.8\n"), 0644); err != nil {
		f.Close()
		return err
	}
	m.network = f
	return nil
}

// ExchangeNetwork injects at most one primary packet and returns at most one
// child packet. The short deadline keeps lifecycle RPCs responsive.
func (m *Manager) ExchangeNetwork(packet []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.network == nil {
		return nil, errors.New("network is not configured")
	}
	if len(packet) > 65535 {
		return nil, errors.New("network packet is too large")
	}
	if len(packet) != 0 {
		if _, err := unix.Write(int(m.network.Fd()), packet); err != nil {
			return nil, err
		}
	}
	out := make([]byte, 65535)
	for deadline := time.Now().Add(2 * time.Millisecond); ; {
		n, err := unix.Read(int(m.network.Fd()), out)
		if err == nil {
			return out[:n], nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func (m *Manager) CloseNetwork() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.network == nil {
		return nil
	}
	err := m.network.Close()
	m.network = nil
	return err
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
