package lifecycle

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/state"
)

type policyResolver struct{ artifacts Artifacts }

func (r policyResolver) Resolve(string) (Artifacts, error) { return r.artifacts, nil }

func TestPreparedBootPolicyGuardsCreateLoadAndRecovery(t *testing.T) {
	for _, stage := range []string{"create", "load", "recovery"} {
		t.Run(stage, func(t *testing.T) {
			service, store, backend := setup(t)
			defer store.Close()
			dir := t.TempDir()
			path := filepath.Join(dir, "archive")
			if err := os.WriteFile(path, []byte("journal-approved-archive"), 0600); err != nil {
				t.Fatal(err)
			}
			prepared := &fakePreparedBootResolver{directoryPath: dir, initrdPath: path}
			service.SetPreparedBootResolver(prepared)
			reject := stage == "create"
			checks := 0
			service.SetArtifactResolver(policyResolver{Artifacts{VerifyPreparedInitramfs: func(ctx context.Context, file *os.File) error {
				checks++
				raw, err := io.ReadAll(file)
				if err != nil || string(raw) != "journal-approved-archive" {
					t.Fatalf("wrong descriptor: %q %v", raw, err)
				}
				if reject {
					return errors.New("component does not match current profile")
				}
				_, err = file.Seek(0, io.SeekStart)
				return err
			}}})
			cfg := config("box-a", 8, 7001)
			created, apiErr := service.Create(t.Context(), cfg, "create")
			if stage == "create" {
				if apiErr == nil || apiErr.Code != "FAILED_PRECONDITION" || len(backend.calls) != 0 {
					t.Fatalf("create accepted or mutated backend: %v %v", apiErr, backend.calls)
				}
				if len(mustJournal(t, store)) != 0 || len(store.Snapshot().Sandboxes) != 0 || len(store.Snapshot().Results) != 0 {
					t.Fatal("rejected archive mutated durable state")
				}
			} else {
				if apiErr != nil {
					t.Fatal(apiErr)
				}
				reject = true
				if stage == "load" {
					if _, apiErr = service.Load(t.Context(), cfg.ID, created.Sandbox.Generation, "load"); apiErr == nil {
						t.Fatal("load accepted wrong components")
					}
				} else {
					sandbox := created.Sandbox
					intent := state.JournalEntry{OperationID: "operation", IdempotencyKey: "load", Fingerprint: "fingerprint", SandboxID: sandbox.ID, Generation: sandbox.Generation, Method: "LoadSandbox", Phase: "intent", State: sandbox.State, Sandbox: &sandbox}
					if err := store.Append(intent); err != nil {
						t.Fatal(err)
					}
					if err := service.Reconcile(t.Context()); err == nil {
						t.Fatal("recovery accepted wrong components")
					}
				}
				for _, call := range backend.calls {
					if strings.HasPrefix(call, "load:") {
						t.Fatalf("wrong components reached loader: %v", backend.calls)
					}
				}
			}
			if checks == 0 {
				t.Fatal("component policy was not invoked")
			}
			if _, err := prepared.directory.Stat(); err == nil {
				t.Fatal("runtime directory descriptor leaked")
			}
			if _, err := prepared.initrd.Stat(); err == nil {
				t.Fatal("rejected archive descriptor leaked")
			}
		})
	}
}
