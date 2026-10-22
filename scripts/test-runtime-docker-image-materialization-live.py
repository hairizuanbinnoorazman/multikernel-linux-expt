#!/usr/bin/env python3
"""Disposable host Docker image materialization; no child or image program runs."""
import hashlib,json,os,pathlib,subprocess,sys,time
assert len(sys.argv)==3, 'usage: SCRIPT PROTECTED_WORK_DIRECTORY PROTECTED_SOURCE_ROOT'
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
for ns in ['mnt','net']:assert os.readlink('/proc/self/ns/'+ns)!=os.readlink('/proc/1/ns/'+ns)
config=json.loads(pathlib.Path('/etc/mkruntime/config.json').read_text())
paths=[pathlib.Path('/etc/mkruntime/config.json'),pathlib.Path(config['state_directory'])/'state.json',pathlib.Path(config['state_directory'])/'journal.jsonl',pathlib.Path('/var/lib/mkruntimed/rootfs/state.json'),pathlib.Path('/var/lib/mkruntimed/storage/state.json'),pathlib.Path('/var/lib/mknetd/state.json')]+sorted(pathlib.Path(config['kernel_manifest_directory']).glob('*.json'))
before={str(p):sha(p) for p in paths};pids=services();boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
instances=pathlib.Path('/sys/fs/multikernel/instances');assert not list(instances.iterdir())
assert pathlib.Path('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert sha(work/'bin/containerd')=='dbc252c7f4e3534dcbe674552b87b20194967e7b6069e66de61632dcdd3acc83'
archive=pathlib.Path('/opt/mkruntime/g8-outer-b42ccf3/image.tar')
assert sha(archive)=='8af069def93985e73e5688b1d6bff1369597937c93a12a4b26475a32d18e7154'
private=work/'preflight';private.mkdir(mode=0o700)
for name in ['docker-data','docker-exec','containerd-root','containerd-state','exported']:(private/name).mkdir(mode=0o755 if name=='exported' else 0o700)
run(['/bin/mount','--make-rprivate','/']);run(['/bin/mount','--bind',private/'docker-data','/var/lib/docker'])
sock=private/'containerd.sock';dsock=private/'docker.sock'
cc=private/'containerd.toml'
cc.write_text("version = 3\nroot = "+repr(str(private/'containerd-root'))+"\nstate = "+repr(str(private/'containerd-state'))+"\ndisabled_plugins = ['io.containerd.cri.v1.images', 'io.containerd.cri.v1.runtime', 'io.containerd.grpc.v1.cri', 'io.containerd.nri.v1.nri']\n[grpc]\naddress = "+repr(str(sock))+"\n[plugins.'io.containerd.shim.v1.manager']\nenv = ['PATH="+str(work/'bin')+":/usr/sbin:/usr/bin:/sbin:/bin']\n")
dc=private/'docker.json'
dc.write_text(json.dumps({'data-root':'/var/lib/docker','exec-root':str(private/'docker-exec'),'pidfile':str(private/'docker.pid'),'hosts':['unix://'+str(dsock)],'containerd':str(sock),'bridge':'none','iptables':False,'ip6tables':False,'ip-forward':False,'ip-masq':False,'features':{'containerd-snapshotter':True},'runtimes':{'io.containerd.multikernel.v2':{'runtimeType':'io.containerd.multikernel.v2'}}}))
children=[];logs=[]
def start(args,name):
 f=(private/name).open('wb');logs.append(f);p=subprocess.Popen([str(x) for x in args],stdout=f,stderr=subprocess.STDOUT,env={**os.environ,'PATH':str(work/'bin')+':/usr/sbin:/usr/bin:/sbin:/bin'});children.append(p);return p
def wait(path,p):
 until=time.monotonic()+30
 while not path.is_socket():
  assert p.poll() is None and time.monotonic()<until,'private daemon unavailable'
  time.sleep(.1)
try:
 c=start([work/'bin/containerd','--config',cc],'containerd.log');wait(sock,c)
 d=start(['/usr/bin/dockerd','--config-file',dc],'docker.log');wait(dsock,d)
 cli=['/usr/bin/docker','--host','unix://'+str(dsock)]
 until=time.monotonic()+30
 while subprocess.run(cli+['info'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:
  assert d.poll() is None and time.monotonic()<until;time.sleep(.1)
 info=subprocess.check_output(cli+['info','--format','{{json .}}']);(private/'docker-info.json').write_bytes(info)
 imported=run(cli+['import',archive,'g8-approved:fixture'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 (private/'import.stdout').write_bytes(imported.stdout);(private/'import.stderr').write_bytes(imported.stderr)
 created=run(cli+['create','--name','g8-materialization','--runtime','io.containerd.multikernel.v2','--network','none','--user','1000:1000','--cap-drop','ALL','--security-opt','no-new-privileges=true','g8-approved:fixture','/usr/local/bin/docker','version'],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
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
 (private/'manifest-differences.json').write_text(json.dumps(differences,indent=2)+'\n')
 assert not differences,'Docker materialization differs; inspect retained manifest-differences.json before approving'
 run(cli+['rm','g8-materialization'],stdout=subprocess.DEVNULL);run(cli+['image','rm','g8-approved:fixture'],stdout=subprocess.DEVNULL)
finally:
 for p in reversed(children):
  if p.poll() is None:
   p.terminate()
   try:p.wait(timeout=15)
   except subprocess.TimeoutExpired:p.kill();p.wait(timeout=5)
 for f in logs:f.close()
assert not list(instances.iterdir()) and pathlib.Path('/sys/devices/system/cpu/online').read_text().strip()=='0-15'
assert {str(p):sha(p) for p in paths}==before and services()==pids
assert pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()==boot
r={'schema_version':1,'invocation_id':os.environ['INVOCATION_ID'],'boot_id':boot,'private_mount_namespace':True,'private_network_namespace':True,'no_child_started':True,'no_image_program_executed':True,'docker_import_create_export_pass':True,'export_matches_approved_source':True,'production_files_unchanged':True,'four_service_pids_unchanged':True,'execution_script_sha256':sha(pathlib.Path(__file__)),'export_root_mode':oct((private/'exported').stat().st_mode&0o777),'strict_source_comparison':True,'source_image_archive_sha256':sha(archive),'export_manifest_sha256':sha(scan),'host_docker_version':subprocess.check_output(['/usr/bin/docker','--version'],text=True).strip(),'qualified_containerd_sha256':sha(work/'bin/containerd')}
(work/'preflight-receipt.json').write_text(json.dumps(r,indent=2)+'\n');print('G8_HOST_DOCKER_IMAGE_PREFLIGHT_PASS',flush=True)
