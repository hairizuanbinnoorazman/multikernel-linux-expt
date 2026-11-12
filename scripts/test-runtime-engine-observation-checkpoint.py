#!/usr/bin/env python3
"""Receipt-bound positive cases and recomputed-hash semantic refusals."""
import copy,hashlib,json,os,subprocess,sys,tempfile,time,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_engine_observation_checkpoint as subject
from runtime_diagnostic_bundle_checkpoint import read_fixture_artifact
from runtime_docker_concurrency import sandbox_id_for_container

BOOT='94779fd9-6dd3-4bdf-8064-6149caccf1c5';INV='f'*32
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
class EngineCheckpoint(unittest.TestCase):
 def setUp(self):self.previous_umask=os.umask(0o077)
 def tearDown(self):os.umask(self.previous_umask)
 def test_real_command_timeout_retains_bounded_streams_without_checkpoint(self):
  with tempfile.TemporaryDirectory() as directory:
   p=Path(directory);phase='while-full'
   command=[sys.executable,'-c',"import os,time; os.write(1,b'x'*100); os.write(2,b'y'*50); time.sleep(30)"]
   start=time.monotonic()
   with self.assertRaises(subprocess.TimeoutExpired):subject.run_probe(command,p,phase,time.monotonic_ns()+500_000_000)
   self.assertLess(time.monotonic()-start,5)
   self.assertEqual((p/'engine-while-full.stdout').read_bytes(),b'x'*100)
   self.assertEqual((p/'engine-while-full.stderr').read_bytes(),b'y'*50)
   self.assertEqual(json.loads((p/'engine-while-full-failure.json').read_text()),dict(version=1,phase=phase,failure='command-timeout',qualified=False,stream_truncated=dict(stdout=False,stderr=False)))
   self.assertFalse((p/'engine-while-full.json').exists())
   self.assertEqual((p/'engine-while-full-failure.json').stat().st_mode&0o777,0o600)
 def test_timeout_without_output_preserves_empty_streams(self):
  with tempfile.TemporaryDirectory() as directory:
   p=Path(directory)
   with self.assertRaises(subprocess.TimeoutExpired):subject.run_probe([sys.executable,'-c','import time; time.sleep(30)'],p,'while-full',time.monotonic_ns()+100_000_000)
   self.assertEqual((p/'engine-while-full.stdout').read_bytes(),b'')
   self.assertEqual((p/'engine-while-full.stderr').read_bytes(),b'')
   self.assertEqual(json.loads((p/'engine-while-full-failure.json').read_text())['stream_truncated'],dict(stdout=False,stderr=False))
 def test_overflow_refuses_before_writer_finishes_with_private_capped_evidence(self):
  for fd,limit,suffix in [(1,2048,'stdout'),(2,256,'stderr')]:
   with self.subTest(stream=suffix),tempfile.TemporaryDirectory() as directory:
    p=Path(directory);marker=p/'writer-finished'
    command=[sys.executable,'-c','import os,pathlib,time;os.write('+str(fd)+',b"x"*(8<<20));pathlib.Path('+repr(str(marker))+').touch();time.sleep(5)']
    prior=os.umask(0)
    try:
     with self.assertRaises(subject.CommandOutputLimit):subject.run_probe(command,p,'while-full',time.monotonic_ns()+2_000_000_000)
    finally:os.umask(prior)
    self.assertFalse(marker.exists())
    self.assertEqual((p/('engine-while-full.'+suffix)).read_bytes(),b'x'*limit)
    for name in ['engine-while-full.stdout','engine-while-full.stderr','engine-while-full-failure.json']:self.assertEqual((p/name).stat().st_mode&0o777,0o600)
    failure=json.loads((p/'engine-while-full-failure.json').read_text())
    self.assertEqual(failure['failure'],'command-output-refused');self.assertIs(failure['qualified'],False)
    self.assertEqual(failure['stream_truncated'],dict(stdout=fd==1,stderr=fd==2))
    self.assertFalse((p/'engine-while-full.json').exists())
 def test_existing_evidence_and_invalid_scope_prevent_command_execution(self):
  for suffix in ['.stdout','.stderr','.json','-failure.json']:
   with self.subTest(suffix=suffix),tempfile.TemporaryDirectory() as directory:
    p=Path(directory);retained=p/('engine-while-full'+suffix);retained.write_bytes(b'SYNTHETIC_RETAINED_PROOF');inode=retained.stat().st_ino
    with patch.object(subject,'bounded_command') as command,self.assertRaises(ValueError):subject.run_probe(['/never-execute'],p,'while-full',time.monotonic_ns()+1_000_000_000)
    command.assert_not_called();self.assertEqual(retained.stat().st_ino,inode);self.assertEqual(retained.read_bytes(),b'SYNTHETIC_RETAINED_PROOF')
    self.assertEqual(len(list(p.iterdir())),1)
  with tempfile.TemporaryDirectory() as directory:
   with patch.object(subject,'bounded_command') as command,self.assertRaises(ValueError):subject.run_probe(['/never-execute'],Path(directory),'../unbound',time.monotonic_ns()+1_000_000_000)
   command.assert_not_called()
 def test_absolute_expired_budget_spawns_no_command_and_retains_refusal(self):
  with tempfile.TemporaryDirectory() as directory:
   p=Path(directory)
   with patch('runtime_bounded_command.subprocess.Popen') as command,self.assertRaises(subprocess.TimeoutExpired):subject.run_probe(['/never-execute'],p,'while-full',time.monotonic_ns()-1)
   command.assert_not_called()
   self.assertEqual(json.loads((p/'engine-while-full-failure.json').read_text())['failure'],'command-timeout')
 def test_actual_capture_all_phases_preserves_owner_and_build_window(self):
  for phase in subject.PHASES:
   with self.subTest(phase=phase),tempfile.TemporaryDirectory() as directory:
    work=Path(directory);p=work/'integration';p.mkdir(mode=0o700);(work/'bin').mkdir(mode=0o700)
    container='a'*64;owner=dict(sandbox_id=sandbox_id_for_container(container),generation='b'*32)
    address=p/'containerd-state/io.containerd.runtime.v2.task/moby'/container/'address';address.parent.mkdir(parents=True,mode=0o700);address.write_text('unix:///private/shim.sock\n')
    subject.write_private(p/'engine-shim-address.json',(json.dumps(dict(address='unix:///private/shim.sock',container_id=container))+'\n').encode())
    (p/'workload.stderr').write_bytes(b'#7 0.1 G8_OVERLAP_RUN_BEGIN\n')
    request=dict(version=1,namespace='moby',container_id=container,**owner)
    value=dict(version=1,owner=request,observation=dict(version=1,source='owned-child-docker-api-and-launcher',status='responding'))
    cli=work/'bin/mk-shimctl'
    cli.write_text('#!/usr/bin/python3\nimport os,sys\n'+('os.write(2,b"Docker engine inspection unavailable\\n");sys.exit(1)\n' if phase=='after-remove' else 'os.write(1,'+repr((json.dumps(value)+'\n').encode())+')\n'));cli.chmod(0o500)
    sample=subject.capture(work,BOOT,INV,container,owner,phase,time.monotonic()+2)
    self.assertEqual(sample['owner'],request);self.assertLessEqual(sample['finished_monotonic_ns'],sample['deadline_monotonic_ns'])
    self.assertEqual((p/('engine-'+phase+'.json')).stat().st_mode&0o777,0o600)
    if phase=='during-build':self.assertEqual((p/'engine-build-before.stderr').read_bytes(),(p/'engine-build-after.stderr').read_bytes())
    with self.assertRaises((ValueError,FileExistsError)):subject.capture(work,BOOT,INV,container,owner,phase,time.monotonic()+2)
 def fixture(self,base):
  work=base/'work';work.mkdir(mode=0o700);p=work/'integration';p.mkdir(mode=0o700)
  for n in ['bin']:(work/n).mkdir(mode=0o700)
  (p/'manifests').mkdir(mode=0o700)
  def write(root,n,value):
   (root/n).write_text(json.dumps(value,sort_keys=True)+'\n')
  original=base/'original.json';original.write_text(json.dumps({'kernel':'unchanged','agent':{'path':'old','sha256':'old'},'initramfs':{'path':'old','sha256':'old'}}));original.chmod(0o600)
  for name in ['mk-agent','mk-shimctl']:(work/'bin'/name).write_bytes(name.encode());(work/'bin'/name).chmod(0o500)
  archive=p/'current-agent.cpio.gz';archive.write_bytes(b'synthetic archive only')
  agent=work/'bin/mk-agent';approval=p/'manifests/gce-mk2.json'
  derived={**json.loads(original.read_text()),'agent':dict(path=str(agent),sha256=sha(agent)),'initramfs':dict(path=str(archive),sha256=sha(archive))};write(approval.parent,approval.name,derived)
  build=dict(source_commit='a'*40,binaries={n:sha(work/'bin'/n) for n in ['mk-agent','mk-shimctl']});write(work,'build.json',build)
  profile=dict(version=1,source_commit=build['source_commit'],original_profile_sha256=sha(original),derived_profile_sha256=sha(approval),agent_sha256=sha(agent),initramfs_sha256=sha(archive),full_original_and_derived_validation=True,installed_approval_changed=False,credential_injection_requested=False);write(p,'current-agent-profile-receipt.json',profile)
  for which in ['original','current']:
   write(p,which+'-agent-profile-validation.stdout',dict(agent=str(agent) if which=='current' else 'old'))
   (p/(which+'-agent-profile-validation.stderr')).write_bytes(b'')
  container='a'*64;sid=sandbox_id_for_container(container);owner=dict(sandbox_id=sid,generation='b'*32,state='RUNNING')
  (p/'create.stdout').write_text(container+'\n');write(p,'pressure-full-ownership.stdout',dict(source='durable-lifecycle',sandboxes=[owner]))
  request=dict(version=1,namespace='moby',container_id=container,sandbox_id=sid,generation=owner['generation'])
  before=b'#7 0.1 G8_OVERLAP_RUN_BEGIN\n';after=before+b'#7 1.0 controlled RUN still waiting\n'
  for which,data in [('before',before),('after',after)]: (p/('engine-build-'+which+'.stderr')).write_bytes(data)
  (p/'workload.stderr').write_bytes(after+b'#7 45.0 G8_OVERLAP_RUN_END\n')
  hashes={}
  for i,phase in enumerate(subject.PHASES):
   stdout=b'' if phase=='after-remove' else (json.dumps(dict(version=1,owner=request,observation=dict(version=1,source='owned-child-docker-api-and-launcher',status='responding')))+'\n').encode()
   stderr=b'Docker engine inspection unavailable\n' if phase=='after-remove' else b''
   for suffix,data in [('stdout',stdout),('stderr',stderr)]:(p/('engine-'+phase+'.'+suffix)).write_bytes(data)
   start=(i+1)*10_000_000_000
   sample=dict(version=1,boot_id=BOOT,invocation_id=INV,phase=phase,owner=request,budget_ns=5_000_000_000,started_monotonic_ns=start,deadline_monotonic_ns=start+5_000_000_000,finished_monotonic_ns=start+1_000_000,exit_status=1 if phase=='after-remove' else 0,stdout_sha256=hashlib.sha256(stdout).hexdigest(),stderr_sha256=hashlib.sha256(stderr).hexdigest(),build_stream_sha256={k:sha(p/('engine-build-'+k+'.stderr')) for k in ['before','after']} if phase=='during-build' else {})
   write(p,'engine-'+phase+'.json',sample);hashes[phase]=sha(p/('engine-'+phase+'.json'))
  write(work,'integration-receipt.json',dict(engine_observation_tested=True,boot_id=BOOT,invocation_id=INV,engine_profile_receipt_sha256=sha(p/'current-agent-profile-receipt.json'),engine_observation_sha256=hashes))
  return work,original
 def qualify(self,work,original):
  with patch.object(subject,'ORIGINAL_PROFILE',original):return subject.inspect_fixture_engine_observation(work,BOOT,INV)
 def rebind(self,work,phase):
  p=work/'integration';f=p/('engine-'+phase+'.json');s=json.loads(f.read_text())
  for suffix in ['stdout','stderr']:s[suffix+'_sha256']=sha(p/('engine-'+phase+'.'+suffix))
  if phase=='during-build':s['build_stream_sha256']={k:sha(p/('engine-build-'+k+'.stderr')) for k in ['before','after']}
  f.write_text(json.dumps(s,sort_keys=True)+'\n');r=json.loads((work/'integration-receipt.json').read_text());r['engine_observation_sha256'][phase]=sha(f);(work/'integration-receipt.json').write_text(json.dumps(r,sort_keys=True)+'\n')
 def test_valid_and_recomputed_reference_semantic_refusals(self):
  variants=['boot','invocation','owner','numeric-version','late','reordered','failed-live','extra-report-field','ready-status','removed-success','unbound-run-prefix','archive-drift']
  for variant in ['valid',*variants]:
   with self.subTest(variant=variant),tempfile.TemporaryDirectory() as directory:
    work,original=self.fixture(Path(directory));p=work/'integration';phase='during-build';f=p/('engine-'+phase+'.json');s=json.loads(f.read_text())
    if variant=='boot':s['boot_id']='0'*36
    if variant=='invocation':s['invocation_id']='0'*32
    if variant=='owner':s['owner']['generation']='c'*32
    if variant=='numeric-version':s['version']=1.0
    if variant=='late':s['finished_monotonic_ns']=s['deadline_monotonic_ns']+1
    if variant=='reordered':s['started_monotonic_ns']=1;s['deadline_monotonic_ns']=2;s['finished_monotonic_ns']=2
    if variant=='failed-live':s['exit_status']=1
    f.write_text(json.dumps(s,sort_keys=True)+'\n')
    if variant in ['extra-report-field','ready-status']:
     f=p/('engine-'+phase+'.stdout');r=json.loads(f.read_text())
     if variant=='extra-report-field':r['output']='private'
     else:r['observation']['status']='ready'
     f.write_text(json.dumps(r)+'\n')
    if variant=='removed-success':phase='after-remove';f=p/('engine-'+phase+'.json');s=json.loads(f.read_text());s['exit_status']=0;f.write_text(json.dumps(s)+'\n')
    if variant=='unbound-run-prefix':
     for which in ['before','after']:(p/('engine-build-'+which+'.stderr')).write_bytes(b'#99 0.1 G8_OVERLAP_RUN_BEGIN\n')
    if variant=='archive-drift':(p/'current-agent.cpio.gz').write_bytes(b'different archive')
    self.rebind(work,phase)
    if variant=='valid':self.assertEqual(self.qualify(work,original),subject.view(BOOT,INV))
    else:
     with self.assertRaises(ValueError):self.qualify(work,original)
 def test_empty_stream_requires_explicit_boolean_opt_in(self):
  with tempfile.TemporaryDirectory() as directory:
   p=Path(directory);p.chmod(0o700);(p/'empty').write_bytes(b'')
   with self.assertRaises(ValueError):read_fixture_artifact(p,'empty',1024)
   self.assertEqual(read_fixture_artifact(p,'empty',1024,allow_empty=True),b'')
   with self.assertRaises(ValueError):read_fixture_artifact(p,'empty',1024,allow_empty=1)
if __name__=='__main__':unittest.main()
