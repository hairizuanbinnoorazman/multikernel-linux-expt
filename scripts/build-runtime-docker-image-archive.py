#!/usr/bin/env python3
"""Package an approved xattr-free guest filesystem for operator Docker import.

This prepares distribution data, never executes image programs and never changes
the source tree. Runtime xattr refusal stays enforced. Only metadata represented
by the approved deterministic root manifest enters the generated tar archive.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import runpy
import tarfile

from runtime_safe_publish import atomic_write

rootfs = runpy.run_path(str(Path(__file__).with_name('build-runtime-rootfs.py')))


def build(root, expected_manifest, output):
    if not re.fullmatch('[0-9a-f]{64}', expected_manifest):
        raise ValueError('approval requires a canonical manifest SHA256')
    if output.parent.resolve(strict=True).is_relative_to(root.resolve(strict=True)):
        raise ValueError('archive destination must be outside the approved source tree')
    entries, contents = rootfs['scan'](root)
    payload = sum(e.size for e in entries if e.kind == 'regular')
    if payload > 1 << 30 or len(entries) > 131072:
        raise ValueError('approved archive input exceeds image limits')
    source_digest = hashlib.sha256(rootfs['manifest'](entries)).hexdigest()
    if source_digest != expected_manifest:
        raise ValueError('source filesystem differs from operator approval')
    free = os.statvfs(output.parent)
    if free.f_bavail * free.f_frsize < payload + len(entries) * 8192 + (1 << 30):
        raise ValueError('insufficient archive capacity and reserve')

    def write(stream):
        hardlinks = {}
        with tarfile.open(fileobj=stream, mode='w|', format=tarfile.PAX_FORMAT) as archive:
            for e in entries:
                item = tarfile.TarInfo(e.path)
                item.mode = e.mode & 0o7777
                item.uid, item.gid, item.mtime = e.uid, e.gid, 0
                if e.kind == 'directory':
                    item.type = tarfile.DIRTYPE
                elif e.kind == 'symlink':
                    item.type, item.linkname = tarfile.SYMTYPE, e.target
                elif e.kind == 'regular':
                    if e.hardlink and e.hardlink in hardlinks:
                        item.type, item.linkname = tarfile.LNKTYPE, hardlinks[e.hardlink]
                    else:
                        if e.hardlink:
                            hardlinks[e.hardlink] = e.path
                        item.size = e.size
                        archive.addfile(item, io.BytesIO(contents[e.path]))
                        continue
                else:
                    raise ValueError('unsupported approved archive entry')
                archive.addfile(item)

    atomic_write(output, write)
    with output.open('rb') as stream:
        archive_digest = hashlib.file_digest(stream, 'sha256').hexdigest()
    return {'schema_version': 1, 'source_manifest_sha256': source_digest,
            'archive_sha256': archive_digest, 'entries': len(entries),
            'payload_bytes': payload, 'xattrs_emitted': False,
            'image_programs_executed': 0}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('output', type=Path)
    parser.add_argument('--manifest-sha256', required=True)
    args = parser.parse_args()
    print(json.dumps(build(args.root, args.manifest_sha256, args.output), sort_keys=True))
