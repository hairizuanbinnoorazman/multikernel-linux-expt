//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/runtime/v2/shim"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/daemon"
	rootfspkg "github.com/hairizuan/multikernel-linux-expt/runtime/internal/rootfs"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func validateServiceIdentity(id, namespace, bundle string) error {
	directory, _, err := openServiceIdentity(id, namespace, bundle)
	if directory != nil {
		_ = directory.Close()
	}
	return err
}

func openServiceIdentity(id, namespace, bundle string) (*safefile.Directory, rootfspkg.DirectoryIdentity, error) {
	if !runtimeIdentifier.MatchString(id) || !runtimeIdentifier.MatchString(namespace) {
		return nil, rootfspkg.DirectoryIdentity{}, fmt.Errorf("%w: invalid task ID or containerd namespace", errdefs.ErrInvalidArgument)
	}
	if !filepath.IsAbs(bundle) || filepath.Clean(bundle) != bundle {
		return nil, rootfspkg.DirectoryIdentity{}, fmt.Errorf("%w: bundle must be an absolute canonical path", errdefs.ErrInvalidArgument)
	}
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		return nil, rootfspkg.DirectoryIdentity{}, fmt.Errorf("%w: open bundle identity: %v", errdefs.ErrInvalidArgument, err)
	}
	identity := directory.Identity()
	observed := rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}
	expected, err := supervisorBundleIdentity()
	if err != nil || expected != nil && *expected != observed {
		_ = directory.Close()
		return nil, rootfspkg.DirectoryIdentity{}, errors.Join(errors.New("bundle identity differs from the supervisor handoff"), err)
	}
	return directory, observed, nil
}

func supervisorBundleIdentity() (*rootfspkg.DirectoryIdentity, error) {
	raw := []string{os.Getenv("MK_SHIM_BUNDLE_DEVICE"), os.Getenv("MK_SHIM_BUNDLE_INODE"), os.Getenv("MK_SHIM_BUNDLE_UID")}
	if raw[0] == "" && raw[1] == "" && raw[2] == "" {
		return nil, nil
	}
	if raw[0] == "" || raw[1] == "" || raw[2] == "" {
		return nil, errors.New("supervisor bundle identity is incomplete")
	}
	device, err := strconv.ParseUint(raw[0], 10, 64)
	if err != nil {
		return nil, err
	}
	inode, err := strconv.ParseUint(raw[1], 10, 64)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(raw[2], 10, 32)
	if err != nil || device == 0 || inode == 0 {
		return nil, errors.Join(errors.New("supervisor bundle identity is invalid"), err)
	}
	identity := &rootfspkg.DirectoryIdentity{Device: device, Inode: inode, UID: uint32(uid)}
	return identity, nil
}

func invocationFlagValue(arguments []string, name string) (string, error) {
	var value string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		matched := false
		var candidate string
		for _, prefix := range []string{"-" + name, "--" + name} {
			if argument == prefix {
				if index+1 >= len(arguments) {
					return "", fmt.Errorf("%s flag has no value", name)
				}
				index++
				candidate, matched = arguments[index], true
				break
			}
			if strings.HasPrefix(argument, prefix+"=") {
				candidate, matched = strings.TrimPrefix(argument, prefix+"="), true
				break
			}
		}
		if !matched {
			continue
		}
		if candidate == "" || value != "" {
			return "", fmt.Errorf("%s flag is empty or duplicated", name)
		}
		value = candidate
	}
	if value == "" {
		return "", fmt.Errorf("%s flag is unavailable", name)
	}
	return value, nil
}

func (s *service) cleanupShimSocket(ctx context.Context, arguments []string) error {
	_, present, err := s.bundleDirectory.EntryIdentity("address")
	if err != nil || !present {
		return err
	}
	data, _, err := s.bundleDirectory.ReadRegularIdentity("address", 0644, 4096)
	if err != nil {
		return err
	}
	containerdAddress, err := invocationFlagValue(arguments, "address")
	if err != nil {
		return err
	}
	expected, err := shim.SocketAddress(namespaces.WithNamespace(ctx, s.namespace), containerdAddress, s.id)
	if err != nil {
		return err
	}
	if string(data) != expected {
		return errors.New("shim address file differs from the deterministic task socket")
	}
	path, err := pathnameShimSocket(expected)
	if err != nil {
		return err
	}
	var owner relayPathOwner
	if s.newShimSocketOwner != nil {
		owner, err = s.newShimSocketOwner(path)
	} else {
		owner, err = unixsocket.Capture(path)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return owner.Remove()
}

func (s *service) cleanupInvocationArguments() []string {
	if s.shimArguments != nil {
		return s.shimArguments
	}
	return os.Args[1:]
}

func (s *service) Cleanup(ctx context.Context) (*taskapi.DeleteResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var failures []error
	runtimeDirectory, err := s.ensureRuntimeDirectory()
	if errors.Is(err, os.ErrNotExist) {
		if socketErr := s.cleanupShimSocket(ctx, s.cleanupInvocationArguments()); socketErr != nil {
			failures = append(failures, fmt.Errorf("authenticate and remove shim socket: %w", socketErr))
		}
		now := timestamppb.Now()
		return &taskapi.DeleteResponse{ExitedAt: now}, errors.Join(failures...)
	}
	if err != nil {
		return nil, fmt.Errorf("open shim cleanup ownership: %w", err)
	}
	p, found, _, err := loadPersistedRecoveryFromDirectory(runtimeDirectory, s.namespace, s.id)
	if err != nil {
		return nil, fmt.Errorf("load shim cleanup ownership: %w", err)
	}
	if !found {
		if socketErr := s.cleanupShimSocket(ctx, s.cleanupInvocationArguments()); socketErr != nil {
			failures = append(failures, fmt.Errorf("authenticate and remove shim socket: %w", socketErr))
		}
		now := timestamppb.Now()
		return &taskapi.DeleteResponse{ExitedAt: now}, errors.Join(failures...)
	}
	if p.BundleIdentity != s.bundleIdentity {
		return nil, errors.New("cleanup recovery bundle identity differs from the service handoff")
	}
	var sandboxes []protocol.Sandbox
	if apiErr := s.daemon.Call(ctx, protocol.Request{Version: 1, RequestID: "shim-cleanup-list-" + s.id, Method: "ListSandboxes"}, &sandboxes); apiErr != nil {
		return nil, fmt.Errorf("list sandboxes for shim cleanup: %s", apiErr.Message)
	}
	for _, sandbox := range sandboxes {
		if sandbox.ID != p.ID {
			continue
		}
		if sandbox.Generation != p.Generation || sandbox.Config.BundleIdentity != s.bundleIdentity {
			return nil, errors.New("daemon reports conflicting cleanup ownership for the held bundle identity")
		}
		break
	}
	if err = s.cleanupShimSocket(ctx, s.cleanupInvocationArguments()); err != nil {
		return nil, fmt.Errorf("authenticate and remove shim socket: %w", err)
	}
	s.netEndpoint = p.Network
	if err = s.stopNetwork(); err != nil {
		failures = append(failures, fmt.Errorf("stop recovered network: %w", err))
	}
	if err = s.stopRelay(); err != nil {
		failures = append(failures, fmt.Errorf("stop recovered relay: %w", err))
	}
	if err = s.releaseNetwork(ctx); err != nil {
		failures = append(failures, fmt.Errorf("release recovered network: %w", err))
	}
	if p.ID != "" {
		if _, stopErr := daemon.Mutation(ctx, s.daemon, "StopSandbox", p.ID, p.Generation, "cleanup-stop-"+p.Generation, nil); stopErr != nil && !daemonErrorCode(stopErr, "NOT_FOUND") {
			failures = append(failures, fmt.Errorf("stop recovered sandbox: %w", stopErr))
		} else if _, deleteErr := daemon.Mutation(ctx, s.daemon, "DeleteSandbox", p.ID, p.Generation, "cleanup-delete-"+p.Generation, nil); deleteErr != nil && !daemonErrorCode(deleteErr, "NOT_FOUND") {
			failures = append(failures, fmt.Errorf("delete recovered sandbox: %w", deleteErr))
		} else if p.StorageSHA256 != "" {
			if cleanupErr := s.cleanupRootfs(ctx, rootfspkg.CleanupRequest{Version: rootfspkg.Version, Bundle: s.bundle, BundleIdentity: p.BundleIdentity, TaskIdentity: p.TaskIdentity, StorageSHA256: p.StorageSHA256}); cleanupErr != nil {
				failures = append(failures, fmt.Errorf("cleanup recovered rootfs: %w", cleanupErr))
			}
		}
	}
	if p.Exited.IsZero() {
		p.Exited = time.Now().UTC()
	}
	return &taskapi.DeleteResponse{Pid: p.PID, ExitStatus: p.Exit, ExitedAt: timestamppb.New(p.Exited)}, errors.Join(failures...)
}

func bundleNetworkNamespaceFrom(directory *safefile.Directory) (string, error) {
	identity, present, err := directory.EntryIdentity("config.json")
	if err != nil || !present {
		return "", errors.Join(errors.New("OCI config.json is missing"), err)
	}
	mode := os.FileMode(identity.Mode).Perm()
	if identity.Mode&unix.S_IFMT != unix.S_IFREG || mode&0022 != 0 || identity.Size > 1<<20 || identity.Size < 0 ||
		identity.UID != uint32(os.Geteuid()) || identity.Links != 1 {
		return "", errors.New("OCI config.json must be a bounded private caller-owned single-link regular file")
	}
	data, opened, err := directory.ReadRegularIdentity("config.json", mode, 1<<20)
	if err != nil {
		return "", err
	}
	if !safefile.SameObject(identity, opened) {
		return "", errors.New("OCI config.json identity changed while reading")
	}
	var spec specs.Spec
	if err = protocol.StrictDecode(data, &spec); err != nil {
		return "", fmt.Errorf("decode OCI config for network ownership: %w", err)
	}
	if spec.Linux == nil {
		return "", errors.New("OCI Linux configuration is required")
	}
	var result string
	count := 0
	for _, namespace := range spec.Linux.Namespaces {
		if namespace.Type != specs.NetworkNamespace {
			continue
		}
		count++
		if count > 1 {
			return "", errors.New("OCI config contains multiple network namespaces")
		}
		result = namespace.Path
	}
	if count != 1 {
		return "", errors.New("OCI config must contain exactly one network namespace")
	}
	if result != "" && (!filepath.IsAbs(result) || filepath.Clean(result) != result) {
		return "", errors.New("OCI network namespace path must be absolute and canonical")
	}
	return result, nil
}

func bundleNetworkNamespace(bundle string) (string, error) {
	directory, err := safefile.OpenOwnedDirectory(bundle)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	return bundleNetworkNamespaceFrom(directory)
}

func (s *service) ensureBundleIdentity() error {
	current, err := safefile.OpenOwnedDirectory(s.bundle)
	if err != nil {
		return err
	}
	identity := current.Identity()
	observed := rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}
	if s.bundleDirectory == nil {
		s.bundleDirectory, s.bundleIdentity = current, observed
		return nil
	}
	defer current.Close()
	if observed != s.bundleIdentity {
		return errors.New("bundle directory identity changed after shim construction")
	}
	return nil
}

func (s *service) ensureRuntimeDirectory() (*safefile.Directory, error) {
	if err := s.ensureBundleIdentity(); err != nil {
		return nil, err
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	current, err := s.bundleDirectory.OpenChildDirectory(".multikernel", 0700)
	if err != nil {
		return nil, err
	}
	if s.runtimeDirectory == nil {
		s.runtimeDirectory = current
		return current, nil
	}
	defer current.Close()
	if !safefile.SameObject(current.Identity(), s.runtimeDirectory.Identity()) {
		return nil, errors.New("runtime directory identity changed after shim binding")
	}
	return s.runtimeDirectory, nil
}

func verifyRuntimeDirectoryHandoff(directory *safefile.Directory, expected rootfspkg.DirectoryIdentity) error {
	identity := directory.Identity()
	observed := rootfspkg.DirectoryIdentity{Device: identity.Device, Inode: identity.Inode, UID: identity.UID}
	if observed != expected {
		return errors.New("runtime directory identity differs from the rootfs handoff")
	}
	return nil
}

func (s *service) closeBundleIdentity() {
	s.bundleClose.Do(func() {
		if s.runtimeDirectory != nil {
			_ = s.runtimeDirectory.Close()
		}
		if s.bundleDirectory != nil {
			_ = s.bundleDirectory.Close()
		}
	})
}
