#!/usr/bin/env python3
"""One guarded same-boot GCE repeat; never reloads the primary transport.

Requires the successful f2d8439 cold trial and its protected staged artifacts.
The existing candidate stays loaded; external idle cold restoration follows.
"""
import fcntl,hashlib,json,os,pathlib,re,shutil,socket,stat,subprocess,tarfile,time,uuid
P=pathlib.Path
BOOT=os.environ['MK_WARM_TRIAL_BOOT_ID'];assert str(uuid.UUID(BOOT))==BOOT
PRIOR_SHA=os.environ['MK_WARM_PRIOR_TRIAL_SHA256'];assert re.fullmatch('[0-9a-f]{64}',PRIOR_SHA)
MODULE=P('/opt/mkruntime/g8-vsock-rx-b688447');FIRST=P('/opt/mkruntime/g8-host-f2d8439');WORK=P('/opt/mkruntime/g8-host-f2d8439-repeat1')
PARENT='g8-vsock-warm-trial-f2d8439-repeat1.service';CHILD='g8-host-docker-storage-pressure-f2d8439-repeat1.service'
SRC='0450DCCF7EDC67FAE2A6CAA';ARCHIVE='077b6091c2eb136b53da307e631b14626b6d8fc5182c638585f36f58aeca0472'
PROGRAMS={'mkruntimed':'f9585ce162c06ab163e11e05113527596e81db39daa75d9b63976fe298f93baa','mknetd':'aedaa42c241b9b9c2a519d5290f2164150029786830f8e35e74739f67376d1df','containerd-shim-multikernel-v2':'4025d58cf30bf9edd542ccc31ccbc7064bd3fae728867a61cfd68fe90ef10bd2','mkvsock-nbd':'c9981a1554230e87098472b586ab0fbe20f01c0b813fbd95df5b5f5c99d4a389','containerd':'dbc252c7f4e3534dcbe674552b87b20194967e7b6069e66de61632dcdd3acc83'}
def sha(p):return hashlib.file_digest(P(p).open('rb'),'sha256').hexdigest()
def output(args):return subprocess.check_output([str(v) for v in args],text=True,timeout=30).strip()
def show(unit,*keys):return dict(line.split('=',1) for line in output(['systemctl','show',unit,*[v for k in keys for v in ['-p',k]]]).splitlines())
def protected(p):
 for path in [p,*p.parents]:
  s=path.lstat();assert not stat.S_ISLNK(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,path
assert os.geteuid()==0 and socket.gethostname()=='mklinux-g7-storage-20261012'
assert P('/proc/sys/kernel/random/boot_id').read_text().strip()==BOOT
for p in [MODULE,FIRST,FIRST/'bin',P(__file__).resolve()]:protected(p)
assert os.environ['MK_WARM_TRIAL_UNIT']==PARENT
scope=show(PARENT,'MainPID','InvocationID','PrivateMounts','KillMode')
assert scope=={'MainPID':str(os.getpid()),'InvocationID':os.environ['INVOCATION_ID'],'PrivateMounts':'yes','KillMode':'process'}
lock=(MODULE/'module-trial.lock').open('w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
prior_path=MODULE/'trial-g8-host-f2d8439/primary-module-trial.json';protected(prior_path);assert sha(prior_path)==PRIOR_SHA
prior=json.loads(prior_path.read_text())
assert prior['boot_id']==BOOT and prior['harness_source_commit']=='f2d8439' and prior['source_archive_sha256']==ARCHIVE
assert prior['candidate_quiescent'] and prior['primary_unchanged'] and prior['cold_restore_required'] and not prior['original_restored']
assert prior['pressure_scope']['MainPID']=='0' and prior['pressure_scope']['Result']=='success' and prior['pressure_scope']['ExecMainStatus']=='0'
assert prior['candidate_sha256']==sha(MODULE/'module-proof/mk_transport.ko')=='74e813765d8551d086a603e047337b27ad0cce28dd4cc5de7e983a6e10a1f69f'
def primary():
 before=prior['primary_after']
 for name in before['service_main_pids']:assert output(['systemctl','is-active',name])=='active'
 return {'file_sha256':{p:sha(p) for p in before['file_sha256']},'service_main_pids':{n:output(['systemctl','show',n,'-p','MainPID','--value']) for n in before['service_main_pids']}}
def resources():
 instances=[v.name for v in P('/sys/fs/multikernel/instances').iterdir()]
 cpus=P('/sys/devices/system/cpu/online').read_text().strip();refs=int(P('/sys/module/mk_transport/refcnt').read_text())
 assert P('/sys/module/mk_transport/srcversion').read_text().strip()==SRC
 sockets=output(['ss','-A','vsock','-an'])
 pool=output(['/opt/mkruntime/kerf-venv/bin/kerf','show']) if not instances and cpus=='0-15' and refs==0 else ''
 return {'instances':instances,'online_cpus':cpus,'refcnt':refs,'vsock':sockets,'kerf':pool,'quiescent':not instances and cpus=='0-15' and refs==0 and len(sockets.splitlines())==1 and 'No memory pool configured' in pool}
before=resources();assert before['quiescent'] and primary()==prior['primary_after']
assert not output(['/usr/bin/docker','--host','unix:///var/run/docker.sock','ps','--all','--quiet'])
assert sha(FIRST/'source.tar')==ARCHIVE and all(sha(FIRST/'bin'/n)==h for n,h in PROGRAMS.items())
assert shutil.disk_usage('/srv/multikernel-storage').free>=4*1024**3 and shutil.disk_usage('/').free>=8*1024**3
assert not WORK.exists();WORK.mkdir(mode=0o700);(WORK/'source').mkdir(mode=0o700)
shutil.copyfile(FIRST/'source.tar',WORK/'source.tar')
with tarfile.open(WORK/'source.tar') as t:t.extractall(WORK/'source',filter='data')
shutil.copytree(FIRST/'bin',WORK/'bin');shutil.copytree(FIRST/'build',WORK/'build')
state={'boot_id':BOOT,'harness_source_commit':'f2d8439','runtime_source_commit':'f2d8439','fixture_id':WORK.name,'source_archive_sha256':ARCHIVE,'prior_module_trial_sha256':PRIOR_SHA,'parent_scope':scope,'driver_sha256':sha(__file__),'primary_transport_reloaded':False,'primary_transport_srcversion':SRC,'primary_before':primary(),'resources_before':before,'taint_before':P('/proc/sys/kernel/tainted').read_text().strip(),'events':[]}
def record(phase,**values):
 state.update(values);state['events'].append({'phase':phase,'monotonic_ns':time.monotonic_ns()})
 temp=WORK/'warm-receipt.tmp';temp.write_text(json.dumps(state,indent=2)+'\n');temp.chmod(0o600);os.replace(temp,WORK/'warm-trial.json')
record('qualified-candidate-reuse-preflight')
subprocess.run(['systemd-run','--unit='+CHILD,'--property=Type=exec','--property=RemainAfterExit=yes','--property=PrivateMounts=yes','--property=KillMode=process','--property=RuntimeMaxSec=1200','--setenv=MK_TEST_UNIT='+CHILD,'/usr/bin/python3',str(WORK/'source/scripts/test-runtime-docker-profile-live.py'),str(WORK),str(WORK/'source'),'--storage-pressure','--harden-services'],check=True,timeout=30)
deadline=time.monotonic()+1230;invocation=None
while True:
 observed=show(CHILD,'InvocationID','MainPID','ActiveState','SubState','Result','ExecMainStatus')
 assert observed['InvocationID'] and (invocation is None or observed['InvocationID']==invocation)
 invocation=observed['InvocationID']
 if observed['MainPID']=='0' and observed['SubState'] in ['exited','failed','dead']:break
 assert time.monotonic()<deadline
 time.sleep(2)
record('pressure-terminal',pressure_scope=observed,pressure_invocation=invocation)
deadline=time.monotonic()+120
while True:
 after=resources()
 if after['quiescent'] or time.monotonic()>=deadline:break
 time.sleep(1)
record('warm-repeat-observed',resources_after=after,primary_after=primary(),taint_after=P('/proc/sys/kernel/tainted').read_text().strip(),cold_restore_required=True)
assert state['primary_after']==state['primary_before'] and state['taint_after']==state['taint_before']
assert observed['Result']=='success' and observed['ExecMainStatus']=='0' and after['quiescent']
print('PINNED_SAME_BOOT_PRESSURE_REPEAT_AND_AUTOMATIC_CLEANUP_PASS; cold restoration required')
