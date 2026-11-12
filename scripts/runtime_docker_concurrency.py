"""Disposable two-child build overlap and filesystem/engine separation checks."""
import json
import hashlib
import re
import secrets
import subprocess
import time
from pathlib import Path
from runtime_trial_output import attach_command, read_output, require_logging
from runtime_primary_docker import observe as observe_primary,unchanged as unchanged_primary

# The authenticated signal targets the complete process group, including sleep.
# Its interruption must not trigger the wrapper's errexit before normal exit.
RELEASE_WAIT = 'while [ "$release" -eq 0 ]; do sleep 1 || :; done'


def run_markers(text):
    # Match actual BuildKit RUN output, never the echoed Dockerfile instruction.
    return {match[1] for match in re.finditer(
        r'^#\d+\s+\d+(?:\.\d+)?\s+G8_OVERLAP_RUN_(BEGIN|END)\s*$', text, re.M)}


def sandbox_id_for_container(container_id):
    if not re.fullmatch('[0-9a-f]{64}', container_id):
        raise ValueError('expected canonical Docker container identity')
    # This fixture uses Docker's moby namespace and the runtime's bounded tuple ID.
    return 'mk-' + container_id[:32] + '-' + hashlib.sha256(('moby\0' + container_id).encode()).hexdigest()[:16]


def owner_for_container(container_id, owners):
    expected = sandbox_id_for_container(container_id)
    matches = [owner for owner in owners if owner['sandbox_id'] == expected]
    if len(matches) != 1:
        raise ValueError('Docker container has no unique recorded sandbox owner')
    return matches[0]


def qualify(cli, private, children, create_arguments, inspect_arguments, kernel_release,boot,invocation):
    names = ['g8-materialization', 'g8-sibling']
    cases = ['a', 'b']
    canaries = {case: secrets.token_hex(16) for case in cases}
    processes = {}
    streams = []
    sibling_created = False
    primary_before=json.loads((private/'primary-docker-before.json').read_text())
    primary_hashes={}
    digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

    def primary(phase,image=None):
        report=observe_primary(image,deadline=time.monotonic()+10)
        unchanged_primary(primary_before,report)
        assert image is None or report['child_image_absent'],'child image appeared in primary Docker'
        path=private/('concurrency-primary-'+phase+'.json');path.write_text(json.dumps(report,indent=2)+'\n')
        primary_hashes[phase]=digest(path)

    def build_id(case,label=None):
        path=private/((label or case+'-workload')+'.stdout')
        values=re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',read_output(path),re.M)
        assert len(values)==1,'missing exact concurrent build image identity'
        return values[0].decode()

    def overlap_capture(phase):
        result={}
        for case in cases:
            raw=read_output(private/(case+'-workload.stderr'))
            assert len(raw)<=131072 and run_markers(raw.decode())=={'BEGIN'} and processes[case].poll() is None,'both RUN windows must remain unfinished'
            path=private/('run-overlap-'+phase+'-'+case+'.stderr');path.write_bytes(raw);result[case]=digest(path)
        return result

    def invoke(arguments, label, success=True, timeout=100):
        try:
            result = subprocess.run([str(x) for x in arguments], stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE, timeout=timeout)
        except subprocess.TimeoutExpired as error:
            # Preserve the original command's partial output before finally
            # starts cleanup. A timeout must never become an unrecorded retry.
            (private / (label + '.stdout')).write_bytes((error.stdout or b'')[:131072])
            (private / (label + '.stderr')).write_bytes((error.stderr or b'')[:131072])
            (private / (label + '-timeout.json')).write_text(json.dumps({
                'deadline_seconds': timeout, 'timed_out': True,
                'stdout_truncated': len(error.stdout or b'') > 131072,
                'stderr_truncated': len(error.stderr or b'') > 131072,
            }, indent=2) + '\n')
            raise
        assert len(result.stdout) < 131072 and len(result.stderr) < 131072
        (private / (label + '.stdout')).write_bytes(result.stdout)
        (private / (label + '.stderr')).write_bytes(result.stderr)
        (private / (label + '.exit.json')).write_text(json.dumps({'returncode':result.returncode})+'\n')
        if success:
            assert result.returncode == 0, label + ': ' + result.stderr.decode(errors='replace')
        return result

    def execute(index, arguments, label, success=True):
        return invoke(cli + ['exec', '--user', '1000:1000', names[index]] + arguments,
                      label, success)

    def diagnose_failure():
        # Keep the wrapper alive so diagnostics retain the original guest view.
        # No retry/export fallback can turn a failed original workload into pass.
        command = 'id; uname -r; df -k /home/rootless/.local/tmp; ls -lain /home/rootless/.local/tmp/g8-build; /usr/local/bin/docker --host=unix:///var/run/docker.sock info --format "{{json .}}"; /usr/local/bin/docker --host=unix:///var/run/docker.sock ps -a'
        for index, case in enumerate(cases):
            try:
                invoke(cli + ['exec', '--user', '1000:1000', names[index], '/bin/sh', '-c', command], 'failure-diagnostic-' + case, False, 25)
            except subprocess.TimeoutExpired as error:
                (private / ('failure-diagnostic-' + case + '.stdout')).write_bytes((error.stdout or b'')[:131072])
                (private / ('failure-diagnostic-' + case + '.stderr')).write_bytes((error.stderr or b'')[:131072] + b'\nDIAGNOSTIC_DEADLINE_EXCEEDED\n')

    def wait_for(marker, limit=360):
        deadline = time.monotonic() + limit
        while True:
            if all(marker in read_output(private / (case + '-workload.stdout')).decode() for case in cases):
                return
            output = {case: read_output(private / (case + '-workload.stdout')).decode() for case in cases}
            failed = any('G8_CONCURRENT_BUILD_FAILED' in value for value in output.values())
            if failed or not all(p.poll() is None for p in processes.values()):
                diagnose_failure()
                raise AssertionError('concurrent workload failed; bounded diagnostics retained')
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
            result.append({'sandbox_id':value['sandbox_id'],'sandbox_generation':value['sandbox_generation'],'export_generation':value['export_generation'],'peer_cid': value['peer_cid'], 'port': value['port'],
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
            value['built_image_id']=build_id(case,'survivor-rebuild' if phase=='survivor-rebuilt' else None)
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
            process = subprocess.Popen(attach_command(cli + ['start', '--attach', name]), stdout=out, stderr=err)
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
            require_logging(inspected['HostConfig'].get('LogConfig'))
            case_owners[case] = owner_for_container(inspected['Id'], owners['sandboxes'])
        assert case_owners['a']['sandbox_id'] != case_owners['b']['sandbox_id']
        storage_before = storage_snapshot('two-child', 2)
        assert {v['peer_cid'] for v in storage_before} == {40, 41}
        for index, case in enumerate(cases):
            execute(index, ['/bin/touch', '/home/rootless/.local/tmp/g8-concurrency/build'], 'release-build-' + case)
        deadline = time.monotonic() + 180
        while True:
            markers = {case: run_markers(read_output(private / (case + '-workload.stderr')).decode()) for case in cases}
            assert not any('END' in value for value in markers.values()), 'RUN overlap was not observed'
            if all('BEGIN' in value for value in markers.values()):
                before_overlap=overlap_capture('before');before_ns=time.monotonic_ns()
                primary('overlap')
                after_overlap=overlap_capture('after');after_ns=time.monotonic_ns()
                overlap = {'version':1,'boot_id':boot,'invocation_id':invocation,'before_monotonic_ns':before_ns,'after_monotonic_ns':after_ns,'captures':{'before':before_overlap,'after':after_overlap},'primary_sha256':primary_hashes['overlap'],'cleanup_qualified':False}
                (private / 'run-overlap.json').write_text(json.dumps(overlap, indent=2) + '\n')
                break
            assert all(p.poll() is None for p in processes.values()) and time.monotonic() < deadline
            time.sleep(.1)
        wait_for('G8_CONCURRENT_BUILD_READY')
        after = {case: snapshot(index, 'after', True) for index, case in enumerate(cases)}
        for case in cases:primary('two-built-'+case,after[case]['built_image_id'])
        for case in cases:
            assert {k: after[case][k] for k in before[case]} == before[case]
            output = read_output(private / (case + '-workload.stdout')).decode()
            errors = read_output(private / (case + '-workload.stderr')).decode()
            for marker in ['G8_MULTISTAGE_BUILD_PASS', 'G8_REPEAT_BUILD_PASS', 'G8_BUILT_IMAGE_RUN_PASS', 'G8_FAILED_BUILD_REJECTED', 'G8_BUILD_WORKLOAD_PASS']:
                assert marker in output
            assert 'CACHED' in errors and 'exit code: 37' in errors
            assert after[case]['saved_image_sha256'] in output
        invoke(cli + ['kill', '--signal=USR1', names[1]], 'release-sibling')
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
        assert len(list(Path('/sys/fs/multikernel/instances').iterdir()))==1,'sibling kernel instance remains'
        primary('survivor',after['a']['built_image_id'])
        # Keep the original archive and demonstrate new work after sibling release.
        rebuilt=execute(0,['/bin/sh','-ec','test ! -e /home/rootless/.local/tmp/g8-build-before-sibling-removal; mv /home/rootless/.local/tmp/g8-build /home/rootless/.local/tmp/g8-build-before-sibling-removal; /bin/sh /work/input/probe.sh'],'survivor-rebuild')
        original=execute(0,['/usr/bin/sha256sum','/home/rootless/.local/tmp/g8-build-before-sibling-removal/image.tar'],'survivor-original-archive').stdout.decode().split()[0]
        assert original==after['a']['saved_image_sha256'],'original survivor archive changed'
        survivor_rebuilt=snapshot(0,'survivor-rebuilt',True)
        assert {k:survivor_rebuilt[k] for k in before['a']}==before['a']
        primary('survivor-rebuild',survivor_rebuilt['built_image_id'])
        result = {'schema_version':2,'boot_id':boot,'invocation_id':invocation,'before': before, 'after': after, 'survivor': survivor,
                  'two_running_owners': owners['sandboxes'], 'case_owners': case_owners, 'remaining_owner': active[0],
                  'two_storage_exports': storage_before, 'survivor_storage_export': storage_survivor,
                  'run_overlap_observed': True, 'sibling_removed_without_survivor_change': True,
                  'survivor_rebuilt':survivor_rebuilt,'original_archive_after_rebuild_sha256':original,
                  'survivor_rebuild_stdout_sha256':hashlib.sha256(rebuilt.stdout).hexdigest(),'survivor_rebuild_stderr_sha256':hashlib.sha256(rebuilt.stderr).hexdigest(),
                  'run_overlap_checkpoint_sha256':digest(private/'run-overlap.json'),'primary_snapshots_sha256':primary_hashes,
                  'case_stdout_sha256':{case:digest(private/(case+'-workload.stdout')) for case in cases},'case_stderr_sha256':{case:digest(private/(case+'-workload.stderr')) for case in cases},
                  'cleanup_qualified': False}
        invoke(cli + ['kill', '--signal=USR1', names[0]], 'release-primary')
        assert processes['a'].wait(timeout=60) == 0
        result['case_stdout_sha256']={case:digest(private/(case+'-workload.stdout')) for case in cases}
        result['case_stderr_sha256']={case:digest(private/(case+'-workload.stderr')) for case in cases}
        (private / 'concurrency-checkpoint.json').write_text(json.dumps(result, indent=2) + '\n')
        return subprocess.CompletedProcess(processes['a'].args, 0)
    finally:
        if sibling_created:
            invoke(cli + ['rm', '--force', names[1]], 'sibling-cleanup', False)
        for stream in streams:
            stream.close()
