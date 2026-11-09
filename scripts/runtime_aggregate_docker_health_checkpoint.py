"""Receipt-bound v9/v3 observations during the ordinary real-child pressure trial.

The existing v8 pair and new aggregate pair share its original five-second
deadline. All health references and the independent engine/agent proof must
requalify before the aggregate lifecycle can qualify. Raw inputs stay private.
"""
import hashlib
import json
import os
import re
import selectors
import signal
import stat
import subprocess
import time
from pathlib import Path

from runtime_diagnostic_bundle_checkpoint import decode, equal, read_fixture_artifact, EXCLUDED
from runtime_docker_concurrency import sandbox_id_for_container
from runtime_health_bundle_checkpoint import qualify as qualify_health
from runtime_health_bundle_checkpoint import inspect_fixture_health, UNOBSERVED
from runtime_engine_observation_checkpoint import inspect_fixture_engine_observation
from runtime_trial_agent_profile import sha as artifact_sha

PHASES = ['while-full', 'while-image-ready', 'after-remove']
SCOPE = 'operational-health-and-all-running-child-engine-observations'
BUNDLE_SCOPE = 'operational-health-child-engine-and-component-observation-snapshot'
TIMING_SCOPE = 'health-and-aggregate-docker-four-report-checkpoint'


class CommandOutputLimit(ValueError):
    def __init__(self, stdout, stderr):
        super().__init__('aggregate command output limit exceeded')
        self.stdout, self.stderr = stdout, stderr


def bounded_command(argv, deadline):
    """Collect at most each stream's limit plus one overflow byte.

    Keep the owned leader unreaped until collection completes so its PID cannot
    be reused before refusal terminates the private process group. Cleanup has
    a separate one-second reap bound; it never grants checkpoint time.
    """
    remaining = (deadline-time.monotonic_ns())/1_000_000_000
    if remaining <= 0:
        raise subprocess.TimeoutExpired(argv, 0, output=b'', stderr=b'')
    process = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True)
    data = {'stdout': bytearray(), 'stderr': bytearray()}
    limits = {'stdout': 1 << 20, 'stderr': 4096}
    complete = False
    try:
        with selectors.DefaultSelector() as poll:
            for name in data:
                stream = getattr(process, name)
                os.set_blocking(stream.fileno(), False)
                poll.register(stream, selectors.EVENT_READ, name)
            while poll.get_map():
                remaining = (deadline-time.monotonic_ns())/1_000_000_000
                if remaining <= 0:
                    raise subprocess.TimeoutExpired(argv, 0, output=bytes(data['stdout']), stderr=bytes(data['stderr']))
                for key, _ in poll.select(remaining):
                    name = key.data
                    try:
                        chunk = os.read(key.fd, min(65536, limits[name]+1-len(data[name])))
                    except BlockingIOError:
                        continue
                    if not chunk:
                        poll.unregister(key.fileobj)
                        continue
                    data[name].extend(chunk)
                    if len(data[name]) > limits[name]:
                        raise CommandOutputLimit(bytes(data['stdout']), bytes(data['stderr']))
        remaining = (deadline-time.monotonic_ns())/1_000_000_000
        if remaining <= 0:
            raise subprocess.TimeoutExpired(argv, 0, output=bytes(data['stdout']), stderr=bytes(data['stderr']))
        try:
            status = process.wait(timeout=remaining)
        except subprocess.TimeoutExpired:
            raise subprocess.TimeoutExpired(argv, remaining, output=bytes(data['stdout']), stderr=bytes(data['stderr'])) from None
        complete = True
        return subprocess.CompletedProcess(argv, status, bytes(data['stdout']), bytes(data['stderr']))
    finally:
        if not complete:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.stdout.close()
        process.stderr.close()
        if not complete:
            process.wait(timeout=1)


def qualify(data, *references, released=False):
    report = decode(data, 1 << 20)
    if type(report) is not dict or 'health' not in report:
        raise ValueError('aggregate health payload missing')
    health = qualify_health(json.dumps(report['health']).encode(), *references, released=released)
    owner = references[1]
    engines = [] if released else [dict(sandbox_id=owner['sandbox_id'], generation=owner['generation'],
        observation=dict(version=1, source='owned-child-docker-api-and-launcher', status='responding'))]
    equal(report, dict(version=9, scope=SCOPE, status='consistent', revision=health['revision'],
        boot_id=health['boot_id'], boot_stable=True, health=health, engines=engines,
        all_running_owners_observed=True, findings=[], admission_guaranteed=False,
        unobserved=[v for v in UNOBSERVED if v != 'docker-engine']))
    return report


def qualify_bundle(data, receipt_data, doctor_data, *references, released=False):
    doctor = qualify(doctor_data, *references, released=released)
    equal(decode(receipt_data, 65536), dict(version=3, scope=BUNDLE_SCOPE, collection_status='captured',
        observation_status='consistent', bytes=len(data), sha256=hashlib.sha256(data).hexdigest()))
    bundle = decode(data, 1 << 20)
    if type(bundle) is not dict:
        raise ValueError('aggregate bundle envelope missing')
    for name in ['started_at', 'finished_at']:
        if type(bundle.get(name)) is not str or not re.fullmatch(
                r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z', bundle[name]):
            raise ValueError('aggregate bundle timestamp invalid')
    equal(bundle, dict(version=3, scope=BUNDLE_SCOPE, collection_status='captured',
        started_at=bundle['started_at'], finished_at=bundle['finished_at'],
        snapshot=dict(components=doctor['health']['operational']['components'], docker_health=doctor), excluded=EXCLUDED))
    return bundle


def qualify_timing(data, phase, health_timing):
    timing = decode(data, 65536)
    keys = {'version', 'scope', 'phase', 'configured_budget_ns', 'started_monotonic_ns',
            'deadline_monotonic_ns', 'aggregate_started_monotonic_ns', 'finished_monotonic_ns',
            'elapsed_ns', 'report_count'}
    if type(timing) is not dict or set(timing) != keys or phase not in PHASES:
        raise ValueError('invalid aggregate checkpoint timing')
    if any(type(timing[k]) is not int for k in keys - {'scope', 'phase'}):
        raise ValueError('invalid aggregate timing number')
    # Requalify the original pair rather than trusting a caller's timing label.
    from runtime_health_bundle_checkpoint import qualify_timing as qualify_pair_timing
    qualify_pair_timing(json.dumps(health_timing).encode(), phase)
    start, end, deadline, aggregate_start = [timing[k] for k in [
        'started_monotonic_ns', 'finished_monotonic_ns', 'deadline_monotonic_ns', 'aggregate_started_monotonic_ns']]
    if (timing['version'] != 1 or timing['scope'] != TIMING_SCOPE or timing['phase'] != phase or
            timing['configured_budget_ns'] != 5_000_000_000 or timing['report_count'] != 4 or
            start != health_timing['started_monotonic_ns'] or deadline != health_timing['deadline_monotonic_ns'] or
            not 0 <= start <= health_timing['finished_monotonic_ns'] <= aggregate_start <= end <= deadline <= start+5_000_000_000 or
            timing['elapsed_ns'] != end-start):
        raise ValueError('aggregate checkpoint changed or exceeded the original health budget')
    return timing


def targets(container, owner, address, released=False):
    if sandbox_id_for_container(container) != owner['sandbox_id'] or not re.fullmatch('[a-f0-9]{32}', owner['generation']):
        raise ValueError('aggregate target owner differs')
    if type(address) is not str or not address.startswith('unix:///') or len(address) > 4096 or any(ord(c) < 33 or ord(c) == 127 for c in address):
        raise ValueError('aggregate target address invalid')
    path = address[len('unix://'):]
    if os.path.normpath(path) != path or path.startswith('//'):
        raise ValueError('aggregate target address is not canonical')
    request = dict(version=1, namespace='moby', container_id=container, sandbox_id=owner['sandbox_id'], generation=owner['generation'])
    return dict(version=1, targets=[] if released else [dict(socket=address, owner=request)])


def write_private(path, value):
    with path.open('x') as output:
        os.fchmod(output.fileno(), 0o600)
        json.dump(value, output, sort_keys=True)
        output.write('\n')
        output.flush()
        os.fsync(output.fileno())


def retain_command_streams(private, phase, command, stdout, stderr, failure=None):
    if phase not in PHASES or command not in ['doctor', 'bundle'] or failure not in [None, 'command-timeout', 'command-output-refused'] or not isinstance(stdout, bytes) or not isinstance(stderr, bytes):
        raise ValueError('invalid aggregate command stream scope')
    limits = {'stdout': 1 << 20, 'stderr': 4096}
    streams = {'stdout': stdout, 'stderr': stderr}
    for suffix, data in streams.items():
        with (private/('aggregate-command-'+command+'-'+phase+'.'+suffix)).open('xb') as output:
            os.fchmod(output.fileno(), 0o600)
            output.write(data[:limits[suffix]])
            output.flush()
            os.fsync(output.fileno())
    if failure:
        write_private(private/('aggregate-command-'+command+'-'+phase+'-failure.json'),
            dict(version=1, phase=phase, command=command, failure=failure, qualified=False,
                 stream_truncated={k: len(v) > limits[k] for k, v in streams.items()}))


def identity(path, data):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_size != len(data):
        raise ValueError('aggregate artifact identity differs')
    return dict(device=info.st_dev, inode=info.st_ino, uid=info.st_uid, links=info.st_nlink,
                mode='0600', bytes=len(data), sha256=hashlib.sha256(data).hexdigest())


def capture(work, container, owner, phase, references, health_data, health_timing, config, network_socket):
    if phase not in PHASES:
        raise ValueError('invalid aggregate capture phase')
    private = work/'integration'
    released = phase == 'after-remove'
    from runtime_health_bundle_checkpoint import qualify_timing as qualify_pair_timing
    qualify_pair_timing(json.dumps(health_timing).encode(), phase)
    aggregate_start = time.monotonic_ns()
    deadline = health_timing['deadline_monotonic_ns']
    if aggregate_start < health_timing['finished_monotonic_ns'] or aggregate_start >= deadline:
        raise ValueError('original health budget exhausted before aggregate capture')
    saved = decode(read_fixture_artifact(private, 'engine-shim-address.json', 4096), 4096)
    if set(saved) != {'address', 'container_id'} or saved['container_id'] != container:
        raise ValueError('saved aggregate shim identity differs')
    mapping = targets(container, owner, saved['address'], released)
    target_path = private/('aggregate-targets-'+phase+'.json')
    write_private(target_path, mapping)
    base = [str(work/'bin/mkruntimed'), '--config='+str(config), '--network-socket='+str(network_socket),
            '--engine-targets='+str(target_path)]
    def invoke(command, args):
        remaining = (deadline-time.monotonic_ns())/1_000_000_000
        if remaining <= 0:
            raise ValueError('original health budget exhausted')
        try:
            result = bounded_command(base+args, deadline)
        except subprocess.TimeoutExpired as failure:
            retain_command_streams(private, phase, command, failure.stdout or b'', failure.stderr or b'', 'command-timeout')
            raise
        except CommandOutputLimit as failure:
            retain_command_streams(private, phase, command, failure.stdout, failure.stderr, 'command-output-refused')
            raise
        valid = result.returncode == 0 and not result.stderr and 0 < len(result.stdout) <= 1 << 20
        retain_command_streams(private, phase, command, result.stdout, result.stderr,
                               None if valid else 'command-output-refused')
        if not valid:
            raise ValueError('aggregate health command refused the steady hold')
        return result.stdout
    doctor = invoke('doctor', ['--doctor-all', '--doctor-format=9'])
    qualified = qualify(doctor, *references, released=released)
    equal(qualified['health'], decode(health_data, 1 << 20))
    write_private(private/('aggregate-doctor-'+phase+'.json'), qualified)
    destination = private/'aggregate-bundles'/(phase+'.json')
    receipt = invoke('bundle', ['--diagnostic-bundle='+str(destination), '--diagnostic-format=3',
                      '--diagnostic-max-files=3', '--diagnostic-max-bytes=1048576'])
    data = read_fixture_artifact(destination.parent, destination.name, 1 << 20)
    qualify_bundle(data, receipt, doctor, *references, released=released)
    write_private(private/('aggregate-bundle-'+phase+'-receipt.json'), decode(receipt, 65536))
    write_private(private/('aggregate-bundle-'+phase+'-identity.json'), identity(destination, data))
    finish = time.monotonic_ns()
    timing = dict(version=1, scope=TIMING_SCOPE, phase=phase, configured_budget_ns=5_000_000_000,
                  started_monotonic_ns=health_timing['started_monotonic_ns'], deadline_monotonic_ns=deadline,
                  aggregate_started_monotonic_ns=aggregate_start, finished_monotonic_ns=finish,
                  elapsed_ns=finish-health_timing['started_monotonic_ns'], report_count=4)
    qualify_timing(json.dumps(timing).encode(), phase, health_timing)
    write_private(private/('aggregate-timing-'+phase+'.json'), timing)


def artifact_paths(work, phase):
    private = work/'integration'
    return dict(doctor=private/('aggregate-doctor-'+phase+'.json'), targets=private/('aggregate-targets-'+phase+'.json'),
                bundle=private/'aggregate-bundles'/(phase+'.json'), bundle_receipt=private/('aggregate-bundle-'+phase+'-receipt.json'),
                bundle_identity=private/('aggregate-bundle-'+phase+'-identity.json'), timing=private/('aggregate-timing-'+phase+'.json'))


def inspect_fixture_aggregate(work, boot, invocation):
    work = Path(work)
    private = work/'integration'
    read = lambda path: read_fixture_artifact(path.parent, path.name, 1 << 20)
    receipt = decode(read(work/'integration-receipt.json'), 1 << 20)
    if receipt.get('aggregate_docker_health_tested') is not True or receipt.get('boot_id') != boot or receipt.get('invocation_id') != invocation:
        raise ValueError('aggregate lifecycle identity differs')
    engine = inspect_fixture_engine_observation(work, boot, invocation)
    health = inspect_fixture_health(work, boot, invocation)
    build = decode(read(work/'build.json'), 1 << 20)
    if type(build.get('source_commit')) is not str or not re.fullmatch('[a-f0-9]{40}', build['source_commit']):
        raise ValueError('aggregate source commit invalid')
    binary = work/'bin/mkruntimed'
    if artifact_sha(binary) != build['binaries']['mkruntimed']:
        raise ValueError('aggregate executable differs from committed trial binary')
    labels = decode(read(work/'compiled-labels.json'), 1 << 20)
    if labels['revisions']['mkruntimed'] != build['source_commit'][:7]:
        raise ValueError('aggregate compiled source label differs')
    owner = decode(read(work/'storage-pressure-full-checkpoint.json'), 1 << 20)['owner']
    container = read_fixture_artifact(private, 'create.stdout', 4096).decode().strip()
    saved = decode(read(private/'engine-shim-address.json'), 4096)
    if set(saved) != {'address', 'container_id'} or saved.get('container_id') != container:
        raise ValueError('aggregate target container differs')
    full_engine = decode(read(private/'engine-while-full.json'), 1 << 20)
    equal(full_engine['owner'], targets(container, owner, saved['address'])['targets'][0]['owner'])
    timings = {}
    for phase in PHASES:
        files = artifact_paths(work, phase)
        values = {}
        for family, path in files.items():
            hashes = receipt.get('aggregate_docker_health_'+family+'_sha256')
            if type(hashes) is not dict or set(hashes) != set(PHASES):
                raise ValueError('incomplete aggregate lifecycle evidence')
            values[family] = read(path)
            if hashlib.sha256(values[family]).hexdigest() != hashes[phase]:
                raise ValueError('aggregate lifecycle receipt digest differs')
        mapping = targets(container, owner, saved['address'], phase == 'after-remove')
        equal(decode(values['targets'], 1 << 20), mapping)
        identity(files['targets'], values['targets'])
        report = lambda prefix: decode(read(private/(prefix+'-'+phase+'.json')), 1 << 20)
        references = (boot, owner, report('doctor'), report('network-doctor'), report('health-endpoint'),
            report('combined-doctor'), report('pool-doctor'), report('unified-doctor'), report('component-builds'),
            report('operational-doctor'), report('journal-status'), report('network-health'), labels['versions'], labels['revisions'])
        qualify_bundle(values['bundle'], values['bundle_receipt'], values['doctor'], *references, released=phase == 'after-remove')
        equal(decode(values['doctor'], 1 << 20)['health'], report('health-doctor'))
        equal(decode(values['bundle_identity'], 65536), identity(files['bundle'], values['bundle']))
        timings[phase] = qualify_timing(values['timing'], phase, health['timings'][phase])
        sample = decode(read(private/('engine-'+phase+'.json')), 1 << 20)
        if sample['finished_monotonic_ns'] > timings[phase]['started_monotonic_ns']:
            raise ValueError('aggregate checkpoint precedes its bound engine hold')
        next_engine = {'while-full': 'during-build', 'while-image-ready': 'after-remove'}.get(phase)
        if next_engine:
            following = decode(read(private/('engine-'+next_engine+'.json')), 1 << 20)
            if timings[phase]['finished_monotonic_ns'] > following['started_monotonic_ns']:
                raise ValueError('aggregate checkpoint escaped its bound engine hold')
    for earlier, later in zip(PHASES, PHASES[1:]):
        if timings[earlier]['finished_monotonic_ns'] > timings[later]['started_monotonic_ns']:
            raise ValueError('aggregate lifecycle phases overlap')
    return dict(version=1, scope='receipt-bound-real-child-aggregate-docker-health', source_commit=build['source_commit'], boot_id=boot,
                invocation_id=invocation, complete_running_owner_coverage=True, original_five_second_health_budget_preserved=True,
                approved_agent_engine_proof_bound=engine['current_agent_and_approved_bootstrap_bound'],
                timings=timings, maximum_elapsed_ns=max(v['elapsed_ns'] for v in timings.values()),
                admission_guaranteed=False, cleanup_qualified=False)
