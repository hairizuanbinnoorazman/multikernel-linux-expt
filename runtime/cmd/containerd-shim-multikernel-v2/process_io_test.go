//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/fifo"
	"golang.org/x/sys/unix"
)

func TestResizePtyStoresInitialSizeBeforeStart(t *testing.T) {
	s := &service{processes: map[string]*process{
		"": {terminal: true, status: tasktypes.Status_CREATED},
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 91, Height: 37}); err != nil {
		t.Fatal(err)
	}
	p := s.processes[""]
	if !p.sizeSet || p.width != 91 || p.height != 37 {
		t.Fatalf("pending size = set:%v %dx%d", p.sizeSet, p.width, p.height)
	}
}

func TestOutputFIFOCanBeReattached(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout")
	initial, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("FIFO identity unavailable")
	}
	w, guard, err := openOutput(context.Background(), path, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	defer guard.Close()
	initial.Close()
	if _, err = w.Write([]byte("reattach-ok")); err != nil {
		t.Fatal(err)
	}
	attached, err := fifo.OpenFifo(context.Background(), path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer attached.Close()
	var got = make([]byte, len("reattach-ok"))
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, err = io.ReadFull(attached, got); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if string(got) != "reattach-ok" {
		t.Fatalf("attached output = %q, err = %v", got, err)
	}
}

func TestOutputFIFOKeeperBridgesWorkerDeath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(descriptor), path)
	defer reader.Close()
	identity, err := inspectBoundProcessIOPath(path, false)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := openKnownProcessIOPath(context.Background(), path, unix.O_RDWR|unix.O_NONBLOCK, identity)
	if err != nil {
		t.Fatal(err)
	}
	keeper, err := startOutputFIFOKeeper(guard, 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate SIGKILL: the worker's descriptors disappear without Close's
	// explicit byte, while the helper retains its inherited guard.
	if err = keeper.guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err = keeper.control.Close(); err != nil {
		t.Fatal(err)
	}
	var empty [1]byte
	if count, readErr := unix.Read(int(reader.Fd()), empty[:]); count != -1 || !errors.Is(readErr, syscall.EAGAIN) {
		t.Fatalf("reader observed FIFO gap: count=%d error=%v", count, readErr)
	}
	replacement, err := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY|unix.O_NONBLOCK, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = replacement.Write([]byte("after-restart")); err != nil {
		t.Fatal(err)
	}
	if err = replacement.Close(); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len("after-restart"))
	if _, err = io.ReadFull(reader, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "after-restart" {
		t.Fatalf("replacement output = %q", got)
	}
	if err = keeper.command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestProductionFIFOKeeperGraceCoversWorkerRecoveryTimeout(t *testing.T) {
	if fifoKeeperGrace <= defaultAgentIOCallTimeout {
		t.Fatalf("FIFO keeper grace %s does not cover agent recovery timeout %s",
			fifoKeeperGrace, defaultAgentIOCallTimeout)
	}
	if fifoKeeperGrace > time.Minute {
		t.Fatalf("FIFO keeper grace %s exceeds the helper's bounded maximum", fifoKeeperGrace)
	}
}

func TestProcessIOPathsRejectUnsafeIdentityAndSymlinkAncestors(t *testing.T) {
	directory := t.TempDir()
	unsafeFIFO := filepath.Join(directory, "unsafe")
	if err := syscall.Mkfifo(unsafeFIFO, 0660); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(unsafeFIFO, false); err == nil {
		t.Fatal("group-accessible FIFO was accepted")
	}

	regular := filepath.Join(directory, "output")
	if err := os.WriteFile(regular, []byte("output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(regular, regular+".other"); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(regular, false); err == nil {
		t.Fatal("hard-linked output was accepted")
	}

	realParent := filepath.Join(directory, "real")
	if err := os.Mkdir(realParent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(realParent, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(directory, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(filepath.Join(linkedParent, "fifo"), false); err == nil {
		t.Fatal("stdio beneath a symlinked parent was accepted")
	}
}

func TestProcessIOPathNormalizesOnlyTrustedRuntimeAlias(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "run")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(directory, "var-run")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(target, "stdout")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeProcessIOPathAlias(filepath.Join(alias, "stdout"), alias, target)
	if err != nil {
		t.Fatal(err)
	}
	if normalized != path {
		t.Fatalf("normalized path = %q, want %q", normalized, path)
	}
	if _, err = inspectBoundProcessIOPath(normalized, false); err != nil {
		t.Fatalf("inspect normalized path: %v", err)
	}

	unrelated := alias + "-other/stdout"
	if normalized, err = normalizeProcessIOPathAlias(unrelated, alias, target); err != nil || normalized != unrelated {
		t.Fatalf("unrelated path normalized to %q, err = %v", normalized, err)
	}
	if err = os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(directory, "other")
	if err = os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if _, err = normalizeProcessIOPathAlias(filepath.Join(alias, "stdout"), alias, target); err == nil {
		t.Fatal("runtime alias with an unexpected target was accepted")
	}
}

func TestProcessIOOpenRejectsReplacementAndCancellation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "output")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok := processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("output identity unavailable")
	}
	if err = os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("same-inode stdio mode change was accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	info, err = inspectProcessIOPath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, ok = processIOIdentityFromInfo(info)
	if !ok {
		t.Fatal("output identity unavailable")
	}
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, openErr := openKnownProcessIOPath(context.Background(), path, unix.O_WRONLY, expected); openErr == nil {
		_ = file.Close()
		t.Fatal("replacement stdio inode was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if file, openErr := openKnownProcessIOPath(ctx, path, unix.O_WRONLY, expected); !errors.Is(openErr, context.Canceled) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("cancelled stdio open error = %v", openErr)
	}
}

func TestInvalidProcessIOFailsBeforeCreateOrExecMutation(t *testing.T) {
	s := &service{id: "task", bundle: "/bundle", processes: make(map[string]*process)}
	if _, err := s.Create(context.Background(), &taskapi.CreateTaskRequest{
		ID: "task", Bundle: "/bundle", Stdin: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Create invalid stdio error = %v", err)
	}
	if len(s.processes) != 0 {
		t.Fatalf("Create mutated process state: %+v", s.processes)
	}
	if _, err := s.Exec(context.Background(), &taskapi.ExecProcessRequest{
		ExecID: "exec", Stdout: "relative-fifo",
	}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("Exec invalid stdio error = %v", err)
	}
}

func TestProcessStdinMustBeFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte("input"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectBoundProcessIOPath(path, true); err == nil {
		t.Fatal("regular-file stdin was accepted")
	}
}
