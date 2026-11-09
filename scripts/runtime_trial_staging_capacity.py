"""Read-only committed-packet measurement and conservative staging admission.

Keep eight GiB free after code staging. This is additional to, and never
replaces, the ordinary two-GiB child image/one-GiB builder reserve checks.
It neither reserves capacity nor proves the complete Docker workload peak.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import tarfile

STORAGE_FLOOR = 8 << 30
PACKET_LIMIT = 128 << 20
SOURCE_LIMIT = 128 << 20
OUTER_LIMIT = 256 << 20
ENTRY_LIMIT = 16384
ROOTS = {'scripts', 'guest', 'deploy', 'runtime', 'tools'}
BINARIES = {'containerd', 'mkruntimed', 'mknetd', 'mk-agent', 'mk-shimctl',
            'containerd-shim-multikernel-v2', 'mkvsock-nbd', 'mkvsock-relay'}


def unique(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise ValueError('duplicate code manifest key')
        value[key] = item
    return value


def signature(info):
    return tuple(getattr(info, key) for key in ['st_dev', 'st_ino', 'st_mode', 'st_uid',
        'st_gid', 'st_nlink', 'st_size', 'st_mtime_ns', 'st_ctime_ns'])


def rounded(size):
    return ((size+4095)//4096)*4096


def source_measurement(data, revision):
    total = 0
    allocated = 0
    seen = set()
    files = 0
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:') as archive:
        if archive.pax_headers.get('comment') != revision:
            raise ValueError('source commit differs')
        for member in archive:
            name = member.name.rstrip('/')
            path = PurePosixPath(name)
            if (name in seen or len(seen) >= ENTRY_LIMIT or not path.parts or path.is_absolute() or
                    '..' in path.parts or str(path) != name or path.parts[0] not in ROOTS or
                    not (member.isdir() or member.isfile()) or member.uid != 0 or member.gid != 0 or
                    member.mode not in ([0o755] if member.isdir() else [0o644, 0o755]) or
                    member.isdir() and member.size != 0 or any(ord(c) < 32 or ord(c) == 127 for c in name) or
                    not 0 <= member.size <= SOURCE_LIMIT):
                raise ValueError('source member refused')
            seen.add(name)
            files += int(member.isfile())
            total += member.size if member.isfile() else 0
            allocated += rounded(member.size) if member.isfile() else 4096
            if total > SOURCE_LIMIT:
                raise ValueError('source payload exceeds its bound')
    if not files:
        raise ValueError('empty source archive')
    return allocated, len(seen)


def measure(packet, expected_digest, revision):
    if type(expected_digest) is not str or type(revision) is not str or not re.fullmatch('[a-f0-9]{64}', expected_digest) or not re.fullmatch('[a-f0-9]{40}', revision):
        raise ValueError('invalid committed packet authority')
    packet = Path(packet)
    fd = os.open(packet, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(fd, 'rb') as stream:
        before = os.fstat(stream.fileno())
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid() or before.st_nlink != 1 or
                before.st_mode & 0o022 or not 0 < before.st_size <= PACKET_LIMIT):
            raise ValueError('unsafe committed code packet')
        digest = hashlib.sha256()
        count = 0
        while chunk := stream.read(1 << 20):
            count += len(chunk)
            if count > before.st_size:
                raise ValueError('code packet grew')
            digest.update(chunk)
        if count != before.st_size or digest.hexdigest() != expected_digest:
            raise ValueError('code packet differs from caller pin')
        stream.seek(0)
        expected = {'source.tar', 'build.json', 'bin', 'tmp'} | {'bin/'+name for name in BINARIES}
        hashes = {}
        source = None
        build = None
        allocated = 0
        total = 0
        seen = set()
        with tarfile.open(fileobj=stream, mode='r|gz') as archive:
            for member in archive:
                name = member.name
                directory = name in {'bin', 'tmp'}
                mode = 0o700 if directory else 0o500 if name.startswith('bin/') else 0o600
                limit = SOURCE_LIMIT if name == 'source.tar' else (1 << 20) if name == 'build.json' else PACKET_LIMIT
                if (name not in expected or name in seen or member.uid != 0 or member.gid != 0 or
                        member.mode != mode or not (member.isdir() if directory else member.isfile()) or
                        not (member.size == 0 if directory else 0 < member.size <= limit)):
                    raise ValueError('packet member refused')
                seen.add(name)
                allocated += 4096 if directory else rounded(member.size)
                if directory:
                    continue
                total += member.size
                if total > OUTER_LIMIT:
                    raise ValueError('packet expansion exceeds its bound')
                payload = archive.extractfile(member)
                if name in {'source.tar', 'build.json'}:
                    data = payload.read(limit+1)
                    if len(data) != member.size:
                        raise ValueError('short code archive member')
                    hashes[name] = hashlib.sha256(data).hexdigest()
                    if name == 'source.tar':
                        source = data
                    else:
                        build = json.loads(data, object_pairs_hook=unique)
                else:
                    digest = hashlib.sha256()
                    size = 0
                    while chunk := payload.read(1 << 20):
                        size += len(chunk)
                        digest.update(chunk)
                    if size != member.size:
                        raise ValueError('short code binary member')
                    hashes[name] = digest.hexdigest()
        if seen != expected or not isinstance(build, dict) or source is None:
            raise ValueError('incomplete code packet')
        if (type(build.get('schema_version')) is not int or build['schema_version'] != 1 or
                build.get('source_commit') != revision or build.get('source_archive_sha256') != hashes['source.tar'] or
                build.get('built_from_committed_snapshot') is not True or build.get('installed') is not False or
                build.get('full_trial_started') is not False or type(build.get('binaries')) is not dict or
                set(build['binaries']) != BINARIES or
                any(build['binaries'][name] != hashes['bin/'+name] for name in BINARIES)):
            raise ValueError('committed packet manifest differs')
        source_allocated, source_entries = source_measurement(source, revision)
        after = os.fstat(stream.fileno())
        named = packet.lstat()
        if signature(before) != signature(after) or signature(before) != signature(named):
            raise ValueError('code packet identity changed')
    # Count the retained compressed packet, unpacked source tar/binaries and
    # expanded source independently. Double data allocation and add 8 KiB per
    # object plus 16 MiB scratch; do not equate payload bytes with disk use.
    entries = len(seen)+source_entries+4
    budget = 2*(rounded(before.st_size)+allocated+source_allocated)+entries*8192+(16 << 20)
    return dict(version=1, scope='committed-code-staging-capacity-plan', source_commit=revision,
                packet_sha256=expected_digest, packet_bytes=before.st_size, packet_entries=len(seen),
                source_entries=source_entries, staging_budget_bytes=budget, staging_budget_inodes=2*entries+1024,
                storage_floor_bytes=STORAGE_FLOOR, storage_required_bytes=STORAGE_FLOOR+budget,
                capacity_reserved=False, full_builder_admission_claimed=False, new_child_started=False)


def observe(storage, plan):
    integers = ['version', 'packet_bytes', 'packet_entries', 'source_entries', 'staging_budget_bytes',
                'staging_budget_inodes', 'storage_floor_bytes', 'storage_required_bytes']
    if (type(plan) is not dict or any(type(plan.get(k)) is not int for k in integers) or
            plan.get('version') != 1 or plan.get('scope') != 'committed-code-staging-capacity-plan' or
            type(plan.get('source_commit')) is not str or not re.fullmatch('[a-f0-9]{40}', plan['source_commit']) or
            type(plan.get('packet_sha256')) is not str or not re.fullmatch('[a-f0-9]{64}', plan['packet_sha256']) or
            plan.get('capacity_reserved') is not False or plan.get('full_builder_admission_claimed') is not False or
            plan.get('new_child_started') is not False or not 0 < plan['packet_bytes'] <= PACKET_LIMIT or
            plan['packet_entries'] != len(BINARIES)+4 or not 0 < plan['source_entries'] <= ENTRY_LIMIT or
            type(plan.get('staging_budget_bytes')) is not int or not 0 < plan['staging_budget_bytes'] <= 2 << 30 or
            type(plan.get('staging_budget_inodes')) is not int or not 1024 < plan['staging_budget_inodes'] <= 2*ENTRY_LIMIT+2048 or
            plan.get('storage_floor_bytes') != STORAGE_FLOOR or
            plan.get('storage_required_bytes') != STORAGE_FLOOR+plan['staging_budget_bytes']):
        raise ValueError('invalid staging capacity plan')
    storage = os.fspath(storage)
    if not os.path.isabs(storage) or os.path.normpath(storage) != storage or storage.startswith('//'):
        raise ValueError('invalid staging capacity scope')
    descriptor = os.open('/', os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in storage.strip('/').split('/') if storage != '/' else []:
            child = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
        capacity = os.fstatvfs(descriptor)
    finally:
        os.close(descriptor)
    free = capacity.f_bavail*capacity.f_frsize
    reasons = []
    if free < plan['storage_required_bytes']:
        reasons.append('STAGING_BYTES_AND_RETAINED_FLOOR')
    if capacity.f_favail < plan['staging_budget_inodes']:
        reasons.append('STAGING_INODES_AND_RETAINED_FLOOR')
    return dict(version=1, scope='committed-code-staging-capacity-observation', source_commit=plan['source_commit'],
                packet_sha256=plan['packet_sha256'], free_bytes=free, free_inodes=capacity.f_favail,
                required_bytes=plan['storage_required_bytes'], required_inodes=plan['staging_budget_inodes'],
                staging_capacity_available=not reasons, refusal_codes=reasons, capacity_reserved=False,
                full_builder_admission_claimed=False, packet_verified_on_guest=False, new_child_started=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--packet', type=Path, required=True)
    parser.add_argument('--packet-sha256', required=True)
    parser.add_argument('--source-commit', required=True)
    parser.add_argument('--check-storage', action='store_true')
    args = parser.parse_args()
    try:
        plan = measure(args.packet, args.packet_sha256, args.source_commit)
        result = observe('/srv/multikernel-storage', plan) if args.check_storage else plan
    except Exception:
        print('Trial code staging preflight refused; inspect private inputs', file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0 if not args.check_storage or result['staging_capacity_available'] else 1


if __name__ == '__main__':
    sys.exit(main())
