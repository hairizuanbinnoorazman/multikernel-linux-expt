"""Bind an explicitly selected private-runtime recovery to its build receipt.

No repair, live-process authority or full gate follows from archived snapshots.
"""
import hashlib,json,re
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_primary_docker import unchanged,fingerprint


def recovery_policy(scenario):
    if type(scenario) is not str or scenario not in {'runtime-restart','runtime-crash','runtime-crash-during-build','runtime-crash-during-io'}:
        raise ValueError('invalid runtime recovery scenario')
    return ('SIGTERM',0,False) if scenario=='runtime-restart' else ('SIGKILL',-9,True)


def recovery_view(boot,invocation,scenario='runtime-restart'):
    recovery_policy(scenario)
    kind={'runtime-restart':'restart','runtime-crash':'crash','runtime-crash-during-build':'inflight-crash','runtime-crash-during-io':'inflight-io-crash'}[scenario]
    view=dict(version=1,scope='receipt-bound-private-runtime-'+kind+'-and-child-build-continuity',boot_id=boot,invocation_id=invocation,
                child_continuity_observed=True,original_archive_preserved=True,post_recovery_build_observed=True,
                exact_child_image_absence_observed=True,primary_engine_and_inventory_unchanged=True,cleanup_qualified=False)
    if scenario in {'runtime-crash-during-build','runtime-crash-during-io'}:view['active_run_window_observed']=True
    if scenario=='runtime-crash-during-io':view.update(synced_write_outage_observed=True,built_image_data_readback_observed=True,native_write_coverage_observed=True)
    return view


def qualify(data,exit_data,receipt,boot,invocation,scenario='runtime-restart'):
    expected_signal,expected_status,expected_crash=recovery_policy(scenario)
    io=scenario=='runtime-crash-during-io';active=io or scenario=='runtime-crash-during-build'
    r=decode(data,1<<20);exit=decode(exit_data,65536)
    if any(type(v) is not dict for v in [r,exit,receipt]):raise ValueError('invalid runtime recovery envelope')
    fields={'schema_version','invocation_id','boot_id','runtime_pid_before','runtime_pid_after','runtime_restarts','runtime_exit_signal','runtime_exit_status','before','after','after_build','post_recovery_build_tested','post_recovery_stdout_sha256','post_recovery_stderr_sha256','child_continuity_pass','cleanup_qualified','runtime_exit_checkpoint_sha256','completed_first_build','crash_during_build','original_image_id','post_recovery_image_id','original_image_sha256_after_rebuild','primary_docker_before_recovery_sha256','primary_docker_after_recovery_sha256','primary_docker_after_rebuild_sha256'}
    if active:fields|={'active_run_checkpoint_sha256','primary_docker_completed_first_sha256'}
    if set(r)!=fields:raise ValueError('invalid runtime recovery fields')
    if (not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',boot) or
            not re.fullmatch('[a-f0-9]{32}',invocation) or r.get('boot_id')!=boot or r.get('invocation_id')!=invocation or
            type(r.get('schema_version')) is not int or r['schema_version']!=(4 if io else 3 if active else 2) or
            receipt.get('boot_id')!=boot or receipt.get('invocation_id')!=invocation or
            receipt.get('runtime_restart_tested') is not True or receipt.get('runtime_crash_tested') is not expected_crash or
            receipt.get('runtime_crash_during_build_tested') is not active or receipt.get('storage_pressure_tested') is not False or
            receipt.get('runtime_io_during_crash_tested',False) is not io or
            receipt.get('production_profile_child_build_pass') is not True or
            type(r.get('runtime_restarts')) is not int or r['runtime_restarts']!=1 or
            r.get('runtime_exit_signal')!=expected_signal or type(r.get('runtime_exit_status')) is not int or r['runtime_exit_status']!=expected_status or
            r.get('crash_during_build') is not active or r.get('post_recovery_build_tested') is not True or
            r.get('child_continuity_pass') is not True or r.get('cleanup_qualified') is not False):
        raise ValueError('runtime recovery checkpoint is unqualified')
    for key in ['runtime_pid_before','runtime_pid_after']:
        if type(r.get(key)) is not int or not 1<r[key]<1<<31:raise ValueError('runtime recovery process identity is invalid')
    if r['runtime_pid_before']==r['runtime_pid_after']:raise ValueError('runtime recovery process did not change')
    expected_exit=dict(invocation_id=invocation,boot_id=boot,runtime_pid=r['runtime_pid_before'],runtime_exit_signal=expected_signal,runtime_exit_status=expected_status,observed_monotonic_ns=exit.get('observed_monotonic_ns'),cleanup_qualified=False)
    if type(exit.get('observed_monotonic_ns')) is not int or exit['observed_monotonic_ns']<0:raise ValueError('invalid runtime exit timing')
    equal(exit,expected_exit)
    if hashlib.sha256(exit_data).hexdigest()!=r.get('runtime_exit_checkpoint_sha256') or hashlib.sha256(data).hexdigest()!=receipt.get('recovery_checkpoint_sha256'):
        raise ValueError('runtime recovery receipt digest differs')
    for key in ['before','after','after_build','completed_first_build']:
        snapshot=r.get(key)
        saved=not active or key not in {'before','after'}
        snapshot_fields={'child_boot_id','engine_id','ownership'}|({'saved_image_sha256'} if saved else set())
        if (type(snapshot) is not dict or set(snapshot)!=snapshot_fields or
                type(snapshot['child_boot_id']) is not str or not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',snapshot['child_boot_id']) or
                snapshot['child_boot_id']==boot or type(snapshot['engine_id']) is not str or not re.fullmatch('[A-Za-z0-9:_-]{1,256}',snapshot['engine_id']) or
                saved and (type(snapshot['saved_image_sha256']) is not str or not re.fullmatch('[a-f0-9]{64}',snapshot['saved_image_sha256'])) or type(snapshot['ownership']) is not list or len(snapshot['ownership'])!=1):
            raise ValueError('runtime recovery child snapshot is invalid')
        owner=snapshot['ownership'][0]
        if (type(owner) is not dict or set(owner)!={'sandbox_id','generation','state','cpus','memory_bytes','child_cid','agent_port'} or
                type(owner['sandbox_id']) is not str or not re.fullmatch('[A-Za-z0-9][A-Za-z0-9_.-]{0,127}',owner['sandbox_id']) or type(owner['generation']) is not str or not re.fullmatch('[a-f0-9]{32}',owner['generation']) or
                owner['state']!='RUNNING' or type(owner['cpus']) is not list or not owner['cpus'] or
                any(type(v) is not int or not 0<=v<1<<31 for v in owner['cpus']) or owner['cpus']!=sorted(set(owner['cpus'])) or
                any(type(owner[k]) is not int or not 0<owner[k]<(1<<64 if k=='memory_bytes' else 1<<32) for k in ['memory_bytes','child_cid','agent_port']) or owner['child_cid']<3):
            raise ValueError('runtime recovery owner is invalid')
    equal(r['before'],r['after'])
    if not active:equal(r['before'],r['completed_first_build'])
    for key in ['child_boot_id','engine_id','ownership']:
        equal(r['before'][key],r['after_build'][key]);equal(r['before'][key],r['completed_first_build'][key])
    if r['original_image_sha256_after_rebuild']!=r['completed_first_build']['saved_image_sha256']:raise ValueError('original archive changed after recovery build')
    for key in ['original_image_id','post_recovery_image_id']:
        if type(r.get(key)) is not str or not re.fullmatch('sha256:[a-f0-9]{64}',r[key]):raise ValueError('invalid child build image identity')
    return r


def inspect_fixture_recovery(work,boot,invocation,scenario='runtime-restart'):
    private=work/'integration'
    read=lambda parent,name:read_fixture_artifact(parent,name,1<<20)
    receipt=decode(read(work,'integration-receipt.json'),1<<20)
    recovery=qualify(read(work,'recovery-checkpoint.json'),read(work,'runtime-exit-checkpoint.json'),receipt,boot,invocation,scenario)
    baseline=decode(read(private,'primary-docker-before.json'),1<<20)
    active=scenario in {'runtime-crash-during-build','runtime-crash-during-io'}
    phases=[('before',None if active else recovery['original_image_id']),('after',None if active else recovery['original_image_id']),('after-rebuild',recovery['post_recovery_image_id'])]
    if active:phases.append(('completed-first',recovery['original_image_id']))
    for phase,image in phases:
        raw=read(private,'primary-docker-recovery-'+phase+'.json');report=decode(raw,1<<20)
        field={'before':'primary_docker_before_recovery_sha256','after':'primary_docker_after_recovery_sha256','after-rebuild':'primary_docker_after_rebuild_sha256','completed-first':'primary_docker_completed_first_sha256'}[phase]
        if hashlib.sha256(raw).hexdigest()!=recovery[field]:raise ValueError('primary recovery observation digest differs')
        unchanged(baseline,report)
        if report.get('checked_absent_image')!=image or report.get('child_image_absent') is not (image is not None) or fingerprint(recovery['before']['engine_id'])==report['engine_id_sha256']:
            raise ValueError('recovery child engine/image isolation is unqualified')
    for stream in ['stdout','stderr']:
        raw=read(private,'post-recovery-workload.'+stream)
        if len(raw)>131072 or hashlib.sha256(raw).hexdigest()!=recovery['post_recovery_'+stream+'_sha256']:raise ValueError('recovery build stream differs')
        if stream=='stdout':
            for marker in ['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']:
                if raw.splitlines().count(marker.encode())!=1:raise ValueError('post-recovery build matrix is incomplete')
            if re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',raw,re.M)!=[recovery['post_recovery_image_id'].encode()]:raise ValueError('post-recovery image marker differs')
        elif b'CACHED' not in raw or b'exit code: 37' not in raw:raise ValueError('post-recovery build cache/failure proof absent')
    if active:inspect_active_run(work,recovery,receipt)
    return recovery_view(boot,invocation,scenario)


def inspect_active_run(work,recovery,receipt):
    """Bind all three unfinished RUN snapshots to the final original stream."""
    private=work/'integration';read=lambda parent,name:read_fixture_artifact(parent,name,1<<20)
    io=recovery['schema_version']==4
    data=read(work,'active-run-checkpoint.json');active=decode(data,1<<20)
    if hashlib.sha256(data).hexdigest()!=recovery['active_run_checkpoint_sha256']:raise ValueError('active RUN checkpoint digest differs')
    if type(active) is not dict or set(active)!={'version','boot_id','invocation_id','stages','cleanup_qualified'}:raise ValueError('invalid active RUN envelope')
    equal({k:v for k,v in active.items() if k!='stages'},dict(version=3 if io else 1,boot_id=recovery['boot_id'],invocation_id=recovery['invocation_id'],cleanup_qualified=False))
    stage_names={'before-kill','after-kill','after-adoption'}|({'confirmed-dead','adoption-baseline'} if io else set())
    if type(active['stages']) is not dict or set(active['stages'])!=stage_names:raise ValueError('incomplete active RUN stages')
    workload_data=read(work,'workload-checkpoint.json');workload=decode(workload_data,1<<20)
    if hashlib.sha256(workload_data).hexdigest()!=receipt['workload_checkpoint_sha256'] or workload['boot_id']!=recovery['boot_id'] or workload['invocation_id']!=recovery['invocation_id']:raise ValueError('original workload receipt differs')
    streams={s:read(private,'workload.'+s) for s in ['stdout','stderr']}
    for stream,raw in streams.items():
        if len(raw)>131072 or hashlib.sha256(raw).hexdigest()!=workload['workload_'+stream+'_sha256']:raise ValueError('original workload stream differs')
    events=lambda raw:re.findall(rb'^#\d+\s+\d+(?:\.\d+)?\s+G8_OVERLAP_RUN_(BEGIN|END)\s*$',raw,re.M)
    if events(streams['stderr'])!=[b'BEGIN',b'END']:raise ValueError('original RUN completion is unqualified')
    for marker in ['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']:
        if streams['stdout'].splitlines().count(marker.encode())!=1:raise ValueError('original in-flight build matrix is incomplete')
    if b'CACHED' not in streams['stderr'] or b'exit code: 37' not in streams['stderr']:raise ValueError('original in-flight cache/failure proof absent')
    if re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',streams['stdout'],re.M)!=[recovery['original_image_id'].encode()]:raise ValueError('original image marker differs')
    times=[];last_size=0
    phases=[('before-kill',recovery['runtime_pid_before'])]
    if io:phases.append(('confirmed-dead',recovery['runtime_pid_before']))
    phases.append(('after-kill',recovery['runtime_pid_before']))
    if io:phases.append(('adoption-baseline',recovery['runtime_pid_after']))
    phases.append(('after-adoption',recovery['runtime_pid_after']))
    for phase,pid in phases:
        frame=active['stages'][phase]
        fields={'observed_monotonic_ns','runtime_pid','markers','stderr_snapshot','stderr_sha256'}|({'io_synced_seq'} if io else set())|({'runtime_exit_status'} if phase=='confirmed-dead' else set())
        if type(frame) is not dict or set(frame)!=fields:raise ValueError('invalid active RUN stage')
        expected=dict(observed_monotonic_ns=frame['observed_monotonic_ns'],runtime_pid=pid,markers=['BEGIN'],stderr_snapshot='active-run-'+phase+'.stderr',stderr_sha256=frame['stderr_sha256'])
        if io:expected['io_synced_seq']=frame['io_synced_seq']
        if phase=='confirmed-dead':expected['runtime_exit_status']=-9
        equal(frame,expected)
        t=frame['observed_monotonic_ns']
        if type(t) is not int or not 0<t<1<<64:raise ValueError('invalid active RUN timing')
        raw=read(private,expected['stderr_snapshot'])
        if not last_size<=len(raw)<=131072 or hashlib.sha256(raw).hexdigest()!=frame['stderr_sha256'] or events(raw)!=[b'BEGIN'] or not streams['stderr'].startswith(raw):raise ValueError('active RUN snapshot differs from original stream')
        last_size=len(raw);times.append(t)
    exit=decode(read(work,'runtime-exit-checkpoint.json'),65536)
    ordered=times[0]<exit['observed_monotonic_ns']<times[1] and all(a<b for a,b in zip(times,times[1:]))
    if not ordered or times[-1]-times[0]>45_000_000_000:raise ValueError('active RUN kill/adoption ordering is unqualified')
    if io:
        from runtime_io_recovery_checkpoint import inspect_synced_io
        inspect_synced_io(work,recovery,streams,active)
