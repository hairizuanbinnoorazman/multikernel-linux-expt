#!/usr/bin/env python3
import copy,importlib.util,json
from pathlib import Path
import unittest
from runtime_combined_doctor_checkpoint import qualify,UNOBSERVED
spec=importlib.util.spec_from_file_location('network_controls',Path(__file__).with_name('test-runtime-network-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)

class CombinedCheckpointTests(unittest.TestCase):
    def fixture(self):
        storage,endpoint,network=controls.NetworkDoctorCheckpointTests().fixture()
        combined=dict(storage,version=5,scope='durable-resource-network-ownership-native-process-identity-and-kernel-presence',unobserved=UNOBSERVED,network=network['network'],network_expectations=network['network_expectations'])
        return storage,endpoint,network,combined
    def test_exact_live_operator_report_is_accepted(self):
        s,e,n,r=self.fixture();self.assertEqual(qualify(json.dumps(r).encode(),controls.BOOT,controls.OWNER,s,n,e),r)
    def test_partial_forged_or_changed_component_cannot_become_combined_success(self):
        for mode in ['legacy','storage','network','native','sequence','health','private','scope','findings','bool','missing','numeric-alias']:
            s,e,n,r=self.fixture();r=copy.deepcopy(r)
            if mode=='legacy':r['version']=4
            elif mode=='storage':r['resources']['storage']['exports'][0]['export_generation']='f'*32
            elif mode=='network':r['network']['endpoints'][0]['endpoint_generation']='f'*32
            elif mode=='native':r['native_processes'][0]['status']='unavailable'
            elif mode=='sequence':r['ownership']['sequence']+=1
            elif mode=='health':r['unobserved']=[]
            elif mode=='private':r['private-token']='synthetic-secret'
            elif mode=='scope':r['scope']='complete-health'
            elif mode=='findings':r['findings']=[dict(code='NETWORK_ENDPOINT_MISSING')]
            elif mode=='bool':r['version']=True
            elif mode=='missing':r.pop('network')
            elif mode=='numeric-alias':r['network']['endpoints'][0]['counters']['rx_drops']=False
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),controls.BOOT,controls.OWNER,s,n,e)
    def test_unqualified_inputs_ambiguous_json_and_bound_refuse(self):
        s,e,n,r=self.fixture();raw=json.dumps(r).encode()
        for data in [b'{"version":5,'+raw[1:],b' '*(2<<20)+raw]:
            with self.assertRaises(ValueError):qualify(data,controls.BOOT,controls.OWNER,s,n,e)
        n['network_expectations'][0]['expectation']='unknown'
        with self.assertRaises(ValueError):qualify(raw,controls.BOOT,controls.OWNER,s,n,e)

if __name__=='__main__':unittest.main()
