"""Bounded private checkpoint command collection against an absolute deadline."""
import os
import selectors
import signal
import subprocess
import time


class CommandOutputLimit(ValueError):
    def __init__(self, stdout, stderr):
        super().__init__('checkpoint command output limit exceeded')
        self.stdout, self.stderr = stdout, stderr


def bounded_command(argv, deadline):
    """Collect at most each stream's limit plus one overflow byte.

    Keep the owned leader unreaped until collection completes so its PID cannot
    be reused before refusal terminates the private process group. Cleanup has
    a separate one-second reap bound; it never grants checkpoint time.
    """
    remaining = (deadline-time.monotonic_ns())/1_000_000_000
    if remaining <= 0:
        raise subprocess.TimeoutExpired(argv, 0, output=b'', stderr=b'')
    process = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True)
    data = {'stdout': bytearray(), 'stderr': bytearray()}
    limits = {'stdout': 1 << 20, 'stderr': 4096}
    complete = False
    try:
        with selectors.DefaultSelector() as poll:
            for name in data:
                stream = getattr(process, name)
                os.set_blocking(stream.fileno(), False)
                poll.register(stream, selectors.EVENT_READ, name)
            while poll.get_map():
                remaining = (deadline-time.monotonic_ns())/1_000_000_000
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(argv, 0, output=bytes(data['stdout']), stderr=bytes(data['stderr']))
                for key, _ in poll.select(remaining):
                    name = key.data
                    try:
                        chunk = os.read(key.fd, min(65536, limits[name]+1-len(data[name])))
                    except BlockingIOError:
                        continue
                    if not chunk:
                        poll.unregister(key.fileobj)
                        continue
                    data[name].extend(chunk)
                    if len(data[name]) > limits[name]:
                        raise CommandOutputLimit(bytes(data['stdout']), bytes(data['stderr']))
        remaining = (deadline-time.monotonic_ns())/1_000_000_000
        if remaining <= 0:
            raise subprocess.TimeoutExpired(argv, 0, output=bytes(data['stdout']), stderr=bytes(data['stderr']))
        try:
            status = process.wait(timeout=remaining)
        except subprocess.TimeoutExpired:
            raise subprocess.TimeoutExpired(argv, remaining, output=bytes(data['stdout']), stderr=bytes(data['stderr'])) from None
        complete = True
        return subprocess.CompletedProcess(argv, status, bytes(data['stdout']), bytes(data['stderr']))
    finally:
        if not complete:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.stdout.close()
        process.stderr.close()
        if not complete:
            process.wait(timeout=1)

