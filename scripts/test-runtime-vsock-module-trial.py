#!/usr/bin/env python3
"""One pinned GCE lab trial; no installed module files or queue policy are changed.

Requires a private systemd unit and already-staged, root-protected test artifacts.
Restoration refuses active children or module users; failed ownership is retained.
"""
import fcntl,hashlib,json,os,pathlib,signal,socket,stat,subprocess,time
P=pathlib.Path
BOOT='1bcdcae0-428b-435e-aff8-02d7637dc7ba'
MODULE=P('/opt/mkruntime/g8-vsock-rx-c5e57e3')
WORK=P('/opt/mkruntime/g8-host-39f32fa')
OLD='65C55BB38C669FF94A4B772';NEW='9361875B2E464E2FD6EDA5B'
OLD_SHA='bef1b888e7c66705f0d652b1437a239835584f2fb376ad6bfea2a77ecaa1c3d2'
NEW_SHA='a4c0f4e63212d9da405f196870deb2c9b34addbf04f88452423e46ad97507926'
NBD_SHA='c9981a1554230e87098472b586ab0fbe20f01c0b813fbd95df5b5f5c99d4a389'
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
unit=os.environ['MK_MODULE_TRIAL_UNIT'];assert unit=='g8-vsock-rx-trial-39f32fa.service'
scope=show(unit,'MainPID','InvocationID','PrivateMounts','KillMode')
assert scope=={'MainPID':str(os.getpid()),'InvocationID':os.environ['INVOCATION_ID'],'PrivateMounts':'yes','KillMode':'process'}
lock=(MODULE/'module-trial.lock').open('w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
assert not instances() and P('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert loaded()==OLD and P('/sys/module/mk_transport/refcnt').read_text().strip()=='0'
assert 'No memory pool configured' in output(['/opt/mkruntime/kerf-venv/bin/kerf','show'])
assert not output(['/usr/bin/docker','--host','unix:///var/run/docker.sock','ps','--all','--quiet'])
assert sha(WORK/'bin/mkvsock-nbd')==NBD_SHA
before_build=json.loads((MODULE/'build-preflight.json').read_text())
assert all(sha(p)==h for p,h in before_build['inputs_before'].items())
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
state={'boot_id':BOOT,'original_sha256':OLD_SHA,'candidate_sha256':NEW_SHA,'nbd_sha256':NBD_SHA,'parent_scope':scope,'primary_before':primary(),'taint_before':P('/proc/sys/kernel/tainted').read_text().strip(),'trial_driver_sha256':sha(__file__),'harness_source_commit':'39f32fa','source_archive_sha256':sha(WORK/'source.tar'),'events':[]}
def record(phase,**values):
    state.update(values);state['events'].append({'phase':phase,'monotonic_ns':time.monotonic_ns(),'loaded_srcversion':loaded()})
    temporary=trial/'receipt.tmp';temporary.write_text(json.dumps(state,indent=2)+'\n');temporary.chmod(0o600);os.replace(temporary,trial/'primary-module-trial.json')
def interrupted(signum,frame):raise RuntimeError('module trial interrupted by signal '+str(signum))
signal.signal(signal.SIGTERM,interrupted);signal.signal(signal.SIGINT,interrupted)
pressure='g8-host-docker-storage-pressure-39f32fa.service';removed=False;failure=None
record('preflight')
try:
    run(['/sbin/rmmod','mk_transport']);removed=True;record('original-unloaded')
    run(['/sbin/insmod',candidate]);assert loaded()==NEW;record('candidate-loaded')
    run(['systemd-run','--unit='+pressure,'--property=Type=exec','--property=RemainAfterExit=yes','--property=PrivateMounts=yes','--property=KillMode=process','--property=RuntimeMaxSec=1200','--setenv=MK_TEST_UNIT='+pressure,'/usr/bin/python3',WORK/'source/scripts/test-runtime-docker-profile-live.py',WORK,WORK/'source','--storage-pressure','--harden-services'])
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
    if removed:
        deadline=time.monotonic()+120
        while loaded()==NEW:
            inactive=all(status in ['loaded','created','ready'] for status in instances().values())
            if inactive and P('/sys/module/mk_transport/refcnt').read_text().strip()=='0':
                result=subprocess.run(['/sbin/rmmod','mk_transport'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=30)
                if result.returncode==0:break
            if time.monotonic()>=deadline:break
            time.sleep(1)
        if loaded() is None:run(['/sbin/insmod',original])
    state['original_restored']=loaded()==OLD
    state['primary_after']=primary();state['taint_after']=P('/proc/sys/kernel/tainted').read_text().strip()
    state['primary_unchanged']=state['primary_after']==state['primary_before']
    # An external unsigned test module can add O/E origin taints. Preserve the
    # observations and reject every other new taint (warnings, oops, etc.).
    state['taint_added_bits']=int(state['taint_after']) & ~int(state['taint_before'])
    state['allowed_module_origin_taint_bits']=(1<<12)|(1<<13)
    state['unexpected_taint_bits']=state['taint_added_bits'] & ~state['allowed_module_origin_taint_bits']
    record('restoration-observed')
assert state.get('original_restored') and state.get('primary_unchanged'),state
if failure:raise failure
assert state['pressure_scope']['Result']=='success' and state['pressure_scope']['ExecMainStatus']=='0','pressure qualification failed; retained test work must be diagnosed'
assert state['unexpected_taint_bits']==0,'unexpected kernel taint added during trial'
print('PINNED_MODULE_PRESSURE_TRIAL_AND_ORIGINAL_RESTORATION_PASS')
