#!/usr/bin/env python3
import importlib.util
import json
from pathlib import Path
import unittest
from runtime_journal_status_checkpoint import qualify

spec=importlib.util.spec_from_file_location('storage_controls',Path(__file__).with_name('test-runtime-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)
BOOT=controls.BOOT;OWNER=controls.OWNER

class JournalStatusCheckpointTests(unittest.TestCase):
    def fixture(self):
        s=controls.DoctorCheckpointTests().report()
        r=dict(version=1,source='in-process-lifecycle-journal',status='idle',sequence=s['ownership']['sequence'],pending_completions=0,journal_failed=False)
        return s,r
    def test_idle_journal_matches_active_and_released_lifecycle_sequence(self):
        s,r=self.fixture();self.assertEqual(qualify(json.dumps(r).encode(),BOOT,OWNER,s),r)
        s['ownership']['sandboxes']=[];s['resources']['storage_bindings']=[]
        s['native_processes'][0]['status']='not-probed-released'
        s['resources']['storage']['exports'][0].update(state='RELEASED',offline_check='e2fsck-clean-sha256:'+'d'*64)
        s.pop('kernel_instances');self.assertEqual(qualify(json.dumps(r).encode(),BOOT,OWNER,s,released=True),r)
    def test_stale_failed_pending_private_missing_and_aliases_refuse(self):
        for mode in ['bool-version','source','status','stale-sequence','bool-sequence','negative-sequence','huge-sequence','pending','bool-pending','failed','integer-failed','private','missing','null','unqualified-reference','wrong-boot']:
            with self.subTest(mode=mode):
                s,r=self.fixture();boot=BOOT
                if mode=='bool-version':r['version']=True
                elif mode=='source':r['source']='filesystem-writable'
                elif mode=='status':r['status']='ready'
                elif mode=='stale-sequence':r['sequence']+=1
                elif mode=='bool-sequence':r['sequence']=True
                elif mode=='negative-sequence':r['sequence']=-1
                elif mode=='huge-sequence':r['sequence']=1<<64
                elif mode=='pending':r['status']='pending';r['pending_completions']=1
                elif mode=='bool-pending':r['pending_completions']=False
                elif mode=='failed':r['status']='failed';r['journal_failed']=True
                elif mode=='integer-failed':r['journal_failed']=0
                elif mode=='private':r['private-token']='credential'
                elif mode=='missing':r.pop('journal_failed')
                elif mode=='null':r['pending_completions']=None
                elif mode=='unqualified-reference':s['native_processes'][0]['status']='unavailable'
                elif mode=='wrong-boot':boot='other'
                with self.assertRaises(ValueError):qualify(json.dumps(r).encode(),boot,OWNER,s)
    def test_ambiguous_oversized_and_nonbyte_reports_refuse(self):
        s,r=self.fixture();raw=json.dumps(r).encode()
        for data in [b'{"version":1,'+raw[1:],raw+b' '*4096,raw.decode(),b'']:
            with self.assertRaises(ValueError):qualify(data,BOOT,OWNER,s)

if __name__=='__main__':unittest.main()
