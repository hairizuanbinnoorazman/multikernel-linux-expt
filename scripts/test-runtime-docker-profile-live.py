#!/usr/bin/env python3
"""Disposable host Docker selects Multikernel and runs the offline child build proof."""
import hashlib,json,os,pathlib,subprocess,sys,time
from runtime_firewall_identity import identity as firewall_identity
from runtime_service_policy import parse_policy, command_with_policy, check_observation
from runtime_docker_concurrency import qualify as qualify_concurrency
assert 3<=len(sys.argv)<=5, 'usage: SCRIPT PROTECTED_WORK_DIRECTORY PROTECTED_SOURCE_ROOT [--restart-runtime|--concurrent] [--harden-services]'
flags=sys.argv[3:]
assert len(flags)==len(set(flags)) and set(flags)<={'--restart-runtime','--harden-services','--concurrent'}
restart_runtime='--restart-runtime' in flags
harden_services='--harden-services' in flags
concurrent='--concurrent' in flags
assert not (concurrent and restart_runtime), 'concurrency and restart are separate bounded scenarios'
work=pathlib.Path(sys.argv[1]);source=pathlib.Path(sys.argv[2])
assert work.is_absolute() and source.is_absolute()
assert str(work).startswith('/opt/mkruntime/g8-host-')
for path in [work,work/'bin',source,source/'scripts']:
 st=path.stat();assert path.is_dir() and not path.is_symlink() and st.st_uid==0 and st.st_mode&0o022==0

sha=lambda p:hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
def run(args,**kw):return subprocess.run([str(x) for x in args],check=True,timeout=100,**kw)
def services():
 d={}
 for name in ['mkruntimed','mknetd','containerd','docker']:
  assert subprocess.check_output(['systemctl','is-active',name],text=True).strip()=='active'
  d[name]=subprocess.check_output(['systemctl','show',name,'-p','MainPID','--value'],text=True).strip()
 return d
for ns in ['mnt']:assert os.readlink('/proc/self/ns/'+ns)!=os.readlink('/proc/1/ns/'+ns)
config=json.loads(pathlib.Path('/etc/mkruntime/config.json').read_text())
paths=[pathlib.Path('/etc/mkruntime/config.json'),pathlib.Path(config['state_directory'])/'state.json',pathlib.Path(config['state_directory'])/'journal.jsonl',pathlib.Path('/var/lib/mkruntimed/rootfs/state.json'),pathlib.Path('/var/lib/mkruntimed/storage/state.json'),pathlib.Path('/var/lib/mknetd/state.json')]+sorted(pathlib.Path(config['kernel_manifest_directory']).glob('*.json'))
before={str(p):sha(p) for p in paths};pids=services();boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
instances=pathlib.Path('/sys/fs/multikernel/instances');assert not list(instances.iterdir())
assert pathlib.Path('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert sha(work/'bin/containerd')=='dbc252c7f4e3534dcbe674552b87b20194967e7b6069e66de61632dcdd3acc83'
archive=pathlib.Path('/opt/mkruntime/g8-outer-b42ccf3/image.tar')
pinned_archive_digest=sha(archive)
assert pinned_archive_digest=='8af069def93985e73e5688b1d6bff1369597937c93a12a4b26475a32d18e7154'
private=work/'integration';private.mkdir(mode=0o700)
(private/'primary-before.json').write_text(json.dumps({'file_sha256':before,'service_main_pids':pids,'boot_id':boot},indent=2)+'\n')
for name in ['docker-data','docker-exec','containerd-root','containerd-state','exported','input','locks','manifests','daemon-state','rootfs-state','storage-state','storage-runtime','network-state']:(private/name).mkdir(mode=0o755 if name in ['exported','input'] else 0o700)
# Publish the exact approved xattr-free tree as Docker import data. The original
# export has capability PAX headers and is deliberately not imported directly.
archive=private/'approved-image.tar'
pack=run(['/usr/bin/python3',source/'scripts/build-runtime-docker-image-archive.py','/opt/mkruntime/g8-outer-b42ccf3/image',archive,'--manifest-sha256','627fa6bea79b118c7be162d0ab68f3f5899a0b6cd465499c11c7947026355ecf'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
(private/'pack.stdout').write_bytes(pack.stdout);(private/'pack.stderr').write_bytes(pack.stderr)
pack_receipt=json.loads(pack.stdout)
assert pack_receipt['archive_sha256']==sha(archive) and pack_receipt['xattrs_emitted'] is False
run(['/bin/mount','--make-rprivate','/']);run(['/bin/mount','--bind',private/'docker-data','/var/lib/docker'])
sock=private/'containerd.sock';dsock=private/'docker.sock'
daemon_socket=private/'control.sock';network_socket=private/'network.sock'
import shutil
for a,b,mode in [('/bin/busybox','busybox',0o555),(source/'guest/runtime-docker-build-probe.sh','probe.sh',0o444)]:
 shutil.copyfile(a,private/'input'/b);(private/'input'/b).chmod(mode)
profile=json.loads(pathlib.Path('/opt/mkruntime/g8-boot-d4643c8/private/profile.json').read_text())
approval=private/'manifests/gce-mk2.json';approval.write_text(json.dumps(profile));approval.chmod(0o600)
config.update(state_directory=str(private/'daemon-state'),socket_path=str(daemon_socket),kernel_manifest_directory=str(private/'manifests'))
host_config=private/'host.json';host_config.write_text(json.dumps(config));host_config.chmod(0o600)
storage_root=pathlib.Path('/srv/multikernel-storage')/work.name
storage_root.mkdir(mode=0o700)
extra={'MK_DAEMON_SOCKET':str(daemon_socket),'MK_NETWORK_SOCKET':str(network_socket),'MK_SHIM_LOCK':str(private/'locks/allocate'),'MK_SANDBOX_CPU_SETS':'8,9,10,11;12,13,14,15' if concurrent else '8,9,10,11','MK_RELAY':'/opt/mkruntime/g8-boot-d4643c8/mkvsock-relay','MK_KERNEL_MANIFEST':str(approval),'MK_KERNEL_MANIFEST_NAME':'gce-mk2','MK_NBD_HELPER':'/opt/mkruntime/g8-boot-d4643c8/mkvsock-nbd','MK_NBD_MODULE':profile['modules'][0]['path']}
# Store firewall rules without packet counters so transient test traffic does
# not invalidate identity checks. Exact rules must return to their prior state.
network_before={name:subprocess.check_output(['/usr/sbin/'+name]) for name in ['iptables-save','ip6tables-save']}
for name,data in network_before.items():(private/('firewall-before-'+name+'.rules')).write_bytes(data)
links_before=subprocess.check_output(['/usr/sbin/ip','-o','link','show']);(private/'links-before.log').write_bytes(links_before)
routes_before=subprocess.check_output(['/usr/sbin/ip','route']);(private/'routes-before.log').write_bytes(routes_before)

cc=private/'containerd.toml'
cc.write_text("version = 3\nroot = "+repr(str(private/'containerd-root'))+"\nstate = "+repr(str(private/'containerd-state'))+"\ndisabled_plugins = ['io.containerd.cri.v1.images', 'io.containerd.cri.v1.runtime', 'io.containerd.grpc.v1.cri', 'io.containerd.nri.v1.nri']\n[grpc]\naddress = "+repr(str(sock))+"\n[plugins.'io.containerd.shim.v1.manager']\nenv = ['PATH="+str(work/'bin')+":/usr/sbin:/usr/bin:/sbin:/bin']\n")
# Append a fresh version of the manager environment instead of duplicate TOML.
text=cc.read_text();text=text[:text.index('[plugins.')]+"[plugins.'io.containerd.shim.v1.manager']\nenv = "+repr(['PATH='+str(work/'bin')+':/usr/sbin:/usr/bin:/sbin:/bin']+[k+'='+v for k,v in extra.items()])+"\n";cc.write_text(text)
dc=private/'docker.json'
dc.write_text(json.dumps({'data-root':'/var/lib/docker','exec-root':str(private/'docker-exec'),'pidfile':str(private/'docker.pid'),'hosts':['unix://'+str(dsock)],'containerd':str(sock),'bridge':'none','iptables':False,'ip6tables':False,'ip-forward':False,'ip-masq':False,'features':{'containerd-snapshotter':True},'runtimes':{'io.containerd.multikernel.v2':{'runtimeType':'io.containerd.multikernel.v2'}}}))
children=[];logs=[]
service_policies={};service_observations={}
if harden_services:
 capability_names=subprocess.check_output(['/usr/bin/setpriv','--list-caps'],text=True).splitlines()
 for service_name in ['mkruntimed','mknetd']:
  unit=source/'deploy/systemd'/(service_name+'.service')
  policy=parse_policy(unit.read_text(),capability_names)
  policy['unit_sha256']=sha(unit)
  service_policies[str(work/'bin'/service_name)]=policy
 (private/'service-policies.json').write_text(json.dumps({'capability_names':capability_names,'setpriv_sha256':sha(pathlib.Path('/usr/bin/setpriv')),'policies':service_policies},indent=2)+'\n')
def start(args,name):
 if str(args[0]) in service_policies:args=command_with_policy(args,service_policies[str(args[0])])
 f=(private/name).open('wb');logs.append(f);p=subprocess.Popen([str(x) for x in args],stdout=f,stderr=subprocess.STDOUT,env={**os.environ,**extra,'PATH':str(work/'bin')+':/usr/sbin:/usr/bin:/sbin:/bin'});children.append(p);return p
def wait(path,p):
 until=time.monotonic()+30
 while not path.is_socket():
  assert p.poll() is None and time.monotonic()<until,'private daemon unavailable: '+str(p.args[0])
  time.sleep(.1)
def observe_service(p,name):
 if not harden_services:return
 assert p.poll() is None,'service stopped before policy observation'
 fields={key:value.strip() for key,value in (line.split(':',1) for line in pathlib.Path('/proc/'+str(p.pid)+'/status').read_text().splitlines() if ':' in line)}
 observed={key:fields[key] for key in ['Uid','CapBnd','CapEff','CapPrm','CapInh','CapAmb','NoNewPrivs']}
 policy=service_policies[str(work/'bin'/name)]
 check_observation(observed,policy)
 observed.update(pid=p.pid,executable_sha256=sha(pathlib.Path('/proc/'+str(p.pid)+'/exe')),unit_sha256=policy['unit_sha256'])
 assert observed['executable_sha256']==sha(work/'bin'/name) and p.poll() is None
 service_observations[name+'-'+str(p.pid)]=observed
 (private/'service-observations.json').write_text(json.dumps(service_observations,indent=2)+'\n')
cli=None;container_created=False
try:
 m=start([work/'bin/mkruntimed','--config='+str(host_config),'--pool-cpus=8,9,10,11,12,13,14,15','--pool-memory=8GB','--pool-memory-reserve=1GB','--storage-state-dir='+str(private/'storage-state'),'--storage-runtime-dir='+str(private/'storage-runtime'),'--storage-server=/opt/mkruntime/g8-boot-d4643c8/mkvsock-nbd','--rootfs-state-dir='+str(private/'rootfs-state'),'--rootfs-storage-dir='+str(storage_root),'--rootfs-builder='+str(source/'scripts/build-runtime-container-initramfs.sh')],'mkruntimed.log');wait(daemon_socket,m);observe_service(m,'mkruntimed')
 n=start([work/'bin/mknetd','--socket='+str(network_socket),'--state-dir='+str(private/'network-state'),'--subnet=172.30.0.0/24','--egress=ens4','--allowed-uid=0'],'mknetd.log');wait(network_socket,n);observe_service(n,'mknetd')
 c=start([work/'bin/containerd','--config',cc],'containerd.log');wait(sock,c)
 d=start(['/usr/bin/dockerd','--config-file',dc],'docker.log');wait(dsock,d)
 cli=['/usr/bin/docker','--host','unix://'+str(dsock)]
 until=time.monotonic()+30
 while subprocess.run(cli+['info'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:
  assert d.poll() is None and time.monotonic()<until;time.sleep(.1)
 info=subprocess.check_output(cli+['info','--format','{{json .}}']);(private/'docker-info.json').write_bytes(info)
 imported=run(cli+['import',archive,'g8-approved:fixture'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 (private/'import.stdout').write_bytes(imported.stdout);(private/'import.stderr').write_bytes(imported.stderr)
 workload=['/bin/sh','/work/input/probe.sh']
 if restart_runtime:
  workload=['/bin/sh','-ec','mkdir -m 700 /home/rootless/.local/tmp/g8-recovery; /bin/sh /work/input/probe.sh; echo G8_RUNTIME_RECOVERY_READY; while [ ! -f /home/rootless/.local/tmp/g8-recovery/release ]; do sleep 1; done; echo G8_RUNTIME_RECOVERY_RELEASED']
 if concurrent:
  workload=['/bin/sh','-ec','mkdir -m 700 /home/rootless/.local/tmp/g8-concurrency; echo G8_CONCURRENT_BOOT_READY; while [ ! -f /home/rootless/.local/tmp/g8-concurrency/build ]; do sleep 1; done; /bin/sh /work/input/probe.sh --overlap-proof; echo G8_CONCURRENT_BUILD_READY; while [ ! -f /home/rootless/.local/tmp/g8-concurrency/release ]; do sleep 1; done; echo G8_CONCURRENT_RELEASED']
 created=run(cli+['create','--name','g8-materialization','--runtime','io.containerd.multikernel.v2','--network','none','--user','1000:1000','--cap-drop','ALL','--security-opt','no-new-privileges=true','--security-opt','apparmor=unconfined','--security-opt','seccomp=unconfined','--sysctl','net.ipv4.ip_unprivileged_port_start=1024','--sysctl','net.ipv4.ping_group_range=1 0','--device-cgroup-rule','a *:* rwm','--mount','type=bind,source='+str(private/'input')+',target=/work/input,readonly','g8-approved:fixture']+workload,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 container_created=True
 (private/'create.stdout').write_bytes(created.stdout);(private/'create.stderr').write_bytes(created.stderr)
 exported=private/'export.tar'
 run(cli+['export','--output',exported,'g8-materialization'],stdout=subprocess.DEVNULL)
 # This archive is the export of the exact hash-pinned, locally imported image;
 # no workload or image program has run. Extraction stays in private scratch.
 run(['/bin/tar','--numeric-owner','-xpf',exported,'-C',private/'exported'])
 scan=private/'export.manifest.json'
 run(['/usr/bin/python3',source/'scripts/build-runtime-rootfs.py',private/'exported',private/'unused',scan,'--manifest-only'],stdout=subprocess.DEVNULL)
 approved=pathlib.Path('/opt/mkruntime/g8-outer-b42ccf3/private/approved.manifest.json')
 left={e['path']:e for e in json.loads(approved.read_bytes())['entries']};right={e['path']:e for e in json.loads(scan.read_bytes())['entries']}
 differences={k:{'approved':left.get(k),'exported':right.get(k)} for k in sorted(left.keys()|right.keys()) if left.get(k)!=right.get(k)}
 (private/'source-differences.json').write_text(json.dumps(differences,indent=2)+'\n')
 assert not differences,'Docker materialization differs; inspect retained source-differences.json before approving'
 if concurrent:
  observed=qualify_concurrency(cli,private,children,created.args,[work/'bin/mkruntimed','--config='+str(host_config),'--inspect-ownership'],profile['kernel_release'])
  shutil.copyfile(private/'a-workload.stdout',private/'workload.stdout');shutil.copyfile(private/'a-workload.stderr',private/'workload.stderr')
 else:
  with (private/'workload.stdout').open('wb') as out,(private/'workload.stderr').open('wb') as err:
   if restart_runtime:
    attach=subprocess.Popen(cli+['start','--attach','g8-materialization'],stdout=out,stderr=err);children.append(attach)
    deadline=time.monotonic()+300
    while 'G8_RUNTIME_RECOVERY_READY' not in (private/'workload.stdout').read_text():
     assert attach.poll() is None and time.monotonic()<deadline,'workload did not reach the recovery checkpoint'
     time.sleep(.2)
    def child_snapshot(phase):
     boot_result=run(cli+['exec','--user','1000:1000','g8-materialization','/bin/cat','/proc/sys/kernel/random/boot_id'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
     engine_result=run(cli+['exec','--user','1000:1000','g8-materialization','/usr/local/bin/docker','info','--format','{{json .}}'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
     image_result=run(cli+['exec','--user','1000:1000','g8-materialization','/usr/bin/sha256sum','/home/rootless/.local/tmp/g8-build/image.tar'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
     owner_result=run([work/'bin/mkruntimed','--config='+str(host_config),'--inspect-ownership'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
     for label,result in [('boot',boot_result),('engine',engine_result),('image',image_result),('ownership',owner_result)]:
      (private/('recovery-'+phase+'-'+label+'.stdout')).write_bytes(result.stdout)
      (private/('recovery-'+phase+'-'+label+'.stderr')).write_bytes(result.stderr)
      assert not result.stderr,'unexpected recovery inspection diagnostic'
     engine=json.loads(engine_result.stdout);ownership=json.loads(owner_result.stdout)
     assert ownership['source']=='durable-lifecycle' and len(ownership['sandboxes'])==1 and ownership['sandboxes'][0]['state']=='RUNNING'
     child_boot=boot_result.stdout.decode().strip();image_sha=image_result.stdout.decode().split()[0]
     assert child_boot!=boot and len(child_boot)==36 and len(image_sha)==64
     assert engine['ID'] and engine['KernelVersion']==profile['kernel_release'] and engine['ServerVersion']=='29.9.0'
     return {'child_boot_id':child_boot,'engine_id':engine['ID'],'saved_image_sha256':image_sha,'ownership':ownership['sandboxes']}
    prior=child_snapshot('before');old_pid=m.pid
    assert m.poll() is None,'runtime already stopped before the intentional restart'
    runtime_arguments=m.args
    m.terminate();assert m.wait(timeout=30)==0,'first runtime did not stop normally'
    m=start(runtime_arguments,'mkruntimed-restarted.log');wait(daemon_socket,m);observe_service(m,'mkruntimed')
    assert m.pid!=old_pid and m.poll() is None
    subsequent=child_snapshot('after');assert subsequent==prior,'child/engine/image/ownership changed across runtime restart'
    recovery={'schema_version':1,'invocation_id':os.environ['INVOCATION_ID'],'boot_id':boot,'runtime_pid_before':old_pid,'runtime_pid_after':m.pid,'runtime_restarts':1,'before':prior,'after':subsequent,'child_continuity_pass':True,'cleanup_qualified':False}
    (work/'recovery-checkpoint.json').write_text(json.dumps(recovery,indent=2)+'\n')
    run(cli+['exec','--user','1000:1000','g8-materialization','/bin/touch','/home/rootless/.local/tmp/g8-recovery/release'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    observed=subprocess.CompletedProcess(attach.args,attach.wait(timeout=60))
   else:
    observed=subprocess.run(cli+['start','--attach','g8-materialization'],stdout=out,stderr=err,timeout=300)
 assert observed.returncode==0,'host Docker start/build failed; retained workload output'
 inspected=subprocess.check_output(cli+['inspect','g8-materialization']);(private/'inspect.json').write_bytes(inspected)
 item=json.loads(inspected)[0];assert item['State']['Status']=='exited' and item['State']['ExitCode']==0 and item['HostConfig']['Runtime']=='io.containerd.multikernel.v2',item['State']
 stdout=(private/'workload.stdout').read_text();stderr=(private/'workload.stderr').read_text()
 assert len(stdout)<131072 and len(stderr)<131072
 for marker in ['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']:assert marker in stdout,marker
 assert profile['kernel_release'] in stdout and '29.9.0' in stdout and 'CACHED' in stderr
 checkpoint={'schema_version':1,'invocation_id':os.environ['INVOCATION_ID'],'boot_id':boot,'host_docker_selects_multikernel':True,'production_profile_child_build_pass':True,'cleanup_qualified':False,'execution_script_sha256':sha(pathlib.Path(__file__)),'source_archive_sha256':sha(work/'source.tar'),'profile_sha256':sha(approval),'approved_root_manifest_sha256':profile['docker_guest']['rootfs_manifest_sha256'],'prepared_archive_sha256':sha(archive),'probe_sha256':sha(private/'input/probe.sh'),'busybox_sha256':sha(private/'input/busybox'),'runtime_program_hashes':{name:sha(work/'bin'/name) for name in ['mkruntimed','mknetd','containerd-shim-multikernel-v2']},'qualified_containerd_sha256':sha(work/'bin/containerd'),'workload_stdout_sha256':sha(private/'workload.stdout'),'workload_stderr_sha256':sha(private/'workload.stderr')}
 (work/'workload-checkpoint.json').write_text(json.dumps(checkpoint,indent=2)+'\n')
 run(cli+['rm','g8-materialization'],stdout=subprocess.DEVNULL);container_created=False;run(cli+['image','rm','g8-approved:fixture'],stdout=subprocess.DEVNULL)
finally:
 try:
  if cli is not None and container_created:
   try:
    cleanup=subprocess.run(cli+['rm','--force','g8-materialization'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=90)
    (private/'cleanup.stdout').write_bytes(cleanup.stdout);(private/'cleanup.stderr').write_bytes(cleanup.stderr)
   except subprocess.TimeoutExpired:
    (private/'cleanup.stderr').write_text('owned Docker cleanup exceeded its bounded deadline; inspect retained ownership\n')
    raise
 finally:
  for p in reversed(children):
   if p.poll() is None:
    p.terminate()
    try:p.wait(timeout=15)
    except subprocess.TimeoutExpired:p.kill();p.wait(timeout=5)
  for f in logs:f.close()
network_after={name:subprocess.check_output(['/usr/sbin/'+name]) for name in network_before}
for name,data in network_after.items():(private/('firewall-after-'+name+'.rules')).write_bytes(data)
links_after=subprocess.check_output(['/usr/sbin/ip','-o','link','show']);(private/'links-after.log').write_bytes(links_after)
routes_after=subprocess.check_output(['/usr/sbin/ip','route']);(private/'routes-after.log').write_bytes(routes_after)
assert {name:firewall_identity(data) for name,data in network_after.items()}=={name:firewall_identity(data) for name,data in network_before.items()}
assert links_after==links_before and routes_after==routes_before
assert not list(instances.iterdir()) and pathlib.Path('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert 'No memory pool configured' in subprocess.check_output([config['kerf_executable'],'show'],text=True)
rootfs=json.loads((private/'rootfs-state/state.json').read_text())
storage=json.loads((private/'storage-state/state.json').read_text())
network=json.loads((private/'network-state/state.json').read_text())
assert not rootfs['records'] and not network['endpoints']
assert all(v['state']=='RELEASED' for v in storage['exports'].values())
assert not list(storage_root.iterdir())

after={str(p):sha(p) for p in paths};after_pids=services()
(private/'primary-after.json').write_text(json.dumps({'file_sha256':after,'service_main_pids':after_pids,'boot_id':pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()},indent=2)+'\n')
assert after==before and after_pids==pids
assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()==boot
r={'schema_version':1,'invocation_id':os.environ['INVOCATION_ID'],'boot_id':boot,'private_mount_namespace':True,'host_docker_selects_multikernel':True,'production_profile_child_build_pass':True,'private_daemon_state':True,'network_rules_and_links_restored':True,'rootfs_records_remaining':0,'network_endpoints_remaining':0,'live_storage_exports_remaining':0,'pool_released':True,'children_remaining':0,'online_cpus_restored':'0-15','docker_import_create_export_pass':True,'export_matches_approved_source':True,'production_files_unchanged':True,'four_service_pids_unchanged':True,'execution_script_sha256':sha(pathlib.Path(__file__)),'workload_checkpoint_sha256':sha(work/'workload-checkpoint.json'),'firewall_comparison':'exact-rules-and-policies-excluding-timestamps-and-chain-counters','export_root_mode':oct((private/'exported').stat().st_mode&0o777),'strict_source_comparison':True,'pinned_original_archive_sha256':pinned_archive_digest,'source_image_archive_sha256':sha(archive),'approved_tree_packaged_without_xattrs':True,'export_manifest_sha256':sha(scan),'host_docker_version':subprocess.check_output(['/usr/bin/docker','--version'],text=True).strip(),'qualified_containerd_sha256':sha(work/'bin/containerd')}
r['runtime_restart_tested']=restart_runtime
r['concurrent_guest_builds_tested']=concurrent
if concurrent:r['concurrency_checkpoint_sha256']=sha(private/'concurrency-checkpoint.json')
r['service_capability_nnp_tested']=harden_services
if harden_services:r.update(service_policy_sha256=sha(private/'service-policies.json'),service_observations_sha256=sha(private/'service-observations.json'))
if restart_runtime:r['recovery_checkpoint_sha256']=sha(work/'recovery-checkpoint.json')
(work/'integration-receipt.json').write_text(json.dumps(r,indent=2)+'\n');print('G8_HOST_DOCKER_CHILD_BUILD_PASS',flush=True)
