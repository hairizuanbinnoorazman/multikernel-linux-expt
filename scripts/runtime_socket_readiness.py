"""Bounded read-only readiness for private fixture Unix listeners."""
import os
import socket
import struct
import time


def wait_for_socket_owner(path, process, timeout=30):
    deadline = time.monotonic() + timeout
    while True:
        if process.poll() is not None:
            raise RuntimeError('private daemon exited before readiness')
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError('private daemon listener did not become ready')
        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as probe:
            probe.settimeout(min(.2, remaining))
            try:
                probe.connect(str(path))
                pid, uid, gid = struct.unpack('3i', probe.getsockopt(
                    socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize('3i')))
                if pid == process.pid and uid == os.geteuid() and process.poll() is None:
                    return {'pid': pid, 'uid': uid, 'gid': gid}
            except OSError:
                pass
        time.sleep(min(.05, max(0, deadline - time.monotonic())))
