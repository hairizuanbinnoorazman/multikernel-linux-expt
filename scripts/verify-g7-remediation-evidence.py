#!/usr/bin/env python3
"""Check the bounded G7 credential/privilege remediation evidence, offline."""
import hashlib
import json
from pathlib import Path
import re
import sys
import tarfile


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    bundle = Path(sys.argv[1])
    identity = json.loads((bundle / 'source-identity.json').read_text())
    sources = json.loads((bundle / 'source-hashes.json').read_text())
    hashes = {s['path']: s['sha256'] for s in sources}
    assert sha((bundle / 'tested-source.tar.gz').read_bytes()) == identity['source_archive_sha256']
    with tarfile.open(bundle / 'tested-source.tar.gz') as archive:
        members = archive.getmembers()
        assert len(members) == len(hashes) and {m.name for m in members} == set(hashes)
        for member in members:
            assert member.isfile()
            assert sha(archive.extractfile(member).read()) == hashes[member.name]
    assert sha((bundle / 'fixtures.tar.gz').read_bytes()) == identity['fixture_archive_sha256']
    with tarfile.open(bundle / 'fixtures.tar.gz') as archive:
        for member in archive.getmembers():
            assert member.isfile()
            assert sha(archive.extractfile(member).read()) == hashes[member.name]
    profiles = ['nonroot', 'root-empty-caps', 'root-default', 'root-ordinary-caps']
    evidence = bundle / 'host/remediation-rerun'
    for case in profiles:
        state = json.loads((evidence / (case + '-result.json')).read_text())['WaitProcess']
        assert state['status'] == 'STOPPED' and state.get('exit_code', 0) == 0
        stdout = state['stdout']
        assert 'SECURITY_AUTHORITY_END' in stdout
        for field in ['KERNEL_COMMAND_LINE_TOKEN_VISIBLE', 'AGENT_ARGUMENT_TOKEN_VISIBLE',
                      'CHILD_BOOTSTRAP_CANARY_READ', 'CHILD_BOOTSTRAP_CANARY_WRITE']:
            assert field + '=no' in stdout
        fields = dict(line.split(':', 1) for line in stdout.splitlines()
                      if line.startswith(('CapEff:', 'CapBnd:', 'NoNewPrivs:')))
        assert fields['NoNewPrivs'].strip() == '1'
        effective = '00000000a80425fb' if case == 'root-ordinary-caps' else '0000000000000000'
        assert fields['CapEff'].strip() == effective
        expected_bound = '000001ffffffffff' if case == 'nonroot' else effective
        assert fields['CapBnd'].strip() == expected_bound
        for path in ['/dev/mem', '/dev/port', '/dev/kmsg', '/dev/nbd0']:
            assert f'PATH_AUTHORITY path={path} exists=no readable=no writable=no' in stdout
        console = (evidence / (case + '-console.log')).read_text()
        assert 'MK_AGENT_START' in console
        assert not re.search(r'mk.token=\S+|--token-hex=\S+', console)
    rejected = ['root-unsafe-cap', 'root-disable-nnp']
    for case in rejected:
        result = json.loads((evidence / (case + '-rejection.json')).read_text())
        assert result == {'case': case, 'client_exit_status': 1, 'rejected_before_start': True}
        assert 'CreateProcess: INTERNAL: guest process creation failed at bundle-load' in (evidence / (case + '-control.err')).read_text()
    for case in profiles + rejected:
        digest = (evidence / (case + '-loaded-initramfs.sha256')).read_text().split()[0]
        assert re.fullmatch('[a-f0-9]{64}', digest)
        assert 'PRIVATE_CREDENTIAL_LOAD_COMPLETED' in (evidence / (case + '-kerf.log')).read_text()
        assert 'Deleted instance' in (evidence / (case + '-cleanup.txt')).read_text()
    final = json.loads((evidence / 'final-inventory.json').read_text())
    assert final == {'online_cpus': '0-15', 'active_test_children': 0, 'fixture_count': 6}
    audit = (bundle / 'pre-delete-audit.log').read_text()
    assert 'No memory pool configured' in audit
    assert 'CLEANUP_ASSERTIONS_PASSED cpus=0-15 children=0 helpers=0' in audit
    before = json.loads((bundle / 'resources-before.json').read_text())
    after = json.loads((bundle / 'resources-after.json').read_text())
    before.pop('captured_at'); after.pop('captured_at')
    assert before == after, 'cloud resources differ from baseline'
    for name in ['deleted-instance-query.json', 'deleted-disk-query.json']:
        assert json.loads((bundle / name).read_text()) == []
    print(json.dumps({'source_files_verified': len(sources), 'successful_workload_profiles': 4,
                      'fail_closed_requests': 2, 'tested_credential_paths_protected': True,
                      'tested_child_bootstrap_canary_access_denied': True,
                      'production_kerf_credential_handoff_exercised': True,
                      'cloud_cleanup_verified': True}, indent=2))


if __name__ == '__main__':
    main()
