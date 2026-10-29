#!/usr/bin/env python3
import copy,json,unittest
from runtime_component_build_checkpoint import qualify,qualify_unsupported,expected_base

BOOT='11111111-1111-4111-8111-111111111111';REV='abcdef0'
VERSIONS={'mkruntimed':'0.0.0-dev','mknetd':'0.0.0-dev'};REVISIONS={'mkruntimed':REV,'mknetd':REV}

class ComponentCheckpointTests(unittest.TestCase):
    def report(self,revisions=None):
        revisions=revisions or REVISIONS;r=expected_base(BOOT,REV)
        r.update(status='consistent',boot_stable=True,compiled_revisions_match=revisions['mkruntimed']==revisions['mknetd'])
        for field,component in [('runtime','mkruntimed'),('network','mknetd')]:
            r[field]=dict(version=1,source='compiled-component-self-report',component=component,build_version=VERSIONS[component],build_revision=revisions[component],artifact_verified=False,kernel_verified=False)
        return r
    def test_exact_known_different_and_unsupported_labels(self):
        for revisions in [REVISIONS,dict(REVISIONS,mknetd='1234567')]:
            r=self.report(revisions);self.assertEqual(qualify(json.dumps(r).encode(),BOOT,REV,VERSIONS,revisions),r)
        for component in ['RUNTIME','NETWORK']:
            r=expected_base(BOOT,REV);r['findings']=[dict(code=component+'_BUILD_UNSUPPORTED')]
            self.assertEqual(qualify_unsupported(json.dumps(r).encode(),BOOT,REV,component),r)
    def test_partial_private_forged_health_numeric_alias_and_revision_refuse(self):
        for mode in ['version','bool','private','scope','health','artifact','kernel','boot','revision','partial','runtime-label','network-label','match','version-label']:
            r=self.report()
            if mode=='version':r['version']=2
            elif mode=='bool':r['version']=True
            elif mode=='private':r['private-token']='synthetic-secret'
            elif mode=='scope':r['scope']='health'
            elif mode=='health':r['unobserved']=[]
            elif mode=='artifact':r['runtime']['artifact_verified']=True
            elif mode=='kernel':r['kernel_verified']=True
            elif mode=='boot':r['boot_id']='other'
            elif mode=='revision':r['revision']='1234567'
            elif mode=='partial':r.pop('network')
            elif mode=='runtime-label':r['runtime']['build_revision']='1234567'
            elif mode=='network-label':r['network']['build_revision']='unknown'
            elif mode=='match':r['compiled_revisions_match']=1
            elif mode=='version-label':r['runtime']['build_version']='1.2.3'
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,REV,VERSIONS,REVISIONS)
    def test_bounds_duplicates_unknown_and_unsupported_partial_refuse(self):
        r=self.report();raw=json.dumps(r).encode()
        for data in [b'{"version":1,'+raw[1:],b' '*(65537)]:
            with self.assertRaises(ValueError):qualify(data,BOOT,REV,VERSIONS,REVISIONS)
        with self.assertRaises(ValueError):qualify(raw,BOOT,REV,VERSIONS,dict(REVISIONS,mknetd='unknown'))
        r=expected_base(BOOT,REV);r['findings']=[dict(code='RUNTIME_BUILD_UNSUPPORTED')];r['runtime']={}
        with self.assertRaises(ValueError):qualify_unsupported(json.dumps(r).encode(),BOOT,REV,'RUNTIME')

if __name__=='__main__':unittest.main()
