#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import unittest
from runtime_network_doctor_checkpoint import qualify, project_endpoint, UNOBSERVED

spec=importlib.util.spec_from_file_location('storage_controls',Path(__file__).with_name('test-runtime-doctor-checkpoint.py'))
storage_controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(storage_controls)
BOOT=storage_controls.BOOT;OWNER=storage_controls.OWNER
FIXTURE=Path(__file__).resolve().parent.parent/'runtime/internal/network/testdata/endpoint-descriptor.json'

class NetworkDoctorCheckpointTests(unittest.TestCase):
    def fixture(self):
        storage=storage_controls.DoctorCheckpointTests().report()
        endpoint=json.loads(FIXTURE.read_text())['endpoint']
        report=dict(version=4,scope='durable-lifecycle-network-ownership-and-kernel-presence',status='consistent',
                    boot_id=BOOT,boot_stable=True,revision=storage['revision'],ownership=copy.deepcopy(storage['ownership']),
                    kernel_instances=['box-a'],kernel_generation_verified=False,findings=[],unobserved=UNOBSERVED,
                    network_expectations=[dict(sandbox_id='box-a',generation='a'*32,expectation='required')],
                    network=dict(version=1,source='durable-network',primary_boot_id=BOOT,endpoints=[project_endpoint(endpoint)]))
        return storage,endpoint,report
    def test_independent_go_descriptor_vector_and_redaction(self):
        fixture=json.loads(FIXTURE.read_text())
        projected=project_endpoint(fixture['endpoint'])
        self.assertEqual(projected['descriptor_sha256'],fixture['descriptor_sha256'])
        self.assertNotIn('synthetic-private',json.dumps(projected))
        changed=copy.deepcopy(fixture['endpoint']);changed['dns']['search'][0]+='x'
        self.assertNotEqual(project_endpoint(changed)['descriptor_sha256'],projected['descriptor_sha256'])
    def test_held_and_exact_release_correlate_with_storage(self):
        s,e,r=self.fixture();self.assertEqual(qualify(json.dumps(r).encode(),BOOT,OWNER,s,e),r)
        for owners in [[],[dict(OWNER,state='ABSENT')]]:
            s,e,r=self.fixture();s['ownership']['sandboxes']=owners
            s['resources']['storage_bindings']=[] if not owners else [dict(s['resources']['storage_descriptors'][0],required=True)]
            s['native_processes'][0]['status']='not-probed-released'
            lease=s['resources']['storage']['exports'][0];lease.update(state='RELEASED',offline_check='e2fsck-clean-sha256:'+'d'*64)
            s.pop('kernel_instances')
            r.update(ownership=copy.deepcopy(s['ownership']),network_expectations=[] if not owners else r['network_expectations'])
            r.pop('kernel_instances');r['network']['endpoints']=[]
            self.assertEqual(qualify(json.dumps(r).encode(),BOOT,OWNER,s,None,released=True),r)
            r['network']['endpoints']=[project_endpoint(e)]
            with self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER,s,None,released=True)
    def test_stale_forged_partial_and_unhealthy_reports_refuse(self):
        for mode in ['version','scope','status','boot','revision','sequence','kernel','claims','findings','unobserved',
                     'unknown-intent','wrong-intent-generation','wrong-endpoint-generation','wrong-owner-generation',
                     'wrong-descriptor','wrong-counters','wrong-network-boot','missing','extra','degraded','private','bool-version','bool-network-version','bool-counter']:
            with self.subTest(mode=mode):
                s,e,r=self.fixture();n=r['network'];p=n['endpoints'][0]
                if mode=='version':r['version']=3
                elif mode=='scope':r['scope']='complete-health'
                elif mode=='status':r['status']='inconclusive'
                elif mode=='boot':r['boot_id']='other'
                elif mode=='revision':r['revision']='7654321'
                elif mode=='sequence':r['ownership']['sequence']+=1
                elif mode=='kernel':r['kernel_instances']=[]
                elif mode=='claims':r['kernel_generation_verified']=True
                elif mode=='findings':r['findings']=[dict(code='NETWORK_ENDPOINT_MISSING')]
                elif mode=='unobserved':r['unobserved']=[]
                elif mode=='unknown-intent':r['network_expectations'][0]['expectation']='unknown'
                elif mode=='wrong-intent-generation':r['network_expectations'][0]['generation']='f'*32
                elif mode=='wrong-endpoint-generation':p['endpoint_generation']='f'*32
                elif mode=='wrong-owner-generation':p['sandbox_generation']='f'*32
                elif mode=='wrong-descriptor':p['descriptor_sha256']='f'*64
                elif mode=='wrong-counters':p['counters']['errors']+=1
                elif mode=='wrong-network-boot':n['primary_boot_id']='other'
                elif mode=='missing':n['endpoints']=[]
                elif mode=='extra':n['endpoints'].append(copy.deepcopy(p))
                elif mode=='degraded':p['state']='DEGRADED'
                elif mode=='private':r['private-token']='synthetic-private'
                elif mode=='bool-version':r['version']=True
                elif mode=='bool-network-version':n['version']=True
                elif mode=='bool-counter':p['counters']['rx_drops']=False
                with self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER,s,e)
    def test_ambiguous_oversized_and_unqualified_storage_refuse(self):
        s,e,r=self.fixture();raw=json.dumps(r).encode()
        for data in [b'{"version":4,'+raw[1:],b' '*(1<<20)+raw]:
            with self.assertRaises(ValueError):qualify(data,BOOT,OWNER,s,e)
        s['native_processes'][0]['status']='unavailable'
        with self.assertRaises(ValueError):qualify(raw,BOOT,OWNER,s,e)
    def test_untrusted_private_endpoint_and_noninteger_counters_refuse(self):
        for mode in ['generation','boot','managed','state','counter','oversized']:
            s,e,r=self.fixture()
            if mode=='generation':e['sandbox_generation']='f'*32
            elif mode=='boot':e['primary_boot_id']='other'
            elif mode=='managed':e['managed_namespace']=False
            elif mode=='state':e['state']='DISCONNECTED'
            elif mode=='counter':e['errors']=True
            elif mode=='oversized':e['dns']['domain']='x'*4097
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER,s,e)

if __name__=='__main__':unittest.main()
