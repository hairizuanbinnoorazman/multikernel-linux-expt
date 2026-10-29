#!/usr/bin/env python3
import copy,hashlib,importlib.util,json,os,subprocess,sys,tempfile,unittest
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import qualify_idle,qualify_live,qualify_timing,qualify_lifecycle_timings,read_fixture_artifact,SCOPE,EXCLUDED,UNOBSERVED
from runtime_component_build_checkpoint import expected_base
BOOT='3d317685-064f-4751-8589-a1f4799525b9';REV='abcdef0'

class BundleTests(unittest.TestCase):
 def fixture(self):
  c=expected_base(BOOT,REV);c['findings']=[dict(code='RUNTIME_BUILD_UNSUPPORTED')]
  o=dict(version=7,scope='resource-network-pool-and-compiled-component-observations',status='inconclusive',revision=REV,boot_id=BOOT,boot_stable=False,findings=[dict(code=c) for c in ['RESOURCE_OBSERVATION_UNAVAILABLE','COMBINED_OBSERVATION_UNAVAILABLE','NATIVE_RESOURCE_IDENTITY_UNSUPPORTED']],admission_guaranteed=False,artifact_verified=False,kernel_verified=False,unobserved=UNOBSERVED)
  b=dict(version=1,scope=SCOPE,collection_status='captured',started_at='2026-10-29T05:00:00Z',finished_at='2026-10-29T05:00:01.123456789Z',snapshot=dict(components=c,operational=o),excluded=EXCLUDED)
  return b,o
 def invoke(self,b,o,receipt=None,data=None):
  data=json.dumps(b).encode() if data is None else data
  receipt=dict(version=1,scope=SCOPE,collection_status='captured',observation_status='inconclusive',bytes=len(data),sha256=hashlib.sha256(data).hexdigest()) if receipt is None else receipt
  return qualify_idle(data,json.dumps(receipt).encode(),json.dumps(o).encode(),BOOT,REV,{}, {},'RUNTIME')
 def test_exact_and_tampered_complete_envelope(self):
  b,o=self.fixture();self.assertEqual(self.invoke(b,o),b)
  for kind in ['version','float','bool','scope','status','time','private','excluded','partial','health','attestation','observation','labels']:
   b,o=copy.deepcopy(self.fixture())
   if kind=='version':b['version']=2
   elif kind=='float':b['version']=1.0
   elif kind=='bool':b['version']=True
   elif kind=='scope':b['scope']='health'
   elif kind=='status':b['collection_status']='healthy'
   elif kind=='time':b['started_at']='private-time'
   elif kind=='private':b['raw-log']='secret'
   elif kind=='excluded':b['excluded']=[]
   elif kind=='partial':b['snapshot'].pop('components')
   elif kind=='health':b['snapshot']['operational']['unobserved']=[]
   elif kind=='attestation':b['snapshot']['operational']['artifact_verified']=True
   elif kind=='observation':b['snapshot']['operational']['status']='consistent'
   elif kind=='labels':b['snapshot']['components']['revision']='1234567'
   with self.subTest(kind=kind),self.assertRaises((ValueError,KeyError)):self.invoke(b,o)
 def test_receipt_reference_duplicates_and_oversize(self):
  b,o=self.fixture();data=json.dumps(b).encode()
  for raw in [b'{"version":1,'+data[1:],b' '*(8<<20)+data]:
   with self.assertRaises(ValueError):self.invoke(b,o,data=raw)
  for receipt in [{},dict(version=1,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(data),sha256=hashlib.sha256(data).hexdigest())]:
   with self.assertRaises(ValueError):self.invoke(b,o,receipt)
  o['status']='consistent'
  with self.assertRaises(ValueError):self.invoke(b,o)
 def test_actual_labels_and_network_legacy_remain_inconclusive(self):
  for mode in [None,'NETWORK']:
   b,o=copy.deepcopy(self.fixture());c=expected_base(BOOT,REV)
   versions={n:'0.0.0-dev' for n in ['mkruntimed','mknetd']};revisions={n:REV for n in versions}
   if mode=='NETWORK':
    c['findings']=[dict(code='NETWORK_BUILD_UNSUPPORTED')]
    codes=['RESOURCE_OBSERVATION_UNAVAILABLE','COMBINED_OBSERVATION_UNAVAILABLE','NETWORK_OWNERSHIP_UNSUPPORTED']
   else:
    c.update(status='consistent',boot_stable=True,compiled_revisions_match=True)
    for field,name in [('runtime','mkruntimed'),('network','mknetd')]:c[field]=dict(version=1,source='compiled-component-self-report',component=name,build_version='0.0.0-dev',build_revision=REV,artifact_verified=False,kernel_verified=False)
    codes=['RESOURCE_OBSERVATION_UNAVAILABLE','POOL_OBSERVATION_UNAVAILABLE','KERNEL_CPU_FREE_LIST_UNAVAILABLE']
   o['findings']=[dict(code=code) for code in codes];b['snapshot']=dict(components=c,operational=o)
   data=json.dumps(b).encode();receipt=dict(version=1,scope=SCOPE,collection_status='captured',observation_status='inconclusive',bytes=len(data),sha256=hashlib.sha256(data).hexdigest())
   self.assertEqual(qualify_idle(data,json.dumps(receipt).encode(),json.dumps(o).encode(),BOOT,REV,versions,revisions,mode),b)

spec=importlib.util.spec_from_file_location('operational_controls',Path(__file__).with_name('test-runtime-operational-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)

class LiveBundleTests(unittest.TestCase):
 def test_private_fixture_reader_rejects_aliases_shared_paths_and_oversize(self):
  with tempfile.TemporaryDirectory() as directory:
   parent=Path(directory);file=parent/'record.json';file.write_bytes(b'bounded');file.chmod(0o600)
   self.assertEqual(read_fixture_artifact(parent,'record.json',64),b'bounded')
   with self.assertRaises(ValueError):read_fixture_artifact(parent,'record.json',1)
   os.symlink(file,parent/'symlink.json')
   with self.assertRaises(OSError):read_fixture_artifact(parent,'symlink.json',64)
   os.link(file,parent/'hardlink.json')
   with self.assertRaises(ValueError):read_fixture_artifact(parent,'hardlink.json',64)
   (parent/'hardlink.json').unlink();file.chmod(0o666)
   with self.assertRaises(ValueError):read_fixture_artifact(parent,'record.json',64)
   file.chmod(0o600);parent.chmod(0o755)
   with self.assertRaises(ValueError):read_fixture_artifact(parent,'record.json',64)
   self.assertEqual(parent.stat().st_mode&0o777,0o755)
   parent.chmod(0o700);(parent/'alias').symlink_to(parent,target_is_directory=True)
   with self.assertRaises(OSError):read_fixture_artifact(parent/'alias','record.json',64)
   with self.assertRaises(ValueError):read_fixture_artifact(parent,'../record.json',64)
   os.mkfifo(parent/'fifo.json',0o600)
   code="import sys;sys.path.insert(0,sys.argv[1]);from runtime_diagnostic_bundle_checkpoint import read_fixture_artifact\ntry:read_fixture_artifact(sys.argv[2],'fifo.json',64)\nexcept ValueError:sys.exit(0)\nsys.exit(1)"
   result=subprocess.run([sys.executable,'-c',code,str(Path(__file__).parent),str(parent)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=2)
   self.assertEqual(result.returncode,0,result.stderr)
 def lifecycle(self):
  frames={}
  for n,phase in enumerate(['while-full','while-image-ready','after-remove']):
   start=100+n*1_000_000_000
   frames[phase]=json.dumps(dict(version=1,scope='doctor-and-diagnostic-bundle-checkpoint',phase=phase,configured_budget_ns=10_000_000_000,started_monotonic_ns=start,deadline_monotonic_ns=start+10_000_000_000,finished_monotonic_ns=start+100,elapsed_ns=100,report_count=8)).encode()
  receipt=dict(schema_version=1,boot_id=BOOT,invocation_id='9'*32,storage_pressure_tested=True,production_profile_child_build_pass=True,runtime_diagnostic_checkpoint_timing_sha256={k:hashlib.sha256(v).hexdigest() for k,v in frames.items()})
  return receipt,frames
 def test_lifecycle_timings_bind_receipt_identity_digests_and_phase_order(self):
  receipt,frames=self.lifecycle()
  report=qualify_lifecycle_timings(json.dumps(receipt).encode(),frames,BOOT,'9'*32)
  self.assertEqual(report['maximum_elapsed_ns'],100);self.assertEqual(set(report['timings']),set(frames))
  for kind in ['boot','invocation','schema','partial','missing-frame','digest','pressure','build','reordered','oversize','duplicate']:
   receipt,frames=self.lifecycle()
   if kind=='boot':receipt['boot_id']='00000000-0000-0000-0000-000000000000'
   elif kind=='invocation':receipt['invocation_id']='8'*32
   elif kind=='schema':receipt['schema_version']=True
   elif kind=='partial':receipt['runtime_diagnostic_checkpoint_timing_sha256'].pop('after-remove')
   elif kind=='missing-frame':frames.pop('while-full')
   elif kind=='digest':receipt['runtime_diagnostic_checkpoint_timing_sha256']['while-full']='0'*64
   elif kind=='pressure':receipt['storage_pressure_tested']=False
   elif kind=='build':receipt['production_profile_child_build_pass']=False
   elif kind=='reordered':
    t=json.loads(frames['while-image-ready']);t.update(started_monotonic_ns=0,finished_monotonic_ns=100,deadline_monotonic_ns=10_000_000_000)
    frames['while-image-ready']=json.dumps(t).encode();receipt['runtime_diagnostic_checkpoint_timing_sha256']['while-image-ready']=hashlib.sha256(frames['while-image-ready']).hexdigest()
   elif kind=='oversize':frames['while-full']=b' '*65537
   elif kind=='duplicate':frames['while-full']=b'{"version":1,'+frames['while-full'][1:];receipt['runtime_diagnostic_checkpoint_timing_sha256']['while-full']=hashlib.sha256(frames['while-full']).hexdigest()
   with self.subTest(kind=kind),self.assertRaises(ValueError):qualify_lifecycle_timings(json.dumps(receipt).encode(),frames,BOOT,'9'*32)
 def test_shared_monotonic_checkpoint_budget_and_numeric_alias_refusal(self):
  t=dict(version=1,scope='doctor-and-diagnostic-bundle-checkpoint',phase='while-image-ready',configured_budget_ns=10_000_000_000,started_monotonic_ns=100,deadline_monotonic_ns=10_000_000_100,finished_monotonic_ns=200,elapsed_ns=100,report_count=8)
  self.assertEqual(qualify_timing(json.dumps(t).encode(),'while-image-ready'),t)
  for key,value in [('deadline_monotonic_ns',180_000_000_100),('finished_monotonic_ns',10_000_000_101),('elapsed_ns',101),('configured_budget_ns',180_000_000_000),('started_monotonic_ns',True),('version',1.0),('report_count',7),('phase','after-remove'),('scope','health')]:
   altered=dict(t);altered[key]=value
   with self.subTest(key=key),self.assertRaises(ValueError):qualify_timing(json.dumps(altered).encode(),'while-image-ready')
 def fixture(self,released=False):
  refs=controls.OperationalCheckpointTests().fixture(released)
  b=dict(version=1,scope=SCOPE,collection_status='captured',started_at='2026-10-29T05:00:00Z',finished_at='2026-10-29T05:00:01Z',snapshot=dict(components=refs[6],operational=refs[-1]),excluded=EXCLUDED)
  return refs,b
 def invoke(self,refs,b,released=False,receipt=None,data=None):
  data=json.dumps(b).encode() if data is None else data
  receipt=dict(version=1,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(data),sha256=hashlib.sha256(data).hexdigest()) if receipt is None else receipt
  s,e,n,c,p,u,component,versions,revisions,op=refs
  return qualify_live(data,json.dumps(receipt).encode(),controls.BOOT,controls.OWNER,s,n,e,c,p,u,component,op,versions,revisions,released=released)
 def test_exact_live_and_released_with_independent_full_reports(self):
  for released in [False,True]:
   refs,b=self.fixture(released);self.assertEqual(self.invoke(refs,b,released),b)
 def test_self_consistent_forged_reference_views_are_requalified(self):
  for index in [0,2,3,4,5,6,9]:
   refs,b=copy.deepcopy(self.fixture());refs=list(refs)
   refs[index]['status']='inconclusive'
   with self.subTest(index=index),self.assertRaises(ValueError):self.invoke(refs,b)
  refs,b=copy.deepcopy(self.fixture());refs[4]['memory_candidate_upper_bound_bytes']+=1
  with self.assertRaises(ValueError):self.invoke(refs,b)
 def test_forged_bundle_fields_cannot_hide_behind_a_matching_digest(self):
  for kind in ['status','admission','artifact','private','partial','sequence','build','capacity','health','float','bool']:
   refs,b=copy.deepcopy(self.fixture());o=b['snapshot']['operational']
   if kind=='status':o['status']='inconclusive'
   elif kind=='admission':o['admission_guaranteed']=True
   elif kind=='artifact':o['artifact_verified']=True
   elif kind=='private':b['raw-log']='private-token'
   elif kind=='partial':b['snapshot'].pop('components')
   elif kind=='sequence':o['unified']['pool']['ownership']['lifecycle']['sequence']+=1
   elif kind=='build':b['snapshot']['components']['runtime']['build_revision']='1234567'
   elif kind=='capacity':o['unified']['pool']['memory_candidate_upper_bound_bytes']+=1
   elif kind=='health':o['unobserved']=[]
   elif kind=='float':b['version']=1.0
   elif kind=='bool':b['version']=True
   with self.subTest(kind=kind),self.assertRaises(ValueError):self.invoke(refs,b)
 def test_receipt_digest_duplicate_keys_bounds_and_wrong_phase_refuse(self):
  refs,b=self.fixture();raw=json.dumps(b).encode()
  for data in [b'{"version":1,'+raw[1:],b' '*(8<<20)+raw]:
   with self.assertRaises(ValueError):self.invoke(refs,b,data=data)
  with self.assertRaises(ValueError):self.invoke(refs,b,receipt=dict(version=1,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(raw),sha256='0'*64))
  refs,b=self.fixture(True)
  with self.assertRaises(ValueError):self.invoke(refs,b,released=False)

if __name__=='__main__':unittest.main()
