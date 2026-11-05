//go:build linux

package guestscope

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScopeGateConcurrentContextsAndLegacyLocksShareOneOwner(t *testing.T) {
	var gate scopeGate
	gate.Lock()
	var wg sync.WaitGroup
	for range 128 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if err := gate.lockContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Error("deadline waiter took owner", err)
			}
		})
	}
	wg.Wait()
	gate.Unlock()
	var owners atomic.Int32
	for index := range 32 {
		wg.Go(func() {
			for range 16 {
				if index%2 == 0 {
					gate.Lock()
				} else {
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					err := gate.lockContext(ctx)
					cancel()
					if err != nil {
						t.Error(err)
						return
					}
				}
				if owners.Add(1) != 1 {
					t.Error("overlapping scope owners")
				}
				owners.Add(-1)
				gate.Unlock()
			}
		})
	}
	wg.Wait()
	for range 256 {
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() {
			err := gate.lockContext(ctx)
			if err == nil {
				gate.Unlock()
			}
			result <- err
		}()
		cancel()
		if err := <-result; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		live, finish := context.WithTimeout(t.Context(), time.Second)
		if err := gate.lockContext(live); err != nil {
			finish()
			t.Fatal("canceled acquisition lost permit", err)
		}
		gate.Unlock()
		finish()
	}
}
