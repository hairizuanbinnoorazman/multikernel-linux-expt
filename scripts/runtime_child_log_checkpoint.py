"""Requalify detached child build/log observations without reading log contents."""
import hashlib,re
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_trial_agent_profile import inspect_current_agent
from runtime_child_log_pressure import WORKLOAD,WRITER_BYTES
from runtime_docker_log_rotation import MAX_FILE,RECORD_SLACK
from runtime_primary_docker import unchanged,fingerprint


def view(boot,invocation):
    return dict(version=3,scope='current-agent-child-build-and-local-log-rotation',boot_id=boot,invocation_id=invocation,
                current_agent_and_approved_bootstrap_bound=True,deployment_generated_logging_defaults_bound=True,complete_child_build_matrix_requalified=True,
                detached_writer_completed=True,finite_writer_exit_and_exact_byte_count_bound=True,local_log_rotation_observed=True,primary_engine_and_image_absence_observed=True,
                log_contents_read=False,loss_accounting_qualified=False,cleanup_qualified=False)


def inspect_fixture_logging(work,boot,invocation):
    from runtime_concurrency_checkpoint import matrix
    private=work/'integration'
    read=lambda name,empty=False:read_fixture_artifact(private,name,1<<20,allow_empty=empty)
    receipt=decode(read_fixture_artifact(work,'integration-receipt.json',1<<20),1<<20)
    if not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',boot) or not re.fullmatch('[a-f0-9]{32}',invocation):raise ValueError('invalid logging epoch')
    if receipt.get('child_log_pressure_tested') is not True or receipt.get('production_profile_child_build_pass') is not True or receipt.get('boot_id')!=boot or receipt.get('invocation_id')!=invocation:raise ValueError('logging lifecycle is unqualified')
    for key in ['storage_pressure_tested','concurrent_guest_builds_tested','runtime_restart_tested','runtime_crash_tested','engine_observation_tested','aggregate_docker_health_tested']:
        if receipt.get(key) is not False:raise ValueError('logging lifecycle scope differs')
    data=read('child-log-pressure.json');r=decode(data,1<<20)
    fields={'deployment_config_receipt_sha256','create_argv_sha256','create_logging_policy_source','version','boot_id','invocation_id','writer_bytes','writer_exit_status','writer_completed_bytes','detached_start','agent_profile_receipt_sha256','build_image_id','saved_image_sha256','rotation','log_primary_sha256','command_stdout_sha256','workload_stdout_sha256','workload_stderr_sha256','log_contents_read','loss_accounting_qualified','cleanup_qualified'}
    if type(r) is not dict or set(r)!=fields or hashlib.sha256(data).hexdigest()!=receipt.get('child_log_pressure_sha256'):raise ValueError('logging envelope differs')
    equal({key:r[key] for key in ['version','boot_id','invocation_id','writer_bytes','writer_exit_status','writer_completed_bytes','detached_start','log_contents_read','loss_accounting_qualified','cleanup_qualified']},dict(version=3,boot_id=boot,invocation_id=invocation,writer_bytes=WRITER_BYTES,writer_exit_status=0,writer_completed_bytes=WRITER_BYTES,detached_start=True,log_contents_read=False,loss_accounting_qualified=False,cleanup_qualified=False))
    if r['create_logging_policy_source']!='daemon-default':raise ValueError('logging policy inheritance differs')
    from runtime_deployment_logging import inspect_defaults
    inspect_defaults(work,r['deployment_config_receipt_sha256'],r['create_argv_sha256'],WORKLOAD)
    inspect_current_agent(work,r['agent_profile_receipt_sha256'])
    if not re.fullmatch('sha256:[a-f0-9]{64}',r['build_image_id']) or not re.fullmatch('[a-f0-9]{64}',r['saved_image_sha256']):raise ValueError('logging build identity differs')
    labels=['log-status','log-writer-status','log-writer-bytes','log-writer-diagnostic','log-build-out','log-build-err','log-saved-image','log-engine','log-wait','log-inspect']
    if type(r['command_stdout_sha256']) is not dict or set(r['command_stdout_sha256'])!=set(labels):raise ValueError('logging captures incomplete')
    for label in labels:
        if hashlib.sha256(read(label+'.stdout')).hexdigest()!=r['command_stdout_sha256'][label]:raise ValueError('logging command differs')
        equal(decode(read(label+'.exit.json'),65536),dict(returncode=0))
        read(label+'.stderr',True)
    if read('log-status.stdout')!=b'0\n' or read('log-wait.stdout')!=b'0\n':raise ValueError('logging workload completion differs')
    if read('log-writer-status.stdout')!=b'0\n' or read('log-writer-bytes.stdout')!=str(WRITER_BYTES).encode()+b'\n':raise ValueError('finite logging writer completion differs')
    stdout,stderr=read('workload.stdout'),read('workload.stderr')
    if stdout!=read('log-build-out.stdout') or stderr!=read('log-build-err.stdout') or hashlib.sha256(stdout).hexdigest()!=r['workload_stdout_sha256'] or hashlib.sha256(stderr).hexdigest()!=r['workload_stderr_sha256']:raise ValueError('logging build streams differ')
    matrix(stdout,stderr,r['build_image_id'],True)
    saved=read('log-saved-image.stdout').decode().split()
    if saved!=[r['saved_image_sha256'],'/home/rootless/.local/tmp/g8-build/image.tar'] or saved[0].encode() not in stdout:raise ValueError('logging archive readback differs')
    item=decode(read('log-inspect.stdout'),1<<20)
    if type(item) is not list or len(item)!=1:raise ValueError('logging container ambiguous')
    item=item[0]
    from runtime_trial_output import require_logging
    require_logging(item['HostConfig'].get('LogConfig'))
    if item['Config']['Cmd']!=WORKLOAD or item['Config']['User']!='1000:1000' or item['HostConfig']['Runtime']!='io.containerd.multikernel.v2' or item['HostConfig']['Privileged'] is not False or item['HostConfig']['CapDrop']!=['ALL'] or 'no-new-privileges=true' not in item['HostConfig']['SecurityOpt']:raise ValueError('logging workload authority differs')
    equal(item['State'],{**item['State'],'Status':'exited','ExitCode':0})
    rotation=r['rotation']
    if type(rotation) is not dict or set(rotation)!={'rotated_file_count','observed_bytes','conservative_sample_bound','log_contents_read'} or type(rotation['rotated_file_count']) is not int or rotation['rotated_file_count']!=2 or type(rotation['observed_bytes']) is not int or not MAX_FILE<rotation['observed_bytes']<=2*(MAX_FILE+RECORD_SLACK) or type(rotation['conservative_sample_bound']) is not int or rotation['conservative_sample_bound']!=2*(MAX_FILE+RECORD_SLACK) or rotation['log_contents_read'] is not False:raise ValueError('logging rotation sample unqualified')
    profile=decode(read_fixture_artifact(private/'manifests','gce-mk2.json',1<<20),1<<20)
    engine=decode(read('log-engine.stdout'),1<<20)
    if not engine.get('ID') or engine.get('KernelVersion')!=profile['kernel_release'] or engine.get('ServerVersion')!='29.9.0' or 'name=rootless' not in engine.get('SecurityOptions',[]):raise ValueError('logging child engine differs')
    primary_data=read('log-primary.json');primary=decode(primary_data,1<<20)
    if hashlib.sha256(primary_data).hexdigest()!=r['log_primary_sha256']:raise ValueError('logging primary sample differs')
    unchanged(decode(read('primary-docker-before.json'),1<<20),primary)
    if primary['checked_absent_image']!=r['build_image_id'] or primary['child_image_absent'] is not True or fingerprint(engine['ID'])==primary['engine_id_sha256']:raise ValueError('logging primary image/engine isolation differs')
    return view(boot,invocation)
