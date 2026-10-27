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
// retains the same requirement. A trusted owner can explicitly rebind an owned
// replacement with ReconnectPeerContext, preserving authentication sequence.
// This checks the local helper, not the remote VSOCK peer or guest incarnation.
func DialPeerContext(ctx context.Context, path, sandboxID, generation string, endpoint uint32, token []byte, peerPID int) (*Client, error) {
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	dialPeer := func(ctx context.Context, path string, peerPID int) (net.Conn, error) {
		if peerPID <= 0 || peerPID > math.MaxInt32 {
			return nil, errors.New("invalid agent relay peer identity")
		}
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
	dial := func(ctx context.Context, path string) (net.Conn, error) { return dialPeer(ctx, path, peerPID) }
	conn, err := dial(ctx, path)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, token: token, sandboxID: sandboxID, generation: generation, endpoint: endpoint, sequence: initialSequence(), reconnectDial: dial, peerDial: dialPeer}, nil
}

// ReconnectPeerContext explicitly selects a replacement local relay. The caller
// must establish ownership of peerPID; the client verifies SO_PEERCRED with that
// PID and the UID/GID captured at its initial checked dial before any request.
// Failed verification preserves both the old policy and sequence. Success pins
// subsequent ordinary reconnects to the replacement, without issuing/replaying
// any RPC or resetting sequence. Unchecked clients cannot use this method.
func (c *Client) ReconnectPeerContext(ctx context.Context, path string, peerPID int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.peerDial == nil {
		return errors.New("agent checked peer policy required")
	}
	conn, err := c.peerDial(ctx, path, peerPID)
	if err != nil {
		return err
	}
	c.replaceConnectionLocked(conn)
	c.reconnectDial = func(ctx context.Context, path string) (net.Conn, error) {
		return c.peerDial(ctx, path, peerPID)
	}
	return nil
}
