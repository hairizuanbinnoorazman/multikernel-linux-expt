//go:build linux

package guestscope

import (
	"context"
	"sync"
)

// scopeGate keeps admission, observations and release serialized. Stop can
// abandon its wait on caller cancellation without touching the active owner.
// Legacy operations keep their blocking Lock contract. The zero value is ready;
// waiters allocate no helper worker or polling timer.
type scopeGate struct {
	once sync.Once
	held chan struct{}
}

func (g *scopeGate) lockContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.once.Do(func() { g.held = make(chan struct{}, 1) })
	select {
	case g.held <- struct{}{}:
		if err := ctx.Err(); err != nil {
			g.Unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *scopeGate) Unlock() { <-g.held }

func (g *scopeGate) Lock() { _ = g.lockContext(context.Background()) }
