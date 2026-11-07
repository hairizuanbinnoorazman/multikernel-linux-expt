#!/usr/bin/env python3
import copy,importlib.util,json
from pathlib import Path
import unittest
from runtime_trial_reuse import qualify
from runtime_recovery_checkpoint import recovery_view
spec=importlib.util.spec_from_file_location('bundle_controls',Path(__file__).with_name('test-runtime-diagnostic-bundle-checkpoint.py'))
c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
BOOT=c.BOOT;PARENT='8'*32;CHILD='9'*32;REV='0acea0b';ARCHIVE='a'*64

class ReuseTests(unittest.TestCase):
    def fixture(self):
        receipt,frames=c.LiveBundleTests().lifecycle()
        from runtime_diagnostic_bundle_checkpoint import qualify_lifecycle_timings
        timing=qualify_lifecycle_timings(json.dumps(receipt).encode(),frames,BOOT,CHILD)
        primary=dict(file_sha256={'private-file':'a'*64},service_main_pids={'private-service':'123'})
        return dict(boot_id=BOOT,harness_source_commit=REV,source_archive_sha256=ARCHIVE,parent_scope=dict(InvocationID=PARENT),pressure_invocation=CHILD,pressure_scope=dict(InvocationID=CHILD,MainPID='0',SubState='exited',Result='success',ExecMainStatus='0'),candidate_quiescent=True,cold_restore_required=True,original_restored=False,primary_unchanged=True,unexpected_taint_bits=0,candidate_sha256='74e813765d8551d086a603e047337b27ad0cce28dd4cc5de7e983a6e10a1f69f',primary_before=primary,primary_after=copy.deepcopy(primary),diagnostic_checkpoint_timings=timing)
    def test_exact_archived_qualification_does_not_claim_current_admission(self):
        p=self.fixture();self.assertEqual(qualify(json.dumps(p).encode(),BOOT,REV,ARCHIVE,PARENT,CHILD),p)
        self.assertNotIn('current_reuse_admitted',p)
    def test_cross_boot_invocation_source_failed_and_numeric_masks_refuse(self):
        for mode in ['boot','parent','child','revision','archive','running','failed','restored','not-quiescent','taint-float','drift','timing']:
            p=self.fixture()
            if mode=='boot':p['boot_id']='6349e148-e008-4fa9-a36c-83549cfe2501'
            elif mode=='parent':p['parent_scope']['InvocationID']='7'*32
            elif mode=='child':p['pressure_scope']['InvocationID']='7'*32
            elif mode=='revision':p['harness_source_commit']='1234567'
            elif mode=='archive':p['source_archive_sha256']='f'*64
            elif mode=='running':p['pressure_scope']['MainPID']='5'
            elif mode=='failed':p['pressure_scope']['Result']='exit-code'
            elif mode=='restored':p['original_restored']=True
            elif mode=='not-quiescent':p['candidate_quiescent']=1
            elif mode=='taint-float':p['unexpected_taint_bits']=0.0
            elif mode=='drift':p['primary_after']['service_main_pids']['private-service']='456'
            elif mode=='timing':p['diagnostic_checkpoint_timings']['timings']['while-full']['elapsed_ns']+=1
            with self.subTest(mode=mode),self.assertRaisesRegex(ValueError,'^prior transport trial is not qualified for same-boot reuse$'):qualify(json.dumps(p).encode(),BOOT,REV,ARCHIVE,PARENT,CHILD)
    def test_ambiguous_missing_oversized_and_private_input_is_not_reflected(self):
        data=json.dumps(self.fixture()).encode()
        for bad in [b'{}',b'{"boot_id":"private-canary",'+data[1:],b' '*(1<<20)+data]:
            with self.assertRaises(ValueError) as caught:qualify(bad,BOOT,REV,ARCHIVE,PARENT,CHILD)
            self.assertNotIn('private-canary',str(caught.exception))
    def test_named_recovery_predecessor_binds_scope_and_boolean_types_without_current_authority(self):
        for scenario in ['runtime-restart','runtime-crash','runtime-crash-during-build']:
            p=self.fixture();p.pop('diagnostic_checkpoint_timings');p['scenario']=scenario
            p['runtime_recovery_checkpoint']=recovery_view(BOOT,CHILD,scenario)
            self.assertEqual(qualify(json.dumps(p).encode(),BOOT,REV,ARCHIVE,PARENT,CHILD,scenario),p)
            with self.assertRaises(ValueError):qualify(json.dumps(p).encode(),BOOT,REV,ARCHIVE,PARENT,CHILD)
            for mode in ['scope','invocation','numeric-bool','cleanup','extra']:
                trial=copy.deepcopy(p);view=trial['runtime_recovery_checkpoint']
                if mode=='scope':view['scope']='receipt-bound-private-runtime-other-and-child-build-continuity'
                elif mode=='invocation':view['invocation_id']='0'*32
                elif mode=='numeric-bool':view['child_continuity_observed']=1
                elif mode=='cleanup':view['cleanup_qualified']=True
                elif mode=='extra':view['unexpected']='private-canary'
                with self.subTest(scenario=scenario,mode=mode),self.assertRaises(ValueError):qualify(json.dumps(trial).encode(),BOOT,REV,ARCHIVE,PARENT,CHILD,scenario)

if __name__=='__main__':unittest.main()
