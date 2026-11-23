"""Explicit held-child worker-fault checkpoint; archived proof is read-only."""
import json,os,re,threading,time
from pathlib import Path
from runtime_child_output_accounting import (FRAME_FIELDS,PROBE,decode,equal,read_fixture_artifact,
    capture,write,digest,number,snapshot,worker_binding,qualify_recovered_pair)
from runtime_shim_worker_fault import WorkerFault

PROBE_RECOVERY=[PROBE[0],PROBE[1],'sleep 10; '+PROBE[2]]
BINDING_FIELDS={'version','scope','phase','boot_id','source_commit','container_id','worker_pid','worker_start_ticks','bundle_identity','executable_sha256'}
POST_BUILD=['/bin/sh','-ec','test ! -e /home/rootless/.local/tmp/g8-build-before-shim-recovery; mv /home/rootless/.local/tmp/g8-build /home/rootless/.local/tmp/g8-build-before-shim-recovery; /bin/sh /work/input/probe.sh']


def recovered_binding(before,after,build,boot,container,identity):
    for phase,value in [('before',before),('after',after)]:
        if type(value) is not dict or set(value)!=BINDING_FIELDS or number(value['version'])!=1 or value['scope']!='held-child-output-worker-binding' or value['phase']!=phase or value['boot_id']!=boot or value['source_commit']!=build['source_commit'] or value['container_id']!=container or value['executable_sha256']!=build['binaries']['containerd-shim-multikernel-v2'] or number(value['worker_pid'],(1<<31)-1)<=1 or not number(value['worker_start_ticks']):raise ValueError('recovered worker binding differs')
        equal(value['bundle_identity'],identity)
    if before['worker_pid']==after['worker_pid'] or after['worker_start_ticks']<before['worker_start_ticks']:raise ValueError('replacement worker not observed')


def saved_processes(bundle,frame):
    saved=decode(read_fixture_artifact(bundle,'.multikernel/sandbox.json',8<<20),8<<20)
    if type(saved) is not dict or saved.get('id')!=frame['sandbox_id'] or saved.get('generation')!=frame['generation'] or saved.get('bundle_identity')!=frame['bundle_identity'] or type(saved.get('processes')) is not list:raise ValueError('live recovery file identity differs')
    processes={}
    for value in saved['processes']:
        if type(value) is not dict or type(value.get('id')) is not str or value['id'] in processes:raise ValueError('live recovery process selectors differ')
        processes[value['id']]=value
    return processes


def undelivered(value):
    if type(value) is not dict or type(value.get('id')) is not str or not re.fullmatch('[a-f0-9]{64}',value['id']) or number(value.get('status'))!=2 or not number(value.get('pid'),(1<<31)-1):raise ValueError('probe is not a recorded running exec')
    if number(value.get('stdout_offset',0)) or number(value.get('stderr_offset',0)) or value.get('exit_event_queued',False) is not False or value.get('final_output_accounting') is not None:raise ValueError('probe output already advanced')
    disposition=value.get('output_disposition')
    if disposition is not None:
        if type(disposition) is not dict or set(disposition)!={'version','stdout','stderr'} or number(disposition['version'])!=1:raise ValueError('probe disposition differs')
        for stream in ['stdout','stderr']:
            d=disposition[stream]
            if type(d) is not dict or set(d)!={'written_bytes','no_destination_bytes','pressure_dropped_bytes','unknown_prefix_bytes','in_flight_bytes','replay_exposure_bytes','exposure_exhausted'} or d['exposure_exhausted'] is not False or any(number(d[k]) for k in set(d)-{'exposure_exhausted'}):raise ValueError('probe already has output or uncertainty')
    return dict(id=value['id'],pid=value['pid'],status=2,stdout_offset=0,stderr_offset=0)


def qualify(cli,private,boot,invocation,unit):
    build=decode(read_fixture_artifact(private.parent,'build.json',1<<20),1<<20)
    deadline=time.monotonic_ns()+30_000_000_000
    authority=WorkerFault(private,build,boot,invocation,unit,deadline)
    authority._scope() # Live epoch admission precedes even read-only PID lookup.
    bundle,binding=worker_binding(private,build,boot,'before');container=binding['container_id']
    inspector=[str(private.parent/'bin/containerd-shim-multikernel-v2'),'--inspect-completed-output','--bundle',str(bundle),'--namespace','moby','--task',container]
    before=capture(inspector,private,'child-recovery-before',True,deadline)
    if before.stderr:raise ValueError('recovery inspection emitted diagnostics')
    frame=snapshot(before.stdout,container);equal(frame['bundle_identity'],binding['bundle_identity'])
    baseline=saved_processes(bundle,frame)
    result={}
    def collect():
        try:result['probe']=capture(cli+['exec','--user','1000:1000','g8-materialization',*PROBE_RECOVERY],private,'child-recovery-probe',False,deadline)
        except BaseException:result['failed']=True
    thread=None
    try:
        with authority as guard:
            equal(guard.binding,{k:v for k,v in binding.items() if k!='phase'})
            thread=threading.Thread(target=collect,daemon=True);thread.start()
            ready_deadline=min(deadline,time.monotonic_ns()+5_000_000_000);selected=None
            for _ in range(101):
                if time.monotonic_ns()>=ready_deadline:break
                if result.get('failed') or 'probe' in result:raise ValueError('probe collection ended before worker fault hold')
                current=saved_processes(bundle,frame);new=set(current)-set(baseline)
                if len(new)>1:raise ValueError('multiple new execs during worker fault')
                if len(new)==1:
                    value=current[next(iter(new))]
                    if value.get('status')==2 and value.get('pid'):
                        selected=undelivered(value);break
                time.sleep(min(.05,max(0,(ready_deadline-time.monotonic_ns())/1e9)))
            if selected is None or time.monotonic_ns()>=ready_deadline:raise ValueError('probe did not start within the observation deadline')
            if result.get('failed') or 'probe' in result:raise ValueError('probe collection ended before worker fault hold')
            guard.pause()
            current=saved_processes(bundle,frame)
            if set(current)-set(baseline)!={selected['id']}:raise ValueError('held exec set changed')
            equal(undelivered(current[selected['id']]),selected)
            write(private,'child-recovery-held.json',(json.dumps(dict(version=1,scope='held-undelivered-child-exec',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],container_id=container,generation=frame['generation'],bundle_identity=frame['bundle_identity'],process=selected),sort_keys=True)+'\n').encode())
            # This hold gives the fixed producer time to finish, but elapsed
            # time never qualifies its exit. The final recovery_drain marker
            # must prove that the replacement actually observed STOPPED.
            if deadline-time.monotonic_ns()<=12_000_000_000:raise ValueError('worker fault lacks producer hold budget')
            time.sleep(12)
            proof=guard.kill_worker()
            proof.update(version=1,scope='live-held-child-worker-fault',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],container_id=container,worker_pid=binding['worker_pid'],worker_start_ticks=binding['worker_start_ticks'],supervisor_pid=guard.supervisor_pid,supervisor_start_ticks=guard.supervisor_start)
            write(private,'child-recovery-fault.json',(json.dumps(proof,sort_keys=True)+'\n').encode())
    finally:
        if thread is not None:thread.join(max(0,(deadline-time.monotonic_ns())/1e9)+1)
    if thread is None or thread.is_alive() or result.get('failed') or 'probe' not in result or time.monotonic_ns()>=deadline:raise ValueError('recovery probe did not complete within its command deadline')
    probe=result['probe'];old=frame.get('tail',dict(deleted_count=0))['deleted_count']
    observation_deadline=min(deadline,time.monotonic_ns()+5_000_000_000);view=None
    for attempt in range(25):
        if time.monotonic_ns()>=observation_deadline:break
        after=capture(inspector,private,'child-recovery-after-'+str(attempt),False,observation_deadline)
        if time.monotonic_ns()>=observation_deadline:raise ValueError('recovery accounting observation was late')
        if after.returncode==0 and not after.stderr:
            current=snapshot(after.stdout,container)
            if current.get('tail',dict(deleted_count=0))['deleted_count']>old:
                view=qualify_recovered_pair(before.stdout,after.stdout,container,probe.stdout,probe.stderr,probe.returncode)
                latest=current['tail']['records'][-1]
                if latest['id']!=selected['id'] or latest['pid']!=selected['pid']:raise ValueError('recovered tail is not the held exec')
                if time.monotonic_ns()>=observation_deadline:raise ValueError('recovery accounting qualification was late')
                break
        time.sleep(min(.1,max(0,(observation_deadline-time.monotonic_ns())/1e9)))
    if view is None:raise ValueError('recovered output accounting not retained within deadline')
    _,last=worker_binding(private,build,boot,'after')
    recovered_binding(binding,last,build,boot,container,frame['bundle_identity'])
    write(private,'child-recovery-replacement.json',(json.dumps(authority.observe_replacement(last),sort_keys=True)+'\n').encode())
    view.update(worker_replacement_qualified=True)
    rebuilt=capture(cli+['exec','--user','1000:1000','g8-materialization',*POST_BUILD],private,'child-recovery-rebuild',True,time.monotonic_ns()+180_000_000_000)
    old_image=capture(cli+['exec','--user','1000:1000','g8-materialization','/usr/bin/sha256sum','/home/rootless/.local/tmp/g8-build-before-shim-recovery/image.tar'],private,'child-recovery-old-image',True,time.monotonic_ns()+30_000_000_000)
    primary_after_rebuild(private,rebuilt.stdout,rebuilt.stderr,old_image.stdout)
    view.update(post_recovery_child_build_matrix_qualified=True,original_child_archive_preserved=True,post_recovery_primary_separation_qualified=True)
    names=artifact_names(attempt)
    checkpoint=dict(version=1,scope='held-child-output-recovery-checkpoint',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],after_attempt=attempt,view=view,artifact_sha256={name:digest(read_fixture_artifact(private,name,131072,allow_empty=True)) for name in names})
    write(private,'child-output-recovery.json',(json.dumps(checkpoint,sort_keys=True)+'\n').encode())


def artifact_names(attempt):
    number(attempt,24)
    return ['child-output-'+phase+'-worker.json' for phase in ['before','after']]+['child-recovery-held.json','child-recovery-fault.json','child-recovery-replacement.json','child-recovery-primary.json']+[label+'.'+suffix for label in ['child-recovery-before','child-recovery-probe','child-recovery-rebuild','child-recovery-old-image',*['child-recovery-after-'+str(i) for i in range(attempt+1)]] for suffix in ['stdout','stderr','exit.json']]


def rebuild_matrix(private,stdout,stderr,old_image):
    from runtime_concurrency_checkpoint import matrix
    images=re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',stdout,re.M)
    if len(images)!=1:raise ValueError('post-recovery build image differs')
    image=images[0].decode();matrix(stdout,stderr,image,True)
    previous=read_fixture_artifact(private,'log-saved-image.stdout',65536)
    if not re.fullmatch(rb'[a-f0-9]{64}  /home/rootless/.local/tmp/g8-build-before-shim-recovery/image[.]tar\n',old_image) or old_image.split()[0]!=previous.split()[0]:raise ValueError('original child image archive changed')
    return image


def primary_after_rebuild(private,stdout,stderr,old_image):
    from runtime_primary_docker import observe,unchanged
    image=rebuild_matrix(private,stdout,stderr,old_image)
    primary=observe(image);baseline=decode(read_fixture_artifact(private,'primary-docker-before.json',1<<20),1<<20)
    unchanged(baseline,primary)
    if primary.get('child_image_absent') is not True:raise ValueError('rebuilt child image appears on primary')
    write(private,'child-recovery-primary.json',(json.dumps(primary,sort_keys=True)+'\n').encode())


def inspect_fixture(work,boot,invocation):
    from runtime_child_log_checkpoint import inspect_fixture_logging
    inspect_fixture_logging(work,boot,invocation)
    private=work/'integration';read=lambda name:read_fixture_artifact(private,name,131072,allow_empty=True)
    receipt=decode(read_fixture_artifact(work,'integration-receipt.json',1<<20),1<<20);raw=read('child-output-recovery.json');value=decode(raw,65536)
    if receipt.get('child_output_recovery_tested') is not True or receipt.get('child_output_accounting_tested') is not False or receipt.get('diagnostic_exhaustion_tested') is not False or receipt.get('child_output_recovery_sha256')!=digest(raw):raise ValueError('child recovery receipt scope differs')
    build=decode(read_fixture_artifact(work,'build.json',1<<20),1<<20);container=read('create.stdout').decode().strip()
    if type(value) is not dict or set(value)!={'version','scope','boot_id','invocation_id','source_commit','after_attempt','view','artifact_sha256'} or number(value['version'])!=1 or value['scope']!='held-child-output-recovery-checkpoint' or value['boot_id']!=boot or value['invocation_id']!=invocation or value['source_commit']!=build['source_commit']:raise ValueError('child recovery checkpoint identity differs')
    attempt=number(value['after_attempt'],24);equal(value['artifact_sha256'],{name:digest(read(name)) for name in artifact_names(attempt)})
    first=snapshot(read('child-recovery-before.stdout'),container);last=snapshot(read('child-recovery-after-'+str(attempt)+'.stdout'),container)
    recovered_binding(decode(read('child-output-before-worker.json'),65536),decode(read('child-output-after-worker.json'),65536),build,boot,container,first['bundle_identity'])
    held=decode(read('child-recovery-held.json'),65536)
    target=last['tail']['records'][-1]
    equal(held,dict(version=1,scope='held-undelivered-child-exec',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],container_id=container,generation=first['generation'],bundle_identity=first['bundle_identity'],process=dict(id=target['id'],pid=target['pid'],status=2,stdout_offset=0,stderr_offset=0)))
    binding=decode(read('child-output-before-worker.json'),65536);fault=decode(read('child-recovery-fault.json'),65536)
    if type(fault) is not dict or number(fault.get('supervisor_pid'),(1<<31)-1)<=1 or not number(fault.get('supervisor_start_ticks')) or fault['supervisor_pid']==binding['worker_pid']:raise ValueError('recovery supervisor binding differs')
    equal(fault,dict(version=1,scope='live-held-child-worker-fault',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],container_id=container,worker_pid=binding['worker_pid'],worker_start_ticks=binding['worker_start_ticks'],supervisor_pid=fault['supervisor_pid'],supervisor_start_ticks=fault['supervisor_start_ticks'],signal='SIGKILL',pidfd_exit_observed=True,source_bundle_parent_and_cgroup_bound=True,supervisor_resumed=True,historical_receipt_authority=False))
    replacement=decode(read('child-recovery-replacement.json'),65536);new_binding=decode(read('child-output-after-worker.json'),65536)
    if fault['supervisor_pid']==new_binding['worker_pid']:raise ValueError('replacement worker is the supervisor')
    equal(replacement,dict(version=1,scope='live-replacement-worker-parent',supervisor_pid=fault['supervisor_pid'],supervisor_start_ticks=fault['supervisor_start_ticks'],worker_pid=new_binding['worker_pid'],worker_start_ticks=new_binding['worker_start_ticks'],same_source_bundle_and_cgroup=True))
    for label,status in [('child-recovery-before',0),('child-recovery-probe',37),('child-recovery-after-'+str(attempt),0),('child-recovery-rebuild',0),('child-recovery-old-image',0)]:equal(decode(read(label+'.exit.json'),65536),dict(returncode=status))
    if read('child-recovery-before.stderr') or read('child-recovery-after-'+str(attempt)+'.stderr'):raise ValueError('recovery inspection emitted diagnostics')
    view=qualify_recovered_pair(read('child-recovery-before.stdout'),read('child-recovery-after-'+str(attempt)+'.stdout'),container,read('child-recovery-probe.stdout'),read('child-recovery-probe.stderr'),37)
    image=rebuild_matrix(private,read('child-recovery-rebuild.stdout'),read('child-recovery-rebuild.stderr'),read('child-recovery-old-image.stdout'))
    from runtime_primary_docker import unchanged
    primary=decode(read('child-recovery-primary.json'),65536);unchanged(decode(read_fixture_artifact(private,'primary-docker-before.json',1<<20),1<<20),primary)
    if primary.get('checked_absent_image')!=image or primary.get('child_image_absent') is not True:raise ValueError('post-recovery primary separation differs')
    view.update(worker_replacement_qualified=True,post_recovery_child_build_matrix_qualified=True,original_child_archive_preserved=True,post_recovery_primary_separation_qualified=True);equal(value['view'],view)
    return dict(version=1,scope='held-child-recovered-exec-output-accounting',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],live_worker_replacement_bound=True,stopped_guest_drain_observed=True,guest_host_and_receiver_counts_bound=True,completed_exec_deletion_observed=True,post_recovery_child_build_matrix_qualified=True,original_child_archive_preserved=True,post_recovery_primary_separation_qualified=True,docker_log_retention_qualified=False,full_g7_g8_qualified=False,cleanup_qualified=False)
