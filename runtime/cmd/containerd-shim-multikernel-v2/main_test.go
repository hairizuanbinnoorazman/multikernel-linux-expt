//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	taskapi "github.com/containerd/containerd/api/runtime/task/v2"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/fifo"
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
	w, guard, err := openOutput(context.Background(), path)
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

func TestResizePtyRejectsInvalidRequests(t *testing.T) {
	s := &service{processes: map[string]*process{
		"": {status: tasktypes.Status_CREATED},
	}}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{}); !errors.Is(err, errdefs.ErrFailedPrecondition) {
		t.Fatalf("non-terminal resize error = %v", err)
	}
	if _, err := s.ResizePty(context.Background(), &taskapi.ResizePtyRequest{Width: 65536}); !errors.Is(err, errdefs.ErrInvalidArgument) {
		t.Fatalf("oversized resize error = %v", err)
	}
}
