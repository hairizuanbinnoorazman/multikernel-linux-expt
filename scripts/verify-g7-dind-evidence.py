#!/usr/bin/env python3
"""Independently decode Docker save output and check nested Docker evidence."""
import base64
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import runpy
import sys
import tarfile


def sha(data):
    return hashlib.sha256(data).hexdigest()


def verify_saved_image(data, inspected):
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:*') as archive:
        def read(name):
            entry = archive.getmember(name)
            assert entry.isfile()
            return archive.extractfile(entry).read()
        manifests = json.loads(read('manifest.json'))
        assert len(manifests) == 1
        manifest = manifests[0]
        assert manifest['RepoTags'] == ['guest-proof:latest']
        configuration = read(manifest['Config'])
        assert inspected['Id'] == 'sha256:' + sha(configuration)
        config = json.loads(configuration)
        assert config['os'] == 'linux' and config['architecture'] == 'amd64'
        assert config['config']['Cmd'] == ['/bin/busybox', 'cat', '/result']
        assert len(manifest['Layers']) == len(config['rootfs']['diff_ids'])
        result_found = busybox_found = False
        for name, diff_id in zip(manifest['Layers'], config['rootfs']['diff_ids']):
            layer = read(name)
            if layer[:2] == b'\x1f\x8b':
                layer = gzip.decompress(layer)
            assert diff_id == 'sha256:' + sha(layer)
            with tarfile.open(fileobj=io.BytesIO(layer)) as filesystem:
                for entry in filesystem.getmembers():
                    if entry.name.lstrip('./') == 'result':
                        assert entry.isfile() and entry.mode == 0o644
                        assert filesystem.extractfile(entry).read() == b'multikernel-docker-build\n'
                        result_found = True
                    if entry.name.lstrip('./') == 'bin/busybox':
                        assert entry.isfile() and entry.mode & 0o111
                        assert filesystem.extractfile(entry).read(4) == b'\x7fELF'
                        busybox_found = True
        assert result_found and busybox_found
    return sha(data)


def main():
    bundle = Path(sys.argv[1])
    shared = runpy.run_path(str(Path(__file__).with_name('verify-g7-guest-build-evidence.py')))
    shared['verify_integrity_and_sources'](bundle)
    baseline = json.loads((bundle / 'host/final/baseline-result.json').read_text())['WaitProcess']
    assert baseline.get('exit_code', 0) == 0
    assert 'BASELINE_START_EXIT=1' in baseline['stdout']
    assert "can't create unix socket /var/run/docker.sock" in baseline['stdout']
    assert 'operation not permitted' in baseline['stdout']
    result = json.loads((bundle / 'host/final/prepared-result.json').read_text())['WaitProcess']
    assert result['status'] == 'STOPPED' and result.get('exit_code', 0) == 0
    stdout, stderr = result['stdout'], result['stderr']
    for marker in ['DOCKER_DAEMON_INFO_PASSED', 'MULTISTAGE_BUILD_PASSED',
                   'REPEAT_BUILD_PASSED', 'NESTED_CONTAINER_RUN_PASSED',
                   'FAILED_BUILD_REJECTED', 'DOCKER_TEST_RESOURCES_REMOVED', 'BUILD_GUEST_PROBE_END']:
        assert marker in stdout, marker
    assert 'CapEff:\t0000000000000000' in stdout
    assert 'CapBnd:\t0000000000000000' in stdout and 'NoNewPrivs:\t1' in stdout
    for field in ['KERNEL_COMMAND_LINE_TOKEN_VISIBLE', 'AGENT_ARGUMENT_TOKEN_VISIBLE',
                  'CHILD_BOOTSTRAP_CANARY_READ', 'CHILD_BOOTSTRAP_CANARY_WRITE']:
        assert field + '=no' in stdout
    for path in ['/dev/mem', '/dev/port', '/dev/kmsg', '/dev/nbd0']:
        assert f'PATH_AUTHORITY path={path} exists=no readable=no writable=no' in stdout
    assert 'CACHED' in stderr
    objects = []
    for line in stdout.splitlines():
        if line.startswith('{'):
            objects.append(json.loads(line))
    info = next(value for value in objects if 'KernelVersion' in value)
    assert info['KernelVersion'] == '7.0.0-mk2-gce-lab'
    assert info['ServerVersion'] == '29.9.0' and info['Driver'] == 'vfs'
    assert 'name=rootless' in info['SecurityOptions']
    assert info['DockerRootDir'] == '/home/rootless/.local/share/docker'
    inspected = json.loads(stdout.split('DOCKER_IMAGE_INSPECT_BEGIN\n', 1)[1]
                           .split('DOCKER_IMAGE_INSPECT_END', 1)[0])[0]
    encoded = stdout.split('DOCKER_IMAGE_BASE64_BEGIN\n', 1)[1].split('DOCKER_IMAGE_BASE64_END', 1)[0]
    data = base64.b64decode(''.join(encoded.split()), validate=True)
    digest = verify_saved_image(data, inspected)
    assert digest + '  /work/image.tar.gz' in stdout
    assert (bundle / 'built-image.docker.tar.gz').read_bytes() == data
    nested = stdout.split('NESTED_CONTAINER_AUTHORITY_BEGIN\n', 1)[1].split('NESTED_CONTAINER_AUTHORITY_END', 1)[0]
    assert 'NoNewPrivs:\t1' in nested and 'Seccomp:\t2' in nested
    assert re.search(r'0\s+1000\s+1', nested)
    console = (bundle / 'host/final/prepared-console.log').read_text()
    authority = console.split('MK_GUEST_DOCKER_AUTHORITY_BEGIN', 1)[1].split('MK_GUEST_DOCKER_AUTHORITY_END', 1)[0]
    assert re.search(r'Uid:\s+1000\s+1000\s+1000\s+1000', authority)
    assert re.search(r'NoNewPrivs:\s+1', authority)
    assert re.search(r'0\s+1000\s+1', authority)
    assert re.search(r'1\s+100000\s+65536', authority)
    assert not re.search(r'mk.token=\S+|--token-hex=\S+', console)
    for case in ['baseline', 'prepared']:
        assert 'PRIVATE_CREDENTIAL_LOAD_COMPLETED' in (bundle / f'host/final/{case}-kerf.log').read_text()
        assert 'Deleted instance' in (bundle / f'host/final/{case}-cleanup.txt').read_text()
    final = json.loads((bundle / 'host/final/final-inventory.json').read_text())
    assert final == {'online_cpus': '0-15', 'active_test_children': 0, 'fixture_count': 2}
    assert 'CLEANUP_ASSERTIONS_PASSED cpus=0-15 children=0 helpers=0' in (bundle / 'pre-delete-audit.log').read_text()
    before = json.loads((bundle / 'resources-before.json').read_text())
    after = json.loads((bundle / 'resources-after.json').read_text())
    before.pop('captured_at'); after.pop('captured_at')
    assert before == after
    for name in ['deleted-instance-query.json', 'deleted-disk-query.json']:
        assert json.loads((bundle / name).read_text()) == []
    print(json.dumps({'docker_daemon_rootless': True, 'docker_build_multistage': True,
                      'docker_build_cache_hit': True, 'docker_run_nested_image': True,
                      'docker_save_contents_verified': True, 'image_sha256': digest,
                      'failed_build_rejected': True, 'outer_client_caps_zero_nnp': True,
                      'daemon_maps_root_to_guest_1000_nnp': True,
                      'inner_container_nnp_and_seccomp': True,
                      'tested_credentials_canary_devices_protected': True,
                      'cloud_cleanup_verified': True}, indent=2))


if __name__ == '__main__':
    main()
