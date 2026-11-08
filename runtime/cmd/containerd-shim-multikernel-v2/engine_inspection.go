//go:build linux

package main

import (
	"context"
	"errors"
	"time"

	"github.com/containerd/containerd/plugin"
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

// shim.Run wraps TaskService before registration, so a separate standard shim
// plugin registers this extension on the same authenticated socket.
func registerEngineInspection(s *service) {
	plugin.Register(&plugin.Registration{Type: plugin.TTRPCPlugin, ID: "multikernel-inspection", InitFn: func(*plugin.InitContext) (interface{}, error) { return &engineInspectionService{owner: s}, nil }})
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
