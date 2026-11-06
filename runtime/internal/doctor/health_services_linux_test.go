//go:build linux

package doctor

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/kerf"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/lifecycle"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/network"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/poolinspect"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/storage"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

// Only Observe is permitted while preparing the synthetic ABSENT history.
// Any unexpected resource operation hits a nil embedded backend and fails the
// test; there is no CLI or real kernel backend that could allocate resources.
type healthHistoryBackend struct {
	kerf.Backend
	observations atomic.Int64
}

func (b *healthHistoryBackend) Observe(context.Context, string) (string, error) {
	b.observations.Add(1)
	return "ABSENT", nil
}

type healthNoNetworkBackend struct{ calls atomic.Int64 }

func (b *healthNoNetworkBackend) Add(context.Context, network.Endpoint) error {
	b.calls.Add(1)
	return errors.New("fixture forbids network mutation")
}
func (b *healthNoNetworkBackend) Check(context.Context, network.Endpoint) error {
	b.calls.Add(1)
	return errors.New("fixture has no endpoints")
}
func (b *healthNoNetworkBackend) Delete(context.Context, network.Endpoint) error {
	b.calls.Add(1)
	return errors.New("fixture forbids network mutation")
}

type healthServices struct {
	root                         string
	store                        *state.Store
	combined                     CombinedInputs
	pool                         PoolInputs
	builds                       ComponentInputs
	health                       HealthInputs
	history                      *healthHistoryBackend
	network                      *healthNoNetworkBackend
	runtimeSocket, networkSocket string
}

func newHealthServices(t *testing.T) *healthServices {
	t.Helper()
	parent := os.Getenv("MK_TEST_HEALTH_FIXTURE_PARENT")
	if parent != "" {
		dir, err := safefile.OpenDirectory(parent, false)
		if err != nil {
			t.Fatal("explicit private health fixture parent refused")
		}
		dir.Close()
	}
	root, err := os.MkdirTemp(parent, "mk-health-rpc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	st, err := state.Open(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	sb := protocol.Sandbox{ID: "health-history", Generation: strings.Repeat("a", 32), State: "ABSENT", Config: protocol.SandboxConfig{ID: "health-history", CPUs: []int{8}, MemoryBytes: 1 << 30, ChildCID: 40, AgentPort: 7200, Labels: map[string]string{protocol.NetworkExpectationLabel: "none", "private": "SYNTHETIC_HEALTH_PRIVATE_CANARY"}}}
	if err = st.SetSandbox(sb); err != nil {
		t.Fatal(err)
	}
	backend := &healthHistoryBackend{}
	service := lifecycle.New(st, backend, lifecycle.Artifacts{})
	service.SetPoolCPUs([]int{8, 9})
	service.SetPoolMemory(8<<30, 1<<30)
	// Reconcile only synthetic ABSENT history, before installing the storage
	// observer. This uses the real initialization/ownership path, not a setter.
	if err = service.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	storageStore, err := storage.OpenStore(filepath.Join(root, "storage"))
	if err != nil {
		t.Fatal(err)
	}
	service.SetStorage(storage.NewService(storageStore, &storage.LinuxBackend{RuntimeDir: filepath.Join(root, "never-open-storage-runtime")}))
	networkStore, err := network.OpenStore(filepath.Join(root, "network"))
	if err != nil {
		t.Fatal(err)
	}
	bootID, err := BootIdentity()
	if err != nil {
		t.Fatal(err)
	}
	netBackend := &healthNoNetworkBackend{}
	netService, err := network.NewService(networkStore, netBackend, "172.29.0.0/24", 1400, network.DNS{}, bootID)
	if err != nil {
		t.Fatal(err)
	}
	runtimeServer := &daemon.Server{Service: service, AllowedUID: uint32(os.Geteuid())}
	networkServer := &network.Server{Service: netService, AllowedUID: uint32(os.Geteuid())}
	ctx, cancel := context.WithCancel(t.Context())
	runtimeSocket, networkSocket := filepath.Join(root, "runtime.sock"), filepath.Join(root, "network.sock")
	runtimeDone, networkDone := make(chan error, 1), make(chan error, 1)
	go func() { runtimeDone <- runtimeServer.Listen(ctx, runtimeSocket) }()
	go func() { networkDone <- networkServer.Listen(ctx, networkSocket) }()
	t.Cleanup(func() {
		cancel()
		runtimeServer.Close()
		networkServer.Close()
		for _, done := range []chan error{runtimeDone, networkDone} {
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(2 * time.Second):
				t.Error("production RPC server did not exit")
			}
		}
	})
	until := time.Now().Add(2 * time.Second)
	for {
		_, a := os.Lstat(runtimeSocket)
		_, b := os.Lstat(networkSocket)
		if a == nil && b == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatal("private RPC listeners unavailable")
		}
		time.Sleep(time.Millisecond)
	}
	runtimeClient := daemon.Client{Path: runtimeSocket, Timeout: time.Second}
	netClient := network.Client{Path: networkSocket, Timeout: time.Second}
	f := &healthServices{root: root, store: st, history: backend, network: netBackend, runtimeSocket: runtimeSocket, networkSocket: networkSocket}
	f.combined = CombinedInputs{Native: func(ctx context.Context) (protocol.NativeResourceReport, error) {
		return daemon.InspectNativeResources(ctx, runtimeClient)
	}, Expectations: func(ctx context.Context) (protocol.NetworkExpectationsReport, error) {
		return daemon.InspectNetworkExpectations(ctx, runtimeClient)
	}, Network: func(ctx context.Context) (network.OwnershipReport, error) {
		return network.InspectOwnership(ctx, netClient)
	}, Kernel: func(context.Context) ([]string, error) { return []string{}, nil }, Boot: BootIdentity}
	f.pool = PoolInputs{Ownership: func(ctx context.Context) (protocol.PoolOwnershipReport, error) {
		return daemon.InspectPoolOwnership(ctx, runtimeClient)
	}, Kernel: func(context.Context) (poolinspect.Snapshot, error) {
		return poolinspect.Snapshot{CPUs: poolinspect.CPUs{Members: []uint64{8, 9}, Available: []uint64{8, 9}}, Memory: poolinspect.Memory{Chunks: 1, TotalBytes: 8 << 30, AvailableBytes: 8 << 30, LargestFreeExtentBytes: 8 << 30}}, nil
	}, Boot: BootIdentity}
	f.builds = ComponentInputs{Runtime: func(ctx context.Context) (protocol.ComponentBuildReport, error) {
		return daemon.InspectComponentBuild(ctx, runtimeClient)
	}, Network: func(ctx context.Context) (protocol.ComponentBuildReport, error) {
		return network.InspectComponentBuild(ctx, netClient)
	}, Boot: BootIdentity}
	f.health = HealthInputs{Journal: func(ctx context.Context) (protocol.JournalStatusReport, error) {
		return daemon.InspectJournalStatus(ctx, runtimeClient)
	}, Network: func(ctx context.Context) (network.HealthReport, error) { return network.InspectHealth(ctx, netClient) }}
	return f
}

func (f *healthServices) pending(t *testing.T) {
	t.Helper()
	if err := f.store.Append(state.JournalEntry{OperationID: "synthetic-operation", SandboxID: "health-history", Generation: strings.Repeat("a", 32), Method: "DeleteSandbox", Phase: "intent", IdempotencyKey: "SYNTHETIC_HEALTH_PRIVATE_CANARY", Fingerprint: "synthetic-fingerprint"}); err != nil {
		t.Fatal(err)
	}
}
func (f *healthServices) failJournal(t *testing.T) {
	t.Helper()
	path := filepath.Join(f.root, "state/journal.jsonl")
	if err := os.Rename(path, path+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("SYNTHETIC_HEALTH_PRIVATE_CANARY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Append(state.JournalEntry{OperationID: "synthetic-refused", SandboxID: "health-history", Phase: "intent"}); err == nil {
		t.Fatal("synthetic replaced journal accepted")
	}
}
func (f *healthServices) files(t *testing.T) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	for _, name := range []string{"state/state.json", "state/journal.jsonl", "state/journal.jsonl.held", "storage/state.json", "network/state.json"} {
		data, err := os.ReadFile(filepath.Join(f.root, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[name] = sha256.Sum256(data)
	}
	return result
}

func TestHealthReportProductionServersIdlePendingAndFailedJournal(t *testing.T) {
	f := newHealthServices(t)
	observations := f.history.observations.Load()
	for _, phase := range []string{"idle", "pending", "failed", "repeat-failed"} {
		if phase == "pending" {
			f.pending(t)
		}
		if phase == "failed" {
			f.failJournal(t)
		}
		files, stateBefore := f.files(t), f.store.Snapshot()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		r := InspectHealth(ctx, f.combined, f.pool, f.builds, f.health, buildinfo.Revision)
		cancel()
		want := "consistent"
		if buildinfo.Revision == "unknown" || phase == "pending" {
			want = "inconclusive"
		}
		if strings.Contains(phase, "failed") {
			want = "degraded"
		}
		if r.Status != want || r.Journal == nil || r.Network == nil || r.Network.Status != "idle" || r.Operational == nil {
			t.Fatalf("production RPC phase %s: status=%s findings=%v", phase, r.Status, r.Findings)
		}
		if want != "consistent" && (len(r.Operational.Unified.Pool.CPUCandidates) != 0 || r.Operational.Unified.Pool.MemoryCandidateUpperBoundBytes != 0) {
			t.Fatal("unsafe capacity in production RPC aggregate")
		}
		if phase == "pending" && r.Journal.Status != "pending" {
			t.Fatal("pending journal masked")
		}
		if strings.Contains(phase, "failed") && (!r.Journal.JournalFailed || !hasFinding(r.Findings, "JOURNAL_FAILED")) {
			t.Fatal("failed journal masked")
		}
		if !reflect.DeepEqual(files, f.files(t)) || !reflect.DeepEqual(stateBefore, f.store.Snapshot()) {
			t.Fatal("inspection repaired or mutated private state")
		}
	}
	if f.history.observations.Load() != observations || f.network.calls.Load() != 0 {
		t.Fatal("inspection invoked a resource backend")
	}
	if _, err := os.Stat(filepath.Join(f.root, "never-open-storage-runtime")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("idle storage observer created runtime state")
	}
	t.Log("PRODUCTION_RPC_IDLE_PENDING_FAILED_REPEATED_INSPECTION_PRESERVES_STATE_AND_BACKENDS")
}
