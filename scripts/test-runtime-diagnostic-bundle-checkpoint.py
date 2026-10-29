#!/usr/bin/env python3
import copy,hashlib,importlib.util,json,unittest
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import qualify_idle,qualify_live,SCOPE,EXCLUDED,UNOBSERVED
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
