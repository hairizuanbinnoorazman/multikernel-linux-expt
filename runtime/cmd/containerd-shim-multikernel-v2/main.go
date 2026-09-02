//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/fifo"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const runtimeName = "io.containerd.multikernel.v2"

type process struct {
	id, stdin, stdout, stderr  string
	terminal                   bool
	width, height              uint32
	sizeSet                    bool
	status                     tasktypes.Status
	exit                       uint32
	exited                     time.Time
	done                       chan struct{}
	stdinReader                io.ReadWriteCloser
	stdoutWriter, stderrWriter io.WriteCloser
	stdoutGuard, stderrGuard   io.Closer
	stdinClosed                bool
}

type service struct {
	mu                    sync.Mutex
	id, namespace, bundle string
	publisher             shim.Publisher
	shutdown              func()
	daemon                daemon.Client
	sandbox               protocol.Sandbox
	token                 []byte
	agent                 *agent.Client
	relay                 *exec.Cmd
	netDevice             *os.File
	netDone               chan struct{}
	netWG                 sync.WaitGroup
	netIf                 string
	netSubnet             string
	netEgress             string
	processes             map[string]*process
}

func getenv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func newService(ctx context.Context, id string, publisher shim.Publisher, shutdown func()) (shim.Shim, error) {
	ns, _ := namespaces.Namespace(ctx)
	bundle, _ := os.Getwd()
	return &service{id: id, namespace: ns, bundle: bundle, publisher: publisher, shutdown: shutdown,
		daemon: daemon.Client{Path: getenv("MK_DAEMON_SOCKET", "/run/mkruntimed.sock")}, processes: map[string]*process{}}, nil
}

func newCommand(ctx context.Context, id string, opts shim.StartOpts) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "-namespace", ns, "-id", id, "-address", opts.Address)
	cmd.Dir, cmd.Env = cwd, append(os.Environ(), "GOMAXPROCS=2")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

func (s *service) StartShim(ctx context.Context, opts shim.StartOpts) (_ string, retErr error) {
	cmd, err := newCommand(ctx, opts.ID, opts)
	if err != nil {
		return "", err
	}
	address, err := shim.SocketAddress(ctx, opts.Address, opts.ID)
	if err != nil {
		return "", err
	}
	socket, err := shim.NewSocket(address)
	if err != nil {
		if !shim.SocketEaddrinuse(err) {
			return "", err
		}
		_ = shim.RemoveSocket(address)
		socket, err = shim.NewSocket(address)
		if err != nil {
			return "", err
		}
	}
	defer func() {
		if retErr != nil {
			socket.Close()
			_ = shim.RemoveSocket(address)
		}
	}()
	if err = shim.WriteAddress("address", address); err != nil {
		return "", err
	}
	f, err := socket.File()
	if err != nil {
		return "", err
	}
	cmd.ExtraFiles = []*os.File{f}
	if err = cmd.Start(); err != nil {
		f.Close()
		return "", err
	}
	f.Close()
	go cmd.Wait()
	if err = shim.WritePidFile("shim.pid", cmd.Process.Pid); err != nil {
		return "", err
	}
	return address, nil
}

type persisted struct {
	ID         string    `json:"id"`
	Generation string    `json:"generation"`
	Exit       uint32    `json:"exit"`
	Exited     time.Time `json:"exited"`
	NetIf      string    `json:"net_if,omitempty"`
	NetSubnet  string    `json:"net_subnet,omitempty"`
	NetEgress  string    `json:"net_egress,omitempty"`
}

func (s *service) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	if address, err := shim.ReadAddress("address"); err == nil {
		_ = shim.RemoveSocket(address)
	}
	var p persisted
	if b, err := os.ReadFile(filepath.Join(".multikernel", "sandbox.json")); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	s.netIf, s.netSubnet, s.netEgress = p.NetIf, p.NetSubnet, p.NetEgress
	s.stopNetwork()
	if p.ID != "" {
		_, _ = daemon.Mutation(ctx, s.daemon, "StopSandbox", p.ID, p.Generation, "cleanup-stop-"+p.Generation, nil)
		_, _ = daemon.Mutation(ctx, s.daemon, "DeleteSandbox", p.ID, p.Generation, "cleanup-delete-"+p.Generation, nil)
	}
	if p.Exited.IsZero() {
		p.Exited = time.Now().UTC()
	}
	return &taskapi.DeleteResponse{Pid: uint32(os.Getpid()), ExitStatus: p.Exit, ExitedAt: timestamppb.New(p.Exited)}, nil
}

func randomToken() ([]byte, string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, "", err
	}
	return b, hex.EncodeToString(b), nil
}

func (s *service) persistRecovery() error {
	p := persisted{ID: s.sandbox.ID, Generation: s.sandbox.Generation,
		NetIf: s.netIf, NetSubnet: s.netSubnet, NetEgress: s.netEgress}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.bundle, ".multikernel", "sandbox.json"), b, 0600)
}

func sandboxID(id string) string {
	var b strings.Builder
	b.WriteString("mk-")
	for _, r := range strings.ToLower(id) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	x := strings.Trim(b.String(), "-")
	if len(x) > 50 {
		x = x[:50]
	}
	if x == "mk" || x == "mk-" {
		x = "mk-task"
	}
	return x
}

func parseCPUSet(s string) ([][]int, error) {
	var sets [][]int
	for _, group := range strings.Split(s, ";") {
		var set []int
		for _, raw := range strings.Split(group, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				return nil, err
			}
			set = append(set, n)
		}
		if len(set) > 0 {
			sets = append(sets, set)
		}
	}
	return sets, nil
}

func (s *service) allocate(ctx context.Context, bundle string) (protocol.SandboxConfig, *os.File, error) {
	lock, err := os.OpenFile(getenv("MK_SHIM_LOCK", "/run/multikernel-shim.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return protocol.SandboxConfig{}, nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, err
	}
	var existing []protocol.Sandbox
	req := protocol.Request{Version: 1, RequestID: "list-" + s.id, Method: "ListSandboxes"}
	if apiErr := s.daemon.Call(ctx, req, &existing); apiErr != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, errors.New(apiErr.Message)
	}
	sets, err := parseCPUSet(getenv("MK_SANDBOX_CPU_SETS", "8,10;12,14;9,11;13,15"))
	if err != nil {
		lock.Close()
		return protocol.SandboxConfig{}, nil, err
	}
	used := map[int]bool{}
	for _, sb := range existing {
		for _, cpu := range sb.Config.CPUs {
			used[cpu] = true
		}
	}
	for i, set := range sets {
		free := true
		for _, cpu := range set {
			if used[cpu] {
				free = false
			}
		}
		if free {
			return protocol.SandboxConfig{SchemaVersion: 1, ID: sandboxID(s.id), CPUs: set, MemoryBytes: 3 << 30, KernelManifest: "gce-mk2", Bundle: bundle, AgentPort: uint32(7200 + i), ChildCID: uint32(40 + i)}, lock, nil
		}
	}
	lock.Close()
	return protocol.SandboxConfig{}, nil, errors.New("no disjoint Multikernel CPU set is available")
}

func (s *service) publish(ctx context.Context, topic string, event any) error {
	return s.publisher.Publish(namespaces.WithNamespace(ctx, s.namespace), topic, event)
}

func (s *service) Create(ctx context.Context, r *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	if r.ID != s.id || r.Bundle == "" {
		return nil, fmt.Errorf("%w: invalid task", errdefs.ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.processes) != 0 {
		return nil, errdefs.ErrAlreadyExists
	}
	root := filepath.Join(r.Bundle, "rootfs")
	if err := os.MkdirAll(root, 0711); err != nil {
		return nil, err
	}
	mounts := make([]mount.Mount, len(r.Rootfs))
	for i, m := range r.Rootfs {
		mounts[i] = mount.Mount{Type: m.Type, Source: m.Source, Options: m.Options}
	}
	if err := mount.All(mounts, root); err != nil {
		return nil, fmt.Errorf("mount rootfs: %w", err)
	}
	fail := true
	defer func() {
		if fail {
			_ = mount.UnmountAll(root, 0)
		}
	}()
	runtimeDir := filepath.Join(r.Bundle, ".multikernel")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		return nil, err
	}
	token, tokenHex, err := randomToken()
	if err != nil {
		return nil, err
	}
	s.token = token
	if err = os.WriteFile(filepath.Join(runtimeDir, "token"), []byte(tokenHex+"\n"), 0600); err != nil {
		return nil, err
	}
	initrd := filepath.Join(runtimeDir, "initramfs.cpio.gz")
	build := exec.CommandContext(ctx, getenv("MK_INITRAMFS_BUILDER", "/usr/local/libexec/multikernel/build-runtime-container-initramfs.sh"), r.Bundle, initrd)
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		return nil, fmt.Errorf("build child root: %w: %s", buildErr, strings.TrimSpace(string(output)))
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "initramfs.path"), []byte(initrd+"\n"), 0600); err != nil {
		return nil, err
	}
	config, lock, err := s.allocate(ctx, r.Bundle)
	if err != nil {
		return nil, err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); lock.Close() }()
	created, err := daemon.Mutation(ctx, s.daemon, "CreateSandbox", "", "", "shim-create-"+s.id+"-"+tokenHex[:12], &config)
	if err != nil {
		return nil, err
	}
	s.sandbox = created.Sandbox
	if _, err = daemon.Mutation(ctx, s.daemon, "LoadSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-load-"+s.sandbox.Generation, nil); err != nil {
		_, _ = daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-rollback-"+s.sandbox.Generation, nil)
		return nil, err
	}
	p := persisted{ID: s.sandbox.ID, Generation: s.sandbox.Generation}
	b, _ := json.Marshal(p)
	_ = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), b, 0600)
	s.bundle = r.Bundle
	s.processes[""] = &process{id: "", stdin: r.Stdin, stdout: r.Stdout, stderr: r.Stderr, terminal: r.Terminal, status: tasktypes.Status_CREATED, done: make(chan struct{})}
	pid := uint32(os.Getpid())
	if err = s.publish(ctx, ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: s.id, Bundle: r.Bundle, Rootfs: r.Rootfs, Pid: pid}); err != nil {
		return nil, err
	}
	fail = false
	return &taskapi.CreateTaskResponse{Pid: pid}, nil
}

func (s *service) connectAgent(ctx context.Context) error {
	// Docker uses 64-byte container IDs and deeply nested runtime bundles;
	// placing the relay socket in the bundle can exceed sockaddr_un.sun_path.
	// Use a generation-qualified short path under /run instead.
	gen := s.sandbox.Generation
	if len(gen) > 12 {
		gen = gen[:12]
	}
	sock := filepath.Join("/run", "mk-agent-"+strconv.Itoa(int(s.sandbox.Config.AgentPort))+"-"+gen+".sock")
	_ = os.Remove(sock)
	if err := s.startNetwork(ctx); err != nil {
		return err
	}
	if err := s.persistRecovery(); err != nil {
		s.stopNetwork()
		return fmt.Errorf("persist network recovery state: %w", err)
	}
	s.relay = exec.Command(getenv("MK_RELAY", "/usr/local/libexec/multikernel/mkvsock-relay"), "server", strconv.Itoa(int(s.sandbox.Config.AgentPort)), sock)
	if err := s.relay.Start(); err != nil {
		s.stopNetwork()
		return err
	}
	if _, err := daemon.Mutation(ctx, s.daemon, "StartSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-start-"+s.sandbox.Generation, nil); err != nil {
		s.stopNetwork()
		return err
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		client, err := agent.Dial(sock, s.sandbox.ID, s.sandbox.Generation, s.sandbox.Config.AgentPort, s.token)
		if err == nil {
			s.agent = client
			slot := int(s.sandbox.Config.AgentPort) - 7200
			third := strconv.Itoa(30 + slot)
			if err = s.agent.Call("ConfigureNetwork", map[string]string{
				"Name": "mkn0", "Address": "172.30." + third + ".2/30", "Gateway": "172.30." + third + ".1",
			}, nil); err != nil {
				_ = s.agent.Close()
				s.agent = nil
				s.stopNetwork()
				return fmt.Errorf("configure child network: %w", err)
			}
			s.startNetworkPump()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.stopNetwork()
	return errors.New("timed out connecting to child agent")
}

func command(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

const (
	tunSetIFF = 0x400454ca
	iffTun    = 0x0001
	iffNoPI   = 0x1000
)

func openTUN(name string) (*os.File, error) {
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	request, err := unix.NewIfreq(name)
	if err != nil {
		f.Close()
		return nil, err
	}
	request.SetUint16(iffTun | iffNoPI)
	if err = unix.IoctlIfreq(int(f.Fd()), tunSetIFF, request); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *service) startNetwork(ctx context.Context) error {
	slot := int(s.sandbox.Config.AgentPort) - 7200
	if slot < 0 || slot >= 64 {
		return errors.New("agent port has no network allocation")
	}
	s.netIf = "mkn" + strconv.Itoa(slot)
	third := strconv.Itoa(30 + slot)
	s.netSubnet = "172.30." + third + ".0/30"
	if err := command(ctx, "/usr/sbin/ip", "tuntap", "add", "dev", s.netIf, "mode", "tun"); err != nil {
		return err
	}
	if err := command(ctx, "/usr/sbin/ip", "address", "add", "172.30."+third+".1/30", "dev", s.netIf); err != nil {
		s.stopNetwork()
		return err
	}
	if err := command(ctx, "/usr/sbin/ip", "link", "set", s.netIf, "up"); err != nil {
		s.stopNetwork()
		return err
	}
	if err := command(ctx, "/usr/sbin/ip", "link", "set", s.netIf, "mtu", "1400"); err != nil {
		s.stopNetwork()
		return err
	}
	route, err := exec.CommandContext(ctx, "/usr/sbin/ip", "route", "show", "default").Output()
	if err != nil {
		s.stopNetwork()
		return err
	}
	fields := strings.Fields(string(route))
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			s.netEgress = fields[i+1]
			break
		}
	}
	if s.netEgress == "" {
		s.stopNetwork()
		return errors.New("default egress interface not found")
	}
	rules := [][]string{
		{"-w", "-t", "nat", "-A", "POSTROUTING", "-s", s.netSubnet, "-o", s.netEgress, "-j", "MASQUERADE"},
		{"-w", "-A", "FORWARD", "-i", s.netIf, "-o", s.netEgress, "-j", "ACCEPT"},
		{"-w", "-A", "FORWARD", "-i", s.netEgress, "-o", s.netIf, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}
	for _, rule := range rules {
		if err := command(ctx, "/usr/sbin/iptables", rule...); err != nil {
			s.stopNetwork()
			return err
		}
	}
	s.netDevice, err = openTUN(s.netIf)
	if err != nil {
		s.stopNetwork()
		return err
	}
	return nil
}

func (s *service) startNetworkPump() {
	s.netDone = make(chan struct{})
	s.netWG.Add(1)
	go func() {
		defer s.netWG.Done()
		buffer := make([]byte, 65535)
		for {
			select {
			case <-s.netDone:
				return
			default:
			}
			var packet []byte
			n, err := unix.Read(int(s.netDevice.Fd()), buffer)
			if err == nil && n > 0 {
				packet = append([]byte(nil), buffer[:n]...)
			} else if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
				fmt.Fprintf(os.Stderr, "multikernel network: host TUN read: %v\n", err)
				return
			}
			var response struct {
				Packet []byte `json:"packet"`
			}
			if err = s.agent.Call("ExchangeNetwork", map[string][]byte{"packet": packet}, &response); err != nil {
				fmt.Fprintf(os.Stderr, "multikernel network: exchange: %v\n", err)
				return
			}
			if len(response.Packet) > 0 {
				if _, err = unix.Write(int(s.netDevice.Fd()), response.Packet); err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
					fmt.Fprintf(os.Stderr, "multikernel network: host TUN write: %v\n", err)
					return
				}
			}
			if len(packet) == 0 && len(response.Packet) == 0 {
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
}

func (s *service) stopNetwork() {
	if s.netDone != nil {
		close(s.netDone)
		s.netWG.Wait()
		s.netDone = nil
	}
	if s.agent != nil {
		_ = s.agent.Call("CloseNetwork", map[string]any{}, nil)
	}
	if s.netDevice != nil {
		_ = s.netDevice.Close()
		s.netDevice = nil
	}
	if s.netIf == "" {
		return
	}
	rules := [][]string{
		{"-w", "-t", "nat", "-D", "POSTROUTING", "-s", s.netSubnet, "-o", s.netEgress, "-j", "MASQUERADE"},
		{"-w", "-D", "FORWARD", "-i", s.netIf, "-o", s.netEgress, "-j", "ACCEPT"},
		{"-w", "-D", "FORWARD", "-i", s.netEgress, "-o", s.netIf, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}
	for _, rule := range rules {
		_ = command(context.Background(), "/usr/sbin/iptables", rule...)
	}
	_ = command(context.Background(), "/usr/sbin/ip", "link", "delete", s.netIf)
	s.netIf = ""
}

func (s *service) Start(ctx context.Context, r *taskapi.StartRequest) (*taskapi.StartResponse, error) {
	s.mu.Lock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status != tasktypes.Status_CREATED {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	if r.ExecID == "" {
		if err := s.connectAgent(ctx); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if err := s.agent.Call("CreateProcess", map[string]any{"ID": "init", "Bundle": "/bundle"}, nil); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	processID := r.ExecID
	if processID == "" {
		processID = "init"
	}
	startRequest := map[string]any{"id": processID}
	if p.terminal && p.sizeSet {
		startRequest["width"] = p.width
		startRequest["height"] = p.height
		startRequest["initial_size"] = true
	}
	if err := s.openProcessIO(ctx, p); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if err := s.agent.Call("StartProcess", startRequest, nil); err != nil {
		closeProcessIO(p)
		s.mu.Unlock()
		return nil, err
	}
	p.status = tasktypes.Status_RUNNING
	pid := uint32(os.Getpid())
	var topic string
	var event any
	if r.ExecID == "" {
		topic, event = ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: s.id, Pid: pid}
	} else {
		topic, event = ctruntime.TaskExecStartedEventTopic, &eventstypes.TaskExecStarted{ContainerID: s.id, ExecID: r.ExecID, Pid: pid}
	}
	if err := s.publish(ctx, topic, event); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()
	go s.pumpStdin(processID, p)
	go s.waitProcess(processID, r.ExecID, p)
	return &taskapi.StartResponse{Pid: pid}, nil
}

func openOutput(ctx context.Context, path string) (io.WriteCloser, io.Closer, error) {
	if path == "" {
		return nil, nil, nil
	}
	isFIFO, err := fifo.IsFifo(path)
	if err != nil {
		return nil, nil, err
	}
	if !isFIFO {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		return f, nil, err
	}
	// O_RDWR opens synchronously and keeps a read endpoint present even when
	// the creating client detaches before another client attaches.
	guard, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	w, err := fifo.OpenFifo(ctx, path, syscall.O_WRONLY, 0)
	if err != nil {
		guard.Close()
		return nil, nil, err
	}
	// A later containerd attach can reopen the same FIFO and consume buffered
	// and future guest output; this guard never reads from the FIFO.
	return w, guard, nil
}

func (s *service) openProcessIO(ctx context.Context, p *process) (err error) {
	p.stdoutWriter, p.stdoutGuard, err = openOutput(ctx, p.stdout)
	if err != nil {
		return fmt.Errorf("open stdout: %w", err)
	}
	if !p.terminal {
		p.stderrWriter, p.stderrGuard, err = openOutput(ctx, p.stderr)
		if err != nil {
			closeProcessIO(p)
			return fmt.Errorf("open stderr: %w", err)
		}
	}
	if p.stdin != "" {
		p.stdinReader, err = fifo.OpenFifo(context.Background(), p.stdin, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			closeProcessIO(p)
			return fmt.Errorf("open stdin: %w", err)
		}
	}
	return nil
}

func closeProcessIO(p *process) {
	if p.stdinReader != nil {
		_ = p.stdinReader.Close()
		p.stdinReader = nil
	}
	if p.stdoutWriter != nil {
		_ = p.stdoutWriter.Close()
		p.stdoutWriter = nil
	}
	if p.stderrWriter != nil {
		_ = p.stderrWriter.Close()
		p.stderrWriter = nil
	}
	if p.stdoutGuard != nil {
		_ = p.stdoutGuard.Close()
		p.stdoutGuard = nil
	}
	if p.stderrGuard != nil {
		_ = p.stderrGuard.Close()
		p.stderrGuard = nil
	}
}

func (s *service) pumpStdin(agentID string, p *process) {
	reader := p.stdinReader
	if reader == nil {
		return
	}
	buffer := make([]byte, 32<<10)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			s.mu.Lock()
			stopped := p.status != tasktypes.Status_RUNNING
			s.mu.Unlock()
			if stopped {
				return
			}
			if callErr := s.agent.Call("WriteProcess", map[string]any{"id": agentID, "data": append([]byte(nil), buffer[:n]...)}, nil); callErr != nil {
				fmt.Fprintf(os.Stderr, "multikernel stdin: %v\n", callErr)
				return
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return
		}
		if n == 0 {
			s.mu.Lock()
			closeRequested := p.stdinClosed
			s.mu.Unlock()
			if closeRequested {
				if callErr := s.agent.Call("CloseProcessStdin", map[string]string{"id": agentID}, nil); callErr != nil {
					fmt.Fprintf(os.Stderr, "multikernel close stdin: %v\n", callErr)
				}
				return
			}
			// FIFO EOF can also mean that an attaching client disconnected.
			// Keep the guest side open until CloseIO explicitly requests EOF.
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *service) waitProcess(agentID, execID string, p *process) {
	var state agent.ProcessState
	var stdoutOffset, stderrOffset uint64
	var err error
	for {
		var output struct {
			Stdout       []byte `json:"stdout"`
			Stderr       []byte `json:"stderr"`
			StdoutOffset uint64 `json:"stdout_offset"`
			StderrOffset uint64 `json:"stderr_offset"`
			Status       string `json:"status"`
		}
		err = s.agent.Call("ReadProcessOutput", map[string]any{
			"id": agentID, "stdout_offset": stdoutOffset, "stderr_offset": stderrOffset, "limit": uint64(32 << 10),
		}, &output)
		if err != nil {
			break
		}
		stdoutOffset, stderrOffset = output.StdoutOffset, output.StderrOffset
		if p.stdoutWriter != nil && len(output.Stdout) > 0 {
			if _, writeErr := p.stdoutWriter.Write(output.Stdout); writeErr != nil {
				fmt.Fprintf(os.Stderr, "multikernel stdout: %v\n", writeErr)
			}
		}
		if p.stderrWriter != nil && len(output.Stderr) > 0 {
			if _, writeErr := p.stderrWriter.Write(output.Stderr); writeErr != nil {
				fmt.Fprintf(os.Stderr, "multikernel stderr: %v\n", writeErr)
			}
		}
		if output.Status == "STOPPED" && len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			err = s.agent.Call("StateProcess", map[string]string{"ID": agentID}, &state)
			break
		}
		if len(output.Stdout) == 0 && len(output.Stderr) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	closeProcessIO(p)
	now := time.Now().UTC()
	exit := uint32(255)
	if err == nil {
		exit = uint32(state.ExitCode)
	}
	s.mu.Lock()
	p.status, p.exit, p.exited = tasktypes.Status_STOPPED, exit, now
	close(p.done)
	s.mu.Unlock()
	ctx := namespaces.WithNamespace(context.Background(), s.namespace)
	eventID := execID
	if eventID == "" {
		eventID = s.id
	}
	_ = s.publisher.Publish(ctx, ctruntime.TaskExitEventTopic, &eventstypes.TaskExit{ContainerID: s.id, ID: eventID, Pid: uint32(os.Getpid()), ExitStatus: exit, ExitedAt: timestamppb.New(now)})
}

func (s *service) State(_ context.Context, r *taskapi.StateRequest) (*taskapi.StateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		return nil, errdefs.ErrNotFound
	}
	return &taskapi.StateResponse{ID: s.id, Bundle: s.bundle, Pid: uint32(os.Getpid()), Status: p.status, Stdin: p.stdin, Stdout: p.stdout, Stderr: p.stderr, ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited), ExecID: r.ExecID}, nil
}

func (s *service) Wait(ctx context.Context, r *taskapi.WaitRequest) (*taskapi.WaitResponse, error) {
	s.mu.Lock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	done := p.done
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return &taskapi.WaitResponse{ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}, nil
}

func (s *service) Kill(_ context.Context, r *taskapi.KillRequest) (*emptypb.Empty, error) {
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if s.agent == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	if err := s.agent.Call("SignalProcess", map[string]any{"ID": id, "Signal": strconv.FormatUint(uint64(r.Signal), 10)}, nil); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func processSpec(p *specs.Process) agent.ProcessSpec {
	gids := make([]uint32, len(p.User.AdditionalGids))
	copy(gids, p.User.AdditionalGids)
	return agent.ProcessSpec{Terminal: p.Terminal, User: agent.User{UID: p.User.UID, GID: p.User.GID, AdditionalGids: gids}, Args: p.Args, Env: p.Env, Cwd: p.Cwd}
}

func (s *service) Exec(ctx context.Context, r *taskapi.ExecProcessRequest) (*emptypb.Empty, error) {
	if r.ExecID == "" || s.agent == nil {
		return nil, errdefs.ErrInvalidArgument
	}
	v, err := typeurl.UnmarshalAny(r.Spec)
	if err != nil {
		return nil, err
	}
	spec, ok := v.(*specs.Process)
	if !ok {
		return nil, errdefs.ErrInvalidArgument
	}
	s.mu.Lock()
	if _, exists := s.processes[r.ExecID]; exists {
		s.mu.Unlock()
		return nil, errdefs.ErrAlreadyExists
	}
	p := &process{id: r.ExecID, stdin: r.Stdin, stdout: r.Stdout, stderr: r.Stderr, terminal: r.Terminal, status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s.processes[r.ExecID] = p
	s.mu.Unlock()
	if err = s.agent.Call("ExecProcess", map[string]any{"id": r.ExecID, "root": "/bundle/rootfs", "spec": processSpec(spec)}, nil); err != nil {
		return nil, err
	}
	if err = s.publish(ctx, ctruntime.TaskExecAddedEventTopic, &eventstypes.TaskExecAdded{ContainerID: s.id, ExecID: r.ExecID}); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *service) Delete(ctx context.Context, r *taskapi.DeleteRequest) (*taskapi.DeleteResponse, error) {
	s.mu.Lock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.status == tasktypes.Status_RUNNING {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	delete(s.processes, r.ExecID)
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if s.agent != nil {
		_ = s.agent.Call("DeleteProcess", map[string]string{"ID": id}, nil)
	}
	if r.ExecID == "" {
		if s.agent != nil {
			s.stopNetwork()
			_ = s.agent.Call("Shutdown", map[string]any{}, nil)
			_ = s.agent.Close()
		}
		_, _ = daemon.Mutation(ctx, s.daemon, "StopSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-stop-"+s.sandbox.Generation, nil)
		_, err := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", s.sandbox.ID, s.sandbox.Generation, "shim-delete-"+s.sandbox.Generation, nil)
		if err != nil {
			return nil, err
		}
		s.stopNetwork()
		_ = mount.UnmountAll(filepath.Join(s.bundle, "rootfs"), 0)
	}
	resp := &taskapi.DeleteResponse{Pid: uint32(os.Getpid()), ExitStatus: p.exit, ExitedAt: timestamppb.New(p.exited)}
	_ = s.publish(ctx, ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: s.id, ID: r.ExecID, Pid: resp.Pid, ExitStatus: p.exit, ExitedAt: resp.ExitedAt})
	return resp, nil
}

func (s *service) Pids(context.Context, *taskapi.PidsRequest) (*taskapi.PidsResponse, error) {
	return &taskapi.PidsResponse{Processes: []*tasktypes.ProcessInfo{{Pid: uint32(os.Getpid())}}}, nil
}
func (s *service) Connect(context.Context, *taskapi.ConnectRequest) (*taskapi.ConnectResponse, error) {
	return &taskapi.ConnectResponse{ShimPid: uint32(os.Getpid()), TaskPid: uint32(os.Getpid()), Version: "multikernel-v1"}, nil
}
func (s *service) Shutdown(context.Context, *taskapi.ShutdownRequest) (*emptypb.Empty, error) {
	go s.shutdown()
	return &emptypb.Empty{}, nil
}
func (s *service) ResizePty(_ context.Context, r *taskapi.ResizePtyRequest) (*emptypb.Empty, error) {
	if r.Width > 65535 || r.Height > 65535 {
		return nil, fmt.Errorf("%w: terminal dimensions exceed the Linux PTY limit", errdefs.ErrInvalidArgument)
	}
	s.mu.Lock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if !p.terminal {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	p.width, p.height, p.sizeSet = r.Width, r.Height, true
	if p.status == tasktypes.Status_CREATED {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	if p.status != tasktypes.Status_RUNNING || s.agent == nil {
		s.mu.Unlock()
		return nil, errdefs.ErrFailedPrecondition
	}
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if err := s.agent.Call("ResizeProcess", map[string]any{"id": id, "width": r.Width, "height": r.Height}, nil); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
func (s *service) Pause(context.Context, *taskapi.PauseRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) Resume(context.Context, *taskapi.ResumeRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) Checkpoint(context.Context, *taskapi.CheckpointTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) CloseIO(_ context.Context, r *taskapi.CloseIORequest) (*emptypb.Empty, error) {
	if !r.Stdin {
		return &emptypb.Empty{}, nil
	}
	s.mu.Lock()
	p, ok := s.processes[r.ExecID]
	if !ok {
		s.mu.Unlock()
		return nil, errdefs.ErrNotFound
	}
	if p.stdinClosed {
		s.mu.Unlock()
		return &emptypb.Empty{}, nil
	}
	p.stdinClosed = true
	hasReader := p.stdinReader != nil
	s.mu.Unlock()
	id := r.ExecID
	if id == "" {
		id = "init"
	}
	if hasReader {
		// The pump must forward bytes already buffered in the FIFO before it
		// closes guest stdin. It observes stdinClosed after reaching FIFO EOF.
		return &emptypb.Empty{}, nil
	}
	if s.agent == nil {
		return nil, errdefs.ErrFailedPrecondition
	}
	if err := s.agent.Call("CloseProcessStdin", map[string]string{"id": id}, nil); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
func (s *service) Update(context.Context, *taskapi.UpdateTaskRequest) (*emptypb.Empty, error) {
	return nil, errdefs.ErrNotImplemented
}
func (s *service) Stats(context.Context, *taskapi.StatsRequest) (*taskapi.StatsResponse, error) {
	return nil, errdefs.ErrNotImplemented
}

var _ taskapi.TaskService = (*service)(nil)

func main() { shim.Run(runtimeName, newService) }
