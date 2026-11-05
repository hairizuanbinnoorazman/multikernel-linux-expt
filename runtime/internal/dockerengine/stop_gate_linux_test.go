//go:build linux

package dockerengine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDockerStopCompletedReleaseRemainsIdempotent(t *testing.T) {
	service := &Service{pidfd: -1}
	service.closed.Store(true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if service.Stop(ctx) != nil || service.Stop(t.Context()) != nil {
		t.Fatal("published completion lost idempotence")
	}
	if service.Stop(nil) == nil {
		t.Fatal("nil context accepted")
	}
}

func TestDockerStopGateConcurrentCancellationKeepsSingleOwner(t *testing.T) {
	var gate stopGate
	if err := gate.lock(t.Context()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 128 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if err := gate.lock(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Error("waiter took held ownership", err)
			}
		})
	}
	wg.Wait()
	gate.unlock()
	var owners atomic.Int32
	for range 32 {
		wg.Go(func() {
			for range 16 {
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				err := gate.lock(ctx)
				cancel()
				if err != nil {
					t.Error(err)
					return
				}
				if owners.Add(1) != 1 {
					t.Error("concurrent teardown owners")
				}
				owners.Add(-1)
				gate.unlock()
			}
		})
	}
	wg.Wait()
	for range 256 {
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() {
			err := gate.lock(ctx)
			if err == nil {
				gate.unlock()
			}
			result <- err
		}()
		cancel()
		if err := <-result; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		live, finish := context.WithTimeout(t.Context(), time.Second)
		if err := gate.lock(live); err != nil {
			finish()
			t.Fatal("canceled admission lost gate token", err)
		}
		gate.unlock()
		finish()
	}
}
