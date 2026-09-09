//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	cgroupstats "github.com/containerd/cgroups/stats/v1"
	eventstypes "github.com/containerd/containerd/api/events"
	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	types "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/events"
	ctruntime "github.com/containerd/containerd/runtime"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/fifo"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
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
	calls     []string
	fail      map[string]error
	stats     agent.ProcessStats
	statsByID map[string]agent.ProcessStats
}

type daemonCallFunc func(context.Context, protocol.Request, any) *protocol.Error

func (f daemonCallFunc) Call(ctx context.Context, request protocol.Request, response any) *protocol.Error {
	return f(ctx, request, response)
}

type closeRetryAgent struct {
	attempts int
	called   chan int
}

func (f *closeRetryAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *closeRetryAgent) CallContext(_ context.Context, method string, _, _ any) error {
	if method != "CloseProcessStdin" {
		return fmt.Errorf("unexpected method %s", method)
	}
	f.attempts++
	f.called <- f.attempts
	if f.attempts == 1 {
		return errors.New("injected close failure")
	}
	return nil
}
func (f *closeRetryAgent) Close() error                                   { return nil }
func (f *closeRetryAgent) Reconnect(string) error                         { return nil }
func (f *closeRetryAgent) ReconnectContext(context.Context, string) error { return nil }

type signalFailureAgent struct {
	attempts int
	failAt   int
	calls    []string
}

func (f *signalFailureAgent) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *signalFailureAgent) CallContext(_ context.Context, method string, request, _ any) error {
	if method != "SignalProcess" {
		return fmt.Errorf("unexpected method %s", method)
	}
	value := request.(map[string]any)
	f.attempts++
	f.calls = append(f.calls, fmt.Sprintf("%s:%s", value["ID"], value["Signal"]))
	if f.attempts == f.failAt {
		return errors.New("injected signal failure")
	}
	return nil
}
func (f *signalFailureAgent) Close() error                                   { return nil }
func (f *signalFailureAgent) Reconnect(string) error                         { return nil }
func (f *signalFailureAgent) ReconnectContext(context.Context, string) error { return nil }

type eofReadWriteCloser struct{}

func (eofReadWriteCloser) Read([]byte) (int, error)    { return 0, io.EOF }
func (eofReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (eofReadWriteCloser) Close() error                { return nil }

func (f *fakeAgentClient) Call(method string, request, response any) error {
	return f.CallContext(context.Background(), method, request, response)
}
func (f *fakeAgentClient) CallContext(_ context.Context, method string, request any, response any) error {
	f.calls = append(f.calls, method)
	if err := f.fail[method]; err != nil {
		return err
	}
	if method == "StatsProcess" {
		value := f.stats
		if f.statsByID != nil {
			value = f.statsByID[request.(map[string]string)["ID"]]
		}
		*(response.(*agent.ProcessStats)) = value
	}
	return nil
}
func (f *fakeAgentClient) Close() error                                   { return nil }
func (f *fakeAgentClient) Reconnect(string) error                         { return nil }
func (f *fakeAgentClient) ReconnectContext(context.Context, string) error { return nil }

type fakePublisher struct {
	topics   []string
	failures int
}

type retryPublisher struct {
	attempts  int
	published chan string
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(value []byte) (int, error) { return f(value) }

type inheritedFileSocket string

func (path inheritedFileSocket) File() (*os.File, error) { return os.Open(string(path)) }

func (f *retryPublisher) Publish(_ context.Context, topic string, _ events.Event) error {
	f.attempts++
	if f.attempts == 1 {
		return errors.New("injected transient disconnect")
	}
	f.published <- topic
	return nil
}
func (f *retryPublisher) Close() error { return nil }

func (f *fakePublisher) Publish(_ context.Context, topic string, _ events.Event) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("injected publication failure")
	}
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
	supported := base()
	supported.User.UID = 0
	supported.NoNewPrivileges = true
	supported.Rlimits = []specs.POSIXRlimit{{Type: "RLIMIT_NOFILE", Soft: 64, Hard: 64}}
	supported.Capabilities = &specs.LinuxCapabilities{Bounding: []string{"CAP_CHOWN"}, Permitted: []string{"CAP_CHOWN"}, Effective: []string{"CAP_CHOWN"}}
	if err := validateExecProcess(supported); err != nil {
		t.Fatalf("standard process controls rejected: %v", err)
	}
	projected := processSpec(supported)
	if projected.NoNewPrivileges == nil || !*projected.NoNewPrivileges || len(projected.Rlimits) != 1 || projected.Capabilities["effective"][0] != "CAP_CHOWN" {
		t.Fatalf("standard process controls were not projected: %+v", projected)
	}
	zero := 0
	umask := uint32(0o22)
	tests := []struct {
		name   string
		mutate func(*specs.Process)
	}{
		{"console-size", func(p *specs.Process) { p.ConsoleSize = &specs.Box{} }},
		{"command-line", func(p *specs.Process) { p.CommandLine = "true" }},
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

func TestSandboxIDBindsCompleteNamespaceAndTaskWithoutAliases(t *testing.T) {
	base := sandboxID("namespace-a", "task")
	if base != sandboxID("namespace-a", "task") || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`).MatchString(base) {
		t.Fatalf("sandbox ID is not stable and daemon-safe: %q", base)
	}
	aliases := []string{
		sandboxID("namespace-b", "task"),
		sandboxID("namespace-a", "task.with.punctuation"),
		sandboxID("namespace-a", "task_with_punctuation"),
		sandboxID("namespace-a", strings.Repeat("a", 127)+"b"),
		sandboxID("namespace-a", strings.Repeat("a", 127)+"c"),
	}
	seen := map[string]bool{base: true}
	for _, candidate := range aliases {
		if seen[candidate] {
			t.Fatalf("distinct namespace/task tuples alias as %q", candidate)
		}
		seen[candidate] = true
		if len(candidate) > 63 {
			t.Fatalf("sandbox ID exceeds daemon limit: %q", candidate)
		}
	}
}

func TestRetryWaitReturnsImmediatelyOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := waitContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitContext error = %v", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("canceled retry wait did not return promptly")
	}
}

func TestPreCancelledShimStartupAndRecoveryDoNotCreateOrInspectState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &service{bundle: filepath.Join(t.TempDir(), "missing-bundle")}
	if address, err := s.StartShim(ctx, shim.StartOpts{ID: "task"}); !errors.Is(err, context.Canceled) || address != "" {
		t.Fatalf("StartShim() = %q, %v", address, err)
	}
	if err := s.recoverExisting(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("recoverExisting() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.bundle, ".multikernel")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled startup created runtime state: %v", err)
	}
}

func TestLaunchShimWorkerCleansProcessGroupAndOwnedArtifactsOnPIDFailure(t *testing.T) {
	directory := t.TempDir()
	address := "unix://" + filepath.Join(directory, "shim.sock")
	inheritedPath := filepath.Join(directory, "inherited-descriptor")
	if err := os.WriteFile(inheritedPath, []byte("descriptor"), 0600); err != nil {
		t.Fatal(err)
	}
	addressPath := filepath.Join(directory, "address")
	pidPath := filepath.Join(directory, "shim.pid")
	if err := os.Mkdir(pidPath, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := launchShimWorker(context.Background(), cmd, inheritedFileSocket(inheritedPath), address, addressPath, pidPath); err == nil {
		t.Fatal("PID-file failure was accepted")
	}
	if _, statErr := os.Lstat(addressPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial address file survived: %v", statErr)
	}
	if info, statErr := os.Stat(pidPath); statErr != nil || !info.IsDir() {
		t.Fatalf("pre-existing PID path was removed or changed: %+v, %v", info, statErr)
	}
	if cmd.Process == nil {
		t.Fatal("worker did not reach the injected post-start failure")
	}
	if killErr := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(killErr, syscall.ESRCH) {
		t.Fatalf("worker process group survived cleanup: %v", killErr)
	}
}

func TestCancelCreateUsesExactOriginalIdentity(t *testing.T) {
	wantConfig := protocol.SandboxConfig{SchemaVersion: 1, ID: "box-a", CPUs: []int{8}, MemoryBytes: 1 << 30,
		KernelManifest: "test", Bundle: "/bundle/box-a", AgentPort: 7001, ChildCID: 3}
	wantKey := "shim-create-box-a-0123456789ab"
	var callErr error
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, response any) *protocol.Error {
		var observed protocol.SandboxConfig
		if decodeErr := protocol.StrictDecode(request.Body, &observed); decodeErr != nil {
			callErr = decodeErr
			return nil
		}
		if request.Method != "CancelCreateSandbox" || request.IdempotencyKey != wantKey || !reflect.DeepEqual(observed, wantConfig) {
			callErr = fmt.Errorf("unexpected cancellation request: %+v %+v", request, observed)
			return nil
		}
		encoded, _ := json.Marshal(map[string]bool{"safe_to_cleanup": true})
		if decodeErr := json.Unmarshal(encoded, response); decodeErr != nil {
			callErr = decodeErr
		}
		return nil
	})
	service := &service{daemon: caller}
	if err := service.cancelCreate(context.Background(), wantConfig, wantKey); err != nil {
		t.Fatal(err)
	}
	if callErr != nil {
		t.Fatal(callErr)
	}
}

func TestLoadOrCreateTokenReusesExactSafeIdentity(t *testing.T) {
	directory := t.TempDir()
	first, encoded, err := loadOrCreateToken(directory)
	if err != nil || len(first) != 32 || len(encoded) != 64 {
		t.Fatalf("first token = %x %q, %v", first, encoded, err)
	}
	second, repeated, err := loadOrCreateToken(directory)
	if err != nil || !reflect.DeepEqual(second, first) || repeated != encoded {
		t.Fatalf("repeated token = %x %q, %v", second, repeated, err)
	}
	info, err := os.Lstat(filepath.Join(directory, "token"))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 65 {
		t.Fatalf("token metadata = %+v, %v", info, err)
	}
}

func TestLoadOrCreateTokenRejectsMalformedAndSymlinkState(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(string) error
	}{
		{"malformed", func(path string) error { return os.WriteFile(path, []byte("short\n"), 0600) }},
		{"permissive", func(path string) error { return os.WriteFile(path, []byte(strings.Repeat("a", 64)+"\n"), 0644) }},
		{"symlink", func(path string) error { return os.Symlink("missing", path) }},
		{"hardlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
				return err
			}
			return os.Link(target, path)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := test.setup(filepath.Join(directory, "token")); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadOrCreateToken(directory); err == nil {
				t.Fatal("unsafe existing token accepted")
			}
		})
	}

	t.Run("symlinked ancestor", func(t *testing.T) {
		directory := t.TempDir()
		realRuntime := filepath.Join(directory, "real")
		if err := os.Mkdir(realRuntime, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(realRuntime, "token"), []byte(strings.Repeat("a", 64)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		linkedRuntime := filepath.Join(directory, "linked")
		if err := os.Symlink(realRuntime, linkedRuntime); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadOrCreateToken(linkedRuntime); err == nil {
			t.Fatal("token beneath a symlinked ancestor was accepted")
		}
	})
}

func TestCreateAmbiguityCancellationRemovesPreparedArtifacts(t *testing.T) {
	bundle := t.TempDir()
	configJSON := `{"ociVersion":"1.0.2","process":{"cwd":"/","args":["/bin/true"],"user":{"uid":0,"gid":0}},"root":{"path":"rootfs"},"linux":{"namespaces":[{"type":"network","path":"/run/netns/test"}]}}`
	if err := os.WriteFile(filepath.Join(bundle, "config.json"), []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(t.TempDir(), "shim.lock")
	t.Setenv("MK_SHIM_LOCK", lock)
	runtimeDir := filepath.Join(bundle, ".multikernel")
	storageHash := strings.Repeat("a", 64)
	var calls []string
	respond := func(output any, value any) *protocol.Error {
		if output == nil {
			return nil
		}
		data, _ := json.Marshal(value)
		if err := json.Unmarshal(data, output); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	caller := daemonCallFunc(func(_ context.Context, request protocol.Request, output any) *protocol.Error {
		calls = append(calls, request.Method)
		switch request.Method {
		case "ListSandboxes":
			return respond(output, []protocol.Sandbox{})
		case "PrepareRootfs":
			if err := os.Mkdir(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			return respond(output, rootfspkg.PrepareResult{Storage: protocol.StorageConfig{
				Path: "/srv/storage/root.ext4", ImageID: "image", FilesystemUUID: "12345678-1234-4234-8234-123456789abc",
				SizeBytes: 64 << 20, QuotaBytes: 64 << 20, InodeLimit: 4096, Port: 4061, SHA256: storageHash,
			}})
		case "CreateSandbox":
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "ambiguous create", Retryable: true}
		case "CancelCreateSandbox":
			if !strings.HasPrefix(request.IdempotencyKey, "shim-create-task-a-") {
				t.Fatalf("unexpected create cancellation key %q", request.IdempotencyKey)
			}
			return respond(output, map[string]bool{"safe_to_cleanup": true})
		case "CleanupRootfs":
			var cleanup rootfspkg.CleanupRequest
			if err := protocol.StrictDecode(request.Body, &cleanup); err != nil || cleanup.Bundle != bundle || cleanup.StorageSHA256 != storageHash {
				t.Fatalf("cleanup request = %+v, %v", cleanup, err)
			}
			if err := os.RemoveAll(runtimeDir); err != nil {
				t.Fatal(err)
			}
			return respond(output, map[string]bool{"cleaned": true})
		default:
			t.Fatalf("unexpected daemon method %q", request.Method)
			return nil
		}
	})
	service := &service{id: "task-a", namespace: "default", bundle: bundle, daemon: caller, processes: map[string]*process{}}
	_, err := service.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task-a", Bundle: bundle})
	if err == nil || !strings.Contains(err.Error(), "ambiguous create") {
		t.Fatalf("Create() error = %v", err)
	}
	wantCalls := []string{"ListSandboxes", "PrepareRootfs", "CreateSandbox", "CancelCreateSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("daemon calls = %v, want %v", calls, wantCalls)
	}
	if _, statErr := os.Lstat(runtimeDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime artifacts survived cancellation: %v", statErr)
	}
	if len(service.token) != 0 || len(service.processes) != 0 || service.sandbox.ID != "" {
		t.Fatalf("shim state survived cancellation: token=%d processes=%d sandbox=%+v", len(service.token), len(service.processes), service.sandbox)
	}
}

func TestRootfsMountsAreSanitizedForDaemonAndValidated(t *testing.T) {
	mounts, err := rootfsMounts([]*types.Mount{{
		Type: "overlay", Source: "overlay", Options: []string{"rw", "lowerdir=/snap", "nodev"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	options := map[string]bool{}
	for _, option := range mounts[0].Options {
		options[option] = true
	}
	if options["rw"] || !options["lowerdir=/snap"] {
		t.Fatalf("sanitized mount options = %v", mounts[0].Options)
	}
	for _, input := range []*types.Mount{
		{Type: "tmpfs", Source: "tmpfs"},
		{Type: "bind", Source: "relative"},
		{Type: "overlay", Source: "overlay", Options: []string{"ro\nmalicious"}},
	} {
		if _, err = rootfsMounts([]*types.Mount{input}); err == nil {
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

func TestProcessIOPathsRejectUnsafeIdentityAndSymlinkAncestors(t *testing.T) {
	directory := t.TempDir()
	unsafeFIFO := filepath.Join(directory, "unsafe")
	if err := syscall.Mkfifo(unsafeFIFO, 0660); err != nil {
		t.Fatal(err)
	}
	if err := validateProcessIOPaths(unsafeFIFO); err == nil {
		t.Fatal("group-accessible FIFO was accepted")
	}

	regular := filepath.Join(directory, "output")
	if err := os.WriteFile(regular, []byte("output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(regular, regular+".other"); err != nil {
		t.Fatal(err)
	}
	if err := validateProcessIOPaths(regular); err == nil {
		t.Fatal("hard-linked output was accepted")
	}

	realParent := filepath.Join(directory, "real")
	if err := os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(realParent, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(directory, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	if err := validateProcessIOPaths(filepath.Join(linkedParent, "fifo")); err == nil {
		t.Fatal("stdio beneath a symlinked parent was accepted")
	}
}

func TestProcessIOOpenRejectsReplacementAndCancellation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "output")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	expected, err := inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("same-inode stdio mode change was accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	expected, err = inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("replacement stdio inode was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if file, openErr := openKnownProcessIOPath(ctx, path, unix.O_WRONLY, expected); !errors.Is(openErr, context.Canceled) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("cancelled stdio open error = %v", openErr)
	}
}

func TestInvalidProcessIOFailsBeforeCreateOrExecMutation(t *testing.T) {
	s := &service{id: "task", bundle: "/bundle", processes: make(map[string]*process)}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{
		ID: "task", Bundle: "/bundle", Stdin: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Create invalid stdio error = %v", err)
	}
	if len(s.processes) != 0 {
		t.Fatalf("Create mutated process state: %+v", s.processes)
	}
	if _, err := s.Exec(context.Background(), &taskapi.ExecProcessRequest{
		ExecID: "exec", Stdout: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Exec invalid stdio error = %v", err)
	}
}

func TestProcessStdinMustBeFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte("input"), 0600); err != nil {
		t.Fatal(err)
	}
	p := &process{stdin: path}
	if err := (&service{}).openProcessIO(context.Background(), p); err == nil {
		t.Fatal("regular-file stdin was accepted")
	}
}

func TestOutputOffsetsAdvanceOnlyAfterDeliveryOrBoundedDrop(t *testing.T) {
	now := time.Unix(100, 0)
	pressure := time.Time{}
	blocked := writerFunc(func([]byte) (int, error) { return 0, syscall.EAGAIN })
	advance, dropped := deliverOutput(blocked, []byte("output"), &pressure, now, time.Second)
	if advance || dropped || !pressure.Equal(now) {
		t.Fatalf("initial pressure advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(999*time.Millisecond), time.Second)
	if advance || dropped {
		t.Fatalf("pre-deadline pressure advanced=%v dropped=%v", advance, dropped)
	}
	advance, dropped = deliverOutput(blocked, []byte("output"), &pressure, now.Add(time.Second), time.Second)
	if !advance || !dropped || !pressure.IsZero() {
		t.Fatalf("bounded drop advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	pressure = now
	complete := writerFunc(func(value []byte) (int, error) { return len(value), nil })
	advance, dropped = deliverOutput(complete, []byte("output"), &pressure, now, time.Second)
	if !advance || dropped || !pressure.IsZero() {
		t.Fatalf("complete delivery advanced=%v dropped=%v since=%v", advance, dropped, pressure)
	}
	advance, dropped = deliverOutput(nil, []byte("discard-by-contract"), &pressure, now, time.Second)
	if !advance || dropped {
		t.Fatalf("unconfigured output advanced=%v dropped=%v", advance, dropped)
	}
}

func TestCloseIORetriesUntilGuestAcknowledges(t *testing.T) {
	client := &fakeAgentClient{fail: map[string]error{"CloseProcessStdin": errors.New("injected close failure")}}
	p := &process{id: "", status: tasktypes.Status_RUNNING, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err == nil {
		t.Fatal("failed guest close was accepted")
	}
	if !p.stdinClosed || p.stdinCloseAcked {
		t.Fatalf("failed close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	delete(client.fail, "CloseProcessStdin")
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !p.stdinClosed || !p.stdinCloseAcked {
		t.Fatalf("successful close state = requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
	}
	calls := len(client.calls)
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != calls {
		t.Fatal("acknowledged repeated CloseIO contacted the guest")
	}
}

func TestCloseIOPersistsCreatedIntentAndDoesNotMutateStoppedProcess(t *testing.T) {
	created := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	stopped := &process{id: "stopped", status: tasktypes.Status_STOPPED, done: make(chan struct{})}
	s := &service{processes: map[string]*process{"": created, "stopped": stopped}}
	request := &taskapi.CloseIORequest{Stdin: true}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !created.stdinClosed || created.stdinCloseAcked {
		t.Fatalf("created close state = requested:%v acknowledged:%v", created.stdinClosed, created.stdinCloseAcked)
	}
	if _, err := s.CloseIO(context.Background(), request); err != nil {
		t.Fatalf("repeated created CloseIO error = %v", err)
	}
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{ExecID: "stopped", Stdin: true}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped CloseIO error = %v", err)
	}
	if stopped.stdinClosed || stopped.stdinCloseAcked {
		t.Fatalf("stopped CloseIO mutated state: %+v", stopped)
	}
}

func TestStdinPumpRetriesRequestedCloseAndRecordsAcknowledgement(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinReader: eofReadWriteCloser{}, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	if _, err := s.CloseIO(context.Background(), &taskapi.CloseIORequest{Stdin: true}); err != nil {
		t.Fatal(err)
	}
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged, reader := p.stdinCloseAcked, p.stdinReader
		s.mu.Unlock()
		if acknowledged && reader == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pump close state = acknowledged:%v reader:%v", acknowledged, reader)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRecoveredNoFIFOPumpRetriesPendingClose(t *testing.T) {
	client := &closeRetryAgent{called: make(chan int, 2)}
	p := &process{id: "", status: tasktypes.Status_RUNNING, stdinClosed: true, done: make(chan struct{})}
	s := &service{agent: client, processes: map[string]*process{"": p}}
	go s.pumpStdin("init", p)
	for expected := 1; expected <= 2; expected++ {
		select {
		case observed := <-client.called:
			if observed != expected {
				t.Fatalf("close attempt = %d, want %d", observed, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("close attempt %d was not observed", expected)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		acknowledged := p.stdinCloseAcked
		s.mu.Unlock()
		if acknowledged {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovered no-FIFO close was not acknowledged")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestResizePtyRejectsInvalidRequests(t *testing.T) {
	stopped := &process{terminal: true, status: tasktypes.Status_STOPPED, width: 80, height: 24, sizeSet: true}
	s := &service{processes: map[string]*process{
		"":        {status: tasktypes.Status_CREATED},
		"stopped": stopped,
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("non-terminal resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 65536}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("oversized resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{ExecID: "stopped", Width: 91, Height: 37}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("stopped resize error = %v", err)
	}
	if stopped.width != 80 || stopped.height != 24 || !stopped.sizeSet {
		t.Fatalf("rejected stopped resize mutated state: %+v", stopped)
	}
}

func TestResizePtyRollsBackIntentWhenGuestRejects(t *testing.T) {
	injected := errors.New("injected resize failure")
	fake := &fakeAgentClient{fail: map[string]error{"ResizeProcess": injected}}
	p := &process{terminal: true, status: tasktypes.Status_RUNNING, width: 80, height: 24, sizeSet: true}
	s := &service{agent: fake, processes: map[string]*process{"": p}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); !errors.Is(err, injected) {
		t.Fatalf("resize error = %v", err)
	}
	if p.width != 80 || p.height != 24 || !p.sizeSet {
		t.Fatalf("failed resize retained new intent: %+v", p)
	}
	delete(fake.fail, "ResizeProcess")
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 100, Height: 40}); err != nil {
		t.Fatal(err)
	}
	if p.width != 100 || p.height != 40 || !p.sizeSet {
		t.Fatalf("successful resize state: %+v", p)
	}
}

func TestExecRollsBackProcessOnAgentFailure(t *testing.T) {
	agentFailure := errors.New("injected agent failure")
	fake := &fakeAgentClient{fail: map[string]error{"ExecProcess": agentFailure}}
	s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
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

func TestExecRejectsGuestUnrepresentableIDBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{}}
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	for _, id := range []string{"", "../exec", "Exec", "exec_id", strings.Repeat("a", 65)} {
		if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: id, Spec: encoded}); !errors.Is(err, errdefs.ErrInvalidArgument) {
			t.Fatalf("ExecID %q error = %v, want invalid argument", id, err)
		}
	}
	if len(fake.calls) != 0 || len(s.processes) != 0 {
		t.Fatalf("invalid exec reached agent or state: calls=%v processes=%v", fake.calls, s.processes)
	}
}

func TestTaskStateGuardsRejectInvalidTransitionsBeforeAgentContact(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	init := &process{id: "", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	execProcess := &process{id: "exec", status: tasktypes.Status_CREATED, done: make(chan struct{})}
	s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}

	if _, err := s.Start(context.Background(), &taskapi.StartRequest{ExecID: "exec"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec start before init error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "missing", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrNotFound) {
		t.Fatalf("missing kill error = %v", err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "", Signal: uint32(syscall.SIGTERM)}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("created kill error = %v", err)
	}
	init.status = tasktypes.Status_STOPPED
	spec := &specs.Process{User: specs.User{}, Args: []string{"/bin/true"}, Cwd: "/"}
	encodedValue, err := typeurl.MarshalAny(spec)
	if err != nil {
		t.Fatal(err)
	}
	encoded := &anypb.Any{TypeUrl: encodedValue.GetTypeUrl(), Value: encodedValue.GetValue()}
	if _, err = s.Exec(context.Background(), &taskapi.ExecProcessRequest{ExecID: "new-exec", Spec: encoded}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("exec after init stop error = %v", err)
	}
	if _, err = s.Delete(context.Background(), &taskapi.DeleteRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("init delete with retained exec error = %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("invalid transitions contacted agent: %v", fake.calls)
	}
}

func TestPreCancelledTaskMutationsPreserveStateAndAvoidGuestContact(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assertCanceled := func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	}

	t.Run("create", func(t *testing.T) {
		s := &service{id: "task", bundle: "/bundle", processes: map[string]*process{}}
		_, err := s.Create(ctx, &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"})
		assertCanceled(t, err)
		if len(s.processes) != 0 {
			t.Fatalf("processes = %+v", s.processes)
		}
	})

	t.Run("start", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		init := &process{status: tasktypes.Status_RUNNING}
		execProcess := &process{status: tasktypes.Status_CREATED}
		s := &service{agent: fake, processes: map[string]*process{"": init, "exec": execProcess}}
		_, err := s.Start(ctx, &taskapi.StartRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if execProcess.status != tasktypes.Status_CREATED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", execProcess.status, fake.calls)
		}
	})

	t.Run("kill", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Kill(ctx, &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM)})
		assertCanceled(t, err)
		if len(fake.calls) != 0 {
			t.Fatalf("calls=%v", fake.calls)
		}
	})

	t.Run("exec", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		s := &service{agent: fake, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
		_, err := s.Exec(ctx, &taskapi.ExecProcessRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if len(s.processes) != 1 || len(fake.calls) != 0 {
			t.Fatalf("processes=%+v calls=%v", s.processes, fake.calls)
		}
	})

	t.Run("delete", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_STOPPED}
		s := &service{agent: fake, processes: map[string]*process{"exec": p}}
		_, err := s.Delete(ctx, &taskapi.DeleteRequest{ExecID: "exec"})
		assertCanceled(t, err)
		if p.deleting || p.exitEventQueued || p.deleteEventQueued || len(fake.calls) != 0 {
			t.Fatalf("process=%+v calls=%v", p, fake.calls)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		s := &service{processes: map[string]*process{}}
		_, err := s.Shutdown(ctx, &taskapi.ShutdownRequest{})
		assertCanceled(t, err)
		if s.shuttingDown {
			t.Fatal("cancelled shutdown sealed service")
		}
	})

	t.Run("resize", func(t *testing.T) {
		p := &process{terminal: true, status: tasktypes.Status_CREATED, width: 80, height: 24, sizeSet: true}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.ResizePty(ctx, &taskapi.ResizePtyRequest{Width: 100, Height: 40})
		assertCanceled(t, err)
		if p.width != 80 || p.height != 24 || !p.sizeSet {
			t.Fatalf("size=%dx%d set=%v", p.width, p.height, p.sizeSet)
		}
	})

	t.Run("pause", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_RUNNING}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Pause(ctx, &taskapi.PauseRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_RUNNING || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("resume", func(t *testing.T) {
		fake := &fakeAgentClient{fail: map[string]error{}}
		p := &process{status: tasktypes.Status_PAUSED}
		s := &service{agent: fake, processes: map[string]*process{"": p}}
		_, err := s.Resume(ctx, &taskapi.ResumeRequest{})
		assertCanceled(t, err)
		if p.status != tasktypes.Status_PAUSED || len(fake.calls) != 0 {
			t.Fatalf("status=%v calls=%v", p.status, fake.calls)
		}
	})

	t.Run("close-io", func(t *testing.T) {
		p := &process{status: tasktypes.Status_CREATED}
		s := &service{processes: map[string]*process{"": p}}
		_, err := s.CloseIO(ctx, &taskapi.CloseIORequest{Stdin: true})
		assertCanceled(t, err)
		if p.stdinClosed || p.stdinCloseAcked {
			t.Fatalf("close state=requested:%v acknowledged:%v", p.stdinClosed, p.stdinCloseAcked)
		}
	})
}

func TestKillForwardsOnlyForLiveKnownProcess(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_PAUSED},
	}}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{Signal: uint32(syscall.SIGTERM), All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Kill(context.Background(), &taskapi.KillRequest{ExecID: "exec", Signal: uint32(syscall.SIGKILL)}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(fake.calls) != "[SignalProcess SignalProcess]" {
		t.Fatalf("kill calls = %v", fake.calls)
	}
}

func TestShutdownWaitsForEmptyOwnershipAndSealsService(t *testing.T) {
	called := make(chan struct{}, 1)
	s := &service{id: "task", bundle: "/bundle", shutdown: func() { called <- struct{}{} }, processes: map[string]*process{
		"": {status: tasktypes.Status_STOPPED},
	}}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task", Now: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("shutdown callback ran while process ownership remained")
	default:
	}
	s.mu.Lock()
	delete(s.processes, "")
	s.mu.Unlock()
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("empty service did not invoke shutdown callback")
	}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{ID: "task", Bundle: "/bundle"}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("create after shutdown error = %v", err)
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("repeated shutdown invoked callback again")
	default:
	}
}

func TestShutdownRetainsOwnershipUntilDurableEventsFlush(t *testing.T) {
	bundle := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundle, ".multikernel"), 0700); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{failures: 2}
	called := make(chan struct{}, 1)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher,
		shutdown: func() { called <- struct{}{} }, processes: map[string]*process{},
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskDeleteEventTopic, &eventstypes.TaskDelete{ContainerID: "task", ID: "task"}); err != nil {
		t.Fatal(err)
	}
	if len(s.events.Pending) != 1 {
		t.Fatalf("pending events = %d", len(s.events.Pending))
	}
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err == nil {
		t.Fatal("shutdown accepted while durable event delivery failed")
	}
	select {
	case <-called:
		t.Fatal("failed event flush invoked shutdown callback")
	default:
	}
	publisher.failures = 0
	if _, err := s.Shutdown(context.Background(), &taskapi.ShutdownRequest{ID: "task"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not proceed after event delivery recovered")
	}
	if len(s.events.Pending) != 0 {
		t.Fatalf("events remain pending after shutdown: %+v", s.events.Pending)
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
		id: "task", namespace: "default", bundle: t.TempDir(), agent: fakeAgent, publisher: fakeEvents,
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING}},
	}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_PAUSED || s.processes["exec"].status != tasktypes.Status_PAUSED {
		t.Fatalf("pause statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if _, err := s.Resume(context.Background(), &taskapi.ResumeRequest{}); err != nil {
		t.Fatal(err)
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("resume statuses = init:%v exec:%v", s.processes[""].status, s.processes["exec"].status)
	}
	if len(fakeAgent.calls) != 4 {
		t.Fatalf("agent calls = %v", fakeAgent.calls)
	}
	if len(fakeEvents.topics) != 2 || fakeEvents.topics[0] != ctruntime.TaskPausedEventTopic || fakeEvents.topics[1] != ctruntime.TaskResumedEventTopic {
		t.Fatalf("event topics = %v", fakeEvents.topics)
	}
}

func TestPauseRollsBackAlreadySignaledProcessOnPartialFailure(t *testing.T) {
	fake := &signalFailureAgent{failAt: 2}
	s := &service{agent: fake, processes: map[string]*process{
		"":     {status: tasktypes.Status_RUNNING},
		"exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Pause(context.Background(), &taskapi.PauseRequest{}); err == nil {
		t.Fatal("partial pause failure was accepted")
	}
	if s.processes[""].status != tasktypes.Status_RUNNING || s.processes["exec"].status != tasktypes.Status_RUNNING {
		t.Fatalf("partial pause mutated states: init=%v exec=%v", s.processes[""].status, s.processes["exec"].status)
	}
	want := fmt.Sprintf("[init:%d exec:%d init:%d]", syscall.SIGSTOP, syscall.SIGSTOP, syscall.SIGCONT)
	if fmt.Sprint(fake.calls) != want {
		t.Fatalf("partial pause calls = %v, want %s", fake.calls, want)
	}
}

func TestStatsReturnsGuestProcessGroupMetrics(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: 11, CPUSystemNS: 7, RSSBytes: 4096, PIDs: 3},
		"exec": {CPUUserNS: 5, CPUSystemNS: 2, RSSBytes: 2048, PIDs: 1},
	}}
	s := &service{
		agent: fake, sandbox: protocol.Sandbox{Config: protocol.SandboxConfig{MemoryBytes: 3 << 30}},
		processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_PAUSED}, "created": {status: tasktypes.Status_CREATED}},
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
	if metrics.CPU.Usage.Total != 25 || metrics.CPU.Usage.User != 16 || metrics.CPU.Usage.Kernel != 9 || metrics.Memory.Usage.Usage != 6144 || metrics.Memory.Usage.Limit != 3<<30 || metrics.Pids.Current != 4 {
		t.Fatalf("stats = %+v", metrics)
	}
	if fmt.Sprint(fake.calls) != "[StatsProcess StatsProcess]" {
		t.Fatalf("stats calls = %v", fake.calls)
	}
}

func TestStatsRejectsAggregateOverflow(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}, statsByID: map[string]agent.ProcessStats{
		"init": {CPUUserNS: ^uint64(0)},
		"exec": {CPUUserNS: 1},
	}}
	s := &service{agent: fake, processes: map[string]*process{
		"": {status: tasktypes.Status_RUNNING}, "exec": {status: tasktypes.Status_RUNNING},
	}}
	if _, err := s.Stats(context.Background(), &taskapi.StatsRequest{}); err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatalf("overflow stats error = %v", err)
	}
}

func TestUpdateAndCheckpointAreExcludedWithoutMutation(t *testing.T) {
	fake := &fakeAgentClient{fail: map[string]error{}}
	events := &fakePublisher{}
	s := &service{agent: fake, publisher: events, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	if _, err := s.Update(context.Background(), &taskapi.UpdateTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Update error = %v", err)
	}
	if _, err := s.Checkpoint(context.Background(), &taskapi.CheckpointTaskRequest{}); !errors.Is(err, errdefs.ErrNotImplemented) {
		t.Fatalf("Checkpoint error = %v", err)
	}
	if len(fake.calls) != 0 || len(events.topics) != 0 || s.processes[""].status != tasktypes.Status_RUNNING {
		t.Fatalf("excluded methods mutated state: calls=%v events=%v status=%v", fake.calls, events.topics, s.processes[""].status)
	}
}

func TestEventJournalPreservesOrderAndReplaysAfterFailure(t *testing.T) {
	bundle := t.TempDir()
	failed := &fakePublisher{failures: 2}
	s := &service{bundle: bundle, namespace: "default", publisher: failed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	if len(failed.topics) != 0 {
		t.Fatalf("later event bypassed failed predecessor: %v", failed.topics)
	}
	replayed := &fakePublisher{}
	recovered := &service{bundle: bundle, namespace: "default", publisher: replayed,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := recovered.loadEventJournal(); err != nil {
		t.Fatal(err)
	}
	if err := recovered.flushEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskCreateEventTopic, ctruntime.TaskStartEventTopic}
	if fmt.Sprint(replayed.topics) != fmt.Sprint(want) {
		t.Fatalf("replayed topics = %v, want %v", replayed.topics, want)
	}
	if _, err := os.Stat(recovered.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged journal remains: %v", err)
	}
}

func TestEventJournalRetriesWithoutAnotherLifecycleRequest(t *testing.T) {
	publisher := &retryPublisher{published: make(chan string, 1)}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskStartEventTopic,
		&eventstypes.TaskStart{ContainerID: "task", Pid: 7}); err != nil {
		t.Fatal(err)
	}
	s.startEventRetry(5 * time.Millisecond)
	defer s.stopEventRetry()
	select {
	case topic := <-publisher.published:
		if topic != ctruntime.TaskStartEventTopic {
			t.Fatalf("retried topic = %q", topic)
		}
	case <-time.After(time.Second):
		t.Fatal("durable event was not retried while the shim remained running")
	}
	s.stopEventRetry()
	if len(s.events.Pending) != 0 {
		t.Fatalf("acknowledged retry remains pending: %+v", s.events.Pending)
	}
	if _, err := os.Stat(s.eventJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("acknowledged retry journal remains: %v", err)
	}
}

func TestEventJournalPersistenceFailurePrecedesPublication(t *testing.T) {
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected journal failure") }
	defer func() { jsonMarshal = originalMarshal }()
	publisher := &fakePublisher{}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	if err := s.publish(context.Background(), ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}); err == nil {
		t.Fatal("journal failure was ignored")
	}
	if len(publisher.topics) != 0 {
		t.Fatalf("event published before durable queue: %v", publisher.topics)
	}
	if len(s.events.Pending) != 0 || s.events.NextSequence != 1 {
		t.Fatalf("failed queue mutated memory: %+v", s.events)
	}
}

func TestEventJournalAckFailureRetainsPublishedEventForReplay(t *testing.T) {
	publisher := &fakePublisher{failures: 2}
	s := &service{bundle: t.TempDir(), namespace: "default", publisher: publisher,
		events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	for _, item := range []struct {
		topic string
		event any
	}{{ctruntime.TaskCreateEventTopic, &eventstypes.TaskCreate{ContainerID: "task"}},
		{ctruntime.TaskStartEventTopic, &eventstypes.TaskStart{ContainerID: "task", Pid: 7}}} {
		if err := s.publish(context.Background(), item.topic, item.event); err != nil {
			t.Fatal(err)
		}
	}
	originalMarshal := jsonMarshal
	jsonMarshal = func(any) ([]byte, error) { return nil, errors.New("injected acknowledgement failure") }
	defer func() { jsonMarshal = originalMarshal }()
	if err := s.flushEvents(context.Background()); err == nil {
		t.Fatal("acknowledgement failure was ignored")
	}
	if len(publisher.topics) != 1 || len(s.events.Pending) != 2 || s.events.Pending[0].Sequence != 1 {
		t.Fatalf("at-least-once replay state was lost: topics=%v events=%+v", publisher.topics, s.events)
	}
}

func TestEventJournalRejectsSymlinkAndPermissiveState(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{"symlink", func(path string) error {
			target := path + ".target"
			if err := os.WriteFile(target, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0600); err != nil {
				return err
			}
			return os.Symlink(target, path)
		}},
		{"permissive", func(path string) error {
			return os.WriteFile(path, []byte(`{"schema_version":1,"next_sequence":1,"pending":[]}`), 0644)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &service{bundle: t.TempDir(), events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
			if err := test.make(s.eventJournalPath()); err != nil {
				t.Fatal(err)
			}
			if err := s.loadEventJournal(); err == nil {
				t.Fatal("unsafe event journal accepted")
			}
		})
	}
}

func TestDeleteRepairsMissingExitEventBeforeDeleteEvent(t *testing.T) {
	publisher := &fakePublisher{}
	fake := &fakeAgentClient{fail: map[string]error{}}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exit: 17,
		exited: time.Unix(123, 0).UTC(), done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: t.TempDir(), publisher: publisher, agent: fake,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	response, err := s.Delete(context.Background(), &taskapi.DeleteRequest{ExecID: "exec"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ctruntime.TaskExitEventTopic, ctruntime.TaskDeleteEventTopic}
	if response.ExitStatus != 17 || fmt.Sprint(publisher.topics) != fmt.Sprint(want) {
		t.Fatalf("response=%+v topics=%v, want %v", response, publisher.topics, want)
	}
	if _, exists := s.processes["exec"]; exists {
		t.Fatal("deleted exec process remains")
	}
}

func TestDeleteRetainsRetryOwnershipAcrossGuestAndEventFailures(t *testing.T) {
	bundle := t.TempDir()
	agentFailure := errors.New("injected guest delete failure")
	fakeAgent := &fakeAgentClient{fail: map[string]error{"DeleteProcess": agentFailure}}
	publisher := &fakePublisher{}
	p := &process{id: "exec", pid: 23, status: tasktypes.Status_STOPPED, exitEventQueued: true, done: make(chan struct{})}
	close(p.done)
	s := &service{id: "task", namespace: "default", bundle: bundle, publisher: publisher, agent: fakeAgent,
		processes: map[string]*process{"exec": p}, events: eventJournal{SchemaVersion: 1, NextSequence: 1}}
	request := &taskapi.DeleteRequest{ExecID: "exec"}
	if _, err := s.Delete(context.Background(), request); !errors.Is(err, agentFailure) {
		t.Fatalf("guest delete error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || p.deleteEventQueued {
		t.Fatalf("guest failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	delete(fakeAgent.fail, "DeleteProcess")
	publisher.failures = 2
	if _, err := s.Delete(context.Background(), request); err == nil || !strings.Contains(err.Error(), "flush task delete event") {
		t.Fatalf("event flush error = %v", err)
	}
	if s.processes["exec"] != p || p.deleting || !p.deleteEventQueued {
		t.Fatalf("event failure lost retry state: process=%p deleting=%v event=%v", s.processes["exec"], p.deleting, p.deleteEventQueued)
	}
	publisher.failures = 0
	response, err := s.Delete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Pid != 23 || s.processes["exec"] != nil || len(publisher.topics) != 1 || publisher.topics[0] != ctruntime.TaskDeleteEventTopic {
		t.Fatalf("retry response=%+v processes=%v topics=%v", response, s.processes, publisher.topics)
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
			"": {id: "", pid: 7, status: tasktypes.Status_RUNNING, stdinClosed: true, stdinCloseAcked: true, stdoutOffset: 123, stderrOffset: 45, exitEventQueued: true, deleteEventQueued: true, done: make(chan struct{})},
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
	if saved.SchemaVersion != 1 || saved.Generation != s.sandbox.Generation || len(saved.Processes) != 1 || saved.Processes[0].PID != 7 || !saved.Processes[0].StdinClosed || !saved.Processes[0].StdinCloseAcked || !saved.Processes[0].ExitEventQueued || !saved.Processes[0].DeleteEventQueued || saved.Processes[0].StdoutOffset != 123 {
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

func TestRelayOwnershipKillsDescendantsAndRetainsSocketCleanup(t *testing.T) {
	t.Run("command has private process group", func(t *testing.T) {
		command := newRelayCommand("/bin/true", 7001, "/tmp/mk-relay-test.sock")
		if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
			t.Fatalf("relay process attributes = %+v", command.SysProcAttr)
		}
	})

	t.Run("stop kills group and removes socket", func(t *testing.T) {
		command := exec.Command("/bin/sh", "-c", "sleep 60 & wait")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		processGroup := command.Process.Pid
		socket := filepath.Join(t.TempDir(), "relay.sock")
		if err := os.WriteFile(socket, []byte("placeholder"), 0600); err != nil {
			t.Fatal(err)
		}
		s := &service{relay: command, relaySocket: socket}
		if err := s.stopRelay(); err != nil {
			t.Fatal(err)
		}
		if s.relay != nil || s.relaySocket != "" {
			t.Fatalf("relay ownership remains: command=%v socket=%q", s.relay, s.relaySocket)
		}
		if _, err := os.Stat(socket); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("relay socket remains: %v", err)
		}
		deadline := time.Now().Add(time.Second)
		for {
			err := syscall.Kill(-processGroup, syscall.Signal(0))
			if errors.Is(err, syscall.ESRCH) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("relay process group %d remains: %v", processGroup, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("socket cleanup remains retryable", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "relay.sock")
		if err := os.Mkdir(socket, 0700); err != nil {
			t.Fatal(err)
		}
		occupied := filepath.Join(socket, "occupied")
		if err := os.WriteFile(occupied, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		s := &service{relaySocket: socket}
		if err := s.stopRelay(); err == nil || s.relaySocket != socket {
			t.Fatalf("failed socket cleanup: error=%v retained=%q", err, s.relaySocket)
		}
		if err := os.Remove(occupied); err != nil {
			t.Fatal(err)
		}
		if err := s.stopRelay(); err != nil || s.relaySocket != "" {
			t.Fatalf("socket cleanup retry: error=%v retained=%q", err, s.relaySocket)
		}
	})
}

func TestStaleRelayCleanupRejectsNonSocketPathsWithoutRemoval(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.sock")
	if err := removeStaleRelaySocket(missing); err != nil {
		t.Fatalf("missing stale socket error = %v", err)
	}
	for name, create := range map[string]func(string) error{
		"regular":   func(path string) error { return os.WriteFile(path, []byte("owned data"), 0600) },
		"directory": func(path string) error { return os.Mkdir(path, 0700) },
		"symlink":   func(path string) error { return os.Symlink("missing-target", path) },
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "relay.sock")
			if err := create(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = removeStaleRelaySocket(path); err == nil {
				t.Fatal("non-socket stale relay path was accepted")
			}
			after, statErr := os.Lstat(path)
			if statErr != nil || before.Mode() != after.Mode() || !os.SameFile(before, after) {
				t.Fatalf("rejected path changed: before=%+v after=%+v error=%v", before, after, statErr)
			}
		})
	}
}

func validPersistedRecovery(namespace, task string) persisted {
	return persisted{
		SchemaVersion: 1,
		ID:            sandboxID(namespace, task),
		Generation:    strings.Repeat("a", 32),
		TaskIdentity:  storageTaskIdentity(namespace, task),
		Processes:     []persistedProcess{{ID: "", Status: tasktypes.Status_STOPPED}},
	}
}

func TestRecoveryStateIsBoundedStrictAndIdentityBound(t *testing.T) {
	namespace, task := "default", "task-a"
	valid := validPersistedRecovery(namespace, task)
	encode := func(t *testing.T, value persisted) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, found, err := loadPersistedRecovery(path, namespace, task)
		if err != nil || !found || loaded.ID != valid.ID || loaded.Generation != valid.Generation {
			t.Fatalf("loaded recovery = %+v, found=%v error=%v", loaded, found, err)
		}
	})

	for name, mutate := range map[string]func(*persisted){
		"wrong sandbox":     func(value *persisted) { value.ID = sandboxID(namespace, "other") },
		"wrong generation":  func(value *persisted) { value.Generation = "short" },
		"wrong task owner":  func(value *persisted) { value.TaskIdentity = storageTaskIdentity(namespace, "other") },
		"partial network":   func(value *persisted) { value.Network.SandboxID = value.ID },
		"duplicate process": func(value *persisted) { value.Processes = append(value.Processes, value.Processes[0]) },
		"invalid state":     func(value *persisted) { value.Processes[0].Status = tasktypes.Status_UNKNOWN },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			value.Processes = append([]persistedProcess(nil), valid.Processes...)
			mutate(&value)
			path := filepath.Join(t.TempDir(), "sandbox.json")
			if err := os.WriteFile(path, encode(t, value), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
				t.Fatalf("unsafe recovery accepted: %+v", value)
			}
		})
	}

	t.Run("unknown field", func(t *testing.T) {
		data := encode(t, valid)
		data = append(data[:len(data)-1], []byte(`,"unknown":true}`)...)
		path := filepath.Join(t.TempDir(), "sandbox.json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("recovery with unknown field was accepted")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target")
		if err := os.WriteFile(target, encode(t, valid), 0600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "sandbox.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadPersistedRecovery(path, namespace, task); err == nil {
			t.Fatal("symlinked recovery was accepted")
		}
	})
}

func TestFallbackCleanupPropagatesStopFailureBeforeDelete(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	data, err := json.Marshal(validPersistedRecovery(namespace, task))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "StopSandbox" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected stop failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected stop failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(calls, []string{"StopSandbox"}) {
		t.Fatalf("cleanup calls after failed stop = %v", calls)
	}
}

func TestFallbackCleanupPropagatesRootfsFailureAfterSandboxDelete(t *testing.T) {
	bundle := t.TempDir()
	runtimeDir := filepath.Join(bundle, ".multikernel")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	namespace, task := "default", "task-a"
	recovery := validPersistedRecovery(namespace, task)
	recovery.StorageSHA256 = strings.Repeat("b", 64)
	data, err := json.Marshal(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(runtimeDir, "sandbox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(bundle); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var calls []string
	s := &service{id: task, namespace: namespace, bundle: bundle, daemon: daemonCallFunc(func(_ context.Context, request protocol.Request, _ any) *protocol.Error {
		calls = append(calls, request.Method)
		if request.Method == "CleanupRootfs" {
			return &protocol.Error{Code: "BACKEND_FAILURE", Message: "injected rootfs cleanup failure"}
		}
		return nil
	})}
	response, err := s.Cleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "injected rootfs cleanup failure") || response == nil {
		t.Fatalf("Cleanup response=%+v error=%v", response, err)
	}
	want := []string{"StopSandbox", "DeleteSandbox", "CleanupRootfs"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("cleanup call order = %v, want %v", calls, want)
	}
}
