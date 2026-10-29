#!/usr/bin/env python3
import copy,importlib.util,json
from pathlib import Path
import unittest
from runtime_operational_doctor_checkpoint import qualify,UNOBSERVED
from runtime_component_build_checkpoint import expected_base
spec=importlib.util.spec_from_file_location('unified_controls',Path(__file__).with_name('test-runtime-unified-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)
BOOT=controls.BOOT;OWNER=controls.OWNER

class OperationalCheckpointTests(unittest.TestCase):
    def fixture(self,released=False):
        s,e,n,c,p,u=controls.UnifiedCheckpointTests().fixture(released)
        versions={'mkruntimed':'0.0.0-dev','mknetd':'0.0.0-dev'};revisions={'mkruntimed':s['revision'],'mknetd':s['revision']}
        components=expected_base(BOOT,s['revision']);components.update(status='consistent',boot_stable=True,compiled_revisions_match=True)
        for field,name in [('runtime','mkruntimed'),('network','mknetd')]:
            components[field]=dict(version=1,source='compiled-component-self-report',component=name,build_version=versions[name],build_revision=revisions[name],artifact_verified=False,kernel_verified=False)
        r=dict(version=7,scope='resource-network-pool-and-compiled-component-observations',status='consistent',revision=s['revision'],boot_id=BOOT,boot_stable=True,unified=u,components=components,findings=[],admission_guaranteed=False,artifact_verified=False,kernel_verified=False,unobserved=UNOBSERVED)
        return s,e,n,c,p,u,components,versions,revisions,r
    def qualify(self,items,data=None,released=False):
        s,e,n,c,p,u,b,versions,revisions,r=items
        return qualify(json.dumps(r).encode() if data is None else data,BOOT,OWNER,s,n,e,c,p,u,b,versions,revisions,released=released)
    def test_exact_live_and_released_nested_reports(self):
        for released in [False,True]:
            values=self.fixture(released);self.assertEqual(self.qualify(values,released=released),values[-1])
    def test_forged_partial_drift_private_numeric_and_health_claims_refuse(self):
        for mode in ['version','float','bool','scope','revision','boot','stability','private','findings','health','admission','artifact','kernel','partial','sequence','capacity','daemon-build','build-source','build-verified','build-match','native','network']:
            values=list(self.fixture());r=copy.deepcopy(values[-1]);values[-1]=r
            if mode=='version':r['version']=6
            elif mode=='float':r['version']=7.0
            elif mode=='bool':r['version']=True
            elif mode=='scope':r['scope']='readiness'
            elif mode=='revision':r['revision']='other'
            elif mode=='boot':r['boot_id']='other'
            elif mode=='stability':r['boot_stable']=1
            elif mode=='private':r['private-token']='synthetic-secret'
            elif mode=='findings':r['findings']=[dict(code='UNAVAILABLE')]
            elif mode=='health':r['unobserved']=[]
            elif mode=='admission':r['admission_guaranteed']=True
            elif mode=='artifact':r['artifact_verified']=True
            elif mode=='kernel':r['kernel_verified']=True
            elif mode=='partial':r.pop('components')
            elif mode=='sequence':r['unified']['pool']['ownership']['lifecycle']['sequence']+=1
            elif mode=='capacity':r['unified']['pool']['memory_candidate_upper_bound_bytes']+=1
            elif mode=='daemon-build':r['components']['runtime']['build_revision']='1234567'
            elif mode=='build-source':r['components']['runtime']['source']='attested'
            elif mode=='build-verified':r['components']['runtime']['artifact_verified']=True
            elif mode=='build-match':r['components']['compiled_revisions_match']=1
            elif mode=='native':r['unified']['combined']['native_processes'][0]['status']='unavailable'
            elif mode=='network':r['unified']['combined']['network']['endpoints']=[]
            with self.subTest(mode=mode),self.assertRaises(ValueError):self.qualify(values)
    def test_unqualified_reference_views_duplicates_and_bounds_refuse(self):
        values=self.fixture();raw=json.dumps(values[-1]).encode()
        for data in [b'{"version":7,'+raw[1:],b' '*(8<<20)+raw]:
            with self.assertRaises(ValueError):self.qualify(values,data)
        for index in [0,2,3,4,5,6]:
            values=list(self.fixture());values[index]=copy.deepcopy(values[index]);values[index]['status']='inconclusive'
            with self.subTest(reference=index),self.assertRaises(ValueError):self.qualify(values)

if __name__=='__main__':unittest.main()
