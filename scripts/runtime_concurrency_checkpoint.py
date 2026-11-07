"""Qualify archived two-child build separation and survivor work, without repair."""
import hashlib,re
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_primary_docker import unchanged,fingerprint
from runtime_docker_concurrency import owner_for_container

BOOT=r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}'
HEX=r'[a-f0-9]{64}'
IMAGE='sha256:'+HEX


def concurrency_view(boot,invocation):
    return dict(version=1,scope='receipt-bound-two-child-build-separation-and-survivor-rebuild',boot_id=boot,invocation_id=invocation,
                two_child_engines_and_storage_separate=True,simultaneous_unfinished_runs_observed=True,sibling_release_preserves_survivor=True,
                survivor_rebuild_and_original_archive_preservation=True,primary_engine_inventory_and_exact_image_absence_observed=True,cleanup_qualified=False)


def valid_owner(owner):
    fields={'sandbox_id','generation','state','cpus','memory_bytes','child_cid','agent_port'}
    if (type(owner) is not dict or set(owner)!=fields or type(owner['sandbox_id']) is not str or not re.fullmatch('[A-Za-z0-9][A-Za-z0-9_.-]{0,127}',owner['sandbox_id']) or
            type(owner['generation']) is not str or not re.fullmatch('[a-f0-9]{32}',owner['generation']) or owner['state']!='RUNNING' or
            type(owner['cpus']) is not list or any(type(v) is not int for v in owner['cpus']) or tuple(owner['cpus']) not in {(8,9,10,11),(12,13,14,15)} or
            any(type(owner[k]) is not int or not 0<owner[k]<1<<64 for k in ['memory_bytes','child_cid','agent_port']) or owner['child_cid'] not in {40,41} or owner['agent_port']>=1<<32):
        raise ValueError('invalid concurrent child owner')


def matrix(stdout,stderr,image,authority=False):
    if len(stdout)>131072 or len(stderr)>131072:raise ValueError('concurrent build stream exceeds bound')
    for marker in ['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']:
        if stdout.splitlines().count(marker.encode())!=1:raise ValueError('concurrent build matrix incomplete')
    if re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',stdout,re.M)!=[image.encode()] or b'CACHED' not in stderr or b'exit code: 37' not in stderr:
        raise ValueError('concurrent build image/cache/failure evidence differs')
    if authority:
        nested=stdout.split(b'G8_NESTED_AUTHORITY_BEGIN\n')
        if len(nested)!=2 or b'G8_NESTED_AUTHORITY_END' not in nested[1]:raise ValueError('nested concurrent authority is missing')
        nested=nested[1].split(b'G8_NESTED_AUTHORITY_END')[0]
        if b'CapEff:\t0000000000000000' not in stdout or b'CapBnd:\t0000000000000000' not in stdout or b'NoNewPrivs:\t1' not in nested or b'Seccomp:\t2' not in nested or not re.search(rb'0\s+1000\s+1',nested):raise ValueError('concurrent workload authority differs')


def inspect_fixture_concurrency(work,boot,invocation):
    private=work/'integration'
    read=lambda p,n:read_fixture_artifact(p,n,1<<20)
    receipt=decode(read(work,'integration-receipt.json'),1<<20)
    if not re.fullmatch(BOOT,boot) or not re.fullmatch('[a-f0-9]{32}',invocation):raise ValueError('invalid concurrency epoch')
    if receipt.get('boot_id')!=boot or receipt.get('invocation_id')!=invocation or receipt.get('concurrent_guest_builds_tested') is not True or receipt.get('production_profile_child_build_pass') is not True:
        raise ValueError('concurrent lifecycle is unqualified')
    for key in ['storage_pressure_tested','runtime_restart_tested','runtime_crash_tested','runtime_crash_during_build_tested']:
        if receipt.get(key) is not False:raise ValueError('concurrent lifecycle scope differs')
    if receipt.get('runtime_io_during_crash_tested',False) is not False:raise ValueError('concurrent lifecycle scope differs')
    data=read(private,'concurrency-checkpoint.json');r=decode(data,1<<20)
    fields={'schema_version','boot_id','invocation_id','before','after','survivor','two_running_owners','case_owners','remaining_owner','two_storage_exports','survivor_storage_export','run_overlap_observed','sibling_removed_without_survivor_change','cleanup_qualified','survivor_rebuilt','original_archive_after_rebuild_sha256','survivor_rebuild_stdout_sha256','survivor_rebuild_stderr_sha256','run_overlap_checkpoint_sha256','primary_snapshots_sha256','case_stdout_sha256','case_stderr_sha256'}
    if type(r) is not dict or set(r)!=fields or hashlib.sha256(data).hexdigest()!=receipt.get('concurrency_checkpoint_sha256'):raise ValueError('concurrency checkpoint envelope differs')
    if type(r['schema_version']) is not int or r['schema_version']!=2 or r['boot_id']!=boot or r['invocation_id']!=invocation or r['run_overlap_observed'] is not True or r['sibling_removed_without_survivor_change'] is not True or r['cleanup_qualified'] is not False:raise ValueError('concurrency checkpoint is unqualified')
    for field in ['before','after','case_owners','case_stdout_sha256','case_stderr_sha256']:
        if type(r[field]) is not dict or set(r[field])!={'a','b'}:raise ValueError('concurrent case identities are ambiguous')
    base={'child_boot_id','engine_id','canary','sentinel_image_id'}
    def snapshot(s,built=False):
        if type(s) is not dict or set(s)!=base|({'saved_image_sha256','built_image_id'} if built else set()):raise ValueError('invalid concurrent snapshot fields')
        if (type(s['child_boot_id']) is not str or not re.fullmatch(BOOT,s['child_boot_id']) or s['child_boot_id']==boot or
                type(s['engine_id']) is not str or not re.fullmatch('[A-Za-z0-9:_-]{1,256}',s['engine_id']) or
                type(s['canary']) is not str or not re.fullmatch('[a-f0-9]{32}',s['canary']) or type(s['sentinel_image_id']) is not str or not re.fullmatch(IMAGE,s['sentinel_image_id'])):raise ValueError('invalid concurrent snapshot')
        if built and (type(s['saved_image_sha256']) is not str or not re.fullmatch(HEX,s['saved_image_sha256']) or type(s['built_image_id']) is not str or not re.fullmatch(IMAGE,s['built_image_id'])):raise ValueError('invalid concurrent build snapshot')
    for case in ['a','b']:
        snapshot(r['before'][case]);snapshot(r['after'][case],True)
        equal(r['before'][case],{k:r['after'][case][k] for k in base})
    for field in base:
        if r['before']['a'][field]==r['before']['b'][field]:raise ValueError('concurrent children share identity or data')
    snapshot(r['survivor'],True);equal(r['survivor'],r['after']['a'])
    snapshot(r['survivor_rebuilt'],True);equal({k:r['survivor_rebuilt'][k] for k in base},r['before']['a'])
    if r['original_archive_after_rebuild_sha256']!=r['after']['a']['saved_image_sha256']:raise ValueError('original survivor archive changed')
    profile=decode(read(private/'manifests','gce-mk2.json'),1<<20)
    for case in ['a','b']:
        phases=[('before',r['before'][case],False),('after',r['after'][case],True)]
        if case=='a':phases.extend([('survivor',r['survivor'],True),('survivor-rebuilt',r['survivor_rebuilt'],True)])
        for phase,view,built in phases:
            prefix=phase+'-'+case
            if read(private,prefix+'-boot.stdout').decode().strip()!=view['child_boot_id'] or read(private,prefix+'-canary.stdout').decode().strip()!=view['canary'] or read(private,prefix+'-own-image.stdout').decode().strip()!=view['sentinel_image_id']:raise ValueError('concurrent snapshot differs from archived child inspection')
            engine=decode(read(private,prefix+'-engine.stdout'),1<<20)
            if engine.get('ID')!=view['engine_id'] or engine.get('KernelVersion')!=profile['kernel_release'] or engine.get('ServerVersion')!='29.9.0' or 'name=rootless' not in engine.get('SecurityOptions',[]):raise ValueError('concurrent child engine inspection differs')
            if built and read(private,prefix+'-saved-image.stdout').decode().split()[0]!=view['saved_image_sha256']:raise ValueError('concurrent saved archive inspection differs')
    if read(private,'survivor-original-archive.stdout').decode().split()[0]!=r['original_archive_after_rebuild_sha256']:raise ValueError('original survivor archive readback differs')
    owners=r['two_running_owners']
    if type(owners) is not list or len(owners)!=2:raise ValueError('two concurrent owners required')
    for owner in owners:valid_owner(owner)
    reported=decode(read(private,'two-child-ownership.stdout'),1<<20)
    if reported.get('source')!='durable-lifecycle' or type(reported.get('version')) is not int or reported['version']!=1:raise ValueError('concurrent ownership source differs')
    equal(reported['sandboxes'],owners)
    if {o['child_cid'] for o in owners}!={40,41} or len({o['sandbox_id'] for o in owners})!=2 or len({o['generation'] for o in owners})!=2 or set(owners[0]['cpus'])&set(owners[1]['cpus']):raise ValueError('concurrent owner allocation overlaps')
    for case in ['a','b']:
        item=decode(read(private,'owner-map-'+case+'.stdout'),1<<20)
        if type(item) is not list or len(item)!=1:raise ValueError('outer container identity is ambiguous')
        item=item[0];equal(r['case_owners'][case],owner_for_container(item['Id'],owners))
        if type(item['Mounts']) is not list or any(type(v.get('RW')) is not bool for v in item['Mounts']):raise ValueError('concurrent mount authority is not canonical')
        if item['Config']['User']!='1000:1000' or item['HostConfig']['Runtime']!='io.containerd.multikernel.v2' or item['HostConfig']['Privileged'] is not False or item['HostConfig']['CapDrop']!=['ALL'] or 'no-new-privileges=true' not in item['HostConfig']['SecurityOpt'] or [{'destination':v['Destination'],'read_only':not v['RW']} for v in item['Mounts']]!=[dict(destination='/work/input',read_only=True)]:raise ValueError('outer concurrent authority differs')
    equal(r['remaining_owner'],r['case_owners']['a'])
    actual_remaining=decode(read(private,'survivor-ownership.stdout'),1<<20)
    equal([v for v in actual_remaining['sandboxes'] if v['state']!='ABSENT'],[r['remaining_owner']])
    storage=r['two_storage_exports']
    fields={'sandbox_id','sandbox_generation','export_generation','peer_cid','port','filesystem_uuid','image_identity','image_path_sha256'}
    if type(storage) is not list or len(storage)!=2:raise ValueError('two native backing identities required')
    for value in storage:
        if type(value) is not dict or set(value)!=fields or type(value['peer_cid']) is not int or type(value['port']) is not int or not 0<value['port']<1<<32 or not re.fullmatch(BOOT,value['filesystem_uuid']) or not re.fullmatch(HEX,value['image_path_sha256']) or not re.fullmatch('[a-f0-9]{32}',value['export_generation']):raise ValueError('invalid concurrent storage identity')
        identity=value['image_identity']
        if type(identity) is not dict or set(identity)!={'device','inode'} or any(type(v) is not int or not 0<v<1<<64 for v in identity.values()):raise ValueError('invalid concurrent backing inode')
        owner=next((o for o in owners if o['child_cid']==value['peer_cid']),None)
        if owner is None or value['sandbox_id']!=owner['sandbox_id'] or value['sandbox_generation']!=owner['generation']:raise ValueError('concurrent storage owner differs')
    for key in ['port','filesystem_uuid','image_path_sha256','export_generation']:
        if len({v[key] for v in storage})!=2:raise ValueError('concurrent backing identities overlap')
    if len({(v['image_identity']['device'],v['image_identity']['inode']) for v in storage})!=2:raise ValueError('concurrent backing files overlap')
    equal(storage,decode(read(private,'two-child-storage.json'),1<<20))
    expected=[v for v in storage if v['peer_cid']==r['remaining_owner']['child_cid']]
    equal(r['survivor_storage_export'],expected);equal(expected,decode(read(private,'survivor-storage.json'),1<<20))
    final=decode(read(private/'storage-state','state.json'),1<<20).get('exports')
    if type(final) is not dict or len(final)!=2:raise ValueError('final concurrent native inventory differs')
    for value in storage:
        leases=[v for v in final.values() if v.get('export_generation')==value['export_generation']]
        if len(leases)!=1:raise ValueError('final concurrent native generation differs')
        lease=leases[0]
        if any(lease.get(k)!=value[k] for k in ['sandbox_id','sandbox_generation','peer_cid','filesystem_uuid','port']) or lease.get('state')!='RELEASED' or not re.fullmatch('e2fsck-clean-sha256:'+HEX,lease.get('offline_check','')):raise ValueError('concurrent native release differs from original owner')
    for case in ['a','b']:
        stdout=read(private,case+'-workload.stdout');stderr=read(private,case+'-workload.stderr')
        if hashlib.sha256(stdout).hexdigest()!=r['case_stdout_sha256'][case] or hashlib.sha256(stderr).hexdigest()!=r['case_stderr_sha256'][case]:raise ValueError('concurrent build stream differs')
        matrix(stdout,stderr,r['after'][case]['built_image_id'],True)
        for marker in [b'G8_CONCURRENT_PROBE_EXIT=0',b'G8_CONCURRENT_BUILD_READY',b'G8_CONCURRENT_RELEASED']:
            if stdout.splitlines().count(marker)!=1:raise ValueError('concurrent wrapper completion is unqualified')
        if r['after'][case]['saved_image_sha256'].encode() not in stdout:raise ValueError('concurrent saved archive evidence missing')
        for phase in ['before','after','survivor','survivor-rebuilt'] if case=='a' else ['before','after']:
            failed=read(private,phase+'-'+case+'-foreign-image.stderr')
            status=decode(read(private,phase+'-'+case+'-foreign-image.exit.json'),65536)
            if type(status) is not dict or set(status)!={'returncode'} or type(status['returncode']) is not int or status['returncode']==0 or b'No such image' not in failed or ('g8-isolation:'+('b' if case=='a' else 'a')).encode() not in failed:raise ValueError('foreign child image absence evidence missing')
    for stream in ['stdout','stderr']:
        data=read(private,'survivor-rebuild.'+stream)
        if hashlib.sha256(data).hexdigest()!=r['survivor_rebuild_'+stream+'_sha256']:raise ValueError('survivor rebuild stream differs')
    matrix(read(private,'survivor-rebuild.stdout'),read(private,'survivor-rebuild.stderr'),r['survivor_rebuilt']['built_image_id'])
    baseline=decode(read(private,'primary-docker-before.json'),1<<20)
    phases={'overlap':None,'two-built-a':r['after']['a']['built_image_id'],'two-built-b':r['after']['b']['built_image_id'],'survivor':r['after']['a']['built_image_id'],'survivor-rebuild':r['survivor_rebuilt']['built_image_id']}
    if type(r['primary_snapshots_sha256']) is not dict or set(r['primary_snapshots_sha256'])!=set(phases):raise ValueError('primary concurrency observations incomplete')
    for phase,image in phases.items():
        data=read(private,'concurrency-primary-'+phase+'.json');sample=decode(data,1<<20)
        if hashlib.sha256(data).hexdigest()!=r['primary_snapshots_sha256'][phase]:raise ValueError('primary concurrency observation digest differs')
        unchanged(baseline,sample)
        if sample.get('checked_absent_image')!=image or sample.get('child_image_absent') is not (image is not None) or any(fingerprint(r['before'][case]['engine_id'])==sample['engine_id_sha256'] for case in ['a','b']):raise ValueError('primary/child concurrent engine or image isolation differs')
    inspect_overlap(private,r,boot,invocation,read)
    return concurrency_view(boot,invocation)


def inspect_overlap(private,r,boot,invocation,read):
    data=read(private,'run-overlap.json');overlap=decode(data,1<<20)
    fields={'version','boot_id','invocation_id','before_monotonic_ns','after_monotonic_ns','captures','primary_sha256','cleanup_qualified'}
    if type(overlap) is not dict or set(overlap)!=fields or hashlib.sha256(data).hexdigest()!=r['run_overlap_checkpoint_sha256']:raise ValueError('overlap checkpoint envelope differs')
    equal({k:overlap[k] for k in ['version','boot_id','invocation_id','primary_sha256','cleanup_qualified']},dict(version=1,boot_id=boot,invocation_id=invocation,primary_sha256=r['primary_snapshots_sha256']['overlap'],cleanup_qualified=False))
    before,after=overlap['before_monotonic_ns'],overlap['after_monotonic_ns']
    if type(before) is not int or type(after) is not int or not 0<before<after<1<<64 or after-before>10_000_000_000:raise ValueError('RUN overlap timing is unqualified')
    if type(overlap['captures']) is not dict or set(overlap['captures'])!={'before','after'}:raise ValueError('overlap captures incomplete')
    events=lambda raw:re.findall(rb'^#\d+\s+\d+(?:\.\d+)?\s+G8_OVERLAP_RUN_(BEGIN|END)\s*$',raw,re.M)
    for case in ['a','b']:
        final=read(private,case+'-workload.stderr');size=0
        if events(final)!=[b'BEGIN',b'END']:raise ValueError('concurrent original RUN completion missing')
        for phase in ['before','after']:
            if type(overlap['captures'][phase]) is not dict or set(overlap['captures'][phase])!={'a','b'}:raise ValueError('overlap case captures incomplete')
            raw=read(private,'run-overlap-'+phase+'-'+case+'.stderr')
            if not size<=len(raw)<=131072 or hashlib.sha256(raw).hexdigest()!=overlap['captures'][phase][case] or events(raw)!=[b'BEGIN'] or not final.startswith(raw):raise ValueError('concurrent overlap not bound to unfinished original RUN')
            size=len(raw)
