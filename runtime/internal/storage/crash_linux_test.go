//go:build linux

package storage

import (
	"bytes"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAcceptedProcessSIGKILLRetainsEveryStoragePhase(t *testing.T) {
	for _, phase := range []string{"PREPARING", "ACTIVE", "QUIESCING"} {
		for _, history := range []string{"accepted", "missing-log"} {
			t.Run(phase+"-"+history, func(t *testing.T) {
				backend, value := startLinuxProcessFixture(t)
				recordPath, logPath := backend.paths(value)
				deadline := time.Now().Add(time.Second)
				for {
					data, err := os.ReadFile(logPath)
					if err != nil {
						t.Fatal(err)
					}
					if bytes.Contains(data, []byte("MKNBD_SERVER_CLIENT_ACCEPTED\n")) {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("fixture acceptance missing")
					}
					time.Sleep(time.Millisecond)
				}
				record, err := readRecord(recordPath, value)
				if err != nil {
					t.Fatal(err)
				}
				pidfd, err := unix.PidfdOpen(record.PID, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer unix.Close(pidfd)
				if err = unix.PidfdSendSignal(pidfd, unix.SIGKILL, nil, 0); err != nil {
					t.Fatal(err)
				}
				select {
				case <-backend.managed[recordPath].done:
				case <-time.After(time.Second):
					t.Fatal("killed fixture not reaped")
				}
				if history == "missing-log" {
					if err := os.Remove(logPath); err != nil {
						t.Fatal(err)
					}
				}
				recordBefore, _ := os.ReadFile(recordPath)
				logBefore, _ := os.ReadFile(logPath)
				imageHash := digestPreparedImage(t, value.Path)
				if bytes.Contains(logBefore, []byte("MKNBD_SERVER_CLOSED")) {
					t.Fatal("SIGKILL produced graceful evidence")
				}
				got, err := backend.InspectProcess(t.Context(), value)
				if err != nil || got.Status != "process-absent" {
					t.Fatalf("death inspection = %+v %v", got, err)
				}
				if _, err = backend.Stop(t.Context(), value); !errors.Is(err, ErrUnsafeSession) {
					t.Fatalf("unsafe stop = %v", err)
				}
				store, err := OpenStore(filepath.Join(t.TempDir(), "store"))
				if err != nil {
					t.Fatal(err)
				}
				value.State = "PREPARING"
				value.CreatedAt = time.Now().UTC()
				value.UpdatedAt = value.CreatedAt
				if err = store.Put(value); err != nil {
					t.Fatal(err)
				}
				if phase != "PREPARING" {
					value.State = "ACTIVE"
					if err = store.Put(value); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "QUIESCING" {
					value.State = phase
					if err = store.Put(value); err != nil {
						t.Fatal(err)
					}
				}
				before := store.List()
				backend.afterImageOpen = func() { t.Error("accepted process death reopened storage for recovery") }
				service := NewService(store, backend)
				err = service.Reconcile(t.Context(), map[string]Owner{value.SandboxID: {Generation: value.SandboxGeneration, PeerCID: value.PeerCID, Image: value.PreparedImage, ExportGeneration: value.ExportGeneration}})
				if !errors.Is(err, ErrUnsafeSession) {
					t.Fatalf("%s recovery = %v", phase, err)
				}
				recordAfter, _ := os.ReadFile(recordPath)
				logAfter, _ := os.ReadFile(logPath)
				if !bytes.Equal(recordBefore, recordAfter) || !bytes.Equal(logBefore, logAfter) || imageHash != digestPreparedImage(t, value.Path) || !reflect.DeepEqual(before, store.List()) {
					t.Fatal("crash recovery changed retained evidence")
				}
			})
		}
	}
}
