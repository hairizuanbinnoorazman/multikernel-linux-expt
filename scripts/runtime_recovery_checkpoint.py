"""Bind a normal private-control-plane restart to its same-boot build receipt.

No repair, live-process authority or full gate follows from archived snapshots.
"""
import hashlib,json,re
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_primary_docker import unchanged,fingerprint


def qualify(data,exit_data,receipt,boot,invocation):
    r=decode(data,1<<20);exit=decode(exit_data,65536)
    if any(type(v) is not dict for v in [r,exit,receipt]):raise ValueError('invalid runtime recovery envelope')
    fields={'schema_version','invocation_id','boot_id','runtime_pid_before','runtime_pid_after','runtime_restarts','runtime_exit_signal','runtime_exit_status','before','after','after_build','post_recovery_build_tested','post_recovery_stdout_sha256','post_recovery_stderr_sha256','child_continuity_pass','cleanup_qualified','runtime_exit_checkpoint_sha256','completed_first_build','crash_during_build','original_image_id','post_recovery_image_id','original_image_sha256_after_rebuild','primary_docker_before_recovery_sha256','primary_docker_after_recovery_sha256','primary_docker_after_rebuild_sha256'}
    if set(r)!=fields:raise ValueError('invalid runtime recovery fields')
    if (not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',boot) or
            not re.fullmatch('[a-f0-9]{32}',invocation) or r.get('boot_id')!=boot or r.get('invocation_id')!=invocation or
            type(r.get('schema_version')) is not int or r['schema_version']!=2 or
            receipt.get('boot_id')!=boot or receipt.get('invocation_id')!=invocation or
            receipt.get('runtime_restart_tested') is not True or receipt.get('runtime_crash_tested') is not False or
            receipt.get('runtime_crash_during_build_tested') is not False or receipt.get('storage_pressure_tested') is not False or
            receipt.get('production_profile_child_build_pass') is not True or
            type(r.get('runtime_restarts')) is not int or r['runtime_restarts']!=1 or
            r.get('runtime_exit_signal')!='SIGTERM' or type(r.get('runtime_exit_status')) is not int or r['runtime_exit_status']!=0 or
            r.get('crash_during_build') is not False or r.get('post_recovery_build_tested') is not True or
            r.get('child_continuity_pass') is not True or r.get('cleanup_qualified') is not False):
        raise ValueError('runtime recovery checkpoint is unqualified')
    for key in ['runtime_pid_before','runtime_pid_after']:
        if type(r.get(key)) is not int or not 1<r[key]<1<<31:raise ValueError('runtime recovery process identity is invalid')
    if r['runtime_pid_before']==r['runtime_pid_after']:raise ValueError('runtime recovery process did not change')
    expected_exit=dict(invocation_id=invocation,boot_id=boot,runtime_pid=r['runtime_pid_before'],runtime_exit_signal='SIGTERM',runtime_exit_status=0,observed_monotonic_ns=exit.get('observed_monotonic_ns'),cleanup_qualified=False)
    if type(exit.get('observed_monotonic_ns')) is not int or exit['observed_monotonic_ns']<0:raise ValueError('invalid runtime exit timing')
    equal(exit,expected_exit)
    if hashlib.sha256(exit_data).hexdigest()!=r.get('runtime_exit_checkpoint_sha256') or hashlib.sha256(data).hexdigest()!=receipt.get('recovery_checkpoint_sha256'):
        raise ValueError('runtime recovery receipt digest differs')
    for key in ['before','after','after_build','completed_first_build']:
        snapshot=r.get(key)
        if (type(snapshot) is not dict or set(snapshot)!={'child_boot_id','engine_id','ownership','saved_image_sha256'} or
                type(snapshot['child_boot_id']) is not str or not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',snapshot['child_boot_id']) or
                snapshot['child_boot_id']==boot or type(snapshot['engine_id']) is not str or not re.fullmatch('[A-Za-z0-9:_-]{1,256}',snapshot['engine_id']) or
                type(snapshot['saved_image_sha256']) is not str or not re.fullmatch('[a-f0-9]{64}',snapshot['saved_image_sha256']) or type(snapshot['ownership']) is not list or len(snapshot['ownership'])!=1):
            raise ValueError('runtime recovery child snapshot is invalid')
        owner=snapshot['ownership'][0]
        if (type(owner) is not dict or set(owner)!={'sandbox_id','generation','state','cpus','memory_bytes','child_cid','agent_port'} or
                type(owner['sandbox_id']) is not str or not re.fullmatch('[A-Za-z0-9][A-Za-z0-9_.-]{0,127}',owner['sandbox_id']) or type(owner['generation']) is not str or not re.fullmatch('[a-f0-9]{32}',owner['generation']) or
                owner['state']!='RUNNING' or type(owner['cpus']) is not list or not owner['cpus'] or
                any(type(v) is not int or not 0<=v<1<<31 for v in owner['cpus']) or owner['cpus']!=sorted(set(owner['cpus'])) or
                any(type(owner[k]) is not int or not 0<owner[k]<(1<<64 if k=='memory_bytes' else 1<<32) for k in ['memory_bytes','child_cid','agent_port']) or owner['child_cid']<3):
            raise ValueError('runtime recovery owner is invalid')
    equal(r['before'],r['after']);equal(r['before'],r['completed_first_build'])
    for key in ['child_boot_id','engine_id','ownership']:equal(r['before'][key],r['after_build'][key])
    if r['original_image_sha256_after_rebuild']!=r['before']['saved_image_sha256']:raise ValueError('original archive changed after recovery build')
    for key in ['original_image_id','post_recovery_image_id']:
        if type(r.get(key)) is not str or not re.fullmatch('sha256:[a-f0-9]{64}',r[key]):raise ValueError('invalid child build image identity')
    return r


def inspect_fixture_recovery(work,boot,invocation):
    private=work/'integration'
    read=lambda parent,name:read_fixture_artifact(parent,name,1<<20)
    receipt=decode(read(work,'integration-receipt.json'),1<<20)
    recovery=qualify(read(work,'recovery-checkpoint.json'),read(work,'runtime-exit-checkpoint.json'),receipt,boot,invocation)
    baseline=decode(read(private,'primary-docker-before.json'),1<<20)
    for phase,image in [('before',recovery['original_image_id']),('after',recovery['original_image_id']),('after-rebuild',recovery['post_recovery_image_id'])]:
        raw=read(private,'primary-docker-recovery-'+phase+'.json');report=decode(raw,1<<20)
        field={'before':'primary_docker_before_recovery_sha256','after':'primary_docker_after_recovery_sha256','after-rebuild':'primary_docker_after_rebuild_sha256'}[phase]
        if hashlib.sha256(raw).hexdigest()!=recovery[field]:raise ValueError('primary recovery observation digest differs')
        unchanged(baseline,report)
        if report.get('checked_absent_image')!=image or report.get('child_image_absent') is not True or fingerprint(recovery['before']['engine_id'])==report['engine_id_sha256']:
            raise ValueError('recovery child engine/image isolation is unqualified')
    for stream in ['stdout','stderr']:
        raw=read(private,'post-recovery-workload.'+stream)
        if len(raw)>131072 or hashlib.sha256(raw).hexdigest()!=recovery['post_recovery_'+stream+'_sha256']:raise ValueError('recovery build stream differs')
        if stream=='stdout':
            for marker in ['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']:
                if raw.splitlines().count(marker.encode())!=1:raise ValueError('post-recovery build matrix is incomplete')
            if re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',raw,re.M)!=[recovery['post_recovery_image_id'].encode()]:raise ValueError('post-recovery image marker differs')
        elif b'CACHED' not in raw or b'exit code: 37' not in raw:raise ValueError('post-recovery build cache/failure proof absent')
    return dict(version=1,scope='receipt-bound-private-runtime-restart-and-child-build-continuity',boot_id=boot,invocation_id=invocation,
                child_continuity_observed=True,original_archive_preserved=True,post_recovery_build_observed=True,
                exact_child_image_absence_observed=True,primary_engine_and_inventory_unchanged=True,cleanup_qualified=False)
