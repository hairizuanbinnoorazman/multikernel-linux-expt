//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/errdefs"

	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

func rootfsMounts(input []*types.Mount) ([]rootfspkg.Mount, error) {
	if len(input) > 8 {
		return nil, fmt.Errorf("%w: at most eight rootfs mounts are supported", errdefs.ErrInvalidArgument)
	}
	result := make([]rootfspkg.Mount, len(input))
	for index, item := range input {
		if item == nil {
			return nil, fmt.Errorf("%w: nil rootfs mount", errdefs.ErrInvalidArgument)
		}
		if item.Type != "overlay" && item.Type != "bind" && item.Type != "none" {
			return nil, fmt.Errorf("%w: unsupported rootfs mount type %q", errdefs.ErrNotImplemented, item.Type)
		}
		if (item.Type == "bind" || item.Type == "none") && !filepath.IsAbs(item.Source) {
			return nil, fmt.Errorf("%w: bind rootfs source must be absolute", errdefs.ErrInvalidArgument)
		}
		options := make([]string, 0, len(item.Options))
		for _, option := range item.Options {
			if option == "rw" {
				continue
			}
			if strings.ContainsAny(option, "\x00\n\r") {
				return nil, fmt.Errorf("%w: unsafe rootfs mount option", errdefs.ErrInvalidArgument)
			}
			options = append(options, option)
		}
		result[index] = rootfspkg.Mount{Type: item.Type, Source: item.Source, Options: options}
	}
	if err := rootfspkg.ValidateMounts(result); err != nil {
		return nil, fmt.Errorf("%w: %v", errdefs.ErrInvalidArgument, err)
	}
	return result, nil
}

func (s *service) prepareRootfs(ctx context.Context, request rootfspkg.PrepareRequest) (rootfspkg.PrepareResult, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return rootfspkg.PrepareResult{}, err
	}
	var result rootfspkg.PrepareResult
	client := daemonCallerWithTimeout(s.daemon, rootfsPrepareDaemonTimeout)
	apiErr := client.Call(ctx, protocol.Request{Version: 1, RequestID: "prepare-rootfs-" + request.TaskIdentity, Method: "PrepareRootfs", Body: body}, &result)
	if apiErr != nil {
		return result, errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return result, nil
}

func (s *service) validateRootfs(ctx context.Context, request rootfspkg.ValidateRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "validate-rootfs-" + s.id,
		Method: "ValidateRootfs", Body: body}, nil)
	if apiErr != nil {
		return errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return nil
}

func (s *service) cleanupRootfs(ctx context.Context, request rootfspkg.CleanupRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "cleanup-rootfs-" + request.TaskIdentity, Method: "CleanupRootfs", Body: body}, nil)
	if apiErr != nil {
		return errors.New(apiErr.Code + ": " + apiErr.Message)
	}
	return nil
}
