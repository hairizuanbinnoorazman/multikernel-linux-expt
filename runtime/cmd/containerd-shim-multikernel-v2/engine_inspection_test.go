//go:build linux

package main

import (
	"context"
	"errors"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/plugin"
	"os"
	"strings"
	"testing"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type inspectionAgent struct {
	fakeAgentClient
	methods     []string
	err         error
	unsupported bool
}

func (a *inspectionAgent) CallContext(ctx context.Context, method string, _ any, response any) error {
	a.methods = append(a.methods, method)
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		return errors.New("missing budget")
	}
	if a.err != nil {
		return a.err
	}
	if method == "Capabilities" {
		features := []string{}
		if !a.unsupported {
			features = append(features, protocol.DockerEngineObservationCapability)
		}
		return setJSONResponse(response, map[string]any{"protocol": 1, "protocol_features": features})
	}
	if method != "InspectDockerEngine" {
		return errors.New("unexpected operation")
	}
	return setJSONResponse(response, protocol.DockerEngineObservation{Version: 1, Source: "owned-child-docker-api-and-launcher", Status: "responding"})
}
func inspectionFixture() (*service, *inspectionAgent, inspection.Request) {
	a := &inspectionAgent{}
	q := inspection.Request{Version: 1, Namespace: "moby", ContainerID: "container", SandboxID: "box", Generation: strings.Repeat("a", 32)}
	s := &service{id: q.ContainerID, namespace: q.Namespace, sandbox: protocol.Sandbox{ID: q.SandboxID, Generation: q.Generation}, agent: a, processes: map[string]*process{"": {status: tasktypes.Status_RUNNING}}}
	return s, a, q
}
func TestShimEngineInspectionReadOnlyOwnerAndErrors(t *testing.T) {
	for _, mode := range []string{"responding", "namespace", "container", "sandbox", "generation", "stopped", "no-agent", "unsupported", "private-error"} {
		t.Run(mode, func(t *testing.T) {
			s, a, q := inspectionFixture()
			want := codes.FailedPrecondition
			calls := 0
			switch mode {
			case "responding":
				want = codes.OK
				calls = 2
			case "namespace":
				q.Namespace = "other"
			case "container":
				q.ContainerID = "other"
			case "sandbox":
				q.SandboxID = "other"
			case "generation":
				q.Generation = strings.Repeat("b", 32)
			case "stopped":
				s.processes[""].status = tasktypes.Status_STOPPED
			case "no-agent":
				s.agent = nil
			case "unsupported":
				a.unsupported = true
				want = codes.Unimplemented
				calls = 1
			case "private-error":
				a.err = errors.New("private-output /private/path token=secret")
				want = codes.Unavailable
				calls = 1
			}
			report, err := s.inspectDockerEngine(t.Context(), q)
			if status.Code(err) != want || len(a.methods) != calls {
				t.Fatalf("classification or calls: %v %v", err, a.methods)
			}
			if err != nil && (strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") || report.Version != 0) {
				t.Fatal("diagnostic escaped")
			}
			if err == nil && (report.Owner != q || report.Observation.Status != "responding") {
				t.Fatal("wrong observation owner")
			}
			for _, method := range a.methods {
				if method != "Capabilities" && method != "InspectDockerEngine" {
					t.Fatal("workload command used")
				}
			}
			if len(a.calls) != 0 {
				t.Fatal("observation reconnected or used legacy call")
			}
		})
	}
}
func TestShimEngineInspectionQueueCancellation(t *testing.T) {
	s, a, q := inspectionFixture()
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	report, err := s.inspectDockerEngine(ctx, q)
	if status.Code(err) != codes.DeadlineExceeded || report.Version != 0 || len(a.methods) != 0 {
		t.Fatal("queued observation reached child", err)
	}
	if s.agent != a || s.sandbox.Generation != q.Generation || s.processes[""].status != tasktypes.Status_RUNNING {
		t.Fatal("queued observation changed owner")
	}
}

// Exercise the actual binary initializer without mutating containerd's global
// plugin registry. Repeated owner construction must remain independently usable.
func TestShimEngineInspectionBinaryInitializerBinding(t *testing.T) {
	for attempt := 0; attempt < 2; attempt++ {
		bundle := t.TempDir()
		if err := os.Chmod(bundle, 0700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(bundle)
		for _, name := range []string{"MK_SHIM_BUNDLE_DEVICE", "MK_SHIM_BUNDLE_INODE", "MK_SHIM_BUNDLE_UID"} {
			t.Setenv(name, "")
		}
		var registration *plugin.Registration
		initialize := inspectionInitializer(func(r *plugin.Registration) {
			if registration != nil {
				t.Fatal("duplicate registration")
			}
			registration = r
		})
		if registration == nil || registration.ID != "multikernel-inspection" || registration.Type != plugin.TTRPCPlugin {
			t.Fatal("wrong extension registration")
		}
		if _, err := registration.InitFn(nil); err == nil {
			t.Fatal("unbound owner published")
		}
		created, err := initialize(namespaces.WithNamespace(t.Context(), "inspection-test"), "task", nil, func() {})
		if err != nil {
			t.Fatal(err)
		}
		facade, ok := created.(*taskRPCShim)
		if !ok {
			t.Fatal("initializer lost protected Task facade")
		}
		owner, ok := facade.delegate.(*service)
		if !ok {
			t.Fatal("initializer lost concrete owner")
		}
		instance, err := registration.InitFn(nil)
		if err != nil {
			t.Fatal(err)
		}
		registered, ok := instance.(*engineInspectionService)
		if !ok || registered.owner != owner {
			t.Fatal("registered inspection targets another service")
		}
		if _, err := initialize(namespaces.WithNamespace(t.Context(), "inspection-test"), "task", nil, func() {}); err == nil {
			t.Fatal("initializer retargeted its published owner")
		}
		owner.stopEventRetry()
		owner.closeBundleIdentity()
	}
}
