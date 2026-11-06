//go:build linux

package guestscope

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"testing"
	"time"
)

func TestScopePeerQueuedContextPreservesOwnerAndAuthority(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			// Invalid sentinels cannot address a host cgroup. The held real scope lock
			// represents admission/inspection ownership; peer inspection must never pass it here.
			peerfd, err := unix.PidfdOpen(os.Getpid(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(peerfd)
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
			go func() {
				close(entered)
				inside, err := scope.ContainsLivePeerContext(ctx, os.Getpid(), peerfd)
				if inside {
					err = errors.New("canceled peer check claimed membership")
				}
				returned <- err
			}()
			<-entered
			if mode == "cancel" {
				select {
				case <-returned:
					t.Fatal("queued peer check returned before cancellation")
				case <-time.After(20 * time.Millisecond):
				}
				cancel()
			}
			err = nil
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
			if _, e := unix.FcntlInt(uintptr(peerfd), unix.F_GETFD, 0); e != nil {
				t.Fatal("canceled waiter closed caller pidfd", e)
			}
			scope.mu.Unlock()
			held = false
			if !early {
				select {
				case err = <-returned:
				case <-time.After(time.Second):
					t.Fatal("peer check remained stuck after owner release")
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
			if _, err := scope.ContainsLivePeerContext(t.Context(), 1, -1); !errors.Is(err, ErrClosed) {
				t.Fatal("fresh completed-release observation failed", err)
			}
		})
	}
}

func TestScopePeerCanceledAdmissionDoesNotClaimStateObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, scope := range []*Scope{{}, {owned: true, closed: true, parent: -1, directory: -1}} {
		if inside, err := scope.ContainsLivePeerContext(ctx, 1, -1); inside || !errors.Is(err, context.Canceled) {
			t.Fatal("canceled peer check observed cached state", inside, err)
		}
		if inside, err := scope.ContainsLivePeerContext(nil, 1, -1); inside || err == nil {
			t.Fatal("nil peer context accepted")
		}
	}
	var zero Scope
	if inside, err := zero.ContainsLivePeerContext(t.Context(), 1, -1); inside || !errors.Is(err, ErrIdentity) {
		t.Fatal("zero scope acquired peer authority", inside, err)
	}
}
