//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/runtime/v2/shim"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

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
	terminal := base()
	terminal.Terminal = true
	terminal.ConsoleSize = &specs.Box{Width: 91, Height: 37}
	if err := validateExecProcess(terminal); err != nil {
		t.Fatalf("terminal console size rejected: %v", err)
	}
	projected = processSpec(terminal)
	if projected.ConsoleSize == nil || projected.ConsoleSize.Width != 91 || projected.ConsoleSize.Height != 37 {
		t.Fatalf("terminal console size was not projected: %+v", projected.ConsoleSize)
	}
	zero := 0
	docker := base()
	docker.ApparmorProfile = "unconfined"
	docker.OOMScoreAdj = &zero
	if err := validateExecProcess(docker); err != nil {
		t.Fatalf("Docker no-op exec controls rejected: %v", err)
	}
	projected = processSpec(docker)
	if projected.NoNewPrivileges != nil || projected.Rlimits != nil || projected.Capabilities != nil {
		t.Fatalf("Docker no-op exec controls changed guest projection: %+v", projected)
	}
	nonzero := 1
	umask := uint32(0o22)
	for _, test := range []struct {
		name   string
		mutate func(*specs.Process)
	}{
		{"console-size-without-terminal", func(p *specs.Process) { p.ConsoleSize = &specs.Box{Width: 91, Height: 37} }},
		{"console-size-overflow", func(p *specs.Process) { p.Terminal = true; p.ConsoleSize = &specs.Box{Width: 65536, Height: 37} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := base()
			test.mutate(process)
			if err := validateExecProcess(process); !errors.Is(err, errdefs.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
		})
	}
	tests := []struct {
		name   string
		mutate func(*specs.Process)
	}{
		{"command-line", func(p *specs.Process) { p.CommandLine = "true" }},
		{"apparmor", func(p *specs.Process) { p.ApparmorProfile = "profile" }},
		{"oom-score", func(p *specs.Process) { p.OOMScoreAdj = &nonzero }},
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
	bundle := privateTestDirectory(t)
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

func TestDeleteInvocationRequiresTerminalExactAction(t *testing.T) {
	for _, arguments := range [][]string{{"-namespace", "default", "delete"}, {"delete"}} {
		if !deleteInvocation(arguments) {
			t.Fatalf("delete invocation rejected: %v", arguments)
		}
	}
	for _, arguments := range [][]string{nil, {"start"}, {"delete", "extra"}, {"-id", "delete"}} {
		if deleteInvocation(arguments) {
			t.Fatalf("non-delete invocation accepted: %v", arguments)
		}
	}
}
