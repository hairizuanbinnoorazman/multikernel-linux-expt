//go:build linux

package dockerengine

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/guestscope"
)

// net/http may supply an independently live dial context after the request's
// explicit probe budget expires. It must not initiate a new socket observation.
func TestReadinessDetachedDialCannotOutliveProbeBudget(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "probe.sock")
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			var budget context.Context
			var cancel context.CancelFunc
			if mode == "cancel" {
				budget, cancel = context.WithCancel(t.Context())
				cancel()
			} else {
				budget, cancel = context.WithTimeout(t.Context(), time.Millisecond)
				defer cancel()
				<-budget.Done()
			}
			transport := readinessTransport(budget, &guestscope.Scope{}, socket)
			defer transport.CloseIdleConnections()
			conn, err := transport.DialContext(context.Background(), "tcp", "ignored.invalid:80")
			if conn != nil {
				conn.Close()
				t.Fatal("expired probe created a connection")
			}
			if err == nil {
				t.Fatal("expired probe accepted detached dial")
			}
			if err = listener.SetDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			accepted, err := listener.AcceptUnix()
			if accepted != nil {
				accepted.Close()
				t.Fatal("expired explicit probe budget contacted socket")
			}
			timeout, ok := err.(net.Error)
			if !ok || !timeout.Timeout() {
				t.Fatal("socket noncontact was not observed", err)
			}
		})
	}
}
