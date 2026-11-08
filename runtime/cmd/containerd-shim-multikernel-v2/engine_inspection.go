//go:build linux

package main

import (
	"context"
	"errors"
	"time"

	"github.com/containerd/containerd/plugin"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/agent"
	"github.com/hairizuan/multikernel-linux-expt/runtime/inspection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type engineInspectionService struct{ owner *service }

func (i *engineInspectionService) RegisterTTRPC(server *ttrpc.Server) error {
	inspection.Register(server, i.owner.inspectDockerEngine)
	return nil
}

// Register once per binary invocation, outside the reusable service constructor.
// shim.Run constructs the owner before loading the plugin graph. The binding is
// immutable once its TTRPC service is published; no process-global owner is used.
func inspectionInitializer(register func(*plugin.Registration)) shim.Init {
	binding := &engineInspectionService{}
	register(&plugin.Registration{Type: plugin.TTRPCPlugin, ID: "multikernel-inspection", InitFn: func(*plugin.InitContext) (interface{}, error) {
		if binding.owner == nil {
			return nil, errors.New("engine inspection owner is not initialized")
		}
		return binding, nil
	}})
	return func(ctx context.Context, id string, publisher shim.Publisher, shutdown func()) (shim.Shim, error) {
		if binding.owner != nil {
			return nil, errors.New("shim inspection owner is already initialized")
		}
		owner, err := newServiceOwner(ctx, id, publisher, shutdown)
		if err != nil {
			return nil, err
		}
		binding.owner = owner
		return newTaskRPCShim(owner), nil
	}
}

func (s *service) inspectDockerEngine(parent context.Context, request inspection.Request) (inspection.Report, error) {
	fail := func(code codes.Code) (inspection.Report, error) {
		return inspection.Report{}, status.Error(code, "Docker engine inspection unavailable")
	}
	if request.Validate() != nil {
		return fail(codes.InvalidArgument)
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := lockContext(ctx, &s.mu); err != nil {
		return fail(codes.DeadlineExceeded)
	}
	defer s.mu.Unlock()
	if request.Namespace != s.namespace || request.ContainerID != s.id || request.SandboxID != s.sandbox.ID || request.Generation != s.sandbox.Generation {
		return fail(codes.FailedPrecondition)
	}
	init, ok := s.processes[""]
	if !ok || init.status.String() != "RUNNING" || s.agent == nil {
		return fail(codes.FailedPrecondition)
	}
	// Use the shim's existing authenticated queue. Observation never reconnects,
	// replaces a relay, signals a child or borrows another shim's credential.
	observation, err := agent.InspectDockerEngine(ctx, s.agent)
	if err != nil {
		if errors.Is(err, agent.ErrDockerEngineObservationUnsupported) {
			return fail(codes.Unimplemented)
		}
		if ctx.Err() != nil {
			return fail(codes.DeadlineExceeded)
		}
		return fail(codes.Unavailable)
	}
	return inspection.Report{Version: 1, Owner: request, Observation: observation}, nil
}
