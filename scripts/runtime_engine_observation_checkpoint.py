"""Collect and requalify the owning-shim engine observation during a real trial."""
import hashlib,json,os,re,stat,subprocess as sp,time
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import read_fixture_artifact,decode,equal
from runtime_docker_concurrency import sandbox_id_for_container,run_markers
from runtime_trial_agent_profile import sha as artifact_sha
from runtime_bounded_command import bounded_command,CommandOutputLimit

PHASES=['while-full','during-build','while-image-ready','after-remove']
ORIGINAL_PROFILE=Path('/opt/mkruntime/g8-boot-d4643c8/private/profile.json')

def write_private(path,data):
    with path.open('xb') as output:
        os.fchmod(output.fileno(),0o600)
        output.write(data);output.flush();os.fsync(output.fileno())

def run_probe(args,private,phase,deadline_ns):
    if phase not in PHASES:raise ValueError('invalid engine probe phase')
    names=['engine-'+phase+suffix for suffix in ['.stdout','.stderr','.json','-failure.json']]
    if any(os.path.lexists(private/name) for name in names):raise ValueError('engine probe evidence already exists')
    try:
        result=bounded_command(args,deadline_ns,stdout_limit=2048,stderr_limit=256)
    except (sp.TimeoutExpired,CommandOutputLimit) as failure:
        # A failed probe is diagnostic evidence only. Never publish a phase
        # checkpoint or extend the qualification budget to wait for the CLI.
        streams={'stdout':failure.stdout or b'','stderr':failure.stderr or b''}
        limits={'stdout':2048,'stderr':256}
        for suffix,data in streams.items():
            write_private(private/('engine-'+phase+'.'+suffix),data[:limits[suffix]])
        record=dict(version=1,phase=phase,failure='command-timeout' if isinstance(failure,sp.TimeoutExpired) else 'command-output-refused',qualified=False,
                    stream_truncated={k:len(v)>limits[k] for k,v in streams.items()})
        write_private(private/('engine-'+phase+'-failure.json'),(json.dumps(record,sort_keys=True)+'\n').encode())
        raise
    for suffix,data in [('stdout',result.stdout),('stderr',result.stderr)]:write_private(private/('engine-'+phase+'.'+suffix),data)
    return result

def capture(work,boot,invocation,container,owner,phase,deadline):
    private=work/'integration';assert phase in PHASES
    request=dict(version=1,namespace='moby',container_id=container,sandbox_id=owner['sandbox_id'],generation=owner['generation'])
    assert sandbox_id_for_container(container)==owner['sandbox_id']
    address_file=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container/'address'
    saved=private/'engine-shim-address.json'
    if not saved.exists():
        info=address_file.lstat();assert stat.S_ISREG(info.st_mode) and info.st_uid==0 and info.st_nlink==1 and not info.st_mode&0o022 and 0<info.st_size<4096
        address=address_file.read_text().strip();assert address.startswith('unix:///') and '\n' not in address
        write_private(saved,(json.dumps(dict(address=address,container_id=container),sort_keys=True)+'\n').encode())
    reference=json.loads(saved.read_text());assert reference['container_id']==container
    if phase!='after-remove':assert address_file.read_text().strip()==reference['address']
    before=None
    if phase=='during-build':
        with (private/'workload.stderr').open('rb') as stream:before=stream.read(131073)
        assert len(before)<=131072 and run_markers(before.decode())=={'BEGIN'}
        write_private(private/'engine-build-before.stderr',before)
    start=time.monotonic_ns();end=min(int(deadline*1_000_000_000),start+5_000_000_000);assert end>start
    result=run_probe([str(work/'bin/mk-shimctl'),'--socket='+reference['address'],'--namespace=moby','--container-id='+container,'--sandbox-id='+owner['sandbox_id'],'--generation='+owner['generation']],private,phase,end)
    assert len(result.stdout)<=2048 and len(result.stderr)<=256
    hashes={}
    if phase=='during-build':
        with (private/'workload.stderr').open('rb') as stream:after=stream.read(131073)
        assert len(after)<=131072 and after.startswith(before) and run_markers(after.decode())=={'BEGIN'}
        write_private(private/'engine-build-after.stderr',after)
        hashes={which:hashlib.sha256(raw).hexdigest() for which,raw in [('before',before),('after',after)]}
    finish=time.monotonic_ns();assert finish<=end
    if phase=='after-remove':assert result.returncode==1 and not result.stdout and result.stderr==b'Docker engine inspection unavailable\n'
    else:assert result.returncode==0 and not result.stderr and json.loads(result.stdout)==dict(version=1,owner=request,observation=dict(version=1,source='owned-child-docker-api-and-launcher',status='responding'))
    record=dict(version=1,boot_id=boot,invocation_id=invocation,phase=phase,owner=request,budget_ns=5_000_000_000,started_monotonic_ns=start,deadline_monotonic_ns=end,finished_monotonic_ns=finish,exit_status=result.returncode,stdout_sha256=hashlib.sha256(result.stdout).hexdigest(),stderr_sha256=hashlib.sha256(result.stderr).hexdigest(),build_stream_sha256=hashes)
    write_private(private/('engine-'+phase+'.json'),(json.dumps(record,sort_keys=True)+'\n').encode())
    return record


def view(boot,invocation):
    return dict(version=1,scope='receipt-bound-shim-engine-observation-through-real-child-build',boot_id=boot,invocation_id=invocation,current_agent_and_approved_bootstrap_bound=True,owned_engine_responds_at_full_build_and_image_holds=True,original_unfinished_run_brackets_observation=True,removed_owner_query_refused=True,admission_guaranteed=False,cleanup_qualified=False)


def inspect_fixture_engine_observation(work,boot,invocation):
    private=work/'integration'
    read=lambda p,n:read_fixture_artifact(p,n,1<<20)
    raw=lambda n:read_fixture_artifact(private,n,131072,allow_empty=True)
    receipt=decode(read(work,'integration-receipt.json'),1<<20)
    if receipt.get('engine_observation_tested') is not True or receipt['boot_id']!=boot or receipt['invocation_id']!=invocation:raise ValueError('wrong engine observation lifecycle scope')
    build=decode(read(work,'build.json'),1<<20)
    profile_bytes=read(private,'current-agent-profile-receipt.json')
    if hashlib.sha256(profile_bytes).hexdigest()!=receipt['engine_profile_receipt_sha256']:raise ValueError('agent approval receipt differs')
    profile_receipt=decode(profile_bytes,1<<20)
    agent=work/'bin/mk-agent';archive=private/'current-agent.cpio.gz';approval=private/'manifests/gce-mk2.json'
    equal(profile_receipt,dict(version=1,source_commit=build['source_commit'],original_profile_sha256=artifact_sha(ORIGINAL_PROFILE),derived_profile_sha256=artifact_sha(approval),agent_sha256=artifact_sha(agent),initramfs_sha256=artifact_sha(archive),full_original_and_derived_validation=True,installed_approval_changed=False,credential_injection_requested=False))
    if profile_receipt['agent_sha256']!=build['binaries']['mk-agent'] or artifact_sha(work/'bin/mk-shimctl')!=build['binaries']['mk-shimctl']:raise ValueError('engine observer source binary differs')
    original=json.loads(ORIGINAL_PROFILE.read_text());derived=decode(read_fixture_artifact(approval.parent,approval.name,1<<20),1<<20)
    expected={**original,'agent':dict(path=str(agent),sha256=profile_receipt['agent_sha256']),'initramfs':dict(path=str(archive),sha256=profile_receipt['initramfs_sha256'])};equal(derived,expected)
    for which in ['original','current']:
        if raw(which+'-agent-profile-validation.stderr'):raise ValueError('bootstrap validation diagnostic')
        validated=decode(raw(which+'-agent-profile-validation.stdout'),1<<20)
        if which=='current' and validated['agent']!=str(agent):raise ValueError('validated agent differs')
    container=raw('create.stdout').decode().strip();expected_id=sandbox_id_for_container(container)
    ownership=decode(raw('pressure-full-ownership.stdout'),1<<20)
    if ownership.get('source')!='durable-lifecycle' or len(ownership['sandboxes'])!=1:raise ValueError('engine owner is ambiguous')
    owner=ownership['sandboxes'][0]
    if owner['sandbox_id']!=expected_id or owner['state']!='RUNNING' or type(owner['generation']) is not str or not re.fullmatch('[a-f0-9]{32}',owner['generation']):raise ValueError('engine owner differs')
    request=dict(version=1,namespace='moby',container_id=container,sandbox_id=expected_id,generation=owner['generation'])
    previous=None
    for phase in PHASES:
        data=read(private,'engine-'+phase+'.json');sample=decode(data,1<<20)
        fields={'version','boot_id','invocation_id','phase','owner','budget_ns','started_monotonic_ns','deadline_monotonic_ns','finished_monotonic_ns','exit_status','stdout_sha256','stderr_sha256','build_stream_sha256'}
        if type(sample) is not dict or set(sample)!=fields or sample['boot_id']!=boot or sample['invocation_id']!=invocation or sample['phase']!=phase or type(sample['version']) is not int or sample['version']!=1:raise ValueError('invalid engine sample epoch')
        equal(sample['owner'],request)
        for k in ['budget_ns','started_monotonic_ns','deadline_monotonic_ns','finished_monotonic_ns','exit_status']:
            if type(sample[k]) is not int:raise ValueError('invalid engine observation number')
        start,end,finish=[sample[k] for k in ['started_monotonic_ns','deadline_monotonic_ns','finished_monotonic_ns']]
        if sample['budget_ns']!=5_000_000_000 or not 0<start<=finish<=end<=start+5_000_000_000 or previous is not None and start<=previous:raise ValueError('engine sample budget/order differs')
        previous=finish
        stdout=raw('engine-'+phase+'.stdout');stderr=raw('engine-'+phase+'.stderr')
        if hashlib.sha256(data).hexdigest()!=receipt['engine_observation_sha256'][phase] or hashlib.sha256(stdout).hexdigest()!=sample['stdout_sha256'] or hashlib.sha256(stderr).hexdigest()!=sample['stderr_sha256']:raise ValueError('engine sample reference differs')
        if phase=='after-remove':
            if sample['exit_status']!=1 or stdout or stderr!=b'Docker engine inspection unavailable\n':raise ValueError('removed engine query was accepted')
        else:
            if sample['exit_status']!=0 or stderr:raise ValueError('live engine query failed')
            equal(decode(stdout,2048),dict(version=1,owner=request,observation=dict(version=1,source='owned-child-docker-api-and-launcher',status='responding')))
        if phase=='during-build':
            streams={which:raw('engine-build-'+which+'.stderr') for which in ['before','after']};final=raw('workload.stderr')
            equal(sample['build_stream_sha256'],{k:hashlib.sha256(v).hexdigest() for k,v in streams.items()})
            if not streams['after'].startswith(streams['before']) or any(run_markers(v.decode())!={'BEGIN'} or not final.startswith(v) for v in streams.values()) or run_markers(final.decode())!={'BEGIN','END'}:raise ValueError('engine observation was not bound to the original unfinished RUN')
        elif sample['build_stream_sha256']!={}:raise ValueError('unexpected build window')
    return view(boot,invocation)
