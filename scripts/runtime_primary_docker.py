"""Bounded primary Docker inventory observations for disposable live fixtures.

Only stable inventory identities leave this module, as counts and digests.
Commands target the primary Unix socket explicitly and never inherit client
contexts, credentials, plugins or remote-host settings from the caller.
"""
import hashlib
import json
import os
import re
import selectors
import subprocess
import time

ENV = {'PATH': '/usr/bin:/bin', 'LANG': 'C', 'LC_ALL': 'C',
       'HOME': '/nonexistent', 'DOCKER_CONFIG': '/proc/self/fd/-1'}
CLI = ['/usr/bin/docker', '--host', 'unix:///var/run/docker.sock']
IMAGE_ID = r'sha256:[0-9a-f]{64}'


def bounded_command(argv, timeout=10, limit=1 << 20):
    if timeout <= 0 or limit < 1:
        raise RuntimeError('primary Docker observation budget exhausted')
    deadline = time.monotonic() + timeout
    process = subprocess.Popen(argv, env=ENV, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT)
    data = bytearray()
    try:
        os.set_blocking(process.stdout.fileno(), False)
        with selectors.DefaultSelector() as poll:
            poll.register(process.stdout, selectors.EVENT_READ)
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not poll.select(remaining):
                    raise RuntimeError('primary Docker observation deadline exceeded')
                chunk = os.read(process.stdout.fileno(), min(65536, limit + 1 - len(data)))
                if not chunk:
                    break
                data.extend(chunk)
                if len(data) > limit:
                    raise RuntimeError('primary Docker observation output limit exceeded')
        try:
            status = process.wait(timeout=max(.001, deadline - time.monotonic()))
        except subprocess.TimeoutExpired:
            raise RuntimeError('primary Docker observation deadline exceeded') from None
        if status != 0:
            # Daemon/CLI errors can contain arbitrary private metadata.
            raise RuntimeError('primary Docker observation command failed')
        return bytes(data)
    finally:
        process.stdout.close()
        if process.poll() is None:
            process.kill()
        process.wait(timeout=2)


def fingerprint(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('ambiguous primary Docker metadata')
        result[key] = value
    return result


def identifiers(data, pattern):
    rows = data.decode('ascii').splitlines()
    if len(rows) > 10000 or len(set(rows)) != len(rows) or any(not re.fullmatch(pattern, row) for row in rows):
        raise ValueError('invalid primary Docker inventory identities')
    return sorted(rows)


def observe(expected_absent_image=None, command=None, deadline=None):
    if command is None:
        def command(args):
            remaining = 10 if deadline is None else min(10, deadline - time.monotonic())
            return bounded_command(args, timeout=remaining)
    if expected_absent_image is not None and not re.fullmatch(IMAGE_ID, expected_absent_image):
        raise ValueError('invalid child image identity')
    engine = json.loads(command(CLI + ['info', '--format', '{{json .ID}}']))
    if not isinstance(engine, str) or not re.fullmatch(r'[A-Za-z0-9:_-]{1,256}', engine):
        raise ValueError('invalid primary Docker engine identity')
    images = []
    lines = command(CLI + ['image', 'ls', '--all', '--digests', '--no-trunc', '--format', '{{json .}}']).splitlines()
    if len(lines) > 10000:
        raise ValueError('primary Docker image row limit exceeded')
    for line in lines:
        row = json.loads(line, object_pairs_hook=unique_object)
        values = [row[key] for key in ['ID', 'Repository', 'Tag', 'Digest']]
        if any(not isinstance(v, str) or not 0 < len(v) <= 2048 or any(ord(c) < 32 for c in v) for v in values):
            raise ValueError('invalid primary Docker image metadata')
        if not re.fullmatch(IMAGE_ID, values[0]) or not (values[3] == '<none>' or re.fullmatch(IMAGE_ID, values[3])):
            raise ValueError('invalid primary Docker image digest')
        images.append(values)
    inventories = {'images': sorted(images)}
    for kind, args, pattern in [
            ('containers', ['container', 'ls', '--all', '--no-trunc', '--quiet'], r'[0-9a-f]{64}'),
            ('volumes', ['volume', 'ls', '--quiet'], r'[A-Za-z0-9][A-Za-z0-9_.-]{0,255}'),
            ('networks', ['network', 'ls', '--no-trunc', '--quiet'], r'[a-z0-9]{12,64}')]:
        inventories[kind] = identifiers(command(CLI + args), pattern)
    return {'schema_version': 1, 'engine_id_sha256': fingerprint(engine),
            'inventories': {kind: {'count': len(rows), 'sha256': fingerprint(rows)}
                            for kind, rows in inventories.items()},
            'checked_absent_image': expected_absent_image,
            'child_image_absent': expected_absent_image is not None and
                                  not any(row[0] == expected_absent_image for row in images)}


def unchanged(before, after):
    if (before['engine_id_sha256'] != after['engine_id_sha256'] or
            before['inventories'] != after['inventories']):
        raise ValueError('primary Docker engine or inventory changed during the child workload')
