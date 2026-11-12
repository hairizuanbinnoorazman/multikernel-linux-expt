#!/usr/bin/env python3
"""Read-only same-boot audit of a committed Docker pressure or runtime-restart trial.

Print only fixed outcome flags and caller-pinned source/epoch identities. Raw
receipts, guest streams, backing paths and inventory fingerprints stay private.
This never restores transport, repairs ownership or qualifies a full gate.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess as sp
import sys
import tarfile
from runtime_trial_layout import validate_stage,scenario_flag,workload_unit


def digest(path, limit=128 << 20):
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(descriptor, 'rb') as file:
        before = os.fstat(file.fileno())
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != 0 or before.st_nlink != 1 or
                before.st_mode & 0o022 or not 0 <= before.st_size <= limit):
            raise ValueError('unsafe audit input')
        result = hashlib.sha256()
        size = 0
        while chunk := file.read(1 << 20):
            size += len(chunk)
            if size > limit:
                raise ValueError('audit input exceeds bound')
            result.update(chunk)
        after = os.fstat(file.fileno())
        named = path.lstat()
        fields = ['st_dev', 'st_ino', 'st_mode', 'st_uid', 'st_nlink', 'st_size', 'st_mtime_ns', 'st_ctime_ns']
        if size != before.st_size or any(getattr(before, k) != getattr(s, k) for s in [after, named] for k in fields):
            raise ValueError('audit input changed')
        return result.hexdigest()


def primary_digest(path):
    """Watch protected deployment aliases; fixture inputs still forbid aliases."""
    resolved = path.resolve(strict=True)
    for value in [path, *path.parents, resolved, *resolved.parents]:
        info = value.lstat()
        if info.st_uid != 0 or (not stat.S_ISLNK(info.st_mode) and info.st_mode & 0o022):
            raise ValueError('unprotected primary watched path')
    result = digest(resolved)
    if path.resolve(strict=True) != resolved:
        raise ValueError('primary deployment alias changed')
    return result


def empty_cgroup(directory):
    """Require an unpopulated leaf, preserving empty versus absent evidence."""
    descriptor = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    try:
        info = os.fstat(descriptor)
        if info.st_uid != os.geteuid() or info.st_mode & 0o022:
            raise ValueError('unprotected trial cgroup')
        def read(name):
            fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK, dir_fd=descriptor)
            with os.fdopen(fd, 'rb') as file:
                return file.read(4097)
        values = read('cgroup.events').decode('ascii').splitlines()
        events = dict(line.split() for line in values)
        if len(events) != len(values) or events.get('populated') != '0' or read('cgroup.procs') != b'':
            raise ValueError('populated trial cgroup')
        for name in os.listdir(descriptor):
            mode = os.stat(name, dir_fd=descriptor, follow_symlinks=False).st_mode
            if stat.S_ISDIR(mode) or stat.S_ISLNK(mode):
                raise ValueError('trial cgroup descendants remain')
        named = directory.lstat()
        if (named.st_dev, named.st_ino) != (info.st_dev, info.st_ino):
            raise ValueError('trial cgroup changed')
    finally:
        os.close(descriptor)


def qualify_trial_logging(read, private, item):
    """Requalify recorded policy, independently of the harness's pass flags."""
    from runtime_trial_output import logging_config, require_logging
    config = read(private, 'docker.json')
    expected = logging_config()
    if ({key: config.get(key) for key in expected} != expected or
            read(private, 'docker-info.json').get('LoggingDriver') != expected['log-driver']):
        raise ValueError('disposable daemon log policy differs')
    require_logging(item['HostConfig'].get('LogConfig'))


def audit(args):
    work = args.work
    scenario=getattr(args,'scenario','pressure');scenario_flag(scenario)
    require_engine=getattr(args,'require_engine_observation',False) or getattr(args,'require_aggregate_docker_health',False)
    if require_engine and scenario!='pressure':raise ValueError('engine observation requires pressure scenario')
    validate_stage(work,args.source_commit[:7])
    if (os.geteuid() != 0 or not work.is_absolute() or str(work) != os.path.normpath(work) or
            work.name != 'g8-host-' + args.source_commit[:7] or
            not re.fullmatch('[a-f0-9]{40}', args.source_commit) or
            not re.fullmatch('[a-f0-9]{64}', args.build_sha256)):
        raise ValueError('invalid audit authority')
    for directory in [work, *work.parents, work/'source', work/'source/scripts', work/'bin', work/'integration']:
        info = directory.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('unprotected audit directory')
    if digest(work/'build.json', 1 << 20) != args.build_sha256:
        raise ValueError('build manifest differs from caller pin')
    # Pin before importing any fixture-specific acceptance code.
    build = json.loads((work/'build.json').read_text())
    if build['source_commit'] != args.source_commit or digest(work/'source.tar') != build['source_archive_sha256']:
        raise ValueError('staged source identity differs')
    with tarfile.open(work/'source.tar') as archive:
        if archive.pax_headers.get('comment') != args.source_commit:
            raise ValueError('source archive lacks exact commit')
        for member in archive.getmembers():
            name = Path(member.name)
            if name.is_absolute() or '..' in name.parts or not (member.isdir() or member.isfile()):
                raise ValueError('unsafe source member')
            if member.isfile() and (name.suffix == '.py' and name.parts[0] == 'scripts' or name.suffix=='.sh' and name.parts[0]=='guest'):
                if hashlib.sha256(archive.extractfile(member).read()).hexdigest() != digest(work/'source'/name):
                    raise ValueError('acceptance source differs from archive')
    for name, expected in build['binaries'].items():
        if Path(name).name != name or digest(work/'bin'/name) != expected:
            raise ValueError('staged program differs from build manifest')
    sys.dont_write_bytecode = True
    sys.path.insert(0, str(work/'source/scripts'))
    from runtime_fixture_epoch import qualify_unit_epoch, unit_properties
    from runtime_diagnostic_bundle_checkpoint import read_fixture_artifact, envelope, bind, inspect_fixture_lifecycle_timings
    from runtime_primary_docker import observe, unchanged
    from runtime_firewall_identity import identity as firewall_identity
    from runtime_storage_pressure import recovered_observation
    from runtime_service_policy import check_observation
    from runtime_journal_status_checkpoint import qualify as qualify_journal
    from runtime_doctor_checkpoint import unique_object

    def raw(parent, name, limit=1 << 20):
        return read_fixture_artifact(parent, name, limit)

    def read(parent, name):
        return json.loads(raw(parent, name), object_pairs_hook=unique_object)

    boot = Path('/proc/sys/kernel/random/boot_id').read_text().strip()
    units = [('g8-vsock-rx-trial-' + args.source_commit[:7] + '-release.service', args.parent_invocation),
             (workload_unit(args.source_commit[:7],scenario), args.workload_invocation)]
    retained_empty_cgroup = False
    for unit, invocation in units:
        text = sp.check_output(['systemctl', 'show', unit, '-p', 'MainPID', '-p', 'InvocationID',
                               '-p', 'SubState', '-p', 'Result', '-p', 'ExecMainStatus', '-p', 'ControlGroup'], text=True, timeout=10)
        value = unit_properties(text)
        if not qualify_unit_epoch(args.boot_id, invocation, boot, value)['unit_completion_qualified']:
            raise ValueError('same-epoch completion is unqualified')
        directory = Path('/sys/fs/cgroup/system.slice')/unit
        if value['ControlGroup']:
            if value['ControlGroup'] != '/system.slice/'+unit:
                raise ValueError('trial cgroup identity differs')
            empty_cgroup(directory)
            retained_empty_cgroup = True
        elif directory.exists() or directory.is_symlink():
            raise ValueError('trial cgroup changed')
    private = work/'integration'
    receipt = read(work, 'integration-receipt.json')
    if receipt['boot_id'] != boot or receipt['invocation_id'] != args.workload_invocation:
        raise ValueError('lifecycle receipt differs from exact epoch')
    for key in ['host_docker_selects_multikernel', 'production_profile_child_build_pass', 'private_mount_namespace',
                'private_daemon_state', 'network_rules_and_links_restored', 'pool_released', 'production_files_unchanged',
                'four_service_pids_unchanged', 'strict_source_comparison', 'service_capability_nnp_tested']:
        if receipt[key] is not True:
            raise ValueError('lifecycle outcome is unqualified')
    if receipt.get('storage_pressure_tested') is not (scenario=='pressure'):
        raise ValueError('trial scenario differs from caller authority')
    if receipt.get('concurrent_guest_builds_tested') is not (scenario=='concurrent'):
        raise ValueError('concurrent scenario differs from caller authority')
    if receipt['execution_script_sha256'] != digest(work/'source/scripts/test-runtime-docker-profile-live.py'):
        raise ValueError('execution source differs from receipt')
    if receipt['source_image_archive_sha256'] != digest(private/'approved-image.tar', 1 << 30):
        raise ValueError('approved import archive changed')
    workload = read(work, 'workload-checkpoint.json')
    if digest(work/'workload-checkpoint.json') != receipt['workload_checkpoint_sha256']:
        raise ValueError('workload checkpoint differs from lifecycle receipt')
    if digest(private/'input/probe.sh')!=workload['probe_sha256'] or workload['probe_sha256']!=digest(work/'source/guest/runtime-docker-build-probe.sh'):
        raise ValueError('workload probe differs from archived source')
    if scenario=='runtime-crash-during-io' and (digest(private/'input/io-run.sh')!=workload['io_run_script_sha256'] or workload['io_run_script_sha256']!=digest(work/'source/guest/runtime-docker-io-run.sh')):
        raise ValueError('workload I/O loop differs from archived source')
    for stream in ['stdout','stderr']:
        if digest(private/('workload.'+stream),131072)!=workload['workload_'+stream+'_sha256']:
            raise ValueError('original workload stream differs from receipt')
    if workload['runtime_program_hashes'] != {name:build['binaries'][name] for name in ['mkruntimed','mknetd','containerd-shim-multikernel-v2']}:
        raise ValueError('runtime receipt differs from staged programs')
    live_health=False;timings=None
    if scenario=='pressure':
        families = {'runtime_doctor': 'doctor', 'runtime_network_health': 'network-health', 'runtime_journal_status': 'journal-status',
                    'runtime_pool_doctor': 'pool-doctor', 'runtime_unified_doctor': 'unified-doctor',
                    'runtime_component_build': 'component-builds', 'runtime_operational_doctor': 'operational-doctor',
                    'runtime_diagnostic_checkpoint_timing': 'diagnostic-checkpoint-timing',
                    'runtime_diagnostic_bundle_receipt': 'diagnostic-bundle', 'runtime_diagnostic_bundle_identity': 'diagnostic-bundle'}
        for family, prefix in families.items():
            suffix = '-receipt' if family.endswith('_receipt') else '-identity' if family.endswith('_identity') else ''
            for phase in ['while-full', 'while-image-ready', 'after-remove']:
                if digest(private/(prefix+'-'+phase+suffix+'.json')) != receipt[family+'_sha256'][phase]:
                    raise ValueError('archived observation differs from lifecycle receipt')
        full = read(work, 'storage-pressure-full-checkpoint.json')
        owner = full['owner']
        labels = read(work, 'compiled-labels.json')
        for phase in ['while-full', 'while-image-ready', 'after-remove']:
            def report(prefix):
                return read(private, prefix+'-'+phase+'.json')
            storage, network = report('doctor'), report('network-doctor')
            released = phase == 'after-remove'
            # These are archived observations already accepted at each live hold.
            # Rebind exact product bytes; do not claim a fresh native CHECK or
            # reconstruct a deleted private endpoint from public metadata.
            data = raw(private, 'diagnostic-bundle-'+phase+'.json', 1 << 20)
            product = envelope(data, raw(private, 'diagnostic-bundle-'+phase+'-receipt.json'), 'consistent')
            components, operational = report('component-builds'), report('operational-doctor')
            bind(product, components, operational)
            if (operational['version'] != 7 or operational['status'] != 'consistent' or
                    operational['boot_id'] != boot or not operational['boot_stable'] or
                    operational['unified'] != report('unified-doctor') or operational['components'] != components or
                    operational['unified']['combined'] != report('combined-doctor') or
                    operational['unified']['pool'] != report('pool-doctor') or operational['findings']):
                raise ValueError('archived diagnostic references differ')
            from runtime_doctor_checkpoint import qualify as qualify_storage
            from runtime_pool_doctor_checkpoint import qualify as qualify_pool
            from runtime_component_build_checkpoint import qualify as qualify_components
            qualify_storage(json.dumps(storage).encode(), boot, owner, released=released)
            qualify_pool(raw(private, 'pool-doctor-'+phase+'.json'), boot, owner, storage, released=released)
            qualify_components(raw(private, 'component-builds-'+phase+'.json'), boot, storage['revision'], labels['versions'], labels['revisions'])
            qualify_journal(raw(private, 'journal-status-'+phase+'.json'), boot, owner, storage, released=released)
            health = report('network-health')
            expected = dict(version=1, source='host-network-backend-check', primary_boot_id=boot,
                            status='idle' if released else 'checked', endpoints=[dict(endpoint_generation=e['endpoint_generation'],
                            descriptor_sha256=e['descriptor_sha256'], code='CHECK_PASSED') for e in network['network']['endpoints']])
            if health != expected:
                raise ValueError('archived network health differs')
            path = private/('diagnostic-bundle-'+phase+'.json')
            info = path.lstat()
            bound = read(private, 'diagnostic-bundle-'+phase+'-identity.json')
            if (stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != 0 or info.st_nlink != 1 or
                    bound != dict(device=info.st_dev, inode=info.st_ino, uid=info.st_uid, links=info.st_nlink,
                                  mode='0600', bytes=info.st_size, sha256=digest(path))):
                raise ValueError('original diagnostic artifact identity changed')
            if digest(path) != receipt['runtime_diagnostic_bundle_sha256'][phase]:
                raise ValueError('diagnostic artifact differs from lifecycle receipt')
        timings = inspect_fixture_lifecycle_timings(work, boot, args.workload_invocation)
        parent = read(Path('/opt/mkruntime/g8-vsock-rx-b688447')/('trial-'+work.name), 'primary-module-trial.json')
        if (parent['boot_id'] != boot or parent['parent_scope']['InvocationID'] != args.parent_invocation or
                parent['pressure_invocation'] != args.workload_invocation or parent['diagnostic_checkpoint_timings'] != timings or
                parent.get('candidate_quiescent') is not True or parent.get('primary_unchanged') is not True or
                parent.get('cold_restore_required') is not True):
            raise ValueError('parent checkpoint differs from exact lifecycle')
        if require_engine:
            from runtime_engine_observation_checkpoint import inspect_fixture_engine_observation
            engine_view=inspect_fixture_engine_observation(work,boot,args.workload_invocation)
            if parent.get('engine_observation_checkpoint')!=engine_view:raise ValueError('parent engine observation checkpoint differs')
        if getattr(args,'require_aggregate_docker_health',False):
            from runtime_aggregate_docker_health_checkpoint import inspect_fixture_aggregate
            if parent.get('aggregate_docker_health_checkpoint') != inspect_fixture_aggregate(work,boot,args.workload_invocation):
                raise ValueError('parent aggregate health checkpoint differs')
        live_health = receipt.get('runtime_health_bundle_tested') is True
        if live_health:
            from runtime_health_bundle_checkpoint import inspect_fixture_health
            health_timings = inspect_fixture_health(work, boot, args.workload_invocation)
            if parent.get('health_checkpoint_timings') != health_timings:
                raise ValueError('parent health checkpoint differs from lifecycle receipt')
    elif scenario=='concurrent':
        from runtime_concurrency_checkpoint import inspect_fixture_concurrency
        concurrent=inspect_fixture_concurrency(work,boot,args.workload_invocation)
        parent=read(Path('/opt/mkruntime/g8-vsock-rx-b688447')/('trial-'+work.name),'primary-module-trial.json')
        if (parent.get('scenario')!='concurrent' or parent.get('concurrency_checkpoint')!=concurrent or parent['boot_id']!=boot or parent['parent_scope']['InvocationID']!=args.parent_invocation or parent['pressure_invocation']!=args.workload_invocation or parent.get('candidate_quiescent') is not True or parent.get('primary_unchanged') is not True or parent.get('cold_restore_required') is not True):
            raise ValueError('parent concurrency checkpoint differs from exact lifecycle')
    else:
        from runtime_recovery_checkpoint import inspect_fixture_recovery
        recovery=inspect_fixture_recovery(work,boot,args.workload_invocation,scenario)
        parent=read(Path('/opt/mkruntime/g8-vsock-rx-b688447')/('trial-'+work.name),'primary-module-trial.json')
        if (parent.get('scenario')!=scenario or parent.get('runtime_recovery_checkpoint')!=recovery or parent['boot_id']!=boot or parent['parent_scope']['InvocationID']!=args.parent_invocation or parent['pressure_invocation']!=args.workload_invocation or parent.get('candidate_quiescent') is not True or parent.get('primary_unchanged') is not True or parent.get('cold_restore_required') is not True):
            raise ValueError('parent recovery checkpoint differs from exact lifecycle')
    before, after = read(private, 'primary-before.json'), read(private, 'primary-after.json')
    if before != after or before['boot_id'] != boot:
        raise ValueError('primary baseline differs')
    for name, expected in before['file_sha256'].items():
        if primary_digest(Path(name)) != expected:
            raise ValueError('primary watched file changed')
    for name, pid in before['service_main_pids'].items():
        if sp.check_output(['systemctl', 'show', name, '-p', 'MainPID', '--value'], text=True, timeout=10).strip() != pid:
            raise ValueError('primary service changed')
    docker_before = read(private, 'primary-docker-before.json')
    for phase in (['while-full', 'while-image-ready', 'after'] if scenario=='pressure' else ['after']):
        unchanged(docker_before, read(private, 'primary-docker-'+phase+'.json'))
    if scenario=='pressure':
        image = read(work, 'storage-pressure-image-checkpoint.json')
        image_primary = read(private, 'primary-docker-while-image-ready.json')
        from runtime_primary_docker import fingerprint
        if (image['boot_id'] != boot or image['invocation_id'] != args.workload_invocation or
                image['child_image_absent_from_primary'] is not True or image_primary['child_image_absent'] is not True or
                image_primary['checked_absent_image'] != image['guest_image_id'] or
                fingerprint(image['guest_engine_id']) == docker_before['engine_id_sha256'] or
                digest(private/'primary-docker-while-image-ready.json') != image['primary_snapshot_sha256']):
            raise ValueError('child-local Docker identity/absence is unqualified')
    unchanged(docker_before, observe())
    for name in ['iptables-save', 'ip6tables-save']:
        a = firewall_identity(raw(private, 'firewall-before-'+name+'.rules'))
        if a != firewall_identity(raw(private, 'firewall-after-'+name+'.rules')) or a != firewall_identity(sp.check_output(['/usr/sbin/'+name], timeout=10)):
            raise ValueError('primary firewall changed')
    for name, command in [('links', ['/usr/sbin/ip', '-o', 'link', 'show']), ('routes', ['/usr/sbin/ip', 'route'])]:
        if raw(private, name+'-before.log') != raw(private, name+'-after.log') or raw(private, name+'-before.log') != sp.check_output(command, timeout=10):
            raise ValueError('primary network changed')
    stdout, stderr = raw(private, 'workload.stdout', 131072), raw(private, 'workload.stderr', 131072)
    if len(stdout) >= 131072 or len(stderr) >= 131072:
        raise ValueError('attached workload reached its acceptance limit')
    if scenario=='pressure':recovered_observation(stdout.decode(), stderr.decode())
    markers = ['G8_MULTISTAGE_BUILD_PASS', 'G8_REPEAT_BUILD_PASS', 'G8_BUILT_IMAGE_RUN_PASS', 'G8_FAILED_BUILD_REJECTED', 'G8_BUILD_RESOURCES_REMOVED', 'G8_BUILD_WORKLOAD_PASS']
    if any(stdout.splitlines().count(m.encode()) != 1 for m in markers) or b'CACHED' not in stderr or b'exit code: 37' not in stderr:
        raise ValueError('build matrix is unqualified')
    item = read(private, 'inspect.json')[0]
    qualify_trial_logging(read, private, item)
    if (item['State']['Status'] != 'exited' or item['State']['ExitCode'] != 0 or item['Config']['User'] != '1000:1000' or
            item['HostConfig']['Runtime'] != 'io.containerd.multikernel.v2' or item['HostConfig']['Privileged'] or item['HostConfig']['CapDrop'] != ['ALL']):
        raise ValueError('outer workload authority differs')
    nested = stdout.decode().split('G8_NESTED_AUTHORITY_BEGIN\n')[1].split('G8_NESTED_AUTHORITY_END')[0]
    if ('no-new-privileges=true' not in item['HostConfig']['SecurityOpt'] or
            [dict(destination=m['Destination'], read_only=not m['RW']) for m in item['Mounts']] != [dict(destination='/work/input', read_only=True)] or
            b'CapEff:\t0000000000000000' not in stdout or b'CapBnd:\t0000000000000000' not in stdout or
            'NoNewPrivs:\t1' not in nested or 'Seccomp:\t2' not in nested or not re.search(r'0\s+1000\s+1', nested)):
        raise ValueError('workload authority observations differ')
    policies = read(private, 'service-policies.json')['policies']
    for name, observed in read(private, 'service-observations.json').items():
        component = 'mkruntimed' if name.startswith('mkruntimed-') else 'mknetd'
        check_observation(observed, policies[str(work/'bin'/component)])
        if observed['executable_sha256'] != build['binaries'][component]:
            raise ValueError('observed service source differs')
    owned = False
    for proc in Path('/proc').iterdir():
        if not proc.name.isdigit():
            continue
        try:
            cgroup = (proc/'cgroup').read_text()
            exe = os.readlink(proc/'exe')
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            continue
        owned |= any(unit in cgroup for unit, _ in units) or exe.startswith(str(work/'bin')+'/')
    if owned:
        raise ValueError('owned fixture processes remain')
    exports = list(read(private/'storage-state', 'state.json')['exports'].values())
    if len(exports) != (2 if scenario=='concurrent' else 1) or any(v['state']!='RELEASED' or not re.fullmatch('e2fsck-clean-sha256:[a-f0-9]{64}',v.get('offline_check','')) for v in exports):
        raise ValueError('native release/offline check is unqualified')
    if any(v['counters']['written_bytes'] < (full['guest_full']['pressure_file_bytes'] if scenario=='pressure' else 1) for v in exports):
        raise ValueError('native counters do not cover pressure writes')
    if read(private/'rootfs-state', 'state.json')['records'] or read(private/'network-state', 'state.json')['endpoints']:
        raise ValueError('private ownership remains')
    if list((Path('/srv/multikernel-storage')/work.name).iterdir()) or list((private/'storage-runtime').glob('*.json')):
        raise ValueError('private storage remains')
    if list(Path('/sys/fs/multikernel/instances').iterdir()) or Path('/sys/devices/system/cpu/online').read_text().strip() != '0-15':
        raise ValueError('kernel resources remain')
    if Path('/sys/module/mk_transport/srcversion').read_text().strip() != '0450DCCF7EDC67FAE2A6CAA' or Path('/sys/module/mk_transport/refcnt').read_text().strip() != '0':
        raise ValueError('candidate transport is not quiescent')
    if len(sp.check_output(['ss', '-A', 'vsock', '-an'], text=True, timeout=10).splitlines()) != 1 or 'No memory pool configured' not in sp.check_output(['/opt/mkruntime/kerf-venv/bin/kerf', 'show'], text=True, timeout=10):
        raise ValueError('VSOCK or pool resources remain')
    return dict(schema=1, source_commit=args.source_commit, boot_id=boot, parent_invocation=args.parent_invocation,
                workload_invocation=args.workload_invocation, same_epoch_completion=True, real_child_aggregate_docker_health_qualified=getattr(args,'require_aggregate_docker_health',False), real_child_engine_observation_qualified=require_engine, live_health_pair_qualified=live_health, trial_scratch_private_mount_qualified=receipt.get('private_trial_tmp_tested') is True, retained_empty_systemd_cgroup=retained_empty_cgroup, enospc_build_recovery=scenario=='pressure', runtime_restart_build_continuity_qualified=scenario=='runtime-restart',
                runtime_crash_build_continuity_qualified=scenario=='runtime-crash',
                runtime_inflight_crash_build_continuity_qualified=scenario=='runtime-crash-during-build',
                runtime_synced_io_crash_continuity_qualified=scenario=='runtime-crash-during-io',
                concurrent_child_build_separation_and_survivor_rebuild_qualified=scenario=='concurrent',
                diagnostic_three_hold_receipts_requalified=scenario=='pressure', primary_docker_and_network_preserved=True,
                native_release_offline_check_and_resource_return=True, cold_restoration_qualified=False,
                raw_private_payloads_exported=False, full_g7_closed=False, full_g8_closed=False,
                diagnostic_maximum_elapsed_ns=timings['maximum_elapsed_ns'] if timings else 0)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work', type=Path, required=True)
    parser.add_argument('--require-engine-observation',action='store_true')
    parser.add_argument('--require-aggregate-docker-health',action='store_true')
    parser.add_argument('--scenario',choices=['pressure','runtime-restart','runtime-crash','runtime-crash-during-build','runtime-crash-during-io','concurrent'],default='pressure')
    for name in ['boot-id', 'parent-invocation', 'workload-invocation', 'source-commit', 'build-sha256']:
        parser.add_argument('--'+name, required=True)
    args = parser.parse_args()
    if args.require_aggregate_docker_health:
        args.require_engine_observation = True
    try:
        result = audit(args)
    except Exception:
        print('Docker trial audit refused; inspect private evidence', file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == '__main__':
    sys.exit(main())
