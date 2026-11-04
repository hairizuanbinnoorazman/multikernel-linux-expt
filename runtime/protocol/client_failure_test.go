package protocol

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type unprintableClientCause struct{}

func (unprintableClientCause) Error() string { panic("local transport cause was formatted") }

type deadlineOnlyContext struct{ context.Context }

func (deadlineOnlyContext) Deadline() (time.Time, bool) { return time.Now().Add(-time.Second), true }

func TestClientFailureNeverFormatsOrRetainsPrivateCause(t *testing.T) {
	for _, stage := range []ClientFailureStage{0, DaemonConnect, DaemonDeadline, DaemonWrite, DaemonRead, NetworkConnect, NetworkDeadline, NetworkWrite, NetworkRead, AttachConnect, AttachDeadline, AttachWrite, AttachRead, 255} {
		err := ClientFailure(context.Background(), stage, unprintableClientCause{})
		if err == nil || len(err.Error()) > 128 || errors.Unwrap(err) != nil {
			t.Fatal("local client failure retains or formats cause")
		}
	}
	secret := errors.New(strings.Repeat("SYNTHETIC_RPC_CLIENT_TOKEN /private/control/socket\n", 256))
	if err := ClientFailure(context.Background(), DaemonConnect, secret); strings.Contains(err.Error(), "SYNTHETIC") || strings.Contains(err.Error(), "/private/") {
		t.Fatal("private transport payload escaped")
	}
}

func TestClientFailurePreservesContextIdentityAndDeadlineBoundary(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, test := range []struct {
		ctx              context.Context
		cause, errorWant error
	}{
		{cancelled, context.DeadlineExceeded, context.Canceled},
		{expired, context.Canceled, context.DeadlineExceeded},
		{deadlineOnlyContext{context.Background()}, context.Canceled, context.DeadlineExceeded},
		{context.Background(), context.Canceled, context.Canceled},
		{context.Background(), context.DeadlineExceeded, context.DeadlineExceeded},
	} {
		if err := ClientFailure(test.ctx, DaemonRead, test.cause); err != test.errorWant {
			t.Fatal("context classification or deadline precedence changed")
		}
	}
}
