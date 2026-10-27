//go:build linux

package agent

import (
	"context"
	"errors"
	"math"
	"net"
	"os"
	"syscall"

	"github.com/hairizuan/multikernel-linux-expt/runtime/internal/unixsocket"
	"golang.org/x/sys/unix"
)

// DialPeerContext requires a protected, captured Unix socket and the exact
// expected helper PID and caller UID/GID before any request is sent. Reconnect
// retains the same requirement; replacing a helper requires a new checked dial.
// This checks the local helper, not the remote VSOCK peer or guest incarnation.
func DialPeerContext(ctx context.Context, path, sandboxID, generation string, endpoint uint32, token []byte, peerPID int) (*Client, error) {
	if peerPID <= 0 || peerPID > math.MaxInt32 {
		return nil, errors.New("invalid agent relay peer identity")
	}
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	dial := func(ctx context.Context, path string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		owner, err := unixsocket.Capture(path)
		if err != nil {
			return nil, errors.New("agent relay socket ownership unavailable")
		}
		defer owner.Close()
		conn, err := owner.DialContext(ctx)
		if err != nil {
			return nil, errors.New("agent relay socket dial failed")
		}
		fail := func() (net.Conn, error) {
			_ = conn.Close()
			return nil, errors.New("agent relay peer identity mismatch")
		}
		local, ok := conn.(syscall.Conn)
		if !ok {
			return fail()
		}
		raw, err := local.SyscallConn()
		if err != nil {
			return fail()
		}
		var peer *unix.Ucred
		var controlErr error
		if err = raw.Control(func(fd uintptr) { peer, controlErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil || controlErr != nil || peer == nil || peer.Pid != int32(peerPID) || peer.Uid != uid || peer.Gid != gid {
			return fail()
		}
		if err = ctx.Err(); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
	conn, err := dial(ctx, path)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, token: token, sandboxID: sandboxID, generation: generation, endpoint: endpoint, sequence: initialSequence(), reconnectDial: dial}, nil
}
