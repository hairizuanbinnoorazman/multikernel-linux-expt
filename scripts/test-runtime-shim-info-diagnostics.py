#!/usr/bin/env python3
"""Trusted compiled shim -info controls only; no daemon or child kernel.

Print fixed verdicts and executable identity, never captured streams. Legacy
mode reproduces the old full-stderr hang and cannot qualify the boundary.
"""
import argparse
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--legacy', action='store_true')
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    results = []
    for case in ('malformed', 'oversized', 'read-error'):
        for transport in ('capture', 'closed-reader', 'full'):
            with tempfile.TemporaryFile() as source, tempfile.TemporaryFile() as errors:
                source.write(b'\xff' if case == 'malformed' else b'x' * ((1 << 20) + 1))
                source.seek(0)
                directory = os.open('/tmp', os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
                reader = writer = None
                try:
                    if transport != 'capture':
                        reader, writer = os.pipe()
                        if transport == 'closed-reader':
                            os.close(reader)
                            reader = None
                        else:
                            flags = fcntl.fcntl(writer, fcntl.F_GETFL)
                            fcntl.fcntl(writer, fcntl.F_SETFL, flags | os.O_NONBLOCK)
                            while True:
                                try:
                                    os.write(writer, b'x' * 4096)
                                except BlockingIOError:
                                    break
                            fcntl.fcntl(writer, fcntl.F_SETFL, flags)
                    child = subprocess.Popen([str(binary), '-info'],
                        stdin=directory if case == 'read-error' else source,
                        stdout=subprocess.PIPE, stderr=errors if writer is None else writer,
                        env={'PATH': '/usr/bin:/bin'}, close_fds=True)
                    timed_out = False
                    try:
                        stdout, _ = child.communicate(timeout=2)
                    except subprocess.TimeoutExpired:
                        timed_out = True
                        child.kill()
                        stdout, _ = child.communicate(timeout=2)
                    assert not stdout, 'failed discovery published protobuf output'
                    if args.legacy:
                        if transport == 'full':
                            assert timed_out, 'legacy hang did not reproduce'
                        elif transport == 'capture':
                            errors.seek(0)
                            raw = errors.read(4096)
                            assert child.returncode == 1 and raw.startswith(b'io.containerd.multikernel.v2: runtime info: ')
                        else:
                            assert child.returncode != 0
                    else:
                        assert not timed_out and child.returncode == 1, 'discovery lost exit status or blocked'
                        if transport == 'capture':
                            errors.seek(0)
                            raw = errors.read(1025)
                            assert len(raw) <= 1024 and raw.count(b'\n') == 1
                            record = json.loads(raw)
                            assert set(record) == {'version', 'component', 'scope', 'code', 'operation', 'pid', 'owner_known', 'at'}
                            assert record['version'] == 1 and record['owner_known'] is False
                            assert record['component'] == 'containerd-shim-multikernel-v2'
                            assert record['scope'] == 'daemon-terminal'
                            assert record['code'] == 'SHIM_RUNTIME_INFO_FAILED'
                            assert record['operation'] == 'write-runtime-info' and record['pid'] == child.pid
                            assert record['at'].endswith('Z')
                            datetime.datetime.fromisoformat(record['at'].replace('Z', '+00:00'))
                    results.append({'case': case, 'stderr': transport, 'passed': True})
                finally:
                    os.close(directory)
                    if reader is not None:
                        os.close(reader)
                    if writer is not None:
                        os.close(writer)
    options = b'\x0a\x07unknown\x12\x06opaque'
    for data in (b'', options):
        success = subprocess.run([str(binary), '-info'], input=data,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=2,
            env={'PATH': '/usr/bin:/bin'}, check=True)
        assert success.stdout and not success.stderr
        if data:
            assert bytes([0x1a, len(options)]) + options in success.stdout
    print(json.dumps({'scope': 'compiled-shim-info-no-child',
        'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest(),
        'failure_controls': results, 'success_controls': 2,
        'legacy_reproduction': args.legacy,
        'bounded_discovery_qualified': not args.legacy,
        'full_g7_g8_qualified': False}, sort_keys=True))


if __name__ == '__main__':
    main()
