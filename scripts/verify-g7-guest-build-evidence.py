#!/usr/bin/env python3
"""Verify guest-local builds, OCI contents, authority and cleanup offline."""
import base64
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import sys
import tarfile


def sha(data):
    return hashlib.sha256(data).hexdigest()


def verify_image(data):
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        def read(name):
            member = archive.getmember(name)
            assert member.isfile(), name
            return archive.extractfile(member).read()

        def blob(descriptor):
            algorithm, digest = descriptor['digest'].split(':')
            assert algorithm == 'sha256'
            content = read('blobs/sha256/' + digest)
            assert sha(content) == digest and len(content) == descriptor['size']
            return content

        assert json.loads(read('oci-layout'))['imageLayoutVersion'] == '1.0.0'
        index = json.loads(read('index.json'))
        assert index['schemaVersion'] == 2
        images = [d for d in index['manifests']
                  if d.get('platform', {}).get('os', 'linux') == 'linux']
        assert len(images) == 1
        manifest = json.loads(blob(images[0]))
        config = json.loads(blob(manifest['config']))
        assert config['os'] == 'linux' and config['architecture'] == 'amd64'
        assert len(manifest['layers']) == len(config['rootfs']['diff_ids']) == 1
        layer = blob(manifest['layers'][0])
        if layer[:2] == b'\x1f\x8b':
            layer = gzip.decompress(layer)
        assert 'sha256:' + sha(layer) == config['rootfs']['diff_ids'][0]
        with tarfile.open(fileobj=io.BytesIO(layer)) as filesystem:
            result = filesystem.getmember('result')
            assert result.isfile() and result.mode == 0o644
            assert filesystem.extractfile(result).read() == b'multikernel-guest-build\n'
    return sha(data)


def verify_integrity_and_sources(bundle):
    recorded = set()
    for line in (bundle / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        recorded.add(name)
        assert sha((bundle / name).read_bytes()) == digest, name
    assert recorded == {p.relative_to(bundle).as_posix() for p in bundle.rglob('*')
                        if p.is_file() and p.name != 'SHA256SUMS'}
    identity = json.loads((bundle / 'source-identity.json').read_text())
    sources = {entry['path']: entry['sha256']
               for entry in json.loads((bundle / 'source-hashes.json').read_text())}
    for name, field in [('tested-source.tar.gz', 'source_archive_sha256'),
                        ('fixtures.tar.gz', 'fixture_archive_sha256')]:
        assert sha((bundle / name).read_bytes()) == identity[field]
        with tarfile.open(bundle / name) as archive:
            for member in archive.getmembers():
                assert member.isfile() and member.name in sources
                assert sha(archive.extractfile(member).read()) == sources[member.name]
            if name == 'tested-source.tar.gz':
                assert {m.name for m in archive.getmembers()} == set(sources)
    for attempt in json.loads((bundle / 'attempt-history.json').read_text()):
        assert sha((bundle / attempt['fixture_archive']).read_bytes()) == attempt['sha256']


def main():
    bundle = Path(sys.argv[1])
    verify_integrity_and_sources(bundle)
    baseline = json.loads((bundle / 'host/final/baseline-result.json').read_text())['WaitProcess']
    assert baseline.get('exit_code', 0) == 0
    assert 'BASELINE_START_EXIT=1' in baseline['stdout']
    assert 'operation not permitted' in baseline['stdout']
    result = json.loads((bundle / 'host/final/prepared-result.json').read_text())['WaitProcess']
    assert result['status'] == 'STOPPED' and result.get('exit_code', 0) == 0
    stdout = result['stdout']
    for marker in ['MULTISTAGE_BUILD_PASSED', 'REPEAT_BUILD_PASSED',
                   'FAILED_BUILD_REJECTED', 'RESTRICTED_DEVICES_PASSED', 'BUILD_GUEST_PROBE_END']:
        assert marker in stdout, marker
    for field in ['KERNEL_COMMAND_LINE_TOKEN_VISIBLE', 'AGENT_ARGUMENT_TOKEN_VISIBLE',
                  'CHILD_BOOTSTRAP_CANARY_READ', 'CHILD_BOOTSTRAP_CANARY_WRITE']:
        assert field + '=no' in stdout
    assert 'CapEff:\t0000000000000000' in stdout and 'CapBnd:\t0000000000000000' in stdout
    assert 'NoNewPrivs:\t1' in stdout
    assert 'CACHED' in result['stderr'], 'repeat build did not demonstrate caching'
    encoded = stdout.split('OCI_IMAGE_BASE64_BEGIN\n', 1)[1].split('OCI_IMAGE_BASE64_END', 1)[0]
    image = base64.b64decode(''.join(encoded.split()), validate=True)
    image_hash = verify_image(image)
    assert image_hash + '  /work/image.tar' in stdout
    assert (bundle / 'built-image.oci.tar').read_bytes() == image
    console = (bundle / 'host/final/prepared-console.log').read_text()
    authority = console.split('MK_GUEST_BUILDER_AUTHORITY_BEGIN', 1)[1].split('MK_GUEST_BUILDER_AUTHORITY_END', 1)[0]
    assert re.search(r'Uid:\s+1000\s+1000\s+1000\s+1000', authority)
    assert re.search(r'NoNewPrivs:\s+1', authority)
    assert re.search(r'0\s+1000\s+1', authority)
    assert re.search(r'1\s+100000\s+65536', authority)
    assert not re.search(r'mk.token=\S+|--token-hex=\S+', console)
    for case in ['baseline', 'prepared']:
        assert 'PRIVATE_CREDENTIAL_LOAD_COMPLETED' in (bundle / f'host/final/{case}-kerf.log').read_text()
        assert 'Deleted instance' in (bundle / f'host/final/{case}-cleanup.txt').read_text()
    inventory = json.loads((bundle / 'host/final/final-inventory.json').read_text())
    assert inventory == {'online_cpus': '0-15', 'active_test_children': 0, 'fixture_count': 2}
    assert 'CLEANUP_ASSERTIONS_PASSED cpus=0-15 children=0 helpers=0' in (bundle / 'pre-delete-audit.log').read_text()
    before = json.loads((bundle / 'resources-before.json').read_text())
    after = json.loads((bundle / 'resources-after.json').read_text())
    before.pop('captured_at'); after.pop('captured_at')
    assert before == after
    for name in ['deleted-instance-query.json', 'deleted-disk-query.json']:
        assert json.loads((bundle / name).read_text()) == []
    print(json.dumps({'guest_multistage_build': True, 'cache_hit': True,
                      'failed_run_rejected': True, 'oci_contents_verified': True,
                      'image_sha256': image_hash, 'workload_caps_zero_nnp_enabled': True,
                      'builder_uid_mapped_to_1000_nnp_enabled': True,
                      'tested_canary_and_credentials_protected': True,
                      'cloud_cleanup_verified': True}, indent=2))


if __name__ == '__main__':
    main()
