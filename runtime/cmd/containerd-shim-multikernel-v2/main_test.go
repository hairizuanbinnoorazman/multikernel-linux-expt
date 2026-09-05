//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	cgroupstats "github.com/containerd/cgroups/stats/v1"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/events"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/fifo"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func TestMain(m *testing.M) {
	if marker := os.Getenv("MK_SHIM_SUPERVISOR_TEST_MARKER"); marker != "" && os.Getenv("MK_SHIM_WORKER") == "1" {
		if _, err := os.Stat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("first-worker-signaled\n"), 0600)
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
			os.Exit(99)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fakeAgentClient struct {
	calls []string
	fail  map[string]error
	stats agent.ProcessStats
}

func (f *fakeAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *fakeAgentClient) CallContext(_ context.Context, method string, _ any, response any) error {
	f.calls = append(f.calls, method)
	if err := f.fail[method]; err != nil {
		return err
	}
	if method == "StatsProcess" {
		*(response.(*agent.ProcessStats)) = f.stats
	}
	return nil
}
func (f *fakeAgentClient) Close() error           { return nil }
func (f *fakeAgentClient) Reconnect(string) error { return nil }

type fakePublisher struct{ topics []string }

func (f *fakePublisher) Publish(_ context.Context, topic string, _ events.Event) error {
	f.topics = append(f.topics, topic)
	return nil
}
func (f *fakePublisher) Close() error { return nil }

func TestValidateExecProcessFailsClosed(t *testing.T) {
	base := func() *specs.Process {
		return &specs.Process{User: specs.User{UID: 1, GID: 2, AdditionalGids: []uint32{3}}, Args: []string{"/bin/true"}, Env: []string{"A=B"}, Cwd: "/"}
	}
	if err := validateExecProcess(base()); err != nil {
		t.Fatalf("supported process rejected: %v", err)
	}
	zero := 0
	umask := uint32(0o22)
	tests := []struct {
		name   string
		mutate func(*specs.Process)
	}{
		{"console-size", func(p *specs.Process) { p.ConsoleSize = &specs.Box{} }},
		{"command-line", func(p *specs.Process) { p.CommandLine = "true" }},
		{"capabilities", func(p *specs.Process) { p.Capabilities = &specs.LinuxCapabilities{} }},
		{"rlimits", func(p *specs.Process) { p.Rlimits = []specs.POSIXRlimit{} }},
		{"no-new-privileges", func(p *specs.Process) { p.NoNewPrivileges = true }},
		{"apparmor", func(p *specs.Process) { p.ApparmorProfile = "profile" }},
		{"oom-score", func(p *specs.Process) { p.OOMScoreAdj = &zero }},
		{"scheduler", func(p *specs.Process) { p.Scheduler = &specs.Scheduler{} }},
		{"selinux", func(p *specs.Process) { p.SelinuxLabel = "label" }},
		{"io-priority", func(p *specs.Process) { p.IOPriority = &specs.LinuxIOPriority{} }},
		{"umask", func(p *specs.Process) { p.User.Umask = &umask }},
		{"username", func(p *specs.Process) { p.User.Username = "root" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process := base()
			test.mutate(process)
			if err := validateExecProcess(process); !errors.Is(err, errdefs.ErrNotImplemented) {
				t.Fatalf("error = %v, want not implemented", err)
			}
		})
	}
}

func TestValidateServiceIdentityRejectsUnsafeValues(t *testing.T) {
	bundle := t.TempDir()
	if err := validateServiceIdentity("task-1", "default", bundle); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	for _, test := range []struct{ id, namespace, bundle string }{
		{"../task", "default", bundle},
		{"task", "../namespace", bundle},
		{"task", "default", "relative"},
		{"task", "default", bundle + "/../" + filepath.Base(bundle)},
	} {
		if err := validateServiceIdentity(test.id, test.namespace, test.bundle); !errors.Is(err, errdefs.ErrInvalidArgument) {
			t.Fatalf("unsafe identity (%q, %q, %q) error = %v", test.id, test.namespace, test.bundle, err)
		}
	}
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(parent, "linked")
	if err := os.Symlink(real, linked); err != nil {
		t.Fatal(err)
	}
	if err := validateServiceIdentity("task", "default", linked); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("symlinked bundle error = %v", err)
	}
}

func TestRootfsMountsAreForcedReadOnlyAndValidated(t *testing.T) {
	mounts, err := readOnlyRootfsMounts([]*types.Mount{{
		Type: "overlay", Source: "overlay", Options: []string{"rw", "lowerdir=/snap", "nodev"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	options := map[string]bool{}
	for _, option := range mounts[0].Options {
		options[option] = true
	}
	if options["rw"] || !options["ro"] || !options["lowerdir=/snap"] {
		t.Fatalf("read-only mount options = %v", mounts[0].Options)
	}
	for _, input := range []*types.Mount{
		{Type: "tmpfs", Source: "tmpfs"},
		{Type: "bind", Source: "relative"},
		{Type: "overlay", Source: "overlay", Options: []string{"ro\nmalicious"}},
	} {
		if _, err = readOnlyRootfsMounts([]*types.Mount{input}); err == nil {
			t.Fatalf("unsafe mount accepted: %+v", input)
		}
	}
}

func TestResizePtyStoresInitialSizeBeforeStart(t *testing.T) {
	s := &service{processes: map[string]*process{
		"": {terminal: true, status: tasktypes.Status_CREATED},
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); err != nil {
		t.Fatal(err)
	}
	p := s.processes[""]
	if !p.sizeSet || p.width != 91 || p.height != 37 {
		t.Fatalf("pending size = set:%v %dx%d", p.sizeSet, p.width, p.height)
	}
}

func TestOutputFIFOCanBeReattached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout")
	initial, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	if err != nil {
		t.Fatal(err)
	}
	w, guard, err := openOutput(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer guard.Close()
	initial.Close()
	if _, err = w.Write([]byte("reattach-ok")); err != nil {
		t.Fatal(err)
	}
	attached, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer attached.Close()
	var got = make([]byte, len("reattach-ok"))
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, err = io.ReadFull(attached, got); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if string(got) != "reattach-ok" {
		t.Fatalf("attached output = %q, err = %v", got, err)
	}
}

func TestResizePtyRejectsInvalidRequests(t *testing.T) {
	s := &service{processes: map[string]*process{
		"": {status: tasktypes.Status_CREATED},
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("non-terminal resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 65536}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("oversized resize error = %v", err)
	}
}

func TestExecRollsBackProcessOnAgentFailure(t *testing.T) {
	agentFailure := errors.New("injected agent failure")
	fake := &fakeAgentClient{fail: map[string]error{"ExecProcess": agentFailure}}
	s := &service{agent: fake, processes: map[string]*process{}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	_, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "failed", Spec: encoded})
	if !errors.Is(err, agentFailure) {
		t.Fatalf("Exec() error = %v, want injected failure", err)
	}
	if _, exists := s.processes["failed"]; exists {
		t.Fatal("failed exec left a stale process")
	}
}

func TestStatePidsAndConnectReportGuestProcessIDs(t *testing.T) {
	s := &service{bundle: "/bundle", processes: map[string]*process{
		"":     {pid: 17, status: tasktypes.Status_RUNNING},
		"exec": {pid: 23, status: tasktypes.Status_RUNNING},
	}}
	state, err := s.State(context.Background(), &taskapi.StateRequest{ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	if state.Pid != 23 {
		t.Fatalf("State PID = %d, want guest PID 23", state.Pid)
	}
	pids, err := s.Pids(context.Background(), &taskapi.PidsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[uint32]bool{}
	for _, process := range pids.Processes {
		got[process.Pid] = true
	}
	if !got[17] || !got[23] || len(got) != 2 {
		t.Fatalf("Pids = %v, want guest PIDs 17 and 23", got)
	}
	connected, err := s.Connect(context.Background(), &taskapi.ConnectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if connected.TaskPid != 17 || connected.ShimPid != uint32(os.Getpid()) {
		t.Fatalf("Connect = task:%d shim:%d", connected.TaskPid, connected.ShimPid)
	}
}

func TestPauseResumeSignalsGuestAndPublishesTransitions(t *testing.T) {
	fakeAgent := &fakeAgentClient{fail: map[string]error{}}
	fakeEvents := &fakePublisher{}
	s := &service{
		id: "task", namespace: "default", agent: fakeAgent, publisher: fakeEvents,
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}},
	}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_PAUSED {
		t.Fatalf("pause status = %v", s.processes[""].status)
	}
	if _, err := s.Resume(context.Background(), &taskapi.ResumeRequest{}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_RUNNING {
		t.Fatalf("resume status = %v", s.processes[""].status)
	}
	if len(fakeAgent.calls) != 2 || fakeAgent.calls[0] != "SignalProcess" || fakeAgent.calls[1] != "SignalProcess" {
		t.Fatalf("agent calls = %v", fakeAgent.calls)
	}
	if len(fakeEvents.topics) != 2 || fakeEvents.topics[0] != ctruntime.TaskPausedEventTopic || fakeEvents.topics[1] != ctruntime.TaskResumedEventTopic {
		t.Fatalf("event topics = %v", fakeEvents.topics)
	}
}

func TestStatsReturnsGuestProcessGroupMetrics(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, stats: agent.ProcessStats{
		CPUUserNS: 11, CPUSystemNS: 7, RSSBytes: 4096, PIDs: 3,
	}}
	s := &service{
		agent: fake, sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{MemoryBytes: 3 << 30}},
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}},
	}
	response, err := s.Stats(context.Background(), &taskapi.StatsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typeurl.UnmarshalAny(response.Stats)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := decoded.(*cgroupstats.Metrics)
	if !ok {
		t.Fatalf("stats type = %T", decoded)
	}
	if metrics.CPU.Usage.Total != 18 || metrics.Memory.Usage.Usage != 4096 || metrics.Memory.Usage.Limit != 3<<30 || metrics.Pids.Current != 3 {
		t.Fatalf("stats = %+v", metrics)
	}
}

func TestRecoveryStatePersistsGenerationProcessesAndOffsetsAtomically(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	s := &service{
		bundle:  bundle,
		sandbox: protocol.Sandbox{ID: "box", Generation: "0123456789abcdef0123456789abcdef"},
		processes: map[string]*process{
			"": {id: "", pid: 7, status: tasktypes.Status_RUNNING, stdoutOffset: 123, stderrOffset: 45, done: make(chan struct{})},
		},
	}
	if err := s.persistRecovery(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(bundle, ".multikernel", "sandbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved persisted
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.SchemaVersion != 1 || saved.Generation != s.sandbox.Generation || len(saved.Processes) != 1 || saved.Processes[0].PID != 7 || saved.Processes[0].StdoutOffset != 123 {
		t.Fatalf("persisted recovery = %+v", saved)
	}
	temporary, err := filepath.Glob(filepath.Join(bundle, ".multikernel", ".sandbox.json.*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary recovery files = %v, error = %v", temporary, err)
	}
}

func TestBundleNetworkNamespaceRequiresCanonicalOCIPath(t *testing.T) {
	for name, test := range map[string]struct {
		path string
		err  bool
	}{
		"runtime creates namespace": {path: ""},
		"named namespace":           {path: "/run/netns/pod-one"},
		"relative rejected":         {path: "run/netns/pod-one", err: true},
	} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			data := fmt.Sprintf(`{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":%q}]}}`, test.path)
			if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			actual, err := bundleNetworkNamespace(bundle)
			if (err != nil) != test.err || (!test.err && actual != test.path) {
				t.Fatalf("namespace=%q error=%v", actual, err)
			}
		})
	}
}

func TestSupervisorRestartsSignaledWorker(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "worker-signaled")
	listener, err := os.CreateTemp(directory, "listener")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	environment := append(os.Environ(), "MK_SHIM_SUPERVISOR_TEST_MARKER="+marker)
	if status := superviseShimWorkerWith(listener, os.Args[0], []string{"-test.run=^$"}, directory, environment); status != 0 {
		t.Fatalf("supervisor status = %d", status)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "first-worker-signaled\n" {
		t.Fatalf("restart marker = %q, error = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".multikernel-worker.pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker PID file remains after clean exit: %v", err)
	}
}
