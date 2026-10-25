package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"reflect"
	"strings"
	"testing"
)

type inspectOnlyBackend struct {
	Backend
	inspect func(context.Context, Export) (ProcessInspection, error)
}

func (b inspectOnlyBackend) InspectProcess(ctx context.Context, v Export) (ProcessInspection, error) {
	return b.inspect(ctx, v)
}
func TestProcessOwnershipReadOnlyBracketsPrivateLeases(t *testing.T) {
	for _, mode := range []string{"matched", "absent", "missing", "private-error", "wrong-generation", "drift", "released", "busy", "limit", "invalid", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			store, err := OpenStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			v := validStoredExport("/PRIVATE_PATH/root.ext4")
			store.data.Exports[exportKey(v.SandboxID, v.SandboxGeneration)] = v
			before := store.List()
			calls := 0
			backend := inspectOnlyBackend{inspect: func(_ context.Context, e Export) (ProcessInspection, error) {
				calls++
				if e != v {
					t.Fatal("private exact lease not passed to inspector")
				}
				if mode == "drift" {
					store.mu.Lock()
					changed := v
					changed.Path = "/different/root.ext4"
					store.data.Exports[exportKey(v.SandboxID, v.SandboxGeneration)] = changed
					store.mu.Unlock()
				}
				if mode == "private-error" {
					return ProcessInspection{}, errors.New("PRIVATE_PATH_SECRET")
				}
				generation := v.ExportGeneration
				if mode == "wrong-generation" {
					generation = strings.Repeat("e", 32)
				}
				status := "process-matched"
				if mode == "absent" {
					status = "process-absent"
				}
				if mode == "missing" {
					status = "record-missing"
				}
				return ProcessInspection{Status: status, Generation: generation}, nil
			}}
			svc := NewService(store, backend)
			svc.mu.Lock()
			defer svc.mu.Unlock()
			ctx := context.Background()
			if mode == "released" {
				v.State = "RELEASED"
				v.OfflineCheck = "e2fsck-clean-sha256:" + strings.Repeat("a", 64)
				v.ReleasedAt = v.UpdatedAt
				store.data.Exports[exportKey(v.SandboxID, v.SandboxGeneration)] = v
				before = store.List()
			}
			if mode == "invalid" {
				v.PeerCID = 0
				store.data.Exports[exportKey(v.SandboxID, v.SandboxGeneration)] = v
			}
			if mode == "limit" {
				for i := 0; i <= protocol.MaxStorageOwnershipRecords; i++ {
					store.data.Exports[strings.Repeat("x", i+1)] = v
				}
			}
			if mode == "busy" {
				store.mu.Lock()
				defer store.mu.Unlock()
			}
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			report, descriptors, processes, failure := svc.ProcessOwnership(ctx)
			switch mode {
			case "drift", "busy", "limit", "invalid", "canceled":
				if failure == nil || report.Exports != nil || descriptors != nil || processes != nil {
					t.Fatal("unsafe partial snapshot")
				}
				if mode != "drift" && calls != 0 {
					t.Fatal("backend called before refusal")
				}
				return
			}
			if failure != nil || len(processes) != 1 || len(descriptors) != 1 {
				t.Fatalf("%+v %v", processes, failure)
			}
			want := "process-matched"
			switch mode {
			case "absent":
				want = "process-absent"
			case "missing":
				want = "record-missing"
			case "private-error", "wrong-generation":
				want = "unavailable"
			case "released":
				want = "not-probed-released"
				if calls != 0 {
					t.Fatal("released history probed")
				}
			}
			if processes[0].Status != want {
				t.Fatalf("status %+v", processes)
			}
			raw, _ := json.Marshal(processes)
			if strings.Contains(string(raw), "PRIVATE") {
				t.Fatal("private error/path leaked")
			}
			if !reflect.DeepEqual(before, store.List()) {
				t.Fatal("inspection mutated store")
			}
		})
	}
}
