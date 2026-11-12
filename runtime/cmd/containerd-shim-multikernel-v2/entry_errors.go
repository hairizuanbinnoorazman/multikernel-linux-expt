//go:build linux

package main

import (
	"context"
	"errors"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/runtime/v2/shim"
)

// The upstream runner formats these terminal errors directly on stderr, outside
// logrus. Keep this boundary outside the reusable service/Task RPC facade so
// internal callers retain their original errors and the runner can unwind its
// normal publisher/shutdown defers before exiting.
func protectShimEntry(initialize shim.Init) shim.Init {
	return func(ctx context.Context, id string, publisher shim.Publisher, shutdown func()) (shim.Shim, error) {
		delegate, err := initialize(ctx, id, publisher, shutdown)
		if err != nil || delegate == nil {
			return nil, errors.New("shim initialization failed")
		}
		return &protectedShimEntry{Shim: delegate}, nil
	}
}

type protectedShimEntry struct{ shim.Shim }

func (s *protectedShimEntry) StartShim(ctx context.Context, opts shim.StartOpts) (string, error) {
	address, err := s.Shim.StartShim(ctx, opts)
	if err != nil {
		return "", errors.New("shim manager startup failed")
	}
	return address, nil
}

func (s *protectedShimEntry) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	result, err := s.Shim.Cleanup(ctx)
	if err != nil {
		return nil, errors.New("shim manager cleanup failed")
	}
	return result, nil
}
