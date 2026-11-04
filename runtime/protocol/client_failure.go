package protocol

import (
	"context"
	"errors"
	"time"
)

// ClientFailureStage selects fixed local transport metadata. It accepts no
// address, request identity or free-form description from a failed operation.
type ClientFailureStage uint8

const (
	DaemonConnect ClientFailureStage = iota + 1
	DaemonDeadline
	DaemonWrite
	DaemonRead
	NetworkConnect
	NetworkDeadline
	NetworkWrite
	NetworkRead
	AttachConnect
	AttachDeadline
	AttachWrite
	AttachRead
)

// ClientFailure discards the local cause, retaining only a fixed stage or a
// standard context sentinel. Neither formatting nor unwrapping the result can
// expose socket paths, wrapped transport text or credential-bearing payloads.
// Authenticated remote API errors are handled separately by each client.
func ClientFailure(ctx context.Context, stage ClientFailureStage, cause error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	// Socket and context timers may wake in either order at the same deadline.
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	if errors.Is(cause, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	descriptions := [...]string{
		"RPC client operation failed",
		"daemon connection failed", "daemon connection deadline setup failed",
		"daemon request write failed", "daemon response read failed",
		"mknetd connection failed", "mknetd connection deadline setup failed",
		"mknetd request write failed", "mknetd response read failed",
		"mknetd ATTACH connection failed", "mknetd ATTACH deadline setup failed",
		"mknetd ATTACH request write failed", "mknetd ATTACH response read failed",
	}
	if int(stage) >= len(descriptions) {
		stage = 0
	}
	return errors.New(descriptions[stage])
}
