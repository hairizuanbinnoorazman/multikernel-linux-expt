//go:build linux

package guestscope

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestScopeStopQueuedContextPreservesOwnerAndAuthority(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			// Invalid sentinels cannot address a host cgroup. The held real scope lock
			// represents admission/inspection ownership; Stop must never pass it here.
			scope := &Scope{owned: true, parent: -1, directory: -1, name: "fixed-test-scope", device: 17, inode: 19}
			scope.mu.Lock()
			held := true
			defer func() {
				if held {
					scope.mu.Unlock()
				}
			}()
			var ctx context.Context
			var cancel context.CancelFunc
			expected := context.Canceled
			if mode == "cancel" {
				ctx, cancel = context.WithCancel(t.Context())
			} else {
				ctx, cancel = context.WithTimeout(t.Context(), 30*time.Millisecond)
				expected = context.DeadlineExceeded
			}
			defer cancel()
			returned := make(chan error, 1)
			entered := make(chan struct{})
			go func() { close(entered); returned <- scope.Stop(ctx) }()
			<-entered
			if mode == "cancel" {
				select {
				case <-returned:
					t.Fatal("queued stop returned before cancellation")
				case <-time.After(20 * time.Millisecond):
				}
				cancel()
			}
			var err error
			early := false
			select {
			case err = <-returned:
				early = true
			case <-time.After(200 * time.Millisecond):
			}
			// Inspect while the owner still holds the actual lock. A canceled waiter
			// must not start closure, touch descriptors or overwrite inventory identity.
			if scope.closing || scope.closed || !scope.owned || scope.parent != -1 || scope.directory != -1 || scope.name != "fixed-test-scope" || scope.device != 17 || scope.inode != 19 {
				t.Fatal("queued cancellation changed scope authority")
			}
			scope.mu.Unlock()
			held = false
			if !early {
				select {
				case err = <-returned:
				case <-time.After(time.Second):
					t.Fatal("stop remained stuck after owner release")
				}
			}
			if !early || !errors.Is(err, expected) {
				t.Error("scope caller did not return before active owner released", err)
			}
			// Existing APIs still serialize through the same gate. Mark only this test
			// scope closed to prove no lost token and no backend access after release.
			scope.mu.Lock()
			scope.closed = true
			scope.mu.Unlock()
			if err := scope.Stop(t.Context()); err != nil {
				t.Fatal("fresh completed-release observation failed", err)
			}
			if empty, err := scope.Empty(); err != nil || !empty {
				t.Fatal("closed inventory observation changed", empty, err)
			}
			if err := scope.Start(exec.Command("/bin/true")); !errors.Is(err, ErrClosed) {
				t.Fatal("closed scope admitted a process", err)
			}
			if _, err := scope.ContainsLivePeer(1, -1); !errors.Is(err, ErrClosed) {
				t.Fatal("closed scope accepted peer", err)
			}
		})
	}
}

// Cancellation before admission reports only the context result; it does not
// observe cached completion or backend identity. A fresh live request still can.
func TestScopeStopCanceledAdmissionDoesNotClaimStateObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, scope := range []*Scope{{}, {owned: true, closed: true, parent: -1, directory: -1}} {
		if err := scope.Stop(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal("canceled admission claimed cached scope state", err)
		}
		if scope.Stop(nil) == nil {
			t.Fatal("nil context accepted")
		}
	}
	var zero Scope
	if err := zero.Stop(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatal("zero scope acquired authority", err)
	}
	closed := &Scope{owned: true, closed: true, parent: -1, directory: -1}
	if err := closed.Stop(t.Context()); err != nil {
		t.Fatal("live completed-release observation lost idempotence", err)
	}
}
