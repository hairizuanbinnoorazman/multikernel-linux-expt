"""Counter-only held-child exec proof; no second guest-agent session."""
import hashlib,json,os,re,stat,time
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_trial_command import capture
from runtime_docker_concurrency import sandbox_id_for_container

U64=(1<<64)-1
STDOUT_BYTES=8192
STDERR=b'accounted-stderr\n'
PROBE=['/bin/sh','-c','/bin/busybox dd if=/dev/zero bs=4096 count=2 2>/dev/null; status=$?; test "$status" -eq 0 || exit "$status"; printf \'accounted-stderr\\n\' >&2; exit 37']
FRAME_FIELDS={'version','scope','status','code','sandbox_id','generation','task_id','namespace','bundle_identity'}
RECORD_FIELDS={'sequence','id','pid','status','exit','stdout_offset','stderr_offset'}
BUFFER_FIELDS={'observed_bytes','acknowledged_bytes','buffered_bytes','pending_bytes','dropped_bytes','buffered_peak_bytes','limit_bytes','truncated','accounting_exhausted'}
DISPOSITION_FIELDS={'written_bytes','no_destination_bytes','pressure_dropped_bytes','unknown_prefix_bytes','in_flight_bytes','replay_exposure_bytes','exposure_exhausted'}


def number(value,maximum=U64):
    if type(value) is not int or not 0<=value<=maximum:raise ValueError('invalid output accounting integer')
    return value


def buffer(value):
    if type(value) is not dict or set(value)!=BUFFER_FIELDS:raise ValueError('output buffer fields differ')
    for key in BUFFER_FIELDS-{'truncated','accounting_exhausted'}:number(value[key])
    if type(value['truncated']) is not bool or value['accounting_exhausted'] is not False:raise ValueError('output buffer is unqualified')
    if (value['limit_bytes']!=4<<20 or not value['buffered_bytes']<=value['buffered_peak_bytes']<=min(4<<20,value['observed_bytes']) or value['truncated']!=(value['dropped_bytes']!=0) or value['observed_bytes']!=sum(value[k] for k in ['acknowledged_bytes','buffered_bytes','pending_bytes','dropped_bytes'])):raise ValueError('output buffer does not conserve bytes')


def record(value):
    if type(value) is not dict or not RECORD_FIELDS<=set(value)<=RECORD_FIELDS|{'final_output_accounting','output_disposition'}:raise ValueError('completed record fields differ')
    for key in RECORD_FIELDS-{'id'}:number(value[key])
    if type(value['id']) is not str or not re.fullmatch('[a-z0-9][a-z0-9-]{0,63}',value['id']) or value['pid']>=1<<31 or value['exit']>255 or value['status'] not in [1,3]:raise ValueError('completed record selector differs')
    if value['status']==1 and (value['pid'] or value['exit'] or value['stdout_offset'] or value['stderr_offset'] or 'final_output_accounting' in value or 'output_disposition' in value):raise ValueError('unstarted exec fabricated output')
    if 'final_output_accounting' in value:
        final=value['final_output_accounting']
        if type(final) is not dict:raise ValueError('invalid final accounting')
        if final.get('state') in ['unsupported','unavailable']:
            if set(final)!={'state'} or value['status']!=3:raise ValueError('unavailable report fabricated counters')
        elif final.get('state')=='validated':
            if set(final)!={'state','report'} or value['status']!=3:raise ValueError('final report fields differ')
            report=final['report']
            if type(report) is not dict or set(report)!={'version','id','pid','status','stdout','stderr'}:raise ValueError('guest report fields differ')
            if number(report['version'])!=1 or report['id']!=value['id'] or number(report['pid'])!=value['pid'] or not value['pid'] or report['status']!='STOPPED':raise ValueError('guest report selector differs')
            for stream in ['stdout','stderr']:
                b=report[stream];buffer(b)
                if b['acknowledged_bytes']!=value[stream+'_offset'] or b['buffered_bytes'] or b['pending_bytes']:raise ValueError('final buffer is not drained')
        else:raise ValueError('final accounting state differs')
    if 'output_disposition' in value:
        disposition=value['output_disposition']
        if type(disposition) is not dict or set(disposition)!={'version','stdout','stderr'} or number(disposition['version'])!=1:raise ValueError('disposition fields differ')
        for stream in ['stdout','stderr']:
            d=disposition[stream]
            if type(d) is not dict or set(d)!=DISPOSITION_FIELDS or type(d['exposure_exhausted']) is not bool:raise ValueError('stream disposition fields differ')
            for key in DISPOSITION_FIELDS-{'exposure_exhausted'}:number(d[key])
            offset=value[stream+'_offset']
            if sum(d[k] for k in ['written_bytes','no_destination_bytes','pressure_dropped_bytes','unknown_prefix_bytes'])!=offset or d['in_flight_bytes']>min(4096,U64-offset) or d['exposure_exhausted'] and d['replay_exposure_bytes']!=U64:raise ValueError('disposition does not conserve offsets')


def snapshot(data,container):
    if type(container) is not str or not re.fullmatch("[a-f0-9]{64}",container):raise ValueError("container selector differs")
    value=decode(data,65536)
    if type(value) is not dict or not FRAME_FIELDS<=set(value)<=FRAME_FIELDS|{'tail'}:raise ValueError('inspection fields differ')
    if number(value['version'])!=1 or value['scope']!='private-shim-completed-output' or value['task_id']!=container or value['namespace']!='moby' or value['sandbox_id']!=sandbox_id_for_container(container) or type(value['generation']) is not str or not re.fullmatch('[a-f0-9]{32}',value['generation']):raise ValueError('inspection scope differs')
    identity=value['bundle_identity']
    if type(identity) is not dict or set(identity)!={'device','inode','uid'} or number(identity['device'])==0 or number(identity['inode'])==0 or number(identity['uid'])!=os.geteuid():raise ValueError('bundle identity differs')
    if value['status']=='unobserved':
        if value['code']!='COMPLETED_OUTPUT_NOT_OBSERVED' or 'tail' in value:raise ValueError('unobserved history fabricated counters')
        return value
    if value['status']!='observed' or value['code']!='COMPLETED_OUTPUT_OBSERVED':raise ValueError('inspection is unqualified')
    tail=value.get('tail')
    if type(tail) is not dict or set(tail)!={'version','deleted_count','evicted_count','accounting_exhausted','records'} or number(tail['version'])!=1 or tail['accounting_exhausted'] is not False or type(tail['records']) is not list or len(tail['records'])>16:raise ValueError('tail is unqualified')
    count=number(tail['deleted_count']);evicted=number(tail['evicted_count']);records=tail['records']
    if count!=evicted+len(records) or evicted and len(records)!=16:raise ValueError('tail count differs')
    for index,item in enumerate(records):
        record(item)
        if item['sequence']!=evicted+index+1:raise ValueError('tail sequence differs')
    return value


def qualify_pair(before_data,after_data,container,stdout,stderr,status):
    if type(status) is not int or status!=37 or stdout!=b'\0'*STDOUT_BYTES or stderr!=STDERR:raise ValueError('finite child output differs')
    before=snapshot(before_data,container);after=snapshot(after_data,container)
    equal({k:before[k] for k in FRAME_FIELDS-{'status','code'}},{k:after[k] for k in FRAME_FIELDS-{'status','code'}})
    old=before.get('tail',dict(deleted_count=0,records=[]))
    new=after.get('tail')
    if new is None or new['deleted_count']!=old['deleted_count']+1 or len(new['records'])!=min(16,len(old['records'])+1):raise ValueError('probe is not the sole new deletion')
    equal(new['records'][:-1],old['records'][-15:])
    latest=new['records'][-1]
    if not re.fullmatch('[a-f0-9]{64}',latest['id']) or latest['id'] in {r['id'] for r in old['records']} or latest['status']!=3 or not latest['pid'] or latest['exit']!=37:raise ValueError('probe completion differs')
    if latest.get('final_output_accounting',{}).get('state')!='validated' or 'output_disposition' not in latest:raise ValueError('probe accounting is unavailable')
    for stream,size in [('stdout',STDOUT_BYTES),('stderr',len(STDERR))]:
        b=latest['final_output_accounting']['report'][stream];d=latest['output_disposition'][stream]
        if latest[stream+'_offset']!=size or b['observed_bytes']!=size or b['dropped_bytes'] or d['written_bytes']!=size or any(d[k] for k in ['no_destination_bytes','pressure_dropped_bytes','unknown_prefix_bytes','in_flight_bytes','replay_exposure_bytes']) or d['exposure_exhausted']:raise ValueError('probe loss or uncertainty differs')
    return dict(version=1,scope='held-child-finite-exec-output-accounting',stdout_observed_bytes=STDOUT_BYTES,stdout_received_bytes=STDOUT_BYTES,stderr_observed_bytes=len(STDERR),stderr_received_bytes=len(STDERR),exit_status=37,guest_dropped_bytes=0,host_pressure_dropped_bytes=0,replay_exposure_bytes=0,deleted_exec_retained=True,owning_agent_session_preserved=True,docker_log_retention_qualified=False,full_g7_g8_qualified=False,cleanup_qualified=False)


def digest(data):return hashlib.sha256(data).hexdigest()

def write(private,name,data):
    with (private/name).open('xb') as file:
        os.fchmod(file.fileno(),0o600);file.write(data);file.flush();os.fsync(file.fileno())


def worker_binding(private,build,boot,phase):
    if phase not in ['before','after'] or Path('/proc/sys/kernel/random/boot_id').read_text().strip()!=boot:raise ValueError('output worker observation epoch differs')
    container=read_fixture_artifact(private,'create.stdout',128).decode().strip()
    if not re.fullmatch('[a-f0-9]{64}',container) or not re.fullmatch('[a-f0-9]{40}',build['source_commit']):raise ValueError('output worker source differs')
    bundle=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container
    marker=read_fixture_artifact(bundle,'.multikernel-worker.pid',64)
    if not re.fullmatch(rb'[1-9][0-9]{0,9}\n',marker):raise ValueError('output worker marker differs')
    pid=int(marker);proc=Path('/proc',str(pid));executable=private.parent/'bin/containerd-shim-multikernel-v2'
    def start():
        raw=(proc/'stat').read_bytes();tail=raw[raw.rfind(b')')+1:].split()
        if len(tail)<20 or tail[0] in [b'Z',b'X']:raise ValueError('output worker is not live')
        return int(tail[19])
    ticks=start()
    if os.readlink(proc/'exe')!=str(executable):raise ValueError('output worker executable differs')
    with (proc/'exe').open('rb') as file:
        first=os.fstat(file.fileno())
        if not stat.S_ISREG(first.st_mode) or first.st_uid!=os.geteuid() or not 0<first.st_size<=128<<20:raise ValueError('output worker executable unsafe')
        actual=hashlib.file_digest(file,'sha256').hexdigest()
        for observed in [os.fstat(file.fileno()),executable.stat()]:
            if any(getattr(first,k)!=getattr(observed,k) for k in ['st_dev','st_ino','st_size','st_mtime_ns','st_ctime_ns']):raise ValueError('output worker executable changed')
    if actual!=build['binaries']['containerd-shim-multikernel-v2']:raise ValueError('output worker binary hash differs')
    identity=bundle.lstat();cwd=(proc/'cwd').stat()
    if not stat.S_ISDIR(identity.st_mode) or stat.S_IMODE(identity.st_mode)!=0o700 or identity.st_uid!=os.geteuid() or (identity.st_dev,identity.st_ino,identity.st_uid)!=(cwd.st_dev,cwd.st_ino,cwd.st_uid) or start()!=ticks or read_fixture_artifact(bundle,'.multikernel-worker.pid',64)!=marker:raise ValueError('output worker changed during observation')
    value=dict(version=1,scope='held-child-output-worker-binding',phase=phase,boot_id=boot,source_commit=build['source_commit'],container_id=container,worker_pid=pid,worker_start_ticks=ticks,bundle_identity=dict(device=identity.st_dev,inode=identity.st_ino,uid=identity.st_uid),executable_sha256=actual)
    write(private,'child-output-'+phase+'-worker.json',(json.dumps(value,sort_keys=True)+'\n').encode())
    return bundle,value


def qualify(cli,private,boot,invocation):
    build=decode(read_fixture_artifact(private.parent,'build.json',1<<20),1<<20)
    bundle,binding=worker_binding(private,build,boot,'before');container=binding['container_id']
    inspector=[str(private.parent/'bin/containerd-shim-multikernel-v2'),'--inspect-completed-output','--bundle',str(bundle),'--namespace','moby','--task',container]
    before=capture(inspector,private,'child-output-before',True,time.monotonic_ns()+30_000_000_000)
    if before.stderr:raise ValueError('output inspection wrote diagnostics')
    initial=snapshot(before.stdout,container)
    equal(initial['bundle_identity'],binding['bundle_identity'])
    old=initial.get('tail',dict(deleted_count=0))['deleted_count']
    probe=capture(cli+['exec','--user','1000:1000','g8-materialization',*PROBE],private,'child-output-probe',False,time.monotonic_ns()+30_000_000_000)
    if probe.returncode!=37 or probe.stdout!=b'\0'*STDOUT_BYTES or probe.stderr!=STDERR:raise ValueError('finite output probe failed')
    deadline=time.monotonic_ns()+5_000_000_000
    qualified=None
    for attempt in range(25):
        if time.monotonic_ns()>=deadline:break
        after=capture(inspector,private,'child-output-after-'+str(attempt),False,deadline)
        if time.monotonic_ns()>=deadline:raise ValueError('output accounting observation exceeded deadline')
        if after.returncode==0 and not after.stderr:
            current=snapshot(after.stdout,container);count=current.get('tail',dict(deleted_count=0))['deleted_count']
            if count>old:
                qualified=qualify_pair(before.stdout,after.stdout,container,probe.stdout,probe.stderr,probe.returncode)
                if time.monotonic_ns()>=deadline:raise ValueError('output accounting qualification exceeded deadline')
                break
        time.sleep(min(.1,max(0,(deadline-time.monotonic_ns())/1e9)))
    if qualified is None:raise ValueError('completed child exec accounting was not retained within deadline')
    _,last=worker_binding(private,build,boot,'after')
    equal({k:v for k,v in binding.items() if k!='phase'},{k:v for k,v in last.items() if k!='phase'})
    names=['child-output-'+phase+'-worker.json' for phase in ['before','after']]+[label+'.'+suffix for label in ['child-output-before','child-output-probe',*['child-output-after-'+str(i) for i in range(attempt+1)]] for suffix in ['stdout','stderr','exit.json']]
    value=dict(version=1,scope='held-child-output-accounting-checkpoint',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],executable_sha256=build['binaries']['containerd-shim-multikernel-v2'],after_attempt=attempt,view=qualified,artifact_sha256={name:digest(read_fixture_artifact(private,name,65536,allow_empty=True)) for name in names})
    write(private,'child-output-accounting.json',(json.dumps(value,sort_keys=True)+'\n').encode())


def inspect_fixture(work,boot,invocation):
    from runtime_child_log_checkpoint import inspect_fixture_logging
    inspect_fixture_logging(work,boot,invocation)
    private=work/'integration';read=lambda name:read_fixture_artifact(private,name,65536,allow_empty=True)
    receipt=decode(read_fixture_artifact(work,'integration-receipt.json',1<<20),1<<20);raw=read('child-output-accounting.json');value=decode(raw,65536)
    if receipt.get('child_output_accounting_tested') is not True or receipt.get('child_output_accounting_sha256')!=digest(raw):raise ValueError('child output accounting receipt differs')
    build=decode(read_fixture_artifact(work,'build.json',1<<20),1<<20);container=read('create.stdout').decode().strip()
    if type(value) is not dict or set(value)!={'version','scope','boot_id','invocation_id','source_commit','executable_sha256','after_attempt','view','artifact_sha256'} or number(value['version'])!=1 or value['scope']!='held-child-output-accounting-checkpoint' or value['boot_id']!=boot or value['invocation_id']!=invocation or value['source_commit']!=build['source_commit'] or value['executable_sha256']!=build['binaries']['containerd-shim-multikernel-v2']:raise ValueError('child output accounting scope differs')
    attempt=number(value['after_attempt'],24)
    names=['child-output-'+phase+'-worker.json' for phase in ['before','after']]+[label+'.'+suffix for label in ['child-output-before','child-output-probe',*['child-output-after-'+str(i) for i in range(attempt+1)]] for suffix in ['stdout','stderr','exit.json']]
    equal(value['artifact_sha256'],{name:digest(read(name)) for name in names})
    for phase in ['before','after']:
        binding=decode(read('child-output-'+phase+'-worker.json'),65536)
        if type(binding) is not dict or set(binding)!={'version','scope','phase','boot_id','source_commit','container_id','worker_pid','worker_start_ticks','bundle_identity','executable_sha256'} or number(binding['version'])!=1 or binding['scope']!='held-child-output-worker-binding' or binding['phase']!=phase or binding['boot_id']!=boot or binding['source_commit']!=build['source_commit'] or binding['container_id']!=container or binding['executable_sha256']!=value['executable_sha256'] or not number(binding['worker_pid'],(1<<31)-1) or not number(binding['worker_start_ticks']):raise ValueError('output worker binding differs')
        equal(binding['bundle_identity'],snapshot(read('child-output-before.stdout'),container)['bundle_identity'])
    first=decode(read('child-output-before-worker.json'),65536);last=decode(read('child-output-after-worker.json'),65536)
    equal({k:v for k,v in first.items() if k!='phase'},{k:v for k,v in last.items() if k!='phase'})
    for label,status in [('child-output-before',0),('child-output-probe',37),('child-output-after-'+str(attempt),0)]:
        equal(decode(read(label+'.exit.json'),65536),dict(returncode=status))
    if read('child-output-before.stderr') or read('child-output-after-'+str(attempt)+'.stderr'):raise ValueError('output inspection emitted diagnostics')
    view=qualify_pair(read('child-output-before.stdout'),read('child-output-after-'+str(attempt)+'.stdout'),container,read('child-output-probe.stdout'),read('child-output-probe.stderr'),37)
    equal(value['view'],view)
    return dict(version=1,scope='held-child-finite-exec-output-accounting',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],finite_exec_output_bound=True,same_worker_and_bundle_bound=True,guest_host_and_receiver_counts_bound=True,completed_exec_deletion_observed=True,owning_agent_session_preserved=True,docker_log_retention_qualified=False,full_g7_g8_qualified=False,cleanup_qualified=False)
