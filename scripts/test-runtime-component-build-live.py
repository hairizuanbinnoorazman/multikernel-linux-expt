#!/usr/bin/env python3
"""One idle pinned GCE component-build trial using actual private daemon mains.

No child, storage export, endpoint, Docker workload or module transition is
requested. Never reuse this fixture's retained private state as live authority.
"""
from pathlib import Path
import fcntl,hashlib,json,os,re,socket,stat,subprocess as sp,sys,time
from runtime_primary_docker import observe as observe_docker, unchanged as require_docker_unchanged
from runtime_socket_readiness import wait_for_socket_owner
from runtime_service_policy import parse_policy, command_with_policy, check_observation
from runtime_firewall_identity import identity as firewall_identity
from runtime_component_build_checkpoint import qualify,qualify_unsupported
from runtime_diagnostic_bundle_checkpoint import qualify_idle as qualify_bundle

P=Path;sha=lambda p:hashlib.file_digest(P(p).open('rb'),'sha256').hexdigest()
work=P(sys.argv[1]);source=work/'source';build=json.loads((work/'build.json').read_text());revision=build['source_commit'][:7]
assert len(sys.argv) in [2,3] and (len(sys.argv)==2 or sys.argv[2]=='--diagnostic-bundles')
diagnostics=len(sys.argv)==3
boot=os.environ['MK_COMPONENT_TRIAL_BOOT_ID'];unit='g8-component-build-'+revision+'.service'
assert os.geteuid()==0 and socket.gethostname()=='mklinux-g7-storage-20261012'
assert re.fullmatch('[0-9a-f]{40}',build['source_commit']) and work==P('/opt/mkruntime/g8-components-'+revision)
for p in [work,work/'bin',source,source/'scripts',*work.parents]:
 s=p.lstat();assert s.st_uid==0 and not stat.S_ISLNK(s.st_mode) and not s.st_mode&0o022
assert P('/proc/sys/kernel/random/boot_id').read_text().strip()==boot
scope=dict(x.split('=',1) for x in sp.check_output(['systemctl','show',unit,'-p','MainPID','-p','InvocationID','-p','KillMode','-p','NoNewPrivileges'],text=True).splitlines())
assert scope==dict(MainPID=str(os.getpid()),InvocationID=os.environ['INVOCATION_ID'],KillMode='control-group',NoNewPrivileges='yes')
assert all(sha(work/'bin'/name)==digest for name,digest in build['binaries'].items())
lock=os.open('/opt/mkruntime/g8-vsock-rx-b688447/module-trial.lock',os.O_RDWR|os.O_NOFOLLOW)
assert stat.S_ISREG(os.fstat(lock).st_mode);fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
old=P('/opt/mkruntime/g8-host-'+('4a3e394' if diagnostics else '31ee79a')+'/cold-restoration');baseline=json.loads((old/'after.json').read_text())
assert baseline['invocation_id']==('451cd1617a8b43e9bfaa8d1262c0ec47' if diagnostics else '5702e90848434faba1d27cde675f02d3') and baseline['retained']['contents_checked']
assert baseline['resources']['boot_id']==boot
failed=P('/opt/mkruntime/g8-host-0c50de9')
def output(args):return sp.check_output([str(v) for v in args],text=True,timeout=15).strip()
def primary():
 r=dict(boot_id=P('/proc/sys/kernel/random/boot_id').read_text().strip(),file_sha256={p:sha(p) for p in baseline['resources']['file_sha256']},service_main_pids={n:output(['systemctl','show',n,'-p','MainPID','--value']) for n in baseline['resources']['service_main_pids']},module_src=P('/sys/module/mk_transport/srcversion').read_text().strip(),module_refs=int(P('/sys/module/mk_transport/refcnt').read_text()),online_cpus=P('/sys/devices/system/cpu/online').read_text().strip(),instances=[p.name for p in P('/sys/fs/multikernel/instances').iterdir()],pool_released='No memory pool configured' in output(['/opt/mkruntime/kerf-venv/bin/kerf','show']),vsock_sockets=len(output(['ss','-A','vsock','-an']).splitlines())-1)
 assert r['boot_id']==boot and r['module_src']=='65C55BB38C669FF94A4B772' and r['module_refs']==r['vsock_sockets']==0 and r['online_cpus']=='0-15' and not r['instances'] and r['pool_released']
 assert r['file_sha256']==baseline['resources']['file_sha256'] and r['service_main_pids']==baseline['resources']['service_main_pids']
 assert all(output(['systemctl','is-active',n])=='active' and int(pid)>0 for n,pid in r['service_main_pids'].items())
 assert {n:sha(failed/'integration'/n/'state.json') for n in baseline['resources']['failed_ledger_sha256']}==baseline['resources']['failed_ledger_sha256']
 assert sha(failed/'integration/daemon-state/journal.jsonl')==baseline['resources']['failed_journal_sha256']
 first=P('/opt/mkruntime/g8-pool-capacity-0e9542e');previous=json.loads((first/'operator-return/receipt.json').read_text())
 assert {p.name:sha(p) for p in (first/'private/state').iterdir()}==previous['original_private_file_sha256']
 preserved=json.loads((old/'private/output-preservation.json').read_text())
 payload=P('/opt/mkruntime/g8-graceful-stop-46dc4e25/private/stopped-output.json');meta=payload.lstat()
 assert stat.S_ISREG(meta.st_mode) and meta.st_uid==0 and stat.S_IMODE(meta.st_mode)==0o600 and meta.st_nlink==1 and meta.st_size==17133
 assert sha(payload)==preserved['sha256']
 return r
before=primary();docker_before=observe_docker(deadline=time.monotonic()+10)
require_docker_unchanged(baseline['primary_docker'],docker_before);assert docker_before['inventories']['containers']['count']==0
mount=P('/srv/multikernel-storage');data=json.loads(P('/opt/mkruntime/g8-data-expand-20261027/after.json').read_text())['inventory']
def retained_metadata():
 values={str(p.relative_to(mount)):(p.stat().st_dev,p.stat().st_ino,p.stat().st_size,p.stat().st_mode,p.stat().st_uid,p.stat().st_nlink) for p in mount.rglob('*') if p.is_file()}
 assert len(values)==93 and all(stat.S_ISREG((mount/name).lstat().st_mode) and not (mount/name).is_symlink() for name in values)
 for name,expected in data['files'].items():
  p=mount/name;s=p.lstat();assert {k:getattr(s,'st_'+{'device':'dev','inode':'ino'}.get(k,k)) for k in expected['identity']}==expected['identity']
 return values
retained_before=retained_metadata()
def network_identity():
 return dict(iptables=firewall_identity(sp.check_output(['/usr/sbin/iptables-save'],timeout=15)),ip6tables=firewall_identity(sp.check_output(['/usr/sbin/ip6tables-save'],timeout=15)),links=output(['/usr/sbin/ip','-o','link','show']),routes=output(['/usr/sbin/ip','route']))
network_before=network_identity()
private=work/'private';private.mkdir(mode=0o700)
config=json.loads(P('/etc/mkruntime/config.json').read_text());config.update(state_directory=str(private/'daemon-state'),socket_path=str(private/'runtime.sock'),backend_timeout_seconds=10)
host=private/'host.json';host.write_text(json.dumps(config));host.chmod(0o600)
network_socket=private/'network.sock';binary=work/'bin/mkruntimed'
versions={};revisions={}
for name in ['mkruntimed','mknetd']:
 text=output([work/'bin'/name,'--version']);match=re.fullmatch(re.escape(name)+r' version=([^\s]+) revision=([0-9a-f]{7,40})',text)
 assert match and match[2]==revision;versions[name],revisions[name]=match.groups()
def inspect(label,config_path,network_path,unsupported=None):
 result=sp.run([str(binary),'--config='+str(config_path),'--inspect-components','--network-socket='+str(network_path)],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15)
 assert result.stderr==b'' and result.returncode==(1 if unsupported else 0),'component inspection command failed'
 if unsupported:report=qualify_unsupported(result.stdout,boot,revision,unsupported)
 else:report=qualify(result.stdout,boot,revision,versions,revisions)
 (work/(label+'.json')).write_text(json.dumps(report,indent=2)+'\n')
 return report
inspect('installed-legacy','/etc/mkruntime/config.json','/run/mknetd.sock','RUNTIME')
bundles={}
def collect_bundle(label,config_path,network_path,unsupported=None):
 directory=private/'bundles';directory.mkdir(mode=0o700,exist_ok=True);destination=directory/(label+'.json')
 result=sp.run([str(binary),'--config='+str(config_path),'--network-socket='+str(network_path),'--diagnostic-bundle='+str(destination)],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15)
 assert result.returncode==1 and result.stderr==b'','idle unavailable collection must retain diagnostic failure'
 data=destination.read_bytes();s=destination.lstat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and s.st_nlink==1 and stat.S_IMODE(s.st_mode)==0o600 and 0<len(data)<=8<<20
 standalone=sp.run([str(binary),'--config='+str(config_path),'--network-socket='+str(network_path),'--doctor-all','--doctor-format=7'],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15)
 assert standalone.returncode==1 and not standalone.stderr
 qualify_bundle(data,result.stdout,standalone.stdout,boot,revision,versions,revisions,unsupported)
 original=sha(destination)
 repeat=sp.run([str(binary),'--config='+str(config_path),'--network-socket='+str(network_path),'--diagnostic-bundle='+str(destination)],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15)
 assert repeat.returncode==1 and repeat.stdout==b'' and repeat.stderr==b'diagnostic bundle destination is unavailable or already exists\n' and sha(destination)==original
 for suffix,kind in [('alias','symlink'),('shared','directory')]:
  target=directory/(label+'-'+suffix+('.json' if kind=='symlink' else ''))
  if kind=='symlink':target.symlink_to(destination);path=target
  else:target.mkdir(mode=0o755);path=target/'forbidden.json'
  refused=sp.run([str(binary),'--config='+str(config_path),'--network-socket='+str(network_path),'--diagnostic-bundle='+str(path)],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15)
  assert refused.returncode==1 and refused.stdout==b'' and sha(destination)==original
  if kind=='directory':assert stat.S_IMODE(target.stat().st_mode)==0o755 and not list(target.iterdir())
 (work/(label+'.json')).write_bytes(data)
 (work/(label+'-receipt.json')).write_bytes(result.stdout)
 (work/(label+'-operational-reference.json')).write_bytes(standalone.stdout)
 bundles[label]=dict(bundle_sha256=sha(destination),receipt_sha256=sha(work/(label+'-receipt.json')),operational_reference_sha256=sha(work/(label+'-operational-reference.json')),returncode=result.returncode,file_mode='0600',byte_count=len(data),existing_symlink_shared_refused=True)
if diagnostics:collect_bundle('bundle-installed-legacy','/etc/mkruntime/config.json','/run/mknetd.sock','RUNTIME')
children=[];logs=[];observations={};policies={}
cap_names=output(['/usr/bin/setpriv','--list-caps']).splitlines()
for name in ['mkruntimed','mknetd']:
 p=source/'deploy/systemd'/(name+'.service');policies[name]=parse_policy(p.read_text(),cap_names);policies[name]['unit_sha256']=sha(p)
def start(name,args,sock):
 f=(private/(name+'.log')).open('xb');os.fchmod(f.fileno(),0o600);logs.append(f)
 p=sp.Popen(command_with_policy([str(x) for x in args],policies[name]),stdout=f,stderr=sp.STDOUT,env={**os.environ,'PATH':str(work/'bin')+':/usr/sbin:/usr/bin:/sbin:/bin'})
 children.append((name,p));owner=wait_for_socket_owner(sock,p,timeout=45)
 fields=dict(x.split(':',1) for x in P('/proc/'+str(p.pid)+'/status').read_text().splitlines() if ':' in x)
 o={k:fields[k].strip() for k in ['Uid','CapBnd','CapEff','CapPrm','CapInh','CapAmb','NoNewPrivs']};check_observation(o,policies[name])
 o.update(pid=p.pid,socket_owner=owner,executable_sha256=sha('/proc/'+str(p.pid)+'/exe'));assert o['executable_sha256']==build['binaries'][name]
 observations[name]=o;return p
exit_codes={};complete=False
try:
 runtime=start('mkruntimed',[binary,'--config='+str(host),'--pool-cpus=8,9,10,11,12,13,14,15','--pool-memory=8GB','--pool-memory-reserve=1GB','--storage-state-dir='+str(private/'storage-state'),'--storage-runtime-dir='+str(private/'storage-runtime'),'--storage-server='+str(work/'bin/mkvsock-nbd'),'--rootfs-state-dir='+str(private/'rootfs-state'),'--rootfs-storage-dir='+str(private/'rootfs-images')],P(config['socket_path']))
 inspect('network-legacy',host,'/run/mknetd.sock','NETWORK')
 if diagnostics:collect_bundle('bundle-network-legacy',host,'/run/mknetd.sock','NETWORK')
 routes=json.loads(sp.check_output(['/usr/sbin/ip','-j','route','show','default'],timeout=15));devices={r['dev'] for r in routes};assert len(devices)==1;egress=devices.pop();assert re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,14}',egress)
 net=start('mknetd',[work/'bin/mknetd','--socket='+str(network_socket),'--state-dir='+str(private/'network-state'),'--egress='+egress,'--subnet=172.30.0.0/24','--allowed-uid=0'],network_socket)
 state_paths=[private/n/'state.json' for n in ['daemon-state','storage-state','rootfs-state','network-state']]+[private/'daemon-state/journal.jsonl']
 state_before={str(p.relative_to(private)):sha(p) if p.exists() else None for p in state_paths}
 inspect('actual-components',host,network_socket);inspect('actual-components-repeat',host,network_socket)
 if diagnostics:collect_bundle('bundle-actual-components',host,network_socket)
 assert {str(p.relative_to(private)):sha(p) if p.exists() else None for p in state_paths}==state_before
 empty_queries=[('empty-runtime-ownership',[binary,'--config='+str(host),'--inspect-ownership'],'sandboxes'),('empty-network-ownership',[work/'bin/mknetd','--socket='+str(network_socket),'--inspect-ownership'],'endpoints')]
 for label,args,key in empty_queries:
  result=sp.run([str(x) for x in args],stdout=sp.PIPE,stderr=sp.PIPE,timeout=15);assert result.returncode==0 and not result.stderr
  value=json.loads(result.stdout);assert value[key]==[];(work/(label+'.json')).write_text(json.dumps(value,indent=2)+'\n')
 for name,p in children:assert p.poll() is None and sha('/proc/'+str(p.pid)+'/exe')==build['binaries'][name]
 complete=True
finally:
 for name,p in reversed(children):
  if p.poll() is None:p.terminate()
  try:p.wait(timeout=15)
  except sp.TimeoutExpired:p.kill();p.wait(timeout=5)
  exit_codes[name]=p.returncode
 for log in logs:log.close()
assert complete and set(exit_codes.values())=={0},'private daemons did not shut down cleanly'
after=primary();assert after==before
require_docker_unchanged(docker_before,observe_docker(deadline=time.monotonic()+10))
assert network_identity()==network_before and retained_metadata()==retained_before
for name,key in [('daemon-state','sandboxes'),('network-state','endpoints'),('storage-state','exports'),('rootfs-state','records')]:
 p=private/name/'state.json';assert not p.exists() or not json.loads(p.read_text())[key]
assert {str(p.relative_to(private)):sha(p) if p.exists() else None for p in state_paths}==state_before
owned=[]
for p in P('/proc').iterdir():
 if not p.name.isdigit() or int(p.name)==os.getpid():continue
 try:
  if '/'+unit in (p/'cgroup').read_text():owned.append(int(p.name))
 except (FileNotFoundError,ProcessLookupError,PermissionError):pass
assert not owned
report=dict(version=1,source_commit=build['source_commit'],boot_id=boot,invocation_id=os.environ['INVOCATION_ID'],scope=scope,primary_before=before,primary_after=after,primary_docker=docker_before,private_daemon_exit_codes=exit_codes,service_policies=policies,service_observations=observations,private_state_sha256=state_before,retained_file_identities_unchanged=True,retained_file_count=93,retained_contents_rehashed=False,failed_fixture_state_and_private_output_preserved=True,network_rules_links_routes_unchanged=True,no_child_pool_export_endpoint_requested=True,owned_processes_remaining=owned,compiled_versions=versions,compiled_revisions=revisions,full_g7_g8_closed=False,raw_logs_exported=False)
if diagnostics:report.update(diagnostic_bundles=bundles,live_docker_bundle_qualified=False)
(work/'receipt.json').write_text(json.dumps(report,indent=2)+'\n')
print('ACTUAL_PRIVATE_DAEMON_COMPONENT_BUILD_RPC_CLI_REPEAT_LEGACY_REFUSAL_AND_CLEAN_SHUTDOWN_PASS_PRIMARY_UNCHANGED_FULL_G7_G8_OPEN')
