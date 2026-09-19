//go:build linux

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/boundedexec"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/safefile"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
)

const ext4SuperblockOffset = 1024
const ext4SuperblockSize = 1024

type managedExport struct {
	command *exec.Cmd
	done    chan error
}

type processRecord struct {
	Version          int    `json:"version"`
	PID              int    `json:"pid"`
	StartTime        uint64 `json:"start_time"`
	Path             string `json:"path"`
	Port             uint32 `json:"port"`
	ImageID          string `json:"image_id"`
	ExportGeneration string `json:"export_generation"`
	ImageDevice      uint64 `json:"image_device"`
	ImageInode       uint64 `json:"image_inode"`
	BinaryDevice     uint64 `json:"binary_device"`
	BinaryInode      uint64 `json:"binary_inode"`
}

type openedPreparedImage struct {
	file      *os.File
	directory *safefile.Directory
	parent    string
	name      string
	identity  safefile.Identity
}

type openedExecutable struct {
	file      *os.File
	directory *safefile.Directory
	parent    string
	name      string
	identity  safefile.Identity
}

func (o *openedExecutable) Close() error {
	return errors.Join(o.file.Close(), o.directory.Close())
}

func (o *openedExecutable) verifyNamedIdentity() error {
	named, found, err := o.directory.EntryIdentity(o.name)
	if err != nil || !found || named != o.identity {
		return errors.New("storage server executable pathname changed while in use")
	}
	current, err := safefile.OpenOwnedDirectory(o.parent)
	if err != nil {
		return errors.New("storage server executable parent changed while in use")
	}
	defer current.Close()
	if current.Identity() != o.directory.Identity() {
		return errors.New("storage server executable parent changed while in use")
	}
	return nil
}

func (o *openedPreparedImage) Close() error {
	return errors.Join(o.file.Close(), o.directory.Close())
}

type LinuxBackend struct {
	Binary          string
	CheckBinary     string
	RuntimeDir      string
	RequiredUID     int
	ReadyTimeout    time.Duration
	StopTimeout     time.Duration
	CheckTimeout    time.Duration
	afterImageOpen  func()
	afterBinaryOpen func()

	mu      sync.Mutex
	managed map[string]*managedExport
	runtime safefile.Identity
}

func (b *LinuxBackend) defaults() {
	if b.Binary == "" {
		b.Binary = "/usr/local/libexec/multikernel/mkvsock-nbd"
	}
	if b.CheckBinary == "" {
		b.CheckBinary = "/usr/sbin/e2fsck"
	}
	if b.RuntimeDir == "" {
		b.RuntimeDir = "/run/mkstorage"
	}
	if b.ReadyTimeout == 0 {
		b.ReadyTimeout = 10 * time.Second
	}
	if b.StopTimeout == 0 {
		b.StopTimeout = 20 * time.Second
	}
	if b.CheckTimeout == 0 {
		b.CheckTimeout = 5 * time.Minute
	}
	if b.managed == nil {
		b.managed = map[string]*managedExport{}
	}
}

func (b *LinuxBackend) paths(value Export) (record, log string) {
	name := value.SandboxID + "-" + value.SandboxGeneration[:12]
	return filepath.Join(b.RuntimeDir, name+".json"), filepath.Join(b.RuntimeDir, name+".log")
}

func (b *LinuxBackend) openRuntimeDirectoryLocked(create bool) (*safefile.Directory, error) {
	directory, err := safefile.OpenDirectory(b.RuntimeDir, create)
	if err != nil {
		return nil, err
	}
	identity := directory.Identity()
	if b.runtime != (safefile.Identity{}) && identity != b.runtime {
		_ = directory.Close()
		return nil, errors.New("storage runtime directory identity changed")
	}
	b.runtime = identity
	return directory, nil
}

func validateBackendLease(value Export) error {
	if !identityRE.MatchString(value.SandboxID) || !generationRE.MatchString(value.SandboxGeneration) ||
		!generationRE.MatchString(value.ExportGeneration) {
		return errors.New("storage backend lease identity is invalid")
	}
	return validatePrepared(value.PreparedImage)
}

func readPrivateRuntimeFileAt(directory *safefile.Directory, name string, limit int64, stable bool) ([]byte, error) {
	if stable {
		data, found, err := directory.ReadPrivate(name, limit)
		if err != nil || !found {
			if err == nil {
				err = os.ErrNotExist
			}
			return nil, err
		}
		return data, nil
	}
	data, found, err := directory.ReadPrivateSnapshot(name, limit)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return nil, err
	}
	return data, nil
}

func readyMarker(value Export) []byte {
	return []byte(fmt.Sprintf("MKNBD_SERVER_READY image=%s image_id=%s generation=%s size=%d port=%d\n",
		"/proc/self/fd/3", value.ImageID, value.ExportGeneration, value.SizeBytes, value.Port))
}

func ext4UUID(value []byte) string {
	parts := []string{
		hex.EncodeToString(value[0:4]), hex.EncodeToString(value[4:6]),
		hex.EncodeToString(value[6:8]), hex.EncodeToString(value[8:10]),
		hex.EncodeToString(value[10:16]),
	}
	return strings.Join(parts, "-")
}

func inspectExt4(descriptor int, expected PreparedImage) error {
	data := make([]byte, ext4SuperblockSize)
	if n, err := unix.Pread(descriptor, data, ext4SuperblockOffset); err != nil || n != len(data) {
		return errors.New("cannot read complete ext4 superblock")
	}
	if binary.LittleEndian.Uint16(data[0x38:0x3a]) != 0xef53 {
		return errors.New("filesystem is not ext4")
	}
	if binary.LittleEndian.Uint16(data[0x3a:0x3c])&1 == 0 {
		return errors.New("ext4 filesystem is not marked clean")
	}
	if ext4UUID(data[0x68:0x78]) != expected.FilesystemUUID {
		return errors.New("ext4 UUID differs from prepared identity")
	}
	inodes := uint64(binary.LittleEndian.Uint32(data[0:4]))
	if inodes != expected.InodeLimit {
		return fmt.Errorf("ext4 inode capacity %d differs from enforced limit %d", inodes, expected.InodeLimit)
	}
	blockSize := uint64(1024) << binary.LittleEndian.Uint32(data[0x18:0x1c])
	blocks := uint64(binary.LittleEndian.Uint32(data[4:8]))
	if binary.LittleEndian.Uint32(data[0x60:0x64])&0x80 != 0 {
		blocks |= uint64(binary.LittleEndian.Uint32(data[0x150:0x154])) << 32
	}
	if blocks == 0 || blockSize == 0 || blocks > ^uint64(0)/blockSize || blocks*blockSize > expected.SizeBytes || expected.SizeBytes-blocks*blockSize >= blockSize {
		return errors.New("ext4 block capacity differs from prepared image size")
	}
	return nil
}

func (b *LinuxBackend) openPreparedImage(image PreparedImage, writable bool) (*openedPreparedImage, error) {
	directory, err := safefile.OpenOwnedDirectory(filepath.Dir(image.Path))
	if err != nil {
		return nil, err
	}
	file, identity, err := directory.OpenPrivateFile(filepath.Base(image.Path), 16<<30, writable)
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	if identity.UID != uint32(b.RequiredUID) || identity.Size < 0 || uint64(identity.Size) != image.SizeBytes ||
		uint64(identity.Size) != image.QuotaBytes {
		_ = file.Close()
		_ = directory.Close()
		return nil, errors.New("storage image ownership, size, or quota differs from its prepared identity")
	}
	var allocated unix.Stat_t
	if err = unix.Fstat(int(file.Fd()), &allocated); err != nil || uint64(allocated.Blocks)*512 < image.SizeBytes {
		_ = file.Close()
		_ = directory.Close()
		return nil, errors.Join(errors.New("storage image is sparse or allocation cannot be inspected"), err)
	}
	opened := &openedPreparedImage{file: file, directory: directory, parent: filepath.Dir(image.Path),
		name: filepath.Base(image.Path), identity: identity}
	if b.afterImageOpen != nil {
		b.afterImageOpen()
	}
	return opened, nil
}

func openExecutable(path string) (*openedExecutable, error) {
	directory, err := safefile.OpenOwnedDirectory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	file, identity, err := directory.OpenTrustedExecutable(filepath.Base(path), 256<<20)
	if err != nil {
		_ = directory.Close()
		return nil, err
	}
	return &openedExecutable{file: file, directory: directory, parent: filepath.Dir(path),
		name: filepath.Base(path), identity: identity}, nil
}

func (b *LinuxBackend) openServerBinary() (*openedExecutable, error) {
	opened, err := openExecutable(b.Binary)
	if err != nil {
		return nil, err
	}
	if b.afterBinaryOpen != nil {
		b.afterBinaryOpen()
	}
	return opened, nil
}

func (o *openedPreparedImage) verifyNamedIdentity() error {
	named, found, err := o.directory.EntryIdentity(o.name)
	if err != nil || !found || named != o.identity {
		return errors.New("storage image pathname changed while in use")
	}
	current, err := safefile.OpenOwnedDirectory(o.parent)
	if err != nil {
		return errors.New("storage image parent changed while in use")
	}
	defer current.Close()
	if current.Identity() != o.directory.Identity() {
		return errors.New("storage image parent changed while in use")
	}
	return nil
}

func lockOpened(opened *openedPreparedImage) error {
	if err := unix.Flock(int(opened.file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("storage image already has an owner")
	}
	return nil
}

// inspectOpened validates an image while its caller retains an exclusive lock.
// Start deliberately transfers that locked open file description to fd 3 of
// the server; Inspect and OfflineCheck release it by closing their descriptor.
func (b *LinuxBackend) inspectOpened(ctx context.Context, image PreparedImage, opened *openedPreparedImage) error {
	descriptor := int(opened.file.Fd())
	if err := inspectExt4(descriptor, image); err != nil {
		return err
	}
	hash := sha256.New()
	buffer := make([]byte, 4<<20)
	for offset := int64(0); offset < opened.identity.Size; {
		if err := ctx.Err(); err != nil {
			return err
		}
		length := len(buffer)
		if remaining := opened.identity.Size - offset; remaining < int64(length) {
			length = int(remaining)
		}
		n, readErr := unix.Pread(descriptor, buffer[:length], offset)
		if readErr != nil || n != length {
			return errors.New("storage image changed or failed while hashing")
		}
		_, _ = hash.Write(buffer[:n])
		offset += int64(n)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	after, err := safefile.InspectOpened(opened.file, 16<<30)
	if err != nil || after != opened.identity {
		return errors.New("storage image mutated during inspection")
	}
	if err = opened.verifyNamedIdentity(); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != image.SHA256 {
		return errors.New("storage image digest differs from prepared identity")
	}
	return nil
}

func (b *LinuxBackend) Inspect(ctx context.Context, image PreparedImage) (ImageIdentity, error) {
	if err := ctx.Err(); err != nil {
		return ImageIdentity{}, err
	}
	if err := validatePrepared(image); err != nil {
		return ImageIdentity{}, err
	}
	b.mu.Lock()
	b.defaults()
	b.mu.Unlock()
	opened, err := b.openPreparedImage(image, true)
	if err != nil {
		return ImageIdentity{}, err
	}
	defer opened.Close()
	if err = lockOpened(opened); err != nil {
		return ImageIdentity{}, err
	}
	if err = b.inspectOpened(ctx, image, opened); err != nil {
		return ImageIdentity{}, err
	}
	return ImageIdentity{Device: opened.identity.Device, Inode: opened.identity.Inode}, nil
}

func parseProcessStartTime(data []byte) (uint64, error) {
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return 0, errors.New("malformed process stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return 0, errors.New("short process stat")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

func processStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	return parseProcessStartTime(data)
}

func atomicRecordAt(directory *safefile.Directory, name string, value processRecord) (safefile.Identity, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return safefile.Identity{}, err
	}
	created, identity, err := directory.PublishExclusiveIdentity(name, append(data, '\n'), 0600)
	if err != nil {
		return safefile.Identity{}, err
	}
	if !created {
		return safefile.Identity{}, errors.New("storage process record already exists")
	}
	return identity, nil
}

func (b *LinuxBackend) Start(ctx context.Context, value Export) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateBackendLease(value); err != nil {
		return err
	}
	b.mu.Lock()
	b.defaults()
	b.mu.Unlock()
	opened, err := b.openPreparedImage(value.PreparedImage, true)
	if err != nil {
		return err
	}
	defer opened.Close()
	if err = lockOpened(opened); err != nil {
		return err
	}
	if err = b.inspectOpened(ctx, value.PreparedImage, opened); err != nil {
		return err
	}
	if value.ImageIdentity != (ImageIdentity{Device: opened.identity.Device, Inode: opened.identity.Inode}) {
		return errors.New("storage image identity changed before server start")
	}
	server, err := b.openServerBinary()
	if err != nil {
		return fmt.Errorf("open exact storage server executable: %w", err)
	}
	defer server.Close()
	b.mu.Lock()
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		return err
	}
	directory, err := b.openRuntimeDirectoryLocked(true)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	defer directory.Close()
	recordPath, logPath := b.paths(value)
	recordName, logName := filepath.Base(recordPath), filepath.Base(logPath)
	if _, found, _, _, inspectErr := directory.ReadPrivateOrQuarantineIdentity(recordName, 4096); inspectErr != nil {
		b.mu.Unlock()
		return inspectErr
	} else if found {
		b.mu.Unlock()
		return errors.New("storage process record already exists")
	}
	if _, found, _, staleLog, inspectErr := directory.ReadPrivateOrQuarantineIdentity(logName, 1<<20); inspectErr != nil {
		b.mu.Unlock()
		return inspectErr
	} else if found {
		if _, err = directory.RemoveIfIdentity(logName, staleLog); err != nil {
			b.mu.Unlock()
			return err
		}
	}
	log, logIdentity, err := directory.OpenAppend(logName, 1<<20, 0600)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	command := &exec.Cmd{Path: "/proc/self/fd/4", Args: []string{b.Binary, "server", "/proc/self/fd/3",
		strconv.Itoa(int(value.Port)), value.ImageID, value.ExportGeneration}}
	command.Stdout, command.Stderr = log, log
	command.ExtraFiles = []*os.File{opened.file, server.file}
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	// The export outlives mkruntimed so a daemon restart cannot sever a live
	// child's root disk. Reconciliation adopts it only when PID start time,
	// argv, path, port, image ID, and export generation all match the durable
	// lease; teardown signals that exact process group.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = ctx.Err(); err != nil {
		_ = log.Close()
		_, _ = directory.RemoveIfIdentity(logName, logIdentity)
		b.mu.Unlock()
		return err
	}
	if err = command.Start(); err != nil {
		_ = log.Close()
		_, _ = directory.RemoveIfIdentity(logName, logIdentity)
		b.mu.Unlock()
		return err
	}
	if err = server.verifyNamedIdentity(); err != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
		_ = command.Wait()
		_ = log.Close()
		_, _ = directory.RemoveIfIdentity(logName, logIdentity)
		b.mu.Unlock()
		return err
	}
	startTime, err := processStartTime(command.Process.Pid)
	if err != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
		_ = command.Wait()
		_ = log.Close()
		_, _ = directory.RemoveIfIdentity(logName, logIdentity)
		b.mu.Unlock()
		return err
	}
	record := processRecord{Version: 3, PID: command.Process.Pid, StartTime: startTime, Path: value.Path,
		Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration,
		ImageDevice: opened.identity.Device, ImageInode: opened.identity.Inode,
		BinaryDevice: server.identity.Device, BinaryInode: server.identity.Inode}
	recordIdentity, err := atomicRecordAt(directory, recordName, record)
	if err != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
		_ = command.Wait()
		_ = log.Close()
		_, _ = directory.RemoveIfIdentity(logName, logIdentity)
		b.mu.Unlock()
		return err
	}
	done := make(chan error, 1)
	b.managed[recordPath] = &managedExport{command: command, done: done}
	b.mu.Unlock()
	go func() {
		done <- command.Wait()
		_ = log.Close()
	}()

	deadline := time.NewTimer(b.ReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err = <-done:
			_, recordErr := directory.RemoveIfIdentity(recordName, recordIdentity)
			_, logErr := directory.RemoveIfIdentity(logName, logIdentity)
			b.mu.Lock()
			delete(b.managed, recordPath)
			b.mu.Unlock()
			return errors.Join(fmt.Errorf("storage server exited before ready: %w", err), recordErr, logErr)
		case <-ctx.Done():
			_ = command.Process.Signal(syscall.SIGTERM)
			return ctx.Err()
		case <-deadline.C:
			_ = command.Process.Signal(syscall.SIGTERM)
			return errors.New("storage server readiness timeout")
		case <-ticker.C:
			data, readErr := readPrivateRuntimeFileAt(directory, logName, 1<<20, false)
			if readErr != nil {
				_ = command.Process.Signal(syscall.SIGTERM)
				return fmt.Errorf("read storage readiness evidence: %w", readErr)
			}
			if bytes.Contains(data, readyMarker(value)) {
				return nil
			}
		}
	}
}

func readRecord(path string, expected Export) (processRecord, error) {
	directory, err := safefile.OpenDirectory(filepath.Dir(path), false)
	if err != nil {
		return processRecord{}, err
	}
	defer directory.Close()
	return readRecordAt(directory, filepath.Base(path), expected)
}

func readRecordAt(directory *safefile.Directory, name string, expected Export) (processRecord, error) {
	value, _, err := readRecordIdentityAt(directory, name, expected)
	return value, err
}

func readRecordIdentityAt(directory *safefile.Directory, name string, expected Export) (processRecord, safefile.Identity, error) {
	var value processRecord
	data, found, _, identity, err := directory.ReadPrivateOrQuarantineIdentity(name, 4096)
	if err != nil || !found {
		if err == nil {
			err = os.ErrNotExist
		}
		return value, safefile.Identity{}, err
	}
	if err = protocol.StrictDecode(data, &value); err != nil || value.Version != 3 {
		return value, safefile.Identity{}, errors.New("storage process record is malformed")
	}
	if value.PID <= 1 || value.StartTime == 0 || value.Path != expected.Path || value.Port != expected.Port ||
		value.ImageID != expected.ImageID || value.ExportGeneration != expected.ExportGeneration ||
		value.ImageDevice != expected.ImageIdentity.Device || value.ImageInode != expected.ImageIdentity.Inode ||
		value.BinaryDevice == 0 || value.BinaryInode == 0 {
		return value, safefile.Identity{}, errors.New("storage process record differs from its exact export lease")
	}
	return value, identity, nil
}

type processHandle struct {
	pidfd int
	proc  *os.File
}

func (h *processHandle) Close() error {
	return errors.Join(unix.Close(h.pidfd), h.proc.Close())
}

func readProcessFileAt(directory *os.File, name string, limit int64) ([]byte, error) {
	descriptor, err := unix.Openat(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), name)
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("process metadata exceeds limit")
	}
	return data, nil
}

func openProcessHandle(record processRecord, binary string) (*processHandle, bool, error) {
	pidfd, err := unix.PidfdOpen(record.PID, 0)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil, false, nil
		}
		return nil, false, errors.New("cannot retain exact storage server pidfd")
	}
	procfd, err := unix.Open(filepath.Join("/proc", strconv.Itoa(record.PID)),
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		probeErr := unix.PidfdSendSignal(pidfd, 0, nil, 0)
		_ = unix.Close(pidfd)
		if errors.Is(probeErr, syscall.ESRCH) {
			return nil, false, nil
		}
		return nil, false, errors.New("storage server proc directory is unavailable")
	}
	handle := &processHandle{pidfd: pidfd, proc: os.NewFile(uintptr(procfd), "proc")}
	statData, err := readProcessFileAt(handle.proc, "stat", 4096)
	if err != nil {
		probeErr := unix.PidfdSendSignal(handle.pidfd, 0, nil, 0)
		_ = handle.Close()
		if errors.Is(probeErr, syscall.ESRCH) {
			return nil, false, nil
		}
		return nil, false, errors.New("storage server process stat is unavailable")
	}
	start, err := parseProcessStartTime(statData)
	if err != nil {
		_ = handle.Close()
		return nil, false, errors.New("storage server process stat is malformed")
	}
	if start != record.StartTime {
		_ = handle.Close()
		return nil, false, nil
	}
	data, err := readProcessFileAt(handle.proc, "cmdline", 4096)
	if err != nil {
		_ = handle.Close()
		return nil, false, errors.New("storage server command identity is unavailable")
	}
	want := strings.Join([]string{binary, "server", "/proc/self/fd/3", strconv.Itoa(int(record.Port)), record.ImageID, record.ExportGeneration, ""}, "\x00")
	if string(data) != want {
		_ = handle.Close()
		return nil, false, errors.New("storage server command identity conflicts with its record")
	}
	var image unix.Stat_t
	if err = unix.Fstatat(int(handle.proc.Fd()), "fd/3", &image, 0); err != nil ||
		uint64(image.Dev) != record.ImageDevice || image.Ino != record.ImageInode {
		_ = handle.Close()
		return nil, false, errors.New("storage server image descriptor conflicts with its record")
	}
	var executable unix.Stat_t
	if err = unix.Fstatat(int(handle.proc.Fd()), "exe", &executable, 0); err != nil ||
		uint64(executable.Dev) != record.BinaryDevice || executable.Ino != record.BinaryInode {
		_ = handle.Close()
		return nil, false, errors.New("storage server executable conflicts with its record")
	}
	// The pidfd was opened before the proc directory. If the original process
	// exited during inspection, signal 0 fails; while it remains live, its
	// numeric PID cannot be reused for a different proc directory.
	if err = unix.PidfdSendSignal(handle.pidfd, 0, nil, 0); err != nil {
		_ = handle.Close()
		if errors.Is(err, syscall.ESRCH) {
			return nil, false, nil
		}
		return nil, false, errors.New("storage server pidfd liveness check failed")
	}
	return handle, true, nil
}

func (b *LinuxBackend) Observe(ctx context.Context, value Export) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	if err := validateBackendLease(value); err != nil {
		return Observation{}, err
	}
	b.mu.Lock()
	b.defaults()
	recordPath, logPath := b.paths(value)
	directory, err := b.openRuntimeDirectoryLocked(false)
	b.mu.Unlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Observation{}, nil
		}
		return Observation{}, err
	}
	defer directory.Close()
	recordName, logName := filepath.Base(recordPath), filepath.Base(logPath)
	record, recordIdentity, err := readRecordIdentityAt(directory, recordName, value)
	if errors.Is(err, os.ErrNotExist) {
		// A daemon may restart after the server completed its graceful close but
		// before the QUIESCING lease was finalized. The generation-specific log
		// is the durable counter record for that exact export.
		counters, counterErr := parseCountersAt(directory, logName, value)
		if counterErr == nil {
			return Observation{Closed: true, Counters: counters}, nil
		}
		return Observation{}, nil
	}
	if err != nil {
		return Observation{}, err
	}
	handle, matches, matchErr := openProcessHandle(record, b.Binary)
	if handle != nil {
		_ = handle.Close()
	}
	if matchErr != nil {
		return Observation{}, matchErr
	}
	if !matches {
		if _, err = directory.RemoveIfIdentity(recordName, recordIdentity); err != nil {
			return Observation{}, err
		}
		return Observation{}, nil
	}
	return Observation{Active: true, Generation: record.ExportGeneration}, nil
}

func waitForProcess(ctx context.Context, done <-chan error, timeout time.Duration) (error, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err, true
	case <-ctx.Done():
		return ctx.Err(), false
	case <-timer.C:
		return nil, false
	}
}

func waitForPidfd(ctx context.Context, pidfd int, timeout time.Duration) (error, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err, false
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, false
		}
		wait := min(remaining, 20*time.Millisecond)
		milliseconds := int(wait / time.Millisecond)
		if milliseconds < 1 {
			milliseconds = 1
		}
		fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
		count, err := unix.Poll(fds, milliseconds)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err, false
		}
		if count > 0 && fds[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0 {
			return nil, true
		}
	}
}

func parseCounters(logPath string, value Export) (Counters, error) {
	directory, err := safefile.OpenDirectory(filepath.Dir(logPath), false)
	if err != nil {
		return Counters{}, err
	}
	defer directory.Close()
	return parseCountersAt(directory, filepath.Base(logPath), value)
}

func parseCountersAt(directory *safefile.Directory, name string, value Export) (Counters, error) {
	data, err := readPrivateRuntimeFileAt(directory, name, 1<<20, true)
	if err != nil {
		return Counters{}, err
	}
	ready := bytes.LastIndex(data, readyMarker(value))
	offset := bytes.LastIndex(data, []byte("MKNBD_SERVER_CLOSED"))
	if ready < 0 || offset < ready {
		return Counters{}, errors.New("exact server ready/close evidence is absent or out of order")
	}
	var result Counters
	var readBytes, writeBytes uint64
	count, err := fmt.Sscanf(string(data[offset:]),
		"MKNBD_SERVER_CLOSED reads=%d read_bytes=%d writes=%d write_bytes=%d flushes=%d",
		&result.Reads, &readBytes, &result.Writes, &writeBytes, &result.Flushes)
	if err != nil || count != 5 {
		return Counters{}, errors.New("server close counter record is malformed")
	}
	result.ReadBytes, result.WrittenBytes = readBytes, writeBytes
	canonical := fmt.Sprintf("MKNBD_SERVER_CLOSED reads=%d read_bytes=%d writes=%d write_bytes=%d flushes=%d\n",
		result.Reads, result.ReadBytes, result.Writes, result.WrittenBytes, result.Flushes)
	if string(data[offset:]) != canonical {
		return Counters{}, errors.New("server close counter record is not canonical or terminal")
	}
	return result, nil
}

func (b *LinuxBackend) Stop(ctx context.Context, value Export) (Counters, error) {
	if err := ctx.Err(); err != nil {
		return Counters{}, err
	}
	if err := validateBackendLease(value); err != nil {
		return Counters{}, err
	}
	b.mu.Lock()
	b.defaults()
	recordPath, logPath := b.paths(value)
	managed := b.managed[recordPath]
	directory, err := b.openRuntimeDirectoryLocked(false)
	b.mu.Unlock()
	if err != nil {
		return Counters{}, err
	}
	defer directory.Close()
	if err := ctx.Err(); err != nil {
		return Counters{}, err
	}
	recordName, logName := filepath.Base(recordPath), filepath.Base(logPath)
	record, recordIdentity, err := readRecordIdentityAt(directory, recordName, value)
	if err != nil {
		return Counters{}, err
	}
	handle, processIsExact, identityErr := openProcessHandle(record, b.Binary)
	if handle != nil {
		defer handle.Close()
	}
	if identityErr != nil {
		return Counters{}, identityErr
	}
	alreadyExited := false
	if !processIsExact && managed != nil {
		select {
		case <-managed.done:
			alreadyExited = true
		default:
		}
	}
	if !processIsExact && !alreadyExited {
		return Counters{}, errors.New("refuse to stop process without exact storage lease identity")
	}
	if err = ctx.Err(); err != nil {
		return Counters{}, err
	}
	if managed != nil {
		if !alreadyExited {
			if err = unix.PidfdSendSignal(handle.pidfd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, syscall.ESRCH) {
				return Counters{}, err
			}
			if waitErr, exited := waitForProcess(ctx, managed.done, b.StopTimeout); !exited {
				return Counters{}, errors.Join(errors.New("storage server did not stop after graceful signal"), waitErr)
			}
		}
	} else {
		if err = unix.PidfdSendSignal(handle.pidfd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, syscall.ESRCH) {
			return Counters{}, err
		}
		if waitErr, exited := waitForPidfd(ctx, handle.pidfd, 15*time.Second); !exited {
			if waitErr != nil {
				return Counters{}, waitErr
			}
			return Counters{}, errors.New("recovered storage server did not stop")
		}
	}
	counters, err := parseCountersAt(directory, logName, value)
	if err != nil {
		return Counters{}, errors.New("storage server stopped without a complete counter record")
	}
	if _, err = directory.RemoveIfIdentity(recordName, recordIdentity); err != nil {
		return Counters{}, err
	}
	b.mu.Lock()
	delete(b.managed, recordPath)
	b.mu.Unlock()
	return counters, nil
}

func (b *LinuxBackend) OfflineCheck(ctx context.Context, value Export) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateBackendLease(value); err != nil {
		return "", err
	}
	b.mu.Lock()
	b.defaults()
	b.mu.Unlock()
	opened, err := b.openPreparedImage(value.PreparedImage, false)
	if err != nil {
		return "", err
	}
	defer opened.Close()
	if err = lockOpened(opened); err != nil {
		return "", err
	}
	if value.ImageIdentity != (ImageIdentity{Device: opened.identity.Device, Inode: opened.identity.Inode}) {
		return "", errors.New("storage image identity changed before offline check")
	}
	output, err := boundedexec.RunWithFiles(ctx, b.CheckTimeout, b.CheckBinary, []string{"-fn", "/proc/self/fd/3"},
		[]string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}, 1<<20, []*os.File{opened.file})
	if err != nil {
		if errors.Is(err, boundedexec.ErrOutputLimit) {
			return "", errors.New("e2fsck output exceeded evidence bound")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("offline filesystem check interrupted: %w", err)
		}
		return "", errors.New("e2fsck reported a non-clean filesystem")
	}
	after, inspectErr := safefile.InspectOpened(opened.file, 16<<30)
	if inspectErr != nil || after != opened.identity || opened.verifyNamedIdentity() != nil {
		return "", errors.New("storage image identity changed during offline check")
	}
	digest := sha256.Sum256(output)
	return "e2fsck-clean-sha256:" + hex.EncodeToString(digest[:]), nil
}
