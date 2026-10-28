#!/usr/bin/env python3
import copy,importlib.util,json
from pathlib import Path
import unittest
from runtime_pool_doctor_checkpoint import qualify,UNOBSERVED
spec=importlib.util.spec_from_file_location('storage_controls',Path(__file__).with_name('test-runtime-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)

class PoolCheckpointTests(unittest.TestCase):
    def fixture(self,released=False):
        s=controls.DoctorCheckpointTests().report()
        if released:
            s['ownership']['sandboxes']=[];s['resources']['storage_bindings']=[];s['native_processes'][0]['status']='not-probed-released'
            s['resources']['storage']['exports'][0].update(state='RELEASED',offline_check='e2fsck-clean-sha256:'+'d'*64);s.pop('kernel_instances')
        claimed=0 if released else 3<<30;cpus=[8,9,12,13];available=cpus if released else [12,13]
        r=dict(version=1,scope='configured-durable-and-kernel-pool-capacity',status='consistent',revision=s['revision'],boot_id=controls.BOOT,boot_stable=True,
               ownership=dict(version=1,source='configured-pool-and-durable-lifecycle',lifecycle=copy.deepcopy(s['ownership']),configured_cpus=cpus,configured_memory_bytes=8<<30,memory_reserve_bytes=1<<30,initialized=True),
               kernel=dict(cpus=dict(members=cpus,available=available),memory=dict(chunks=1,total_bytes=8<<30,allocated_bytes=claimed,available_bytes=(8<<30)-claimed,largest_free_extent_bytes=(8<<30)-claimed)),
               findings=[],cpu_candidates=available,remaining_configured_memory_bytes=(7<<30)-claimed,memory_candidate_upper_bound_bytes=(7<<30)-claimed,admission_guaranteed=False,unobserved=UNOBSERVED)
        return s,r
    def test_live_and_released_capacity_match_same_lifecycle(self):
        for released in [False,True]:
            s,r=self.fixture(released);self.assertEqual(qualify(json.dumps(r).encode(),controls.BOOT,controls.OWNER,s,released),r)
        s,r=self.fixture();r['kernel']['memory']['largest_free_extent_bytes']=2<<30;r['memory_candidate_upper_bound_bytes']=2<<30
        self.assertEqual(qualify(json.dumps(r).encode(),controls.BOOT,controls.OWNER,s),r)
    def test_wrong_capacity_lifecycle_or_health_claims_refuse(self):
        for mode in ['version','bool','private','scope','readiness','generation','sequence','free-claimed','foreign-cpu','memory','candidate','reserve-bool','admission','missing','revision','cpu-type']:
            s,r=self.fixture();r=copy.deepcopy(r)
            if mode=='version':r['version']=2
            elif mode=='bool':r['kernel']['memory']['chunks']=True
            elif mode=='private':r['private-token']='synthetic-secret'
            elif mode=='scope':r['scope']='complete-health'
            elif mode=='readiness':r['unobserved']=[]
            elif mode=='generation':r['ownership']['lifecycle']['sandboxes'][0]['generation']='f'*32
            elif mode=='sequence':r['ownership']['lifecycle']['sequence']+=1
            elif mode=='free-claimed':r['kernel']['cpus']['available']=[8,12,13]
            elif mode=='foreign-cpu':r['kernel']['cpus']['members']=[8,9,12,13,14]
            elif mode=='memory':r['kernel']['memory']['allocated_bytes']=0
            elif mode=='candidate':r['memory_candidate_upper_bound_bytes']+=1
            elif mode=='reserve-bool':r['ownership']['memory_reserve_bytes']=True
            elif mode=='admission':r['admission_guaranteed']=True
            elif mode=='missing':r.pop('kernel')
            elif mode=='revision':r['revision']='different'
            elif mode=='cpu-type':r['kernel']['cpus']=None
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),controls.BOOT,controls.OWNER,s)
    def test_duplicate_fields_and_oversized_samples_refuse(self):
        s,r=self.fixture();raw=json.dumps(r).encode()
        for data in [b'{"version":1,'+raw[1:],b' '*(1<<20)+raw]:
            with self.assertRaises(ValueError):qualify(data,controls.BOOT,controls.OWNER,s)

if __name__=='__main__':unittest.main()
