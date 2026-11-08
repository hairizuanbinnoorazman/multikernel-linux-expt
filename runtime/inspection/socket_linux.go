//go:build linux

package inspection

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"github.com/containerd/ttrpc"
	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"github.com/hairizuan/multikernel-linux-expt/runtime/protocol"
	"golang.org/x/sys/unix"
)

// ObserveSocket makes one bounded, inode-anchored query to an existing owned
// shim. It checks the peer UID and never retries, reconnects or replays.
func ObserveSocket(ctx context.Context, socket string, owner Request) (Report, error) {
	if ctx == nil {
		return Report{}, errors.New("inspection context is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if owner.Validate() != nil {
		return Report{}, errors.New("invalid inspection identity")
	}
	socket = strings.TrimPrefix(socket, "unix://")
	path, err := unixsocket.Capture(socket)
	if err != nil {
		return Report{}, err
	}
	defer path.Close()
	conn, err := path.DialContext(ctx)
	if err != nil {
		return Report{}, err
	}
	defer conn.Close()
	stop := protocol.CloseOnContext(ctx, conn)
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return Report{}, err
		}
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return Report{}, errors.New("inspection socket is not Unix")
	}
	raw, err := u.SyscallConn()
	if err != nil {
		return Report{}, err
	}
	var peerErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil || cred.Uid != uint32(os.Geteuid()) {
			peerErr = errors.New("inspection peer owner refused")
		}
	}); err != nil {
		return Report{}, err
	}
	if peerErr != nil {
		return Report{}, peerErr
	}
	client := ttrpc.NewClient(conn)
	defer client.Close()
	return Inspect(ctx, client, owner)
}
