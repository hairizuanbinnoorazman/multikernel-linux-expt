"""Private source/incarnation binding for the explicitly instrumented worker."""
import hashlib,json,os,re,stat
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import read_fixture_artifact,decode,equal
from runtime_trial_shim_variant import EXHAUSTION,variant

RECEIPT='diagnostic-exhaustion.fixture.json'
LIMIT=1 << 20
PRELOAD={'version','scope','source_revision','worker_pid','boot_id','worker_start_ticks',
         'bundle_device','bundle_inode','bundle_uid','limit_bytes','reserved_bytes',
         'synthetic_preload_attempts','synthetic_preload_admitted',
         'allowance_closed_before_production_entry','instrumented_test_executable'}
FRAME=dict(version=1,component='containerd-shim-multikernel-v2',operation='read-process-output',code='OUTPUT_READ_FAILED',sandbox_id='qualification',generation='0'*32,process_id='synthetic-preload',at='2026-01-01T00:00:00Z')
FRAME_BYTES=len(json.dumps(FRAME,separators=(',',':')).encode())+1
PHASES=('after-start','before-release')
sha=lambda data:hashlib.sha256(data).hexdigest()


def validate_preload(value,revision,boot):
    if type(value) is not dict or set(value)!=PRELOAD:raise ValueError('exhaustion preload schema differs')
    for key in ['version','worker_pid','worker_start_ticks','bundle_device','bundle_inode','bundle_uid','limit_bytes','reserved_bytes','synthetic_preload_attempts','synthetic_preload_admitted']:
        if type(value[key]) is not int or value[key]<0:raise ValueError('exhaustion preload number differs')
    frames=LIMIT//FRAME_BYTES
    if (value['version']!=2 or value['scope']!='diagnostic-exhaustion-test-worker' or
        value['source_revision']!=revision or value['boot_id']!=boot or
        not 0<value['worker_pid']<1<<31 or not value['worker_start_ticks'] or not value['bundle_inode'] or
        value['limit_bytes']!=LIMIT or value['reserved_bytes']!=frames*FRAME_BYTES or
        value['synthetic_preload_attempts']!=frames+bool(LIMIT%FRAME_BYTES) or
        not 0<value['synthetic_preload_admitted']<=frames or
        value['allowance_closed_before_production_entry'] is not True or
        value['instrumented_test_executable'] is not True):raise ValueError('exhaustion preload identity or allowance differs')


def process_start(pid):
    data=Path('/proc',str(pid),'stat').read_bytes()
    tail=data[data.rfind(b')')+1:].split()
    if len(tail)<20 or tail[0] in [b'Z',b'X']:raise ValueError('exhaustion worker is not live')
    return int(tail[19])


def write(private,name,data):
    with (private/name).open('xb') as file:
        os.fchmod(file.fileno(),0o600);file.write(data);file.flush();os.fsync(file.fileno())


def capture_worker(private,build,boot,phase):
    if Path('/proc/sys/kernel/random/boot_id').read_text().strip()!=boot:raise ValueError('exhaustion live boot differs')
    if phase not in PHASES or variant(build)!=EXHAUSTION:raise ValueError('exhaustion capture authority differs')
    source=build['source_commit'];expected=build['binaries']['containerd-shim-multikernel-v2']
    if not re.fullmatch('[a-f0-9]{40}',source) or not re.fullmatch('[a-f0-9]{64}',expected):raise ValueError('exhaustion source differs')
    container=read_fixture_artifact(private,'create.stdout',128).decode().strip()
    if not re.fullmatch('[a-f0-9]{64}',container):raise ValueError('exhaustion container differs')
    bundle=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container
    raw=read_fixture_artifact(bundle,RECEIPT,8192);preload=decode(raw,8192)
    validate_preload(preload,source[:7],boot)
    pid=preload['worker_pid'];start=process_start(pid)
    marker=read_fixture_artifact(bundle,'.multikernel-worker.pid',64)
    if marker!=str(pid).encode()+b'\n' or start!=preload['worker_start_ticks']:raise ValueError('exhaustion worker incarnation differs')
    executable=private.parent/'bin/containerd-shim-multikernel-v2'
    if os.readlink('/proc/'+str(pid)+'/exe')!=str(executable):raise ValueError('exhaustion executable path differs')
    with open('/proc/'+str(pid)+'/exe','rb') as file:
        before=os.fstat(file.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_uid!=os.geteuid() or not 0<before.st_size<=128<<20:raise ValueError('exhaustion executable is unsafe')
        actual=hashlib.file_digest(file,'sha256').hexdigest()
        after=os.fstat(file.fileno())
        named=executable.stat()
        for observed in [after,named]:
            if (observed.st_dev,observed.st_ino,observed.st_size,observed.st_mtime_ns,observed.st_ctime_ns)!=(before.st_dev,before.st_ino,before.st_size,before.st_mtime_ns,before.st_ctime_ns):raise ValueError('exhaustion executable changed')
    if actual!=expected:raise ValueError('exhaustion executable hash differs')
    here=os.stat(bundle,follow_symlinks=False);cwd=os.stat('/proc/'+str(pid)+'/cwd')
    identity=(here.st_dev,here.st_ino,here.st_uid)
    if not stat.S_ISDIR(here.st_mode) or stat.S_IMODE(here.st_mode)!=0o700 or identity!=(cwd.st_dev,cwd.st_ino,cwd.st_uid) or identity!=(preload['bundle_device'],preload['bundle_inode'],preload['bundle_uid']) or here.st_uid!=os.geteuid():raise ValueError('exhaustion bundle identity differs')
    if process_start(pid)!=start or read_fixture_artifact(bundle,RECEIPT,8192)!=raw or read_fixture_artifact(bundle,'.multikernel-worker.pid',64)!=marker:raise ValueError('exhaustion worker changed during capture')
    snapshot=dict(version=1,scope='instrumented-worker-live-binding',phase=phase,boot_id=boot,source_commit=source,container_id=container,worker_pid=pid,worker_start_ticks=start,bundle_device=identity[0],bundle_inode=identity[1],bundle_uid=identity[2],executable_sha256=actual,preload_sha256=sha(raw))
    write(private,'exhaustion-'+phase+'-preload.json',raw)
    write(private,'exhaustion-'+phase+'-worker.json',(json.dumps(snapshot,sort_keys=True)+'\n').encode())
    return snapshot


def inspect_binding(private,build,boot):
    if variant(build)!=EXHAUSTION or not re.fullmatch('[a-f0-9]{40}',build.get('source_commit','')) or not re.fullmatch('[a-f0-9]{64}',build.get('binaries',{}).get('containerd-shim-multikernel-v2','')) or not re.fullmatch('[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}',boot):raise ValueError('exhaustion binding authority differs')
    container=read_fixture_artifact(private,'create.stdout',128).decode().strip()
    if not re.fullmatch('[a-f0-9]{64}',container):raise ValueError('exhaustion container differs')
    read=lambda name:read_fixture_artifact(private,name,8192)
    snapshots=[]
    for phase in PHASES:
        raw=read('exhaustion-'+phase+'-preload.json');preload=decode(raw,8192)
        validate_preload(preload,build['source_commit'][:7],boot)
        snapshot=decode(read('exhaustion-'+phase+'-worker.json'),8192)
        expected=dict(version=1,scope='instrumented-worker-live-binding',phase=phase,boot_id=boot,source_commit=build['source_commit'],container_id=read_fixture_artifact(private,'create.stdout',128).decode().strip(),worker_pid=preload['worker_pid'],worker_start_ticks=preload['worker_start_ticks'],bundle_device=preload['bundle_device'],bundle_inode=preload['bundle_inode'],bundle_uid=preload['bundle_uid'],executable_sha256=build['binaries']['containerd-shim-multikernel-v2'],preload_sha256=sha(raw))
        equal(snapshot,expected);snapshot.pop('phase');snapshots.append(snapshot)
    equal(*snapshots)
    return dict(version=1,scope='instrumented-worker-exhaustion-binding',source_commit=build['source_commit'],boot_id=boot,allowance_closed_before_production_entry=True,live_worker_incarnation_bound=True,worker_unchanged_through_child_matrix=True,instrumented_test_executable=True,cleanup_qualified=False)


def checkpoint(private,build,boot,invocation):
    view=inspect_binding(private,build,boot)
    names=['exhaustion-'+phase+'-'+kind+'.json' for phase in PHASES for kind in ['preload','worker']]
    value=dict(version=1,scope='instrumented-child-diagnostic-exhaustion',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],binary_sha256=build['binaries']['containerd-shim-multikernel-v2'],binding=view,artifact_sha256={name:sha(read_fixture_artifact(private,name,8192)) for name in names},child_log_pressure_sha256=sha(read_fixture_artifact(private,'child-log-pressure.json',1<<20)))
    write(private,'diagnostic-exhaustion-checkpoint.json',(json.dumps(value,sort_keys=True)+'\n').encode())


def inspect_fixture_exhaustion(work,boot,invocation):
    private=work/'integration'
    build=decode(read_fixture_artifact(work,'build.json',1<<20),1<<20)
    if variant(build)!=EXHAUSTION:raise ValueError('exhaustion packet variant differs')
    receipt=decode(read_fixture_artifact(work,'integration-receipt.json',1<<20),1<<20)
    raw=read_fixture_artifact(private,'diagnostic-exhaustion-checkpoint.json',1<<20)
    if receipt.get('diagnostic_exhaustion_tested') is not True or receipt.get('diagnostic_exhaustion_sha256')!=sha(raw):raise ValueError('exhaustion lifecycle receipt differs')
    value=decode(raw,1<<20)
    binding=inspect_binding(private,build,boot)
    names=['exhaustion-'+phase+'-'+kind+'.json' for phase in PHASES for kind in ['preload','worker']]
    equal(value,dict(version=1,scope='instrumented-child-diagnostic-exhaustion',boot_id=boot,invocation_id=invocation,source_commit=build['source_commit'],binary_sha256=build['binaries']['containerd-shim-multikernel-v2'],binding=binding,artifact_sha256={name:sha(read_fixture_artifact(private,name,8192)) for name in names},child_log_pressure_sha256=receipt['child_log_pressure_sha256']))
    from runtime_child_log_checkpoint import inspect_fixture_logging
    inspect_fixture_logging(work,boot,invocation)
    return {**binding,'complete_child_build_matrix_bound':True,'child_zero_exit_bound':True}
