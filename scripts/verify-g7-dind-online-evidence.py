#!/usr/bin/env python3
"""Verify scoped online Docker guest receipts, provenance and cleanup offline."""
import json
from pathlib import Path
import re
import runpy
import sys
from urllib.parse import urlparse


def section(text, marker):
    return text.split(marker + '_BEGIN\n', 1)[1].split(marker + '_END', 1)[0]


def main():
    bundle = Path(sys.argv[1])
    shared = runpy.run_path(str(Path(__file__).with_name('verify-g7-guest-build-evidence.py')))
    shared['verify_integrity_and_sources'](bundle)
    receipt = json.loads((bundle / 'host/final/prepared-result.json').read_text())
    result = receipt['WaitProcess']
    assert result['status'] == 'STOPPED' and result.get('exit_code', 0) == 0
    stdout, stderr = result['stdout'], result['stderr']
    for marker in ['EMPTY_DAEMON_IMAGE_STORE_CONFIRMED', 'PYTHON_DOCKERHUB_PULL_PASSED',
                   'BASE_REQUESTS_ABSENT_CONFIRMED',
                   'ONLINE_PYTHON_BUILD_PASSED', 'ONLINE_PYTHON_RUN_PASSED',
                   'ONLINE_PYTHON_CACHE_PASSED', 'ONLINE_DOCKER_RESOURCES_REMOVED',
                   'ONLINE_DOCKER_PROBE_END']:
        assert marker in stdout, marker
    assert 'CACHED' in stderr
    assert 'CapEff:\t0000000000000000' in stdout and 'CapBnd:\t0000000000000000' in stdout
    assert 'NoNewPrivs:\t1' in stdout
    info = next(json.loads(line) for line in stdout.splitlines()
                if line.startswith('{') and 'KernelVersion' in line)
    assert info['KernelVersion'] == '7.0.0-mk2-gce-lab'
    assert info['ServerVersion'] == '29.9.0' and info['Driver'] == 'vfs'
    assert 'name=rootless' in info['SecurityOptions']
    assert info['HttpProxy'] == info['HttpsProxy'] == ''
    for field in ['KERNEL_COMMAND_LINE_TOKEN_VISIBLE', 'AGENT_ARGUMENT_TOKEN_VISIBLE',
                  'CHILD_BOOTSTRAP_CANARY_READ', 'CHILD_BOOTSTRAP_CANARY_WRITE']:
        assert field + '=no' in stdout
    for path in ['/dev/mem', '/dev/port', '/dev/kmsg', '/dev/nbd0', '/dev/net/tun']:
        assert f'PATH_AUTHORITY path={path} exists=no readable=no writable=no' in stdout
    base = json.loads(section(stdout, 'PYTHON_BASE_INSPECT'))[0]
    assert any(re.fullmatch(r'python@sha256:[0-9a-f]{64}', d) for d in base['RepoDigests'])
    assert base['Architecture'] == 'amd64' and base['Os'] == 'linux'
    report = json.loads(section(stdout, 'PIP_REPORT'))
    installed = {item['metadata']['name'].lower().replace('_', '-'): item for item in report['install']}
    assert {'requests', 'urllib3', 'certifi', 'idna', 'charset-normalizer'} <= installed.keys()
    assert installed['requests']['metadata']['version'] == '2.34.2'
    releases = {item['name'].lower().replace('_', '-'): item
                for item in json.loads((bundle / 'pypi-release-hashes.json').read_text())}
    assert releases.keys() == installed.keys()
    for item in installed.values():
        download = item['download_info']
        url = urlparse(download['url'])
        assert url.scheme == 'https' and url.hostname == 'files.pythonhosted.org'
        assert re.fullmatch(r'[0-9a-f]{64}', download['archive_info']['hashes']['sha256'])
        release = releases[item['metadata']['name'].lower().replace('_', '-')]
        assert release['version'] == item['metadata']['version']
        assert release['source'].startswith('https://pypi.org/pypi/')
        published = next(f for f in release['files'] if f['url'] == download['url'])
        assert published['sha256'] == download['archive_info']['hashes']['sha256']
    app = json.loads(section(stdout, 'PYTHON_APP_RESULT'))
    assert app['app'] == 'multikernel-python-online'
    assert app['requests'] == '2.34.2' and app['https_status'] == 200
    assert app['source'] == 'https://pypi.org/pypi/requests/json' and app['tls_verification'] is True
    assert app['python'].startswith('3.13.')
    image = json.loads(section(stdout, 'PYTHON_IMAGE_INSPECT'))[0]
    assert image['Config']['Cmd'] == ['python', 'app.py']
    assert image['Config']['WorkingDir'] == '/app'
    for name, value in [('python-base-image.json', base), ('python-built-image.json', image),
                        ('pip-install-report.json', report), ('python-app-result.json', app)]:
        assert json.loads((bundle / name).read_text()) == value
    nested = section(stdout, 'PYTHON_INNER_AUTHORITY')
    assert 'NoNewPrivs:\t1' in nested and 'Seccomp:\t2' in nested
    assert re.search(r'0\s+1000\s+1', nested)
    console = (bundle / 'host/final/prepared-console.log').read_text()
    authority = console.split('MK_GUEST_DOCKER_AUTHORITY_BEGIN', 1)[1].split('MK_GUEST_DOCKER_AUTHORITY_END', 1)[0]
    assert re.search(r'Uid:\s+1000\s+1000\s+1000\s+1000', authority)
    assert re.search(r'NoNewPrivs:\s+1', authority)
    assert not re.search(r'mk.token=\S+|--token-hex=\S+', console)
    assert all(n > 0 for n in receipt['NetworkPacketCounts'].values())
    assert 'PRIVATE_CREDENTIAL_LOAD_COMPLETED' in (bundle / 'host/final/prepared-kerf.log').read_text()
    assert 'Deleted instance' in (bundle / 'host/final/prepared-cleanup.txt').read_text()
    final = json.loads((bundle / 'host/final/final-inventory.json').read_text())
    assert final == {'online_cpus': '0-15', 'active_test_children': 0, 'fixture_count': 1}
    assert 'CLEANUP_ASSERTIONS_PASSED cpus=0-15 children=0 helpers=0' in (bundle / 'pre-delete-audit.log').read_text()
    assert 'NETWORK_CLEANUP_ASSERTIONS_PASSED tun=absent rules=absent ip_forward=0' in (bundle / 'pre-delete-audit.log').read_text()
    rules = (bundle / 'host/final/primary-network-rules.txt').read_text()
    assert '-A INPUT -i mkonline0 -j DROP' in rules
    assert '-A MKONLINE7 -d 169.254.0.0/16 -j REJECT' in rules
    assert '--dport 443 -j ACCEPT' in rules and '--dport 53 -j ACCEPT' in rules
    after_rules = (bundle / 'host/final/primary-network-rules-after.txt').read_text()
    assert not re.search(r'MKONLINE7|mkonline0|10\.231\.7\.2', after_rules)
    before = json.loads((bundle / 'resources-before.json').read_text())
    after = json.loads((bundle / 'resources-after.json').read_text())
    before.pop('captured_at'); after.pop('captured_at')
    assert before == after
    for name in ['deleted-instance-query.json', 'deleted-disk-query.json']:
        assert json.loads((bundle / name).read_text()) == []
    print(json.dumps({'dockerhub_python_pull': True, 'base_repo_digests': base['RepoDigests'],
                      'pypi_package_downloads': {name: item['metadata']['version'] for name, item in sorted(installed.items())},
                      'online_build_and_run': True, 'runtime_verified_https': True,
                      'cache_hit': True, 'outer_caps_zero_nnp': True,
                      'nested_nnp_seccomp': True, 'scoped_cleanup_verified': True}, indent=2))


if __name__ == '__main__':
    main()
