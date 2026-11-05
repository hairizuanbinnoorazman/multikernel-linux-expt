package agent

import (
	"context"
	"sync"
)

// clientGate serializes the framed stream and reconnect policy without making
// a cancelled caller wait for an unrelated RPC. The zero value is ready to use.
// Waiters allocate no worker goroutine and polling never competes with the owner.
// A cancellation racing acquisition releases the token before returning.
type clientGate struct {
	once sync.Once
	held chan struct{}
}

func (g *clientGate) lock(ctx context.Context) error {
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
func (g *clientGate) unlock() { <-g.held }
