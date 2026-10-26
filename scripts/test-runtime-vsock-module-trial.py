#!/usr/bin/env python3
"""One pinned GCE lab trial; no installed module files or queue policy are changed.

Requires a private systemd unit and already-staged, root-protected test artifacts.
Restoration after traffic uses an external idle reboot; cleanup is measured first.
"""
import fcntl,hashlib,json,os,pathlib,re,signal,socket,stat,subprocess,time,uuid
from runtime_trial_admission import observe as observe_trial_capacity, require as require_trial_capacity
from runtime_trial_helper import verify as verify_trial_helper
P=pathlib.Path
BOOT=os.environ['MK_MODULE_TRIAL_BOOT_ID'];assert str(uuid.UUID(BOOT))==BOOT
MODULE=P('/opt/mkruntime/g8-vsock-rx-b688447')
REVISION=os.environ['MK_MODULE_TRIAL_REVISION']
assert re.fullmatch(r'[0-9a-f]{7,40}',REVISION),'expected a committed source revision'
ARCHIVE_SHA=os.environ['MK_MODULE_TRIAL_ARCHIVE_SHA256']
assert re.fullmatch(r'[0-9a-f]{64}',ARCHIVE_SHA),'expected a pinned source archive digest'
WORK=P('/opt/mkruntime/g8-host-'+REVISION)
RECORD_FAULT=os.environ.get('MK_MODULE_TRIAL_RECORD_FAULT','0');assert RECORD_FAULT in ['0','1','fifo','crash']
OLD='65C55BB38C669FF94A4B772';NEW='0450DCCF7EDC67FAE2A6CAA'
OLD_SHA='bef1b888e7c66705f0d652b1437a239835584f2fb376ad6bfea2a77ecaa1c3d2'
NEW_SHA='74e813765d8551d086a603e047337b27ad0cce28dd4cc5de7e983a6e10a1f69f'

def run(args,**kw):return subprocess.run([str(v) for v in args],check=True,timeout=30,**kw)
def output(args):return subprocess.check_output([str(v) for v in args],text=True,timeout=30).strip()
def sha(path):return hashlib.file_digest(P(path).open('rb'),'sha256').hexdigest()
def show(unit,*keys):return dict(line.split('=',1) for line in output(['systemctl','show',unit,*[v for k in keys for v in ['-p',k]]]).splitlines() if '=' in line)
def loaded():
    p=P('/sys/module/mk_transport/srcversion')
    return p.read_text().strip() if p.exists() else None
def instances():return {p.name:(p/'status').read_text().strip() for p in P('/sys/fs/multikernel/instances').iterdir()}
def protected(path):
    for p in [path,*path.parents]:
        s=p.lstat();assert not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,p
assert os.geteuid()==0 and socket.gethostname()=='mklinux-g7-storage-20261012'
assert P('/proc/sys/kernel/random/boot_id').read_text().strip()==BOOT
for path in [MODULE,WORK,WORK/'bin',WORK/'source',WORK/'source/scripts']:protected(path)
unit=os.environ['MK_MODULE_TRIAL_UNIT'];assert unit=='g8-vsock-rx-trial-'+REVISION+'-release.service'
scope=show(unit,'MainPID','InvocationID','PrivateMounts','KillMode')
assert scope=={'MainPID':str(os.getpid()),'InvocationID':os.environ['INVOCATION_ID'],'PrivateMounts':'yes','KillMode':'process'}
lock=(MODULE/'module-trial.lock').open('w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
assert not instances() and P('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert loaded()==OLD and P('/sys/module/mk_transport/refcnt').read_text().strip()=='0'
vsock_before=output(['ss','-A','vsock','-an']);assert len(vsock_before.splitlines())==1
assert 'No memory pool configured' in output(['/opt/mkruntime/kerf-venv/bin/kerf','show'])
assert not output(['/usr/bin/docker','--host','unix:///var/run/docker.sock','ps','--all','--quiet'])
helper_selection=verify_trial_helper(WORK/'bin/mkvsock-nbd',os.environ.get('MK_MODULE_TRIAL_HELPER_VARIANT','qualified'))
NBD_SHA=helper_selection['helper_sha256']
assert sha(WORK/'source.tar')==ARCHIVE_SHA,'staged source archive differs from the requested trial'
before_build=json.loads((MODULE/'build-preflight.json').read_text())
assert output(['uname','-r'])==before_build['kernel_release']
assert all(sha(p)==h for p,h in before_build['inputs_before'].items())
capacity=observe_trial_capacity('/srv/multikernel-storage')
(WORK/'storage-admission-parent.json').write_text(json.dumps(capacity,indent=2)+'\n')
require_trial_capacity(capacity)
trial=MODULE/('trial-'+WORK.name);trial.mkdir(mode=0o700)
original=trial/'original.ko';candidate=trial/'candidate.ko'
for src,dst,digest in [(P(before_build['source_root'])/'net/vmw_vsock/mk_transport.ko',original,OLD_SHA),(MODULE/'module-proof/mk_transport.ko',candidate,NEW_SHA)]:
    fd=os.open(src,os.O_RDONLY|os.O_NOFOLLOW)
    with os.fdopen(fd,'rb') as source,dst.open('xb') as target:
        assert stat.S_ISREG(os.fstat(source.fileno()).st_mode)
        while data:=source.read(65536):target.write(data)
    dst.chmod(0o400);assert sha(dst)==digest
assert output(['modinfo','-F','vermagic',candidate])==output(['modinfo','-F','vermagic',original])
def primary():
    config=json.loads(P('/etc/mkruntime/config.json').read_text())
    assert not json.loads((P(config['state_directory'])/'state.json').read_text())['sandboxes']
    paths=[P('/etc/mkruntime/config.json'),P(config['state_directory'])/'state.json',P(config['state_directory'])/'journal.jsonl',P('/var/lib/mkruntimed/rootfs/state.json'),P('/var/lib/mkruntimed/storage/state.json'),P('/var/lib/mknetd/state.json')]+sorted(P(config['kernel_manifest_directory']).glob('*.json'))
    services={n:output(['systemctl','show',n,'-p','MainPID','--value']) for n in ['mkruntimed','mknetd','containerd','docker']}
    assert all(output(['systemctl','is-active',n])=='active' and int(pid)>0 for n,pid in services.items())
    return {'file_sha256':{str(p):sha(p) for p in paths},'service_main_pids':services}
state={'boot_id':BOOT,'original_sha256':OLD_SHA,'candidate_sha256':NEW_SHA,'nbd_sha256':NBD_SHA,'helper_selection':helper_selection,'parent_scope':scope,'vsock_before':vsock_before,'primary_before':primary(),'taint_before':P('/proc/sys/kernel/tainted').read_text().strip(),'trial_driver_sha256':sha(__file__),'harness_source_commit':REVISION,'source_archive_sha256':ARCHIVE_SHA,'native_record_fault_requested':RECORD_FAULT!='0','native_record_fault_kind':RECORD_FAULT,'events':[]}
def record(phase,**values):
    state.update(values);state['events'].append({'phase':phase,'monotonic_ns':time.monotonic_ns(),'loaded_srcversion':loaded(),'module_refcnt':int(P('/sys/module/mk_transport/refcnt').read_text()) if loaded() else None})
    temporary=trial/'receipt.tmp';temporary.write_text(json.dumps(state,indent=2)+'\n');temporary.chmod(0o600);os.replace(temporary,trial/'primary-module-trial.json')
def interrupted(signum,frame):raise RuntimeError('module trial interrupted by signal '+str(signum))
signal.signal(signal.SIGTERM,interrupted);signal.signal(signal.SIGINT,interrupted)
pressure='g8-host-docker-storage-pressure-'+REVISION+'.service';removed=False;failure=None
record('preflight')
try:
    run(['/sbin/rmmod','mk_transport']);removed=True;record('original-unloaded')
    run(['/sbin/insmod',candidate]);assert loaded()==NEW;record('candidate-loaded')
    run(['systemd-run','--unit='+pressure,'--property=Type=exec','--property=RemainAfterExit=yes','--property=PrivateMounts=yes','--property=KillMode=process','--property=RuntimeMaxSec=1200','--setenv=MK_TEST_UNIT='+pressure,'--setenv=MK_TRIAL_HELPER_VARIANT='+helper_selection['helper_variant'],'/usr/bin/python3',WORK/'source/scripts/test-runtime-docker-profile-live.py',WORK,WORK/'source','--storage-export-crash' if RECORD_FAULT=='crash' else '--storage-record-fifo' if RECORD_FAULT=='fifo' else '--storage-record-loss' if RECORD_FAULT=='1' else '--storage-pressure','--harden-services'])
    deadline=time.monotonic()+1230;invocation=None
    while True:
        observed=show(pressure,'InvocationID','MainPID','ActiveState','SubState','Result','ExecMainStatus')
        assert observed['InvocationID'] and (invocation is None or invocation==observed['InvocationID'])
        invocation=observed['InvocationID']
        if observed['MainPID']=='0' and observed['SubState'] in ['exited','failed','dead']:break
        assert time.monotonic()<deadline,'pressure service exceeded its bounded observation window'
        time.sleep(2)
    record('pressure-terminal',pressure_scope=observed,pressure_invocation=invocation)
except BaseException as error:
    failure=error;record('trial-error',error=str(error))
finally:
    if removed and loaded() is None:
        # Failed candidate initialization, before any child trial: reload the
        # frozen original. Never hot-unload a transport after live traffic here.
        run(['/sbin/insmod',original])
    if loaded()==NEW and RECORD_FAULT=='crash':
        state['candidate_quiescent']=False
        state['candidate_refs_after_trial']=int(P('/sys/module/mk_transport/refcnt').read_text())
        state['vsock_after_trial']=output(['ss','-A','vsock','-an'])
        record('crash-ownership-retained-for-diagnosis')
    elif loaded()==NEW:
        deadline=time.monotonic()+120
        while True:
            empty=not instances() and P('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
            refs=int(P('/sys/module/mk_transport/refcnt').read_text())
            sockets=output(['ss','-A','vsock','-an'])
            pool=output(['/opt/mkruntime/kerf-venv/bin/kerf','show']) if empty and refs==0 else ''
            if empty and refs==0 and len(sockets.splitlines())==1 and 'No memory pool configured' in pool:
                state['kerf_after_trial']=pool
                state['vsock_after_trial']=sockets;state['candidate_refs_after_trial']=refs
                state['candidate_quiescent']=True
                record('candidate-quiescent-cold-restore-required');break
            if time.monotonic()>=deadline:
                state['candidate_quiescent']=False;state['candidate_refs_after_trial']=refs
                state['vsock_after_trial']=sockets;record('candidate-retained-not-quiescent');break
            time.sleep(1)
    state['original_restored']=loaded()==OLD
    state['cold_restore_required']=loaded()==NEW
    record('module-state-observed')
    state['primary_after']=primary();state['taint_after']=P('/proc/sys/kernel/tainted').read_text().strip()
    state['primary_unchanged']=state['primary_after']==state['primary_before']
    # An external unsigned test module can add O/E origin taints. Preserve the
    # observations and reject every other new taint (warnings, oops, etc.).
    state['taint_added_bits']=int(state['taint_after']) & ~int(state['taint_before'])
    state['allowed_module_origin_taint_bits']=(1<<12)|(1<<13)
    state['unexpected_taint_bits']=state['taint_added_bits'] & ~state['allowed_module_origin_taint_bits']
    record('restoration-observed')
assert state.get('primary_unchanged'),state
if failure:raise failure
if RECORD_FAULT=='crash':
    retained=json.loads((WORK/'native-export-crash-retention.json').read_text())
    assert state['pressure_scope']['ExecMainStatus']=='77' and retained['owner_error']['code']=='OPERATOR_ACTION' and not retained['owner_error']['retryable']
    assert retained['image_retained'] and retained['storage_state']=='QUIESCING' and not retained['automatic_storage_release_claimed']
    assert state['unexpected_taint_bits']==0
    print('NATIVE_EXPORT_CRASH_REFUSAL_RETAINED_NO_NORMAL_CLEANUP_CLAIMED')
else:
    assert state['pressure_scope']['Result']=='success' and state['pressure_scope']['ExecMainStatus']=='0','pressure qualification failed; retained test work must be diagnosed'
    assert state.get('candidate_quiescent') and state.get('cold_restore_required'),state
    assert state['unexpected_taint_bits']==0,'unexpected kernel taint added during trial'
    print('PINNED_MODULE_PRESSURE_TRIAL_QUIESCENT_COLD_RESTORE_REQUIRED')
