"""Bounded private-lab CHECK of one exact mknetd endpoint; no repair or fallback."""
import hashlib
import json
import os
import re
import socket
import struct
import time
import uuid
from runtime_doctor_checkpoint import unique_object


def inspect(path, daemon, endpoint, boot_id, owner, deadline):
    if (daemon.poll() is not None or endpoint.get('state') != 'READY' or
            endpoint.get('owner') != 'runtime' or endpoint.get('managed_namespace') is not True or
            endpoint.get('primary_boot_id') != boot_id or
            endpoint.get('sandbox_id') != owner['sandbox_id'] or
            endpoint.get('sandbox_generation') != owner['generation'] or
            not isinstance(endpoint.get('generation'), str) or
            not re.fullmatch('[a-f0-9]{32}', endpoint['generation'])):
        raise ValueError('network checkpoint lacks exact active endpoint ownership')
    request_id = 'g8-check-' + uuid.uuid4().hex
    data = json.dumps(dict(version=1, request_id=request_id, method='CHECK', endpoint=endpoint),
                      separators=(',', ':')).encode() + b'\n'
    if len(data) > 16384:
        raise ValueError('network checkpoint request exceeds its bound')
    def remaining():
        value = deadline - time.monotonic()
        if value <= 0:
            raise TimeoutError('network checkpoint deadline expired')
        return value
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(remaining())
        connection.connect(str(path))
        pid, uid, gid = struct.unpack('3i', connection.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
        if pid != daemon.pid or uid != os.geteuid() or gid != os.getegid() or daemon.poll() is not None:
            raise ValueError('network checkpoint connected to another process')
        connection.settimeout(remaining())
        connection.sendall(data)
        connection.shutdown(socket.SHUT_WR)
        chunks = bytearray()
        while True:
            connection.settimeout(remaining())
            chunk = connection.recv(min(4096, 65537 - len(chunks)))
            if not chunk:
                break
            chunks.extend(chunk)
            if len(chunks) > 65536:
                raise ValueError('network checkpoint response exceeds its bound')
    report = json.loads(chunks, object_pairs_hook=unique_object)
    if (not isinstance(report, dict) or set(report) != {'version', 'request_id', 'endpoint'} or
            type(report['version']) is not int or report['version'] != 1 or report['request_id'] != request_id or
            report['endpoint'] != endpoint or daemon.poll() is not None):
        raise ValueError('network checkpoint refused or endpoint changed')
    return dict(schema_version=1, boot_id=boot_id, sandbox_id=owner['sandbox_id'],
                sandbox_generation=owner['generation'], endpoint_generation=endpoint['generation'],
                state='READY', authenticated_check_pass=True,
                endpoint_sha256=hashlib.sha256(json.dumps(endpoint, sort_keys=True, separators=(',', ':')).encode()).hexdigest(),
                response_sha256=hashlib.sha256(chunks).hexdigest(),
                scope='sampled-kernel-network-policy-check', automatic_repair_claimed=False)
