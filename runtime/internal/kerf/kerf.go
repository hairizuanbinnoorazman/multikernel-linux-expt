package kerf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
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
type InventoryBackend interface {
	ListInstances(context.Context) ([]string, error)
}
type CLI struct {
	Path       string
	Sysfs      string
	PoolCPUs   []int
	PoolMemory string
	Timeout    time.Duration
	MaxOutput  int
	Kernel     string
	Initrd     string
	Cmdline    string
}

func (c *CLI) run(ctx context.Context, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maximumOutput := c.MaxOutput
	if maximumOutput <= 0 {
		maximumOutput = 1 << 20
	}
	output, err := boundedexec.Run(ctx, timeout, c.Path, args,
		[]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}, maximumOutput)
	if err != nil {
		digest := sha256.Sum256(output)
		return fmt.Errorf("kerf %s: %w (retained_output_bytes=%d retained_output_sha256=%s)",
			args[0], err, len(output), hex.EncodeToString(digest[:]))
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
func (c *CLI) Observe(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b, e := os.ReadFile(filepath.Join(c.Sysfs, "instances", id, "status"))
	if errors.Is(e, os.ErrNotExist) {
		return "ABSENT", nil
	}
	if e != nil {
		return "", e
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch strings.TrimSpace(string(b)) {
	case "created", "ready":
		return "CREATED", nil
	case "active":
		return "RUNNING", nil
	case "loaded":
		return "LOADED", nil
	default:
		return "", fmt.Errorf("unrecognized Kerf instance status %q", strings.TrimSpace(string(b)))
	}
}
func (c *CLI) ListInstances(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(c.Sysfs, "instances"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	instances := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			instances = append(instances, entry.Name())
		}
	}
	sort.Strings(instances)
	return instances, nil
}
func (c *CLI) Create(ctx context.Context, s protocol.Sandbox) error {
	err := c.run(ctx, "create", s.ID, "--id="+strconv.FormatUint(uint64(s.Config.ChildCID), 10), "--cpus="+ints(s.Config.CPUs), "--memory="+fmt.Sprintf("%d", s.Config.MemoryBytes), "--verbose")
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
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
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if s.Config.Storage != nil {
		if s.Storage == nil || s.Storage.State != "ACTIVE" || len(s.Storage.ExportGeneration) != 32 {
			return errors.New("runtime storage export is not generation-bound and active")
		}
		cmdline += " mk.image_id=" + s.Config.Storage.ImageID +
			" mk.storage_generation=" + s.Storage.ExportGeneration +
			" mk.root_uuid=" + s.Config.Storage.FilesystemUUID +
			" mk.storage_port=" + strconv.FormatUint(uint64(s.Config.Storage.Port), 10) +
			" mk.storage_size=" + strconv.FormatUint(s.Config.Storage.SizeBytes, 10)
	}
	err := c.run(ctx, "load", s.ID, "--kernel="+kernel, "--initrd="+initrd, "--cmdline="+cmdline, "--verbose")
	return c.acceptObserved(ctx, s.ID, "LOADED", err)
}
func (c *CLI) Start(ctx context.Context, s protocol.Sandbox) error {
	err := c.run(ctx, "exec", s.ID, "--verbose")
	return c.acceptObserved(ctx, s.ID, "RUNNING", err)
}
func (c *CLI) Stop(ctx context.Context, s protocol.Sandbox) error {
	err := c.run(ctx, "kill", s.ID, "--force", "--verbose")
	return c.acceptObserved(ctx, s.ID, "LOADED", err)
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
		err := c.run(ctx, "delete", s.ID, "--verbose")
		return c.acceptObserved(ctx, s.ID, "ABSENT", err)
	}
	return nil
}

func (c *CLI) acceptObserved(ctx context.Context, id, expected string, commandErr error) error {
	if commandErr == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	observed, observeErr := c.Observe(ctx, id)
	if observeErr == nil && observed == expected {
		return nil
	}
	return commandErr
}
func (c *CLI) ReleasePool(ctx context.Context) error {
	return c.run(ctx, "init", "--cpus=none", "--memory=none", "--devices=none", "--verbose")
}
