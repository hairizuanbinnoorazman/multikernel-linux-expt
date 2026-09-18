package kerf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

type Backend interface {
	EnsurePool(context.Context) error
	Observe(context.Context, string) (string, error)
	Create(context.Context, protocol.Sandbox) error
	Load(context.Context, protocol.Sandbox, string, string, string, LoadFiles) error
	Start(context.Context, protocol.Sandbox) error
	Stop(context.Context, protocol.Sandbox) error
	Delete(context.Context, protocol.Sandbox) error
	ReleasePool(context.Context) error
}
type LoadFiles struct {
	RuntimeDir *os.File
	Initramfs  *os.File
	Kernel     *os.File
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
	return c.runWithFiles(ctx, nil, args...)
}

func (c *CLI) runWithFiles(ctx context.Context, files []*os.File, args ...string) error {
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
	output, err := boundedexec.RunWithFiles(ctx, timeout, c.Path, args,
		[]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}, maximumOutput, files)
	if err != nil {
		digest := sha256.Sum256(output)
		return fmt.Errorf("kerf %s: %w (retained_output_bytes=%d retained_output_sha256=%s)",
			args[0], err, len(output), hex.EncodeToString(digest[:]))
	}
	return nil
}

func sameArtifactIdentity(left, right *unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino && left.Size == right.Size &&
		left.Mtim == right.Mtim && left.Ctim == right.Ctim
}

func openRuntimeFile(directory *os.File, name string, maximum int64) (*os.File, *unix.Stat_t, error) {
	descriptor, err := unix.Openat2(int(directory.Fd()), name, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(descriptor), name)
	var identity unix.Stat_t
	if err = unix.Fstat(descriptor, &identity); err != nil || identity.Mode&unix.S_IFMT != unix.S_IFREG ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 || identity.Mode&0077 != 0 ||
		identity.Size <= 0 || identity.Size > maximum {
		_ = file.Close()
		return nil, nil, errors.New("runtime artifact must be a bounded private caller-owned single-link regular file")
	}
	return file, &identity, nil
}

func validateRuntimeFile(file *os.File, maximum int64) error {
	if file == nil {
		return errors.New("runtime artifact descriptor is nil")
	}
	var identity unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &identity); err != nil || identity.Mode&unix.S_IFMT != unix.S_IFREG ||
		identity.Uid != uint32(os.Geteuid()) || identity.Nlink != 1 || identity.Mode&0077 != 0 ||
		identity.Size <= 0 || identity.Size > maximum {
		return errors.New("runtime artifact descriptor is unsafe")
	}
	return nil
}

func validateApprovedBootFile(file *os.File, maximum int64) error {
	if file == nil {
		return errors.New("approved boot artifact descriptor is nil")
	}
	var identity unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &identity); err != nil || identity.Mode&unix.S_IFMT != unix.S_IFREG ||
		identity.Nlink != 1 || identity.Mode&0022 != 0 || identity.Size <= 0 || identity.Size > maximum {
		return errors.New("approved boot artifact descriptor is unsafe")
	}
	return nil
}

func validateRuntimeDirectory(directory *os.File) error {
	if directory == nil {
		return errors.New("runtime artifact directory descriptor is nil")
	}
	var identity unix.Stat_t
	if err := unix.Fstat(int(directory.Fd()), &identity); err != nil || identity.Mode&unix.S_IFMT != unix.S_IFDIR ||
		identity.Uid != uint32(os.Geteuid()) || identity.Mode&0077 != 0 {
		return errors.New("runtime artifact directory must be private and caller-owned")
	}
	return nil
}

func readRuntimeFile(directory *os.File, name string, maximum int64) ([]byte, error) {
	file, before, err := openRuntimeFile(directory, name, maximum)
	if err != nil {
		return nil, err
	}
	value, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	var after unix.Stat_t
	statErr := unix.Fstat(int(file.Fd()), &after)
	closeErr := file.Close()
	if err = errors.Join(readErr, statErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(value)) > maximum || !sameArtifactIdentity(before, &after) {
		return nil, errors.New("runtime artifact changed while reading")
	}
	return value, nil
}

func openRuntimeDirectory(bundle string) (*os.File, bool, error) {
	runtimePath := filepath.Join(bundle, ".multikernel")
	descriptor, err := unix.Openat2(unix.AT_FDCWD, runtimePath, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, syscall.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	directory := os.NewFile(uintptr(descriptor), runtimePath)
	if err = validateRuntimeDirectory(directory); err != nil {
		_ = directory.Close()
		return nil, false, err
	}
	return directory, true, nil
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
func (c *CLI) Load(ctx context.Context, s protocol.Sandbox, kernel, initrd, cmdline string, files LoadFiles) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var inherited []*os.File
	var runtimeDir, preparedInitrd *os.File
	var present bool
	var err error
	if files.RuntimeDir != nil {
		runtimeDir, preparedInitrd, present = files.RuntimeDir, files.Initramfs, true
		if err = validateRuntimeDirectory(runtimeDir); err != nil {
			return err
		}
		if err = validateRuntimeFile(preparedInitrd, 16<<30); err != nil {
			return err
		}
	} else if files.Initramfs != nil {
		if err = validateApprovedBootFile(files.Initramfs, 16<<30); err != nil {
			return err
		}
		inherited = append(inherited, files.Initramfs)
		initrd = fmt.Sprintf("/proc/self/fd/%d", len(inherited)+2)
	} else {
		runtimeDir, present, err = openRuntimeDirectory(s.Config.Bundle)
		if err != nil {
			return fmt.Errorf("open runtime artifact directory: %w", err)
		}
		if present {
			defer runtimeDir.Close()
		}
	}
	if present {
		logicalInitrd := filepath.Join(s.Config.Bundle, ".multikernel", "initramfs.cpio.gz")
		pathValue, pathErr := readRuntimeFile(runtimeDir, "initramfs.path", 1<<20)
		if pathErr != nil || string(pathValue) != logicalInitrd+"\n" {
			return errors.New("runtime initramfs path is missing, unsafe, or noncanonical")
		}
		initrdFile := preparedInitrd
		if initrdFile == nil {
			var openErr error
			initrdFile, _, openErr = openRuntimeFile(runtimeDir, "initramfs.cpio.gz", 16<<30)
			if openErr != nil {
				return fmt.Errorf("open runtime initramfs: %w", openErr)
			}
			defer initrdFile.Close()
		}
		inherited = append(inherited, initrdFile)
		initrd = "/proc/self/fd/3"

		tokenValue, tokenErr := readRuntimeFile(runtimeDir, "token", 65)
		if tokenErr == nil {
			token := strings.TrimSpace(string(tokenValue))
			decoded, decodeErr := hex.DecodeString(token)
			if decodeErr != nil || len(decoded) != 32 || len(tokenValue) != 65 || tokenValue[64] != '\n' {
				return errors.New("invalid runtime agent token")
			}
			cmdline += " mk.sandbox_id=" + s.ID + " mk.generation=" + s.Generation +
				" mk.token=" + token + " mk.agent_port=" + strconv.FormatUint(uint64(s.Config.AgentPort), 10)
		} else if !errors.Is(tokenErr, syscall.ENOENT) {
			return fmt.Errorf("read runtime agent token: %w", tokenErr)
		}
	}
	if files.Kernel != nil {
		if err = validateApprovedBootFile(files.Kernel, 16<<30); err != nil {
			return err
		}
		inherited = append(inherited, files.Kernel)
		kernel = fmt.Sprintf("/proc/self/fd/%d", len(inherited)+2)
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
	err = c.runWithFiles(ctx, inherited, "load", s.ID, "--kernel="+kernel, "--initrd="+initrd, "--cmdline="+cmdline, "--verbose")
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
