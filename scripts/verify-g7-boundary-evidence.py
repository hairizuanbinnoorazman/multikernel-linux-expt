#!/usr/bin/env python3
"""Verify the source archives and cleanup claims in a G7 boundary bundle."""
import argparse
import hashlib
import json
from pathlib import Path
import tarfile


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("bundle", type=Path)
    args = parser.parse_args()
    bundle = args.bundle
    identity = json.loads((bundle / "source-identity.json").read_text())
    for name, key in [("tested-source.tar.gz", "source_archive_sha256"),
                      ("kernel-reviewed-source.tar.gz", "kernel_source_archive_sha256")]:
        if digest((bundle / name).read_bytes()) != identity[key]:
            raise SystemExit(f"archive identity mismatch: {name}")
    sources = json.loads((bundle / "source-hashes.json").read_text())
    # Capture transcripts wrap the pretty-printed audit JSON in two JSON lines.
    audit_text = (bundle / 'source-audit.log').read_text().split('\n', 1)[1]
    audit, _ = json.JSONDecoder().raw_decode(audit_text)
    with tarfile.open(bundle / 'kernel-reviewed-source.tar.gz') as archive:
        names = [member.name for member in archive.getmembers()]
        if len(set(names)) != len(names):
            raise SystemExit('duplicate kernel source archive member')
        for source in audit['files']:
            member = archive.getmember(source['path'])
            if not member.isfile():
                raise SystemExit('kernel source member is not a regular file')
            data = archive.extractfile(member).read()
            if digest(data) != source['sha256'] or len(data) != source['size_bytes']:
                raise SystemExit(f"kernel source bytes mismatch: {source['path']}")
    if identity['live_probe_status'] != 'not executed: upload authorization pending':
        raise SystemExit('unexpected authority-probe status for this scoped bundle')
    if 'G7_NO_FIXTURE_UPLOADED_OR_CHILD_STARTED' not in (bundle / 'pre-delete-audit.log').read_text():
        raise SystemExit('missing pre-delete scope assertion')
    with tarfile.open(bundle / "tested-source.tar.gz") as archive:
        members = archive.getmembers()
        if len({m.name for m in members}) != len(members):
            raise SystemExit("duplicate source archive member")
        if {m.name for m in members} != {s['path'] for s in sources}:
            raise SystemExit("source archive inventory mismatch")
        for source in sources:
            member = archive.getmember(source['path'])
            if not member.isfile():
                raise SystemExit("source member is not a regular file")
            data = archive.extractfile(member).read()
            if digest(data) != source['sha256'] or len(data) != source['size_bytes']:
                raise SystemExit(f"source bytes mismatch: {source['path']}")
    before = json.loads((bundle / "resources-before.json").read_text())
    after = json.loads((bundle / "resources-after.json").read_text())
    before.pop('captured_at', None)
    after.pop('captured_at', None)
    if before != after:
        raise SystemExit("project resource inventories differ")
    for name in ['instance-after-delete.json', 'disk-after-delete.json']:
        if json.loads((bundle / name).read_text()) != []:
            raise SystemExit(f"test resource remains: {name}")
    print(json.dumps({'source_archives_verified': True,
                      'source_file_count': len(sources),
                      'kernel_source_file_count': len(audit['files']),
                      'cloud_cleanup_verified': True,
                      'live_authority_probes_executed': False,
                      'g7_qualified': False}, indent=2))


if __name__ == '__main__':
    main()
