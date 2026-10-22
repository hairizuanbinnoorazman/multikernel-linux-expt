"""Disposable two-child build overlap and filesystem/engine separation checks."""
import json
import hashlib
import re
import secrets
import subprocess
import time


def run_markers(text):
    # Match actual BuildKit RUN output, never the echoed Dockerfile instruction.
    return {match[1] for match in re.finditer(
        r'^#\d+\s+\d+(?:\.\d+)?\s+G8_OVERLAP_RUN_(BEGIN|END)\s*$', text, re.M)}


def owner_for_container(container_id, owners):
    if not re.fullmatch('[0-9a-f]{64}', container_id):
        raise ValueError('expected canonical Docker container identity')
    # This fixture uses Docker's moby namespace and the runtime's bounded tuple ID.
    expected = 'mk-' + container_id[:32] + '-' + hashlib.sha256(('moby\0' + container_id).encode()).hexdigest()[:16]
    matches = [owner for owner in owners if owner['sandbox_id'] == expected]
    if len(matches) != 1:
        raise ValueError('Docker container has no unique recorded sandbox owner')
    return matches[0]


def qualify(cli, private, children, create_arguments, inspect_arguments, kernel_release):
    names = ['g8-materialization', 'g8-sibling']
    cases = ['a', 'b']
    canaries = {case: secrets.token_hex(16) for case in cases}
    processes = {}
    streams = []
    sibling_created = False

    def invoke(arguments, label, success=True):
        result = subprocess.run([str(x) for x in arguments], stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=100)
        assert len(result.stdout) < 131072 and len(result.stderr) < 131072
        (private / (label + '.stdout')).write_bytes(result.stdout)
        (private / (label + '.stderr')).write_bytes(result.stderr)
        if success:
            assert result.returncode == 0, label + ': ' + result.stderr.decode(errors='replace')
        return result

    def execute(index, arguments, label, success=True):
        return invoke(cli + ['exec', '--user', '1000:1000', names[index]] + arguments,
                      label, success)

    def wait_for(marker, limit=360):
        deadline = time.monotonic() + limit
        while True:
            if all(marker in (private / (case + '-workload.stdout')).read_text() for case in cases):
                return
            assert all(p.poll() is None for p in processes.values()), 'build client stopped before barrier'
            assert time.monotonic() < deadline, 'concurrent workload barrier deadline exceeded'
            time.sleep(.2)

    def storage_snapshot(phase, count):
        exports = json.loads((private / 'storage-state/state.json').read_text())['exports']
        active = [value for value in exports.values() if value['state'] == 'ACTIVE']
        assert len(active) == count
        result = []
        for value in active:
            import os
            observed = os.stat(value['path'])
            assert value['image_identity'] == {'device': observed.st_dev, 'inode': observed.st_ino}
            result.append({'peer_cid': value['peer_cid'], 'port': value['port'],
                           'filesystem_uuid': value['filesystem_uuid'], 'image_identity': value['image_identity'],
                           'image_path_sha256': hashlib.sha256(value['path'].encode()).hexdigest()})
        assert len({(v['image_identity']['device'], v['image_identity']['inode']) for v in result}) == count
        assert len({v['image_path_sha256'] for v in result}) == count
        assert len({v['port'] for v in result}) == count
        (private / (phase + '-storage.json')).write_text(json.dumps(result, indent=2) + '\n')
        return result

    def snapshot(index, phase, built=False):
        case = cases[index]
        prefix = phase + '-' + case
        boot = execute(index, ['/bin/cat', '/proc/sys/kernel/random/boot_id'], prefix + '-boot').stdout.decode().strip()
        engine = json.loads(execute(index, ['/usr/local/bin/docker', 'info', '--format', '{{json .}}'], prefix + '-engine').stdout)
        canary = execute(index, ['/bin/cat', '/home/rootless/.local/tmp/g8-canary'], prefix + '-canary').stdout.decode().strip()
        own = execute(index, ['/usr/local/bin/docker', 'image', 'inspect', '--format', '{{.Id}}', 'g8-isolation:' + case], prefix + '-own-image').stdout.decode().strip()
        foreign_case = cases[1-index]
        foreign = execute(index, ['/usr/local/bin/docker', 'image', 'inspect', 'g8-isolation:' + foreign_case], prefix + '-foreign-image', False)
        assert foreign.returncode != 0 and b'No such image' in foreign.stderr and ('g8-isolation:' + foreign_case).encode() in foreign.stderr
        assert canary == canaries[case] and engine['ID'] and engine['KernelVersion'] == kernel_release
        assert engine['ServerVersion'] == '29.9.0' and 'name=rootless' in engine['SecurityOptions']
        value = {'child_boot_id': boot, 'engine_id': engine['ID'], 'canary': canary, 'sentinel_image_id': own}
        if built:
            value['saved_image_sha256'] = execute(index, ['/usr/bin/sha256sum', '/home/rootless/.local/tmp/g8-build/image.tar'], prefix + '-saved-image').stdout.decode().split()[0]
        return value

    try:
        sibling = list(create_arguments)
        sibling[sibling.index('g8-materialization')] = names[1]
        invoke(sibling, 'sibling-create')
        sibling_created = True
        for name, case in zip(names, cases):
            out = (private / (case + '-workload.stdout')).open('wb')
            err = (private / (case + '-workload.stderr')).open('wb')
            streams += [out, err]
            process = subprocess.Popen(cli + ['start', '--attach', name], stdout=out, stderr=err)
            children.append(process)
            processes[case] = process
        wait_for('G8_CONCURRENT_BOOT_READY')
        for index, case in enumerate(cases):
            seed = 'mkdir -m 700 /home/rootless/.local/tmp/g8-sentinel; printf "%s\\n" "$1" > /home/rootless/.local/tmp/g8-canary; printf "FROM scratch\\nLABEL g8.case=%s\\n" "$1" > /home/rootless/.local/tmp/g8-sentinel/Dockerfile; docker build --progress=plain --network=none --pull=false -t "g8-isolation:$2" /home/rootless/.local/tmp/g8-sentinel'
            execute(index, ['/bin/sh', '-ec', seed, 'seed', canaries[case], case], 'seed-' + case)
        before = {case: snapshot(index, 'before') for index, case in enumerate(cases)}
        for field in ['child_boot_id', 'engine_id', 'canary', 'sentinel_image_id']:
            assert before['a'][field] != before['b'][field], field + ' is shared'
        owners = json.loads(invoke(inspect_arguments, 'two-child-ownership').stdout)
        assert len(owners['sandboxes']) == 2 and all(s['state'] == 'RUNNING' for s in owners['sandboxes'])
        assert {s['child_cid'] for s in owners['sandboxes']} == {40, 41}
        assert set(owners['sandboxes'][0]['cpus']).isdisjoint(owners['sandboxes'][1]['cpus'])
        case_owners = {}
        for name, case in zip(names, cases):
            inspected = json.loads(invoke(cli + ['inspect', name], 'owner-map-' + case).stdout)[0]
            case_owners[case] = owner_for_container(inspected['Id'], owners['sandboxes'])
        assert case_owners['a']['sandbox_id'] != case_owners['b']['sandbox_id']
        storage_before = storage_snapshot('two-child', 2)
        assert {v['peer_cid'] for v in storage_before} == {40, 41}
        for index, case in enumerate(cases):
            execute(index, ['/bin/touch', '/home/rootless/.local/tmp/g8-concurrency/build'], 'release-build-' + case)
        deadline = time.monotonic() + 180
        while True:
            markers = {case: run_markers((private / (case + '-workload.stderr')).read_text()) for case in cases}
            assert not any('END' in value for value in markers.values()), 'RUN overlap was not observed'
            if all('BEGIN' in value for value in markers.values()):
                overlap = {'observed_monotonic_ns': time.monotonic_ns(), 'markers': {k: sorted(v) for k, v in markers.items()}}
                (private / 'run-overlap.json').write_text(json.dumps(overlap, indent=2) + '\n')
                break
            assert all(p.poll() is None for p in processes.values()) and time.monotonic() < deadline
            time.sleep(.1)
        wait_for('G8_CONCURRENT_BUILD_READY')
        after = {case: snapshot(index, 'after', True) for index, case in enumerate(cases)}
        for case in cases:
            assert {k: after[case][k] for k in before[case]} == before[case]
            output = (private / (case + '-workload.stdout')).read_text()
            errors = (private / (case + '-workload.stderr')).read_text()
            for marker in ['G8_MULTISTAGE_BUILD_PASS', 'G8_REPEAT_BUILD_PASS', 'G8_BUILT_IMAGE_RUN_PASS', 'G8_FAILED_BUILD_REJECTED', 'G8_BUILD_WORKLOAD_PASS']:
                assert marker in output
            assert 'CACHED' in errors and 'exit code: 37' in errors
            assert after[case]['saved_image_sha256'] in output
        execute(1, ['/bin/touch', '/home/rootless/.local/tmp/g8-concurrency/release'], 'release-sibling')
        assert processes['b'].wait(timeout=60) == 0
        inspected = json.loads(invoke(cli + ['inspect', names[1]], 'sibling-inspect').stdout)[0]
        assert inspected['State']['ExitCode'] == 0
        invoke(cli + ['rm', names[1]], 'sibling-remove')
        sibling_created = False
        storage_survivor = storage_snapshot('survivor', 1)
        assert storage_survivor == [v for v in storage_before if v['peer_cid'] == case_owners['a']['child_cid']]
        survivor = snapshot(0, 'survivor', True)
        assert survivor == after['a'] and processes['a'].poll() is None
        remaining = json.loads(invoke(inspect_arguments, 'survivor-ownership').stdout)
        active = [s for s in remaining['sandboxes'] if s['state'] != 'ABSENT']
        assert active == [case_owners['a']]
        result = {'before': before, 'after': after, 'survivor': survivor,
                  'two_running_owners': owners['sandboxes'], 'case_owners': case_owners, 'remaining_owner': active[0],
                  'two_storage_exports': storage_before, 'survivor_storage_export': storage_survivor,
                  'run_overlap_observed': True, 'sibling_removed_without_survivor_change': True,
                  'cleanup_qualified': False}
        (private / 'concurrency-checkpoint.json').write_text(json.dumps(result, indent=2) + '\n')
        execute(0, ['/bin/touch', '/home/rootless/.local/tmp/g8-concurrency/release'], 'release-primary')
        assert processes['a'].wait(timeout=60) == 0
        return subprocess.CompletedProcess(processes['a'].args, 0)
    finally:
        if sibling_created:
            invoke(cli + ['rm', '--force', names[1]], 'sibling-cleanup', False)
        for stream in streams:
            stream.close()
