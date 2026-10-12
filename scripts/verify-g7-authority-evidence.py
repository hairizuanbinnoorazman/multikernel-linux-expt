#!/usr/bin/env python3
"""Verify scoped G7 live authority observations, source bytes, and cleanup."""
import hashlib
import json
from pathlib import Path
import sys
import tarfile


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    bundle = Path(sys.argv[1])
    identity = json.loads((bundle / 'source-identity.json').read_text())
    sources = json.loads((bundle / 'source-hashes.json').read_text())
    assert digest((bundle / 'tested-source.tar.gz').read_bytes()) == identity['source_archive_sha256']
    assert digest((bundle / 'fixtures-v4.tar.gz').read_bytes()) == identity['fixture_archive_sha256']
    with tarfile.open(bundle / 'tested-source.tar.gz') as archive:
        members = archive.getmembers()
        assert len({m.name for m in members}) == len(members)
        assert {m.name for m in members} == {s['path'] for s in sources}
        for source in sources:
            member = archive.getmember(source['path'])
            assert member.isfile()
            data = archive.extractfile(member).read()
            assert len(data) == source['size_bytes']
            assert digest(data) == source['sha256']
    source_hashes = {s['path']: s['sha256'] for s in sources}
    with tarfile.open(bundle / 'fixtures-v4.tar.gz') as archive:
        for member in archive.getmembers():
            assert member.isfile()
            assert digest(archive.extractfile(member).read()) == source_hashes[member.name]
    kernel_archive = bundle / identity['kernel_source_reference']
    assert digest(kernel_archive.read_bytes()) == identity['kernel_source_archive_sha256']
    observations = json.loads((bundle / 'observations.json').read_text())
    assert [o['case'] for o in observations] == ['nonroot', 'root-empty-caps', 'root-default']
    for row in observations:
        case = row['case']
        raw = json.loads((bundle / 'host/authority-completed' / (case + '-result.json')).read_text())
        state = raw['WaitProcess']
        assert state['status'] == row['status'] == 'STOPPED'
        assert state.get('exit_code', 0) == row['exit_code'] == 0
        assert 'SECURITY_AUTHORITY_END' in state['stdout']
        for field, value in row['observations'].items():
            line = next(line for line in state['stdout'].splitlines()
                        if line.startswith(field + ':') or line.startswith(field + '='))
            actual = line.split(':', 1)[1].strip() if ':' in line else line.split('=', 1)[1]
            assert value == actual, field
        fields = row['observations']
        assert fields['KERNEL_COMMAND_LINE_TOKEN_VISIBLE'] == 'yes'
        assert fields['AGENT_ARGUMENT_TOKEN_VISIBLE'] == 'yes'
        expected = 'yes' if case == 'root-default' else 'no'
        assert fields['CHILD_BOOTSTRAP_CANARY_READ'] == expected
        assert fields['CHILD_BOOTSTRAP_CANARY_WRITE'] == expected
        assert fields['CapEff'] == ('000001ffffffffff' if case == 'root-default' else '0000000000000000')
        assert fields['NoNewPrivs'] == ('1' if case == 'root-empty-caps' else '0')
        cleanup = (bundle / 'host/authority-completed' / (case + '-cleanup.txt')).read_text()
        assert 'Deleted instance' in cleanup and 'Pool updated' in cleanup
    final = json.loads((bundle / 'host/authority-completed/final-inventory.json').read_text())
    assert final == {'online_cpus': '0-15', 'active_test_children': 0, 'fixture_count': 3}
    audit = (bundle / 'pre-delete-audit-v2.log').read_text()
    assert 'No memory pool configured' in audit
    assert 'CLEANUP_ASSERTIONS_PASSED cpus=0-15 children=0 helpers=0' in audit
    before = json.loads((bundle / 'resources-before.json').read_text())
    after = json.loads((bundle / 'resources-after.json').read_text())
    before.pop('captured_at'); after.pop('captured_at')
    assert before == after, 'cloud resources differ from baseline'
    for name in ['deleted-instance-query.json', 'deleted-disk-query.json']:
        assert json.loads((bundle / name).read_text()) == []
    print(json.dumps({'source_bytes_verified': len(sources), 'fixture_profiles_verified': 3,
                      'credential_exposure_confirmed': True,
                      'default_root_child_bootstrap_access_confirmed': True,
                      'cloud_cleanup_verified': True}, indent=2))


if __name__ == '__main__':
    main()
