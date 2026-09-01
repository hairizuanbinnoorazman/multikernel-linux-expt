package kerf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

type Backend interface {
	EnsurePool(context.Context) error
	Observe(context.Context, string) (string, error)
	Create(context.Context, protocol.Sandbox) error
	Load(context.Context, protocol.Sandbox, string, string, string) error
	Start(context.Context, protocol.Sandbox) error
	Stop(context.Context, protocol.Sandbox) error
	Delete(context.Context, protocol.Sandbox) error
	ReleasePool(context.Context) error
}
type CLI struct {
	Path       string
	Sysfs      string
	PoolCPUs   []int
	PoolMemory string
	Timeout    time.Duration
	Kernel     string
	Initrd     string
	Cmdline    string
}

func (c *CLI) run(ctx context.Context, args ...string) error {
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	x, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(x, c.Path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	b, e := cmd.CombinedOutput()
	if x.Err() != nil {
		return fmt.Errorf("kerf timeout: %w", x.Err())
	}
	if e != nil {
		return fmt.Errorf("kerf %s: %w: %s", args[0], e, strings.TrimSpace(string(b)))
	}
	return nil
}
func ints(v []int) string {
	p := make([]string, len(v))
	for i, n := range v {
		p[i] = strconv.Itoa(n)
	}
	return strings.Join(p, ",")
}
func (c *CLI) EnsurePool(ctx context.Context) error {
	return c.run(ctx, "init", "--cpus="+ints(c.PoolCPUs), "--memory="+c.PoolMemory, "--devices=none", "--verbose")
}
func (c *CLI) Observe(_ context.Context, id string) (string, error) {
	b, e := os.ReadFile(filepath.Join(c.Sysfs, "instances", id, "status"))
	if errors.Is(e, os.ErrNotExist) {
		return "ABSENT", nil
	}
	if e != nil {
		return "", e
	}
	switch strings.TrimSpace(string(b)) {
	case "active":
		return "RUNNING", nil
	case "loaded":
		return "LOADED", nil
	default:
		return "CREATED", nil
	}
}
func (c *CLI) Create(ctx context.Context, s protocol.Sandbox) error {
	err := c.run(ctx, "create", s.ID, "--id="+strconv.FormatUint(uint64(s.Config.ChildCID), 10), "--cpus="+ints(s.Config.CPUs), "--memory="+fmt.Sprintf("%d", s.Config.MemoryBytes), "--verbose")
	if err == nil {
		return nil
	}
	// Kerf v0.2.0 can commit create and then raise KeyError while refreshing
	// its in-memory view. Accept only a positive observation of this exact ID.
	observed, observeErr := c.Observe(ctx, s.ID)
	if observeErr == nil && observed != "ABSENT" {
		return nil
	}
	return err
}
func (c *CLI) Load(ctx context.Context, s protocol.Sandbox, kernel, initrd, cmdline string) error {
	runtimeDir := filepath.Join(s.Config.Bundle, ".multikernel")
	if b, err := os.ReadFile(filepath.Join(runtimeDir, "initramfs.path")); err == nil {
		candidate := strings.TrimSpace(string(b))
		if !filepath.IsAbs(candidate) {
			return errors.New("runtime initramfs path must be absolute")
		}
		initrd = candidate
	}
	if b, err := os.ReadFile(filepath.Join(runtimeDir, "token")); err == nil {
		token := strings.TrimSpace(string(b))
		if len(token) != 64 {
			return errors.New("invalid runtime agent token")
		}
		cmdline += " mk.sandbox_id=" + s.ID + " mk.generation=" + s.Generation +
			" mk.token=" + token + " mk.agent_port=" + strconv.FormatUint(uint64(s.Config.AgentPort), 10)
	}
	return c.run(ctx, "load", s.ID, "--kernel="+kernel, "--initrd="+initrd, "--cmdline="+cmdline, "--verbose")
}
func (c *CLI) Start(ctx context.Context, s protocol.Sandbox) error {
	return c.run(ctx, "exec", s.ID, "--verbose")
}
func (c *CLI) Stop(ctx context.Context, s protocol.Sandbox) error {
	return c.run(ctx, "kill", s.ID, "--force", "--verbose")
}
func (c *CLI) Delete(ctx context.Context, s protocol.Sandbox) error {
	state, e := c.Observe(ctx, s.ID)
	if e != nil {
		return e
	}
	if state == "LOADED" {
		if e = c.run(ctx, "unload", s.ID, "--verbose"); e != nil {
			return e
		}
	}
	if state != "ABSENT" {
		return c.run(ctx, "delete", s.ID, "--verbose")
	}
	return nil
}
func (c *CLI) ReleasePool(ctx context.Context) error {
	return c.run(ctx, "init", "--cpus=none", "--memory=none", "--devices=none", "--verbose")
}
