#!/usr/bin/env python3
"""Own compiled supervisor/worker clients and logs only; never create a child.

All streams stay in the new private probe output. The controlled legacy mode
reproduces the previous raw-path/stack diagnostic, but cannot qualify redaction.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import select
import signal
import socket
import stat
import struct
import subprocess
import tarfile
import tempfile
import time

SPECS = {
    'SHIM_LIBRARY_EVENT': 'observe-library-event', 'SHIM_RPC_RECEIVE_FAILED': 'receive-rpc',
    'SHIM_RPC_SEND_FAILED': 'send-rpc', 'SHIM_RPC_INACTIVE_STREAM': 'receive-rpc',
    'SHIM_RPC_MESSAGE_FAILED': 'handle-rpc', 'SHIM_RPC_PEER_REFUSED': 'authenticate-rpc',
    'SHIM_RPC_CONNECTION_FAILED': 'open-rpc', 'SHIM_RPC_SERVER_FAILED': 'serve-rpc',
    'SHIM_REAPER_FAILED': 'reap-process', 'SHIM_LISTENER_STARTED': 'serve-rpc',
    'SHIM_SIGNAL_LOOP_STARTED': 'observe-signal', 'SHIM_EXIT_SIGNAL': 'observe-signal',
    'SHIM_STACK_DUMP_REDACTED': 'redact-stack-dump',
}


def sha(path):
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(descriptor, 'rb') as source:
        info = os.fstat(source.fileno())
        assert stat.S_ISREG(info.st_mode) and info.st_uid == os.geteuid() and info.st_nlink == 1 and not info.st_mode & 0o022 and 0 < info.st_size <= 128 << 20
        return hashlib.file_digest(source, 'sha256').hexdigest()


def vi(number):
    out = bytearray()
    while number > 127:
        out.append((number & 127) | 128); number >>= 7
    out.append(number)
    return bytes(out)


def field(number, data):
    return vi(number*8+2)+vi(len(data))+data


def frame(task=False, stream=1):
    owner = json.dumps(dict(version=1, namespace='moby', container_id='synthetic', sandbox_id='synthetic', generation='a'*32)).encode()
    service = b'containerd.task.v2.Task' if task else b'multikernel.inspection.v1.Inspection'
    body = field(1, b'synthetic' if task else owner)
    payload = field(1, service)+field(2, b'Connect' if task else b'InspectDockerEngine')+field(3, body)+vi(32)+vi(1_000_000_000)
    return struct.pack('>IIBB', len(payload), stream, 1, 0)+payload


def value(data, index):
    number = shift = 0
    while True:
        byte = data[index]; index += 1
        number |= (byte & 127) << shift
        if byte < 128:
            return number, index
        shift += 7
        assert shift < 70


def fields(data):
    result = {}; index = 0
    while index < len(data):
        key, index = value(data, index); wire = key & 7
        if wire == 0:
            item, index = value(data, index)
        else:
            assert wire == 2
            size, index = value(data, index); item = data[index:index+size]
            assert len(item) == size; index += size
        assert key >> 3 not in result
        result[key >> 3] = item
    return result


def response(conn, code, stream=1):
    def exact(size):
        data = b''
        while len(data) < size:
            more = conn.recv(size-len(data)); assert more; data += more
        return data
    length, observed, kind, flags = struct.unpack('>IIBB', exact(10))
    assert observed == stream and kind == 2 and flags == 0 and length < 2048
    data = fields(exact(length)); status = fields(data.get(1, b''))
    assert status.get(1, 0) == code
    if code:
        assert not data.get(2)


def private_write(path, data):
    with path.open('xb') as output:
        os.fchmod(output.fileno(), 0o600); output.write(data); output.flush(); os.fsync(output.fileno())


def run(args):
    stage = args.stage
    assert stage.is_absolute() and not stage.is_symlink()
    build = json.loads((stage/'build.json').read_text())
    assert re.fullmatch('[a-f0-9]{40}', build['source_commit'])
    binary = stage/'bin/containerd-shim-multikernel-v2'
    assert sha(binary) == build['binaries'][binary.name]
    assert sha(stage/'source.tar') == build['source_archive_sha256']
    with tarfile.open(stage/'source.tar') as archive:
        assert archive.pax_headers.get('comment') == build['source_commit']
        if not args.expect_unredacted:
            assert archive.extractfile('scripts/test-runtime-shim-library-logging.py').read() == Path(__file__).read_bytes()
    private = stage/'probe-private'; private.mkdir(mode=0o700)
    assert stat.S_IMODE(private.lstat().st_mode) == 0o700
    scratch = os.environ.get('MK_TEST_SHIM_LOG_FIXTURE_PARENT', '/tmp')
    with tempfile.TemporaryDirectory(prefix='SYNTHETIC_PRIVATE_LIB-', dir=scratch) as directory:
        root = Path(directory); root.chmod(0o700)
        os.mkfifo(root/'log', 0o600); log = os.open(root/'log', os.O_RDWR | os.O_NONBLOCK | os.O_CLOEXEC)
        listener = socket.socket(socket.AF_UNIX); listener.bind(str(root/'shim.sock'))
        os.chmod(root/'shim.sock', 0o600); listener.listen(8)
        listener_fd = listener.fileno()
        def child():
            os.dup2(listener_fd, 3)
        log_output = (private/'supervisor.stderr').open('xb'); os.fchmod(log_output.fileno(), 0o600)
        env = dict(PATH='/usr/sbin:/usr/bin:/sbin:/bin', LANG='C', LC_ALL='C', TTRPC_ADDRESS=str(root/'absent-publisher.sock'))
        process = subprocess.Popen([str(binary), '-debug', '-namespace', 'moby', '-id', 'synthetic'],
            cwd=root, env=env, pass_fds=tuple(set((3, listener_fd))), preexec_fn=child,
            start_new_session=True, stdout=subprocess.DEVNULL, stderr=log_output)
        clients = []
        captured = bytearray(); worker_fd = None
        def drain():
            while True:
                try: data = os.read(log, 4096)
                except BlockingIOError: break
                assert data
                captured.extend(data); assert len(captured) <= 1 << 20
        def connect(task, stream=1, existing=None):
            conn = existing
            if conn is None:
                conn = socket.socket(socket.AF_UNIX); conn.settimeout(3); clients.append(conn); conn.connect(str(root/'shim.sock'))
            conn.sendall(frame(task, stream)); response(conn, 0 if task else 9, stream)
            return conn
        try:
            original = connect(True); inspect = connect(False)
            assert process.poll() is None
            pidfile = root/'.multikernel-worker.pid'; info = pidfile.lstat()
            assert stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o600 and info.st_uid == os.geteuid() and info.st_nlink == 1
            worker = int(pidfile.read_text().strip()); assert worker > 1 and os.getpgid(worker) == process.pid
            worker_fd = os.pidfd_open(worker, 0)
            worker_stat = Path('/proc')/str(worker)/'stat'
            before = worker_stat.read_text().rsplit(')', 1)[1].split()
            assert int(before[1]) == process.pid and pidfile.read_text().strip() == str(worker)
            drain()
            # Blank-line filler makes the FIFO truly full down to one byte,
            # without mixing arbitrary bytes into the structured record stream.
            filled = 0
            for chunk in [b'\n'*4096, b'\n']:
                while True:
                    try: count = os.write(log, chunk)
                    except BlockingIOError: break
                    assert count == len(chunk); filled += count
                    assert filled <= 1 << 20
            assert filled > 0
            signal.pidfd_send_signal(worker_fd, signal.SIGUSR1)
            connect(True, 3, original)
            # The router serves one inspection frame per independent client.
            inspect.close(); connect(False)
            assert worker_stat.read_text().rsplit(')', 1)[1].split()[19] == before[19]
            deadline = time.monotonic()+5
            while True:
                drain()
                if args.expect_unredacted:
                    complete = b'=== END goroutine stack dump ===' in captured
                else:
                    complete = any(b'SHIM_STACK_DUMP_REDACTED' in line for line in captured.splitlines())
                if complete:
                    break
                assert process.poll() is None and time.monotonic() < deadline
                select.select([log], [], [], min(.1, deadline-time.monotonic()))
            assert process.poll() is None and pidfile.read_text().strip() == str(worker)
            if args.expect_unredacted:
                assert b'SYNTHETIC_PRIVATE_LIB-' in captured and b'=== BEGIN goroutine stack dump ===' in captured
            else:
                seen = set()
                for raw in captured.splitlines():
                    if not raw: continue
                    assert len(raw) < 1024 and b'SYNTHETIC_PRIVATE' not in raw
                    record = json.loads(raw)
                    assert set(record) == {'version', 'component', 'scope', 'code', 'operation', 'severity', 'pid', 'owner_known', 'at'}
                    assert type(record['version']) is int and record['version'] == 1 and record['component'] == binary.name and record['scope'] == 'shim-library' and record['owner_known'] is False
                    assert type(record['pid']) is int and record['pid'] in {process.pid, worker}
                    assert record['code'] in SPECS and record['operation'] == SPECS[record['code']]
                    assert record['severity'] in {'panic', 'fatal', 'error', 'warning', 'info', 'debug', 'trace'}
                    assert re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z', record['at'])
                    key = (record['pid'], record['code'], record['severity']); assert key not in seen; seen.add(key)
                assert any(code == 'SHIM_STACK_DUMP_REDACTED' for _, code, _ in seen)
            connect(True, 5, original)
            result = dict(scope='actual-compiled-shim-library-logging-no-child', source_commit=build['source_commit'],
                actual_debug_and_stack_dump_exercised=True, own_worker_pidfd_and_parentage_verified=True,
                full_fifo_keeps_original_task_and_new_inspection_clients_responsive=True,
                worker_incarnation_unchanged=True, library_log_redaction_qualified=not args.expect_unredacted,
                legacy_private_path_reproduced=args.expect_unredacted, raw_private_streams_exported=False, child_kernel_started=False)
            private_write(private/'result.json', json.dumps(result, sort_keys=True).encode()+b'\n')
        finally:
            drain(); private_write(private/'fifo.log', bytes(captured))
            for conn in clients: conn.close()
            listener.close(); os.close(log)
            if worker_fd is not None: os.close(worker_fd)
            # Only this newly created session, held by our live Popen object.
            if process.poll() is None: os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5); log_output.close()
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stage', type=Path, required=True)
    parser.add_argument('--expect-unredacted', action='store_true')
    run(parser.parse_args())
