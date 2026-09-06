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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

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
}

type LinuxBackend struct {
	Binary       string
	CheckBinary  string
	RuntimeDir   string
	RequiredUID  int
	ReadyTimeout time.Duration
	StopTimeout  time.Duration

	mu      sync.Mutex
	managed map[string]*managedExport
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
	if b.managed == nil {
		b.managed = map[string]*managedExport{}
	}
}

func (b *LinuxBackend) paths(value Export) (record, log string) {
	name := value.SandboxID + "-" + value.SandboxGeneration[:12]
	return filepath.Join(b.RuntimeDir, name+".json"), filepath.Join(b.RuntimeDir, name+".log")
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

func (b *LinuxBackend) Inspect(_ context.Context, image PreparedImage) error {
	b.mu.Lock()
	b.defaults()
	b.mu.Unlock()
	resolved, err := filepath.EvalSymlinks(image.Path)
	if err != nil || resolved != image.Path {
		return errors.New("storage image path may not contain symlinks")
	}
	descriptor, err := unix.Open(image.Path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(descriptor)
	var info unix.Stat_t
	if err = unix.Fstat(descriptor, &info); err != nil {
		return err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Mode&0077 != 0 || info.Nlink != 1 || int(info.Uid) != b.RequiredUID {
		return errors.New("storage image must be a private single-link regular file owned by the configured UID")
	}
	if uint64(info.Size) != image.SizeBytes || uint64(info.Blocks)*512 < image.SizeBytes {
		return errors.New("storage image size/quota differs or the image is sparse")
	}
	if err = unix.Flock(descriptor, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("storage image already has an owner")
	}
	defer unix.Flock(descriptor, unix.LOCK_UN)
	if err = inspectExt4(descriptor, image); err != nil {
		return err
	}
	hash := sha256.New()
	buffer := make([]byte, 4<<20)
	for offset := int64(0); offset < info.Size; {
		length := len(buffer)
		if remaining := info.Size - offset; remaining < int64(length) {
			length = int(remaining)
		}
		n, readErr := unix.Pread(descriptor, buffer[:length], offset)
		if readErr != nil || n != length {
			return errors.New("storage image changed or failed while hashing")
		}
		_, _ = hash.Write(buffer[:n])
		offset += int64(n)
	}
	var after unix.Stat_t
	if err = unix.Fstat(descriptor, &after); err != nil || after.Size != info.Size || after.Mtim != info.Mtim || after.Ctim != info.Ctim {
		return errors.New("storage image mutated during inspection")
	}
	if hex.EncodeToString(hash.Sum(nil)) != image.SHA256 {
		return errors.New("storage image digest differs from prepared identity")
	}
	return nil
}

func processStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
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

func atomicRecord(path string, value processRecord) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".record.*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err = temporary.Chmod(0600); err == nil {
		_, err = temporary.Write(append(data, '\n'))
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	cleanup = false
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	err = dir.Sync()
	return errors.Join(err, dir.Close())
}

func (b *LinuxBackend) Start(ctx context.Context, value Export) error {
	b.mu.Lock()
	b.defaults()
	if err := os.MkdirAll(b.RuntimeDir, 0700); err != nil {
		b.mu.Unlock()
		return err
	}
	if err := os.Chmod(b.RuntimeDir, 0700); err != nil {
		b.mu.Unlock()
		return err
	}
	recordPath, logPath := b.paths(value)
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		_ = os.Remove(logPath)
		log, err = os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		b.mu.Unlock()
		return err
	}
	command := exec.Command(b.Binary, "server", value.Path, strconv.Itoa(int(value.Port)), value.ImageID, value.ExportGeneration)
	command.Stdout, command.Stderr = log, log
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	// The export outlives mkruntimed so a daemon restart cannot sever a live
	// child's root disk. Reconciliation adopts it only when PID start time,
	// argv, path, port, image ID, and export generation all match the durable
	// lease; teardown signals that exact process group.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = command.Start(); err != nil {
		_ = log.Close()
		b.mu.Unlock()
		return err
	}
	startTime, err := processStartTime(command.Process.Pid)
	if err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		_ = log.Close()
		b.mu.Unlock()
		return err
	}
	record := processRecord{Version: 1, PID: command.Process.Pid, StartTime: startTime, Path: value.Path,
		Port: value.Port, ImageID: value.ImageID, ExportGeneration: value.ExportGeneration}
	if err = atomicRecord(recordPath, record); err != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		_ = log.Close()
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
			_ = os.Remove(recordPath)
			return fmt.Errorf("storage server exited before ready: %w", err)
		case <-ctx.Done():
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
			return ctx.Err()
		case <-deadline.C:
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
			return errors.New("storage server readiness timeout")
		case <-ticker.C:
			data, readErr := os.ReadFile(logPath)
			if readErr == nil && bytes.Contains(data, []byte("MKNBD_SERVER_READY ")) {
				return nil
			}
		}
	}
}

func readRecord(path string) (processRecord, error) {
	var value processRecord
	info, err := os.Lstat(path)
	if err != nil {
		return value, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4096 {
		return value, errors.New("storage process record is not private and bounded")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err = protocol.StrictDecode(data, &value); err != nil || value.Version != 1 {
		return value, errors.New("storage process record is malformed")
	}
	return value, nil
}

func processMatches(record processRecord, binary string) bool {
	start, err := processStartTime(record.PID)
	if err != nil || start != record.StartTime {
		return false
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(record.PID), "cmdline"))
	if err != nil {
		return false
	}
	want := strings.Join([]string{binary, "server", record.Path, strconv.Itoa(int(record.Port)), record.ImageID, record.ExportGeneration, ""}, "\x00")
	return string(data) == want
}

func (b *LinuxBackend) Observe(_ context.Context, value Export) (Observation, error) {
	b.mu.Lock()
	b.defaults()
	recordPath, logPath := b.paths(value)
	b.mu.Unlock()
	record, err := readRecord(recordPath)
	if errors.Is(err, os.ErrNotExist) {
		// A daemon may restart after the server completed its graceful close but
		// before the QUIESCING lease was finalized. The generation-specific log
		// is the durable counter record for that exact export.
		counters, counterErr := parseCounters(logPath)
		if counterErr == nil {
			return Observation{Closed: true, Counters: counters}, nil
		}
		return Observation{}, nil
	}
	if err != nil {
		return Observation{}, err
	}
	if !processMatches(record, b.Binary) {
		_ = os.Remove(recordPath)
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

func parseCounters(logPath string) (Counters, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return Counters{}, err
	}
	offset := bytes.LastIndex(data, []byte("MKNBD_SERVER_CLOSED"))
	if offset < 0 {
		return Counters{}, errors.New("server close marker is absent")
	}
	var result Counters
	var readBytes, writeBytes uint64
	_, err = fmt.Sscanf(string(data[offset:]),
		"MKNBD_SERVER_CLOSED reads=%d read_bytes=%d writes=%d write_bytes=%d flushes=%d",
		&result.Reads, &readBytes, &result.Writes, &writeBytes, &result.Flushes)
	result.ReadBytes, result.WrittenBytes = readBytes, writeBytes
	return result, err
}

func (b *LinuxBackend) Stop(ctx context.Context, value Export) (Counters, error) {
	b.mu.Lock()
	b.defaults()
	recordPath, logPath := b.paths(value)
	managed := b.managed[recordPath]
	b.mu.Unlock()
	record, err := readRecord(recordPath)
	if err != nil {
		return Counters{}, err
	}
	if record.ExportGeneration != value.ExportGeneration || !processMatches(record, b.Binary) {
		if managed == nil || record.ExportGeneration != value.ExportGeneration {
			return Counters{}, errors.New("refuse to stop process without exact storage lease identity")
		}
	}
	if managed != nil {
		if _, exited := waitForProcess(ctx, managed.done, b.StopTimeout); !exited {
			if err = syscall.Kill(-record.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
				return Counters{}, err
			}
			if waitErr, exited := waitForProcess(ctx, managed.done, 15*time.Second); !exited {
				return Counters{}, errors.Join(errors.New("storage server did not stop after graceful signal"), waitErr)
			}
		}
	} else {
		if err = syscall.Kill(-record.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return Counters{}, err
		}
		deadline := time.Now().Add(15 * time.Second)
		for processMatches(record, b.Binary) && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return Counters{}, ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
		}
		if processMatches(record, b.Binary) {
			return Counters{}, errors.New("recovered storage server did not stop")
		}
	}
	counters, err := parseCounters(logPath)
	if err != nil {
		return Counters{}, errors.New("storage server stopped without a complete counter record")
	}
	if err = os.Remove(recordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Counters{}, err
	}
	b.mu.Lock()
	delete(b.managed, recordPath)
	b.mu.Unlock()
	return counters, nil
}

func (b *LinuxBackend) OfflineCheck(ctx context.Context, value Export) (string, error) {
	b.mu.Lock()
	b.defaults()
	b.mu.Unlock()
	command := exec.CommandContext(ctx, b.CheckBinary, "-fn", value.Path)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			output = append(output, exit.Stderr...)
		}
		return "", errors.New("e2fsck reported a non-clean filesystem")
	}
	if len(output) > 1<<20 {
		return "", errors.New("e2fsck output exceeded evidence bound")
	}
	digest := sha256.Sum256(output)
	return "e2fsck-clean-sha256:" + hex.EncodeToString(digest[:]), nil
}
