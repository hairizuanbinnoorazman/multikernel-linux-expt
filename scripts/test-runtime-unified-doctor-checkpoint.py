#!/usr/bin/env python3
import copy,importlib.util,json
from pathlib import Path
import unittest
from runtime_unified_doctor_checkpoint import qualify,UNOBSERVED

def load(name):
    spec=importlib.util.spec_from_file_location(name,Path(__file__).with_name('test-runtime-'+name+'-checkpoint.py'))
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module
combined_controls=load('combined-doctor');pool_controls=load('pool-doctor')
BOOT=combined_controls.controls.BOOT;OWNER=combined_controls.controls.OWNER

class UnifiedCheckpointTests(unittest.TestCase):
    def fixture(self,released=False):
        s,e,n,c=combined_controls.CombinedCheckpointTests().fixture()
        other,p=pool_controls.PoolCheckpointTests().fixture(released)
        if released:
            s=other;e=None
            n.update(ownership=copy.deepcopy(s['ownership']),network_expectations=[])
            n.pop('kernel_instances');n['network']['endpoints']=[]
            c=dict(s,version=5,scope=c['scope'],unobserved=c['unobserved'],network=n['network'],network_expectations=[])
        else:self.assertEqual(s,other)
        r=dict(version=6,scope='durable-resource-network-native-process-kernel-presence-and-pool-capacity',status='consistent',revision=s['revision'],boot_id=BOOT,boot_stable=True,combined=c,pool=p,findings=[],admission_guaranteed=False,unobserved=UNOBSERVED)
        return s,e,n,c,p,r
    def test_exact_live_and_released_operator_reports(self):
        for released in [False,True]:
            s,e,n,c,p,r=self.fixture(released)
            self.assertEqual(qualify(json.dumps(r).encode(),BOOT,OWNER,s,n,e,c,p,released),r)
    def test_forged_partial_numeric_alias_or_cross_component_drift_refuses(self):
        for mode in ['version','bool','float','scope','revision','boot','stability','missing','findings','private','health','admission','sequence','generation','capacity','native','network','counter-bool','pool-float']:
            s,e,n,c,p,r=self.fixture();r=copy.deepcopy(r)
            if mode=='version':r['version']=5
            elif mode=='bool':r['version']=True
            elif mode=='float':r['version']=6.0
            elif mode=='scope':r['scope']='readiness'
            elif mode=='revision':r['revision']='other'
            elif mode=='boot':r['boot_id']='other'
            elif mode=='stability':r['boot_stable']=1
            elif mode=='missing':r.pop('pool')
            elif mode=='findings':r['findings']=[dict(code='UNOBSERVED')]
            elif mode=='private':r['private-token']='synthetic-secret'
            elif mode=='health':r['unobserved']=[]
            elif mode=='admission':r['admission_guaranteed']=True
            elif mode=='sequence':r['pool']['ownership']['lifecycle']['sequence']+=1
            elif mode=='generation':r['pool']['ownership']['lifecycle']['sandboxes'][0]['generation']='f'*32
            elif mode=='capacity':r['pool']['memory_candidate_upper_bound_bytes']+=1
            elif mode=='native':r['combined']['native_processes'][0]['status']='unavailable'
            elif mode=='network':r['combined']['network']['endpoints']=[]
            elif mode=='counter-bool':r['combined']['network']['endpoints'][0]['counters']['errors']=False
            elif mode=='pool-float':r['pool']['memory_candidate_upper_bound_bytes']=float(r['pool']['memory_candidate_upper_bound_bytes'])
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER,s,n,e,c,p)
    def test_unqualified_inputs_duplicate_fields_and_bound_refuse(self):
        s,e,n,c,p,r=self.fixture();raw=json.dumps(r).encode()
        for data in [b'{"version":6,'+raw[1:],b' '*(4<<20)+raw]:
            with self.assertRaises(ValueError):qualify(data,BOOT,OWNER,s,n,e,c,p)
        for component in ['storage','network','combined','pool']:
            s,e,n,c,p,r=self.fixture();{'storage':s,'network':n,'combined':c,'pool':p}[component]['status']='inconclusive'
            with self.subTest(component=component),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER,s,n,e,c,p)

if __name__=='__main__':unittest.main()
