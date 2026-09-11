package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/console"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
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
	NoNewPrivileges *bool               `json:"noNewPrivileges,omitempty"`
	Rlimits         []Rlimit            `json:"rlimits,omitempty"`
	Capabilities    map[string][]string `json:"capabilities,omitempty"`
}
type Root struct {
	Path     string `json:"path"`
	Readonly *bool  `json:"readonly,omitempty"`
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
	ID              string `json:"id"`
	Status          string `json:"status"`
	PID             int    `json:"pid,omitempty"`
	ExitCode        int    `json:"exit_code,omitempty"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}
type ProcessStats struct {
	CPUUserNS   uint64 `json:"cpu_user_ns"`
	CPUSystemNS uint64 `json:"cpu_system_ns"`
	RSSBytes    uint64 `json:"rss_bytes"`
	PIDs        uint64 `json:"pids"`
}
type process struct {
	spec           ProcessSpec
	root           string
	cmd            *exec.Cmd
	terminal       console.Console
	stdin          io.WriteCloser
	inputMu        sync.Mutex
	stdinClosed    bool
	stdinOffset    uint64
	stdinLastStart uint64
	stdinLastSize  uint64
	stdinLastHash  [sha256.Size]byte
	stdinHasLast   bool
	stdout, stderr lockedBuffer
	outputDone     chan struct{}
	done           chan struct{}
	state          ProcessState
	waited         bool
}

type lockedBuffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	base      uint64
	space     chan struct{}
	truncated bool
}

const (
	maxProcesses       = 1024
	maxOutputBytes     = 4 << 20
	maxProcessArgs     = 256
	maxProcessEnv      = 1024
	maxProcessText     = 128 << 10
	maxSupplementalGID = 256
)

func (b *lockedBuffer) Write(p []byte) (int, error) {
	total := len(p)
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	for len(p) > 0 {
		b.mu.Lock()
		if b.space == nil {
			b.space = make(chan struct{})
		}
		available := maxOutputBytes - b.b.Len()
		if available > 0 {
			if available > len(p) {
				available = len(p)
			}
			_, _ = b.b.Write(p[:available])
			p = p[available:]
			b.mu.Unlock()
			continue
		}
		space := b.space
		b.mu.Unlock()
		select {
		case <-space:
		case <-timer.C:
			b.mu.Lock()
			b.truncated = true
			b.mu.Unlock()
			return total, nil
		}
	}
	return total, nil
}

func (b *lockedBuffer) slice(offset, limit uint64) ([]byte, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if offset > b.base {
		discard := offset - b.base
		if discard > uint64(b.b.Len()) {
			discard = uint64(b.b.Len())
		}
		if discard > 0 {
			b.b.Next(int(discard))
			b.base += discard
			if b.space != nil {
				close(b.space)
				b.space = make(chan struct{})
			}
		}
	}
	if offset < b.base {
		offset = b.base
	}
	end := b.base + uint64(b.b.Len())
	if offset >= end {
		return nil, end
	}
	if limit > 0 && end-offset > limit {
		end = offset + limit
	}
	startIndex := offset - b.base
	endIndex := end - b.base
	data := append([]byte(nil), b.b.Bytes()[startIndex:endIndex]...)
	return data, end
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (b *lockedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

type Manager struct {
	mu          sync.Mutex
	processes   map[string]*process
	network     *os.File
	networkName string
	networkMTU  int
	networkExec func(context.Context, ...string) ([]byte, error)
	dnsOriginal []byte
	dnsSymlink  string
	dnsMode     os.FileMode
	dnsExisted  bool
	dnsManaged  bool
	dnsPath     string
	policySet   bool
	NoChroot    bool
}

func NewManager(noChroot bool) *Manager {
	return &Manager{processes: map[string]*process{}, networkExec: runNetworkCommand, dnsPath: "/bundle/rootfs/etc/resolv.conf", NoChroot: noChroot}
}

func strictJSON(path string, v any) error {
	descriptor, e := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(descriptor), path)
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return e
	}
	identity, ok := before.Sys().(*syscall.Stat_t)
	if !ok || !before.Mode().IsRegular() || identity.Nlink != 1 || identity.Uid != uint32(os.Geteuid()) || before.Mode().Perm()&0022 != 0 || before.Size() > 1<<20 {
		return errors.New("JSON input must be a bounded private caller-owned single-link regular file")
	}
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil || len(data) > 1<<20 {
		return errors.New("JSON input exceeds the retained byte limit")
	}
	after, e := f.Stat()
	if e != nil {
		return e
	}
	afterIdentity, ok := after.Sys().(*syscall.Stat_t)
	if !ok || identity.Dev != afterIdentity.Dev || identity.Ino != afterIdentity.Ino ||
		identity.Size != afterIdentity.Size || identity.Mtim != afterIdentity.Mtim || identity.Ctim != afterIdentity.Ctim {
		return errors.New("JSON input identity changed while reading")
	}
	d := json.NewDecoder(bytes.NewReader(data))
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
	if e := secureDirectory(bundle); e != nil {
		return OCIConfig{}, "", e
	}
	var c OCIConfig
	configPath := filepath.Join(bundle, "config.json")
	if e := strictJSON(configPath, &c); e != nil {
		return c, "", e
	}
	if c.OCIVersion != "1.1.0" {
		return c, "", fmt.Errorf("unsupported OCI version %q", c.OCIVersion)
	}
	if e := validateProcessSpec(c.Process); e != nil {
		return c, "", e
	}
	if c.Mounts != nil || c.Hooks != nil {
		return c, "", errors.New("mounts and hooks are not implemented")
	}
	if c.Linux != nil {
		if c.Linux.Namespaces != nil || c.Linux.Resources != nil || c.Linux.Seccomp != nil {
			return c, "", errors.New("Linux namespaces/resources/seccomp are not implemented in the guest projection")
		}
		if err := validateRootPolicy(c.Linux.MaskedPaths, c.Linux.ReadonlyPaths); err != nil {
			return c, "", err
		}
	}
	if c.Annotations != nil {
		return c, "", errors.New("annotations are not implemented")
	}
	root := c.Root.Path
	if filepath.IsAbs(root) || root == "" || filepath.Clean(root) != root || root == ".." || strings.HasPrefix(root, "../") {
		return c, "", errors.New("root.path must be a safe bundle-relative directory")
	}
	root = filepath.Join(bundle, root)
	if e := secureDirectory(root); e != nil {
		return c, "", e
	}
	return c, root, nil
}

func secureDirectory(directory string) error {
	if !filepath.IsAbs(directory) {
		return errors.New("directory must be absolute")
	}
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Clean(directory), string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return errors.New("root path component is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("root path must contain only real directories")
		}
	}
	return nil
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
	if len(m.processes) >= maxProcesses {
		return errors.New("process retention limit reached")
	}
	if !m.policySet {
		if m.NoChroot && (c.Hostname != "" || c.Root.Readonly != nil || c.Linux != nil) {
			return errors.New("root policy cannot be applied in no-chroot test mode")
		}
		if e = applyRootPolicy(c, root); e != nil {
			return e
		}
		m.policySet = true
	}
	m.processes[id] = &process{spec: c.Process, root: root, done: make(chan struct{}), state: ProcessState{ID: id, Status: "CREATED"}}
	return nil
}

// Exec creates an additional process in the validated root of an existing
// parent process. Protocol callers cannot select a new host path.
func (m *Manager) Exec(id, parentID string, spec ProcessSpec) error {
	if !validProcessID(id) {
		return errors.New("invalid process ID")
	}
	if err := validateProcessSpec(spec); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, ok := m.processes[parentID]
	if !ok {
		return errors.New("parent process not found")
	}
	root := parent.root
	if err := secureDirectory(root); err != nil {
		return err
	}
	if _, ok := m.processes[id]; ok {
		return errors.New("process exists")
	}
	if len(m.processes) >= maxProcesses {
		return errors.New("process retention limit reached")
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

func validateProcessSpec(spec ProcessSpec) error {
	if len(spec.Args) == 0 || len(spec.Args) > maxProcessArgs || spec.Args[0] == "" {
		return errors.New("process args must contain a bounded non-empty argv[0]")
	}
	textBytes := 0
	for _, argument := range spec.Args {
		if strings.IndexByte(argument, 0) >= 0 {
			return errors.New("process args contain NUL")
		}
		textBytes += len(argument) + 1
		if textBytes > maxProcessText {
			return errors.New("process args exceed the retained byte limit")
		}
	}
	if len(spec.Env) > maxProcessEnv {
		return errors.New("too many process environment entries")
	}
	seenEnvironment := make(map[string]struct{}, len(spec.Env))
	for _, value := range spec.Env {
		separator := strings.IndexByte(value, '=')
		if separator < 1 || strings.IndexByte(value, 0) >= 0 {
			return errors.New("invalid environment entry")
		}
		name := value[:separator]
		if _, duplicate := seenEnvironment[name]; duplicate {
			return fmt.Errorf("duplicate environment name %q", name)
		}
		seenEnvironment[name] = struct{}{}
		textBytes += len(value) + 1
		if textBytes > maxProcessText {
			return errors.New("process args and environment exceed the retained byte limit")
		}
	}
	if spec.Cwd == "" || !filepath.IsAbs(spec.Cwd) || filepath.Clean(spec.Cwd) != spec.Cwd || strings.IndexByte(spec.Cwd, 0) >= 0 || len(spec.Cwd) > 4096 {
		return errors.New("process cwd must be absolute, canonical, and bounded")
	}
	if len(spec.User.AdditionalGids) > maxSupplementalGID {
		return errors.New("too many supplemental groups")
	}
	seenGroups := make(map[uint32]struct{}, len(spec.User.AdditionalGids))
	for _, group := range spec.User.AdditionalGids {
		if _, duplicate := seenGroups[group]; duplicate {
			return fmt.Errorf("duplicate supplemental group %d", group)
		}
		seenGroups[group] = struct{}{}
	}
	return validateExecConstraints(spec)
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
	if e := validateExecutableArchitecture(filepath.Join(p.root, exe)); e != nil {
		m.mu.Unlock()
		return e
	}
	commandPath := exe
	commandArgs := p.spec.Args[1:]
	if p.spec.NoNewPrivileges != nil || p.spec.Rlimits != nil || p.spec.Capabilities != nil {
		constraints, constraintErr := encodeExecConstraints(p.spec)
		if constraintErr != nil {
			m.mu.Unlock()
			return constraintErr
		}
		commandPath = "/mk-agent"
		commandArgs = append([]string{"__oci_exec", constraints}, p.spec.Args...)
	}
	cmd := exec.Command(commandPath, commandArgs...)
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
	var err error
	if p.spec.Terminal {
		var slavePath string
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
		p.stdin, err = cmd.StdinPipe()
		if err != nil {
			m.mu.Unlock()
			return err
		}
		cmd.Stdout = &p.stdout
		cmd.Stderr = &p.stderr
	}
	if e := cmd.Start(); e != nil {
		p.inputMu.Lock()
		if p.stdin != nil {
			_ = p.stdin.Close()
			p.stdin = nil
		}
		p.stdinClosed = false
		p.inputMu.Unlock()
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
		p.stdin = p.terminal
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

func validateExecutableArchitecture(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("executable is not a regular file")
	}
	binary, err := elf.Open(path)
	if err != nil {
		return errors.New("executable is not a supported ELF image")
	}
	defer binary.Close()
	if binary.FileHeader.Class != elf.ELFCLASS64 || binary.FileHeader.Machine != elf.EM_X86_64 {
		return fmt.Errorf("executable architecture is not linux/amd64: class=%s machine=%s", binary.FileHeader.Class, binary.FileHeader.Machine)
	}
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
	p.inputMu.Lock()
	if !p.stdinClosed {
		p.stdinClosed = true
		if p.stdin != nil && !p.spec.Terminal {
			_ = p.stdin.Close()
		}
	}
	p.inputMu.Unlock()
	if p.terminal != nil {
		_ = p.terminal.Close()
	}
	p.state.Status = "STOPPED"
	p.state.ExitCode = code
	p.state.Stdout = p.stdout.String()
	p.state.Stderr = p.stderr.String()
	p.state.StdoutTruncated = p.stdout.Truncated()
	p.state.StderrTruncated = p.stderr.Truncated()
	p.terminal = nil
	close(p.done)
	m.mu.Unlock()
}

func (m *Manager) Write(id string, data []byte) error {
	m.mu.Lock()
	p, ok := m.processes[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("process is not running")
	}
	p.inputMu.Lock()
	offset := p.stdinOffset
	p.inputMu.Unlock()
	_, err := m.WriteAt(id, offset, data)
	return err
}

// WriteAt delivers one stdin chunk exactly once for a live agent process.
// Replaying the most recently acknowledged offset and bytes is idempotent,
// which makes a lost transport response safe to retry after reconnect.
func (m *Manager) WriteAt(id string, offset uint64, data []byte) (uint64, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return 0, errors.New("stdin chunk must contain between 1 byte and 64 KiB")
	}
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return 0, errors.New("process is not running")
	}
	p.inputMu.Lock()
	running := p.state.Status == "RUNNING"
	m.mu.Unlock()
	defer p.inputMu.Unlock()
	digest := sha256.Sum256(data)
	if p.stdinHasLast && offset == p.stdinLastStart && uint64(len(data)) == p.stdinLastSize && digest == p.stdinLastHash &&
		p.stdinOffset == offset+uint64(len(data)) {
		return p.stdinOffset, nil
	}
	if !running {
		return p.stdinOffset, errors.New("process is not running")
	}
	if p.stdinClosed || p.stdin == nil {
		return p.stdinOffset, errors.New("process stdin is closed")
	}
	if offset != p.stdinOffset || uint64(len(data)) > ^uint64(0)-offset {
		return p.stdinOffset, errors.New("stdin chunk offset differs from the exact acknowledged position")
	}
	written, err := p.stdin.Write(data)
	if err != nil {
		return p.stdinOffset, err
	}
	if written != len(data) {
		return p.stdinOffset, io.ErrShortWrite
	}
	p.stdinLastStart, p.stdinLastSize, p.stdinLastHash, p.stdinHasLast = offset, uint64(len(data)), digest, true
	p.stdinOffset += uint64(len(data))
	return p.stdinOffset, nil
}

func (m *Manager) CloseStdin(id string) error {
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("process not found")
	}
	running := p.state.Status == "RUNNING"
	m.mu.Unlock()
	if !running {
		p.inputMu.Lock()
		closed := p.stdinClosed
		p.inputMu.Unlock()
		if closed {
			return nil
		}
		return errors.New("process is not running")
	}
	p.inputMu.Lock()
	defer p.inputMu.Unlock()
	if p.stdinClosed {
		return nil
	}
	p.stdinClosed = true
	if p.stdin == nil {
		return nil
	}
	if p.spec.Terminal {
		// A PTY has no half-close. In canonical mode, EOT supplies the same
		// EOF indication without closing the output side of the console.
		_, err := p.stdin.Write([]byte{4})
		return err
	}
	return p.stdin.Close()
}

func (m *Manager) ReadOutput(id string, stdoutOffset, stderrOffset, limit uint64) ([]byte, []byte, uint64, uint64, string, error) {
	if limit == 0 || limit > 64<<10 {
		return nil, nil, 0, 0, "", errors.New("output read limit must be between 1 and 64 KiB")
	}
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok {
		m.mu.Unlock()
		return nil, nil, 0, 0, "", errors.New("process not found")
	}
	status := p.state.Status
	m.mu.Unlock()
	stdout, nextStdout := p.stdout.slice(stdoutOffset, limit)
	stderr, nextStderr := p.stderr.slice(stderrOffset, limit)
	return stdout, stderr, nextStdout, nextStderr, status, nil
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
	p.waited = true
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
	x.StdoutTruncated = p.stdout.Truncated()
	x.StderrTruncated = p.stderr.Truncated()
	return x, nil
}

type procStat struct {
	pgrp, userTicks, systemTicks, rssPages uint64
}

func readProcStat(path string) (procStat, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return procStat{}, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 || end+2 >= len(data) {
		return procStat{}, errors.New("malformed proc stat")
	}
	fields := strings.Fields(string(data[end+2:]))
	if len(fields) <= 21 {
		return procStat{}, errors.New("short proc stat")
	}
	parse := func(index int) (uint64, error) { return strconv.ParseUint(fields[index], 10, 64) }
	pgrp, err := parse(2)
	if err != nil {
		return procStat{}, err
	}
	user, err := parse(11)
	if err != nil {
		return procStat{}, err
	}
	system, err := parse(12)
	if err != nil {
		return procStat{}, err
	}
	rss, err := parse(21)
	if err != nil {
		return procStat{}, err
	}
	return procStat{pgrp: pgrp, userTicks: user, systemTicks: system, rssPages: rss}, nil
}

func clockTicks() uint64 {
	data, err := os.ReadFile("/proc/self/auxv")
	if err == nil {
		for len(data) >= 16 {
			key := binary.LittleEndian.Uint64(data[:8])
			value := binary.LittleEndian.Uint64(data[8:16])
			if key == 17 && value > 0 { // AT_CLKTCK
				return value
			}
			data = data[16:]
		}
	}
	return 100
}

func (m *Manager) Stats(id string) (ProcessStats, error) {
	m.mu.Lock()
	p, ok := m.processes[id]
	if !ok || p.state.PID <= 0 {
		m.mu.Unlock()
		return ProcessStats{}, errors.New("running process not found")
	}
	group := uint64(p.state.PID)
	m.mu.Unlock()
	paths, err := filepath.Glob("/proc/[0-9]*/stat")
	if err != nil {
		return ProcessStats{}, err
	}
	sort.Strings(paths)
	var result ProcessStats
	pageSize := uint64(os.Getpagesize())
	for _, path := range paths {
		stat, readErr := readProcStat(path)
		if readErr != nil || stat.pgrp != group {
			continue
		}
		result.CPUUserNS += stat.userTicks
		result.CPUSystemNS += stat.systemTicks
		result.RSSBytes += stat.rssPages * pageSize
		result.PIDs++
	}
	if result.PIDs == 0 {
		return ProcessStats{}, errors.New("process group is no longer present")
	}
	ticks := clockTicks()
	result.CPUUserNS = result.CPUUserNS * uint64(time.Second) / ticks
	result.CPUSystemNS = result.CPUSystemNS * uint64(time.Second) / ticks
	return result, nil
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
	if p.state.Status == "STOPPED" && !p.waited {
		return errors.New("process exit has not been waited")
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
type NetworkConfig struct {
	Name        string   `json:"name"`
	Address     string   `json:"address"`
	Gateway     string   `json:"gateway"`
	MTU         int      `json:"mtu"`
	Nameservers []string `json:"nameservers,omitempty"`
}

func runNetworkCommand(ctx context.Context, arguments ...string) ([]byte, error) {
	return runBoundedNetworkCommand(ctx, "/bin/ip", arguments...)
}

func runBoundedNetworkCommand(ctx context.Context, binary string, arguments ...string) ([]byte, error) {
	const (
		timeout         = 30 * time.Second
		maximumOutput   = 1 << 20
		diagnosticLimit = 16 << 10
	)
	output, err := boundedexec.Run(ctx, timeout, binary, arguments,
		[]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}, maximumOutput)
	if err != nil && len(output) > diagnosticLimit {
		output = output[:diagnosticLimit]
	}
	return output, err
}

func (m *Manager) executeNetworkCommand(ctx context.Context, arguments ...string) ([]byte, error) {
	if m.networkExec == nil {
		return runNetworkCommand(ctx, arguments...)
	}
	return m.networkExec(ctx, arguments...)
}

func replaceDNS(path string, nameservers []string) (original []byte, symlink string, mode os.FileMode, existed bool, retErr error) {
	if info, err := os.Lstat(path); err == nil {
		existed, mode = true, info.Mode().Perm()
		switch {
		case info.Mode().IsRegular():
			original, retErr = os.ReadFile(path)
		case info.Mode()&os.ModeSymlink != 0:
			symlink, retErr = os.Readlink(path)
		default:
			retErr = errors.New("resolv.conf must be a regular file or symlink")
		}
		if retErr != nil {
			return
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		retErr = err
		return
	}
	if retErr = os.Remove(path); retErr != nil && !errors.Is(retErr, os.ErrNotExist) {
		return
	}
	var resolv strings.Builder
	for _, server := range nameservers {
		fmt.Fprintf(&resolv, "nameserver %s\n", server)
	}
	retErr = os.WriteFile(path, []byte(resolv.String()), 0644)
	if retErr != nil {
		if restoreErr := restoreDNS(path, original, symlink, mode, existed); restoreErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("restore DNS after replacement failure: %w", restoreErr))
		}
	}
	return
}

func restoreDNS(path string, original []byte, symlink string, mode os.FileMode, existed bool) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !existed {
		return nil
	}
	if symlink != "" {
		return os.Symlink(symlink, path)
	}
	return os.WriteFile(path, original, mode)
}

func (m *Manager) ConfigureNetwork(config NetworkConfig) error {
	return m.ConfigureNetworkContext(context.Background(), config)
}

func (m *Manager) ConfigureNetworkContext(ctx context.Context, config NetworkConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if config.Name == "" || len(config.Name) > 15 || config.Address == "" || config.Gateway == "" || config.MTU < 576 || config.MTU > 65515 {
		return errors.New("valid network name, address, gateway, and MTU are required")
	}
	ip, subnet, err := net.ParseCIDR(config.Address)
	if err != nil || ip.To4() == nil {
		return errors.New("network address must be IPv4 CIDR")
	}
	gateway := net.ParseIP(config.Gateway)
	if gateway == nil || !subnet.Contains(gateway) || gateway.Equal(ip) {
		return errors.New("network gateway must be a distinct address in the endpoint subnet")
	}
	for _, server := range config.Nameservers {
		if net.ParseIP(server) == nil {
			return errors.New("network DNS server is not an IP address")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.network != nil || m.networkName != "" || m.dnsManaged {
		return errors.New("network is already configured or cleanup is pending")
	}
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	request, err := unix.NewIfreq(config.Name)
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
		{"address", "add", config.Address, "dev", config.Name},
		{"link", "set", config.Name, "mtu", strconv.Itoa(config.MTU), "up"},
		{"route", "add", "default", "via", config.Gateway, "dev", config.Name},
	}
	for _, args := range commands {
		if output, commandErr := m.executeNetworkCommand(ctx, args...); commandErr != nil {
			f.Close()
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			cleanupOutput, cleanupErr := m.executeNetworkCommand(cleanupCtx, "link", "delete", config.Name)
			cancel()
			operationErr := fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), commandErr, strings.TrimSpace(string(output)))
			if cleanupErr != nil {
				m.networkName = config.Name
				cleanupErr = fmt.Errorf("delete child network after setup failure: %w: %s", cleanupErr, strings.TrimSpace(string(cleanupOutput)))
			}
			return errors.Join(operationErr, cleanupErr)
		}
	}
	m.dnsOriginal, m.dnsSymlink, m.dnsMode, m.dnsExisted, err = replaceDNS(m.dnsPath, config.Nameservers)
	if err != nil {
		f.Close()
		m.dnsOriginal, m.dnsSymlink, m.dnsMode, m.dnsExisted, m.dnsManaged = nil, "", 0, false, false
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		cleanupOutput, cleanupErr := m.executeNetworkCommand(cleanupCtx, "link", "delete", config.Name)
		cancel()
		if cleanupErr != nil {
			m.networkName = config.Name
			cleanupErr = fmt.Errorf("delete child network after DNS failure: %w: %s", cleanupErr, strings.TrimSpace(string(cleanupOutput)))
		}
		return errors.Join(err, cleanupErr)
	}
	m.network = f
	m.networkName = config.Name
	m.networkMTU = config.MTU
	m.dnsManaged = true
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
	if len(packet) > m.networkMTU {
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
	return m.CloseNetworkContext(context.Background())
}

func (m *Manager) CloseNetworkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var failures []error
	if m.network != nil {
		failures = append(failures, m.network.Close())
		m.network = nil
	}
	if m.networkName != "" {
		if output, err := m.executeNetworkCommand(ctx, "link", "delete", m.networkName); err != nil {
			failures = append(failures, fmt.Errorf("delete child network: %w: %s", err, strings.TrimSpace(string(output))))
		} else {
			m.networkName = ""
			m.networkMTU = 0
		}
	}
	if m.dnsManaged {
		if err := restoreDNS(m.dnsPath, m.dnsOriginal, m.dnsSymlink, m.dnsMode, m.dnsExisted); err != nil {
			failures = append(failures, fmt.Errorf("restore DNS: %w", err))
		} else {
			m.dnsOriginal, m.dnsSymlink, m.dnsMode, m.dnsExisted, m.dnsManaged = nil, "", 0, false, false
		}
	}
	return errors.Join(failures...)
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
