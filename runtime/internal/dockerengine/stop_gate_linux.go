//go:build linux

package dockerengine

import (
	"context"
	"sync"
)

// stopGate serializes service teardown while allowing a queued caller to leave
// on cancellation. The owner keeps its pidfd/mount/scope authority. The zero
// value is ready; waiters have no helper worker or polling timer. Cancellation
// racing admission returns the token before any teardown side effect.
type stopGate struct {
	once sync.Once
	held chan struct{}
}

func (g *stopGate) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.once.Do(func() { g.held = make(chan struct{}, 1) })
	select {
	case g.held <- struct{}{}:
		if err := ctx.Err(); err != nil {
			g.unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *stopGate) unlock() { <-g.held }
