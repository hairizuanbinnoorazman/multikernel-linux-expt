#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import unittest
from runtime_network_health_checkpoint import qualify

spec=importlib.util.spec_from_file_location('network_controls',Path(__file__).with_name('test-runtime-network-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)
BOOT=controls.BOOT;OWNER=controls.OWNER

class HealthCheckpointTests(unittest.TestCase):
    def fixture(self):
        s,e,n=controls.NetworkDoctorCheckpointTests().fixture()
        h=dict(version=1,source='host-network-backend-check',primary_boot_id=BOOT,status='checked',endpoints=[dict(endpoint_generation=e['generation'],descriptor_sha256=n['network']['endpoints'][0]['descriptor_sha256'],code='CHECK_PASSED')])
        return s,e,n,h
    def check(self,s,e,n,h,**kw):return qualify(json.dumps(h).encode(),BOOT,OWNER,s,n,e,**kw)
    def test_active_and_released_health_bind_to_independent_references(self):
        s,e,n,h=self.fixture();self.assertEqual(self.check(s,e,n,h),h)
        s['ownership']['sandboxes']=[];s['resources']['storage_bindings']=[]
        s['native_processes'][0]['status']='not-probed-released'
        s['resources']['storage']['exports'][0].update(state='RELEASED',offline_check='e2fsck-clean-sha256:'+'d'*64)
        s.pop('kernel_instances');n.pop('kernel_instances');n['ownership']=copy.deepcopy(s['ownership']);n['network_expectations']=[];n['network']['endpoints']=[]
        h.update(status='idle',endpoints=[])
        self.assertEqual(self.check(s,None,n,h,released=True),h)
        h['status']='checked'
        with self.assertRaises(ValueError):self.check(s,None,n,h,released=True)
    def test_stale_degraded_private_partial_and_forged_references_refuse(self):
        for mode in ['bool-version','boot','source','status','generation','digest','failure','duplicate','empty','private','private-endpoint','forged-both','unqualified-storage','wrong-owner']:
            with self.subTest(mode=mode):
                s,e,n,h=self.fixture();v=h['endpoints'][0]
                if mode=='bool-version':h['version']=True
                elif mode=='boot':h['primary_boot_id']='other'
                elif mode=='source':h['source']='complete-readiness'
                elif mode=='status':h['status']='degraded'
                elif mode=='generation':v['endpoint_generation']='f'*32
                elif mode=='digest':v['descriptor_sha256']='f'*64
                elif mode=='failure':v['code']='BACKEND_CHECK_FAILED'
                elif mode=='duplicate':h['endpoints'].append(copy.deepcopy(v))
                elif mode=='empty':h['endpoints']=[]
                elif mode=='private':h['private-token']='private'
                elif mode=='private-endpoint':v['netns']='/private/namespace'
                elif mode=='forged-both':v['descriptor_sha256']=n['network']['endpoints'][0]['descriptor_sha256']='f'*64
                elif mode=='unqualified-storage':s['native_processes'][0]['status']='unavailable'
                elif mode=='wrong-owner':e['sandbox_generation']='f'*32
                with self.assertRaises(ValueError):self.check(s,e,n,h)
    def test_ambiguous_oversized_and_nonbyte_input_refuse(self):
        s,e,n,h=self.fixture();data=json.dumps(h).encode()
        for raw in [b'{"version":1,'+data[1:],data+b' '*(64<<10),data.decode(),b'']:
            with self.assertRaises(ValueError):qualify(raw,BOOT,OWNER,s,n,e)

if __name__=='__main__':unittest.main()
