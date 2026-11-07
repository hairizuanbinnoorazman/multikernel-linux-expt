#!/usr/bin/env python3
import copy,hashlib,json,tempfile,unittest
from pathlib import Path
from runtime_recovery_checkpoint import qualify,inspect_fixture_recovery
from runtime_primary_docker import fingerprint
BOOT='5ad22222-bf6f-4419-9693-9702cc2ee13b';CHILD='11111111-1111-4111-8111-111111111111';INV='9'*32
sha=lambda value:hashlib.sha256(value).hexdigest()
raw=lambda value:json.dumps(value).encode()

class RecoveryTests(unittest.TestCase):
    def fixture(self):
        owner=dict(sandbox_id='mk-fixture',generation='a'*32,state='RUNNING',cpus=[8,9,10,11],memory_bytes=3<<30,child_cid=40,agent_port=7200)
        snapshot=dict(child_boot_id=CHILD,engine_id='child-engine',ownership=[owner],saved_image_sha256='a'*64)
        exit=dict(invocation_id=INV,boot_id=BOOT,runtime_pid=101,runtime_exit_signal='SIGTERM',runtime_exit_status=0,observed_monotonic_ns=100,cleanup_qualified=False)
        r=dict(schema_version=2,invocation_id=INV,boot_id=BOOT,runtime_pid_before=101,runtime_pid_after=102,runtime_restarts=1,runtime_exit_signal='SIGTERM',runtime_exit_status=0,before=snapshot,after=copy.deepcopy(snapshot),after_build=dict(copy.deepcopy(snapshot),saved_image_sha256='b'*64),completed_first_build=copy.deepcopy(snapshot),post_recovery_build_tested=True,child_continuity_pass=True,cleanup_qualified=False,runtime_exit_checkpoint_sha256=sha(raw(exit)),crash_during_build=False,original_image_id='sha256:'+'c'*64,post_recovery_image_id='sha256:'+'d'*64,original_image_sha256_after_rebuild='a'*64,primary_docker_before_recovery_sha256='e'*64,primary_docker_after_recovery_sha256='e'*64,primary_docker_after_rebuild_sha256='e'*64,post_recovery_stdout_sha256='f'*64,post_recovery_stderr_sha256='f'*64)
        receipt=dict(boot_id=BOOT,invocation_id=INV,runtime_restart_tested=True,runtime_crash_tested=False,runtime_crash_during_build_tested=False,storage_pressure_tested=False,production_profile_child_build_pass=True,recovery_checkpoint_sha256=sha(raw(r)))
        return r,exit,receipt
    def bind(self,r,exit,receipt,scenario='runtime-restart'):
        r['runtime_exit_checkpoint_sha256']=sha(raw(exit));receipt['recovery_checkpoint_sha256']=sha(raw(r));return qualify(raw(r),raw(exit),receipt,BOOT,INV,scenario)
    def test_crash_requires_explicit_caller_selection_and_exact_signal_status_receipt(self):
        r,e,receipt=self.fixture()
        for v in [r,e]:v.update(runtime_exit_signal='SIGKILL',runtime_exit_status=-9)
        receipt['runtime_crash_tested']=True
        self.assertEqual(self.bind(r,e,receipt,'runtime-crash'),r)
        with self.assertRaises(ValueError):self.bind(r,e,receipt)
        for mode in ['signal','status','receipt','active-run','drift']:
            trial,exit,life=copy.deepcopy((r,e,receipt))
            if mode=='signal':trial['runtime_exit_signal']='SIGTERM'
            elif mode=='status':exit['runtime_exit_status']=0
            elif mode=='receipt':life['runtime_crash_tested']=False
            elif mode=='active-run':trial['crash_during_build']=True;life['runtime_crash_during_build_tested']=True
            elif mode=='drift':trial['after']['engine_id']='replacement-child-engine'
            with self.subTest(mode=mode),self.assertRaises(ValueError):self.bind(trial,exit,life,'runtime-crash')
        for bad in [None,True,'crash-during-build','--crash-runtime']:
            with self.assertRaises(ValueError):self.bind(r,e,receipt,bad)
    def test_exact_same_boot_recovery_preserves_continuity_without_cleanup_authority(self):
        r,e,receipt=self.fixture();self.assertEqual(self.bind(r,e,receipt),r);self.assertIs(r['cleanup_qualified'],False)
    def test_wrong_epoch_signal_pid_or_child_drift_refuses_recomputed_receipts(self):
        for mode in ['boot','invocation','same-pid','pid-bool','restart-float','signal','exit-status','child-boot','engine','owner','archive','cleanup','extra','legacy']:
            r,e,receipt=copy.deepcopy(self.fixture())
            if mode=='boot':r['boot_id']=CHILD
            elif mode=='invocation':r['invocation_id']='8'*32
            elif mode=='same-pid':r['runtime_pid_after']=r['runtime_pid_before']
            elif mode=='pid-bool':r['runtime_pid_before']=True
            elif mode=='restart-float':r['runtime_restarts']=1.0
            elif mode=='signal':r['runtime_exit_signal']='SIGKILL'
            elif mode=='exit-status':e['runtime_exit_status']=1
            elif mode=='child-boot':r['after']['child_boot_id']='22222222-2222-4222-8222-222222222222'
            elif mode=='engine':r['after']['engine_id']='other-engine'
            elif mode=='owner':r['after_build']['ownership'][0]['generation']='b'*32
            elif mode=='archive':r['original_image_sha256_after_rebuild']='b'*64
            elif mode=='cleanup':r['cleanup_qualified']=True
            elif mode=='extra':r['private']='synthetic-private-canary'
            elif mode=='legacy':r['schema_version']=1
            with self.subTest(mode=mode),self.assertRaises(ValueError):self.bind(r,e,receipt)
    def test_duplicate_digest_and_oversize_refusal(self):
        r,e,receipt=self.fixture()
        for data in [b'{"schema_version":2,'+raw(r)[1:],b' '*(1<<20)+raw(r)]:
            with self.assertRaises(ValueError):qualify(data,raw(e),receipt,BOOT,INV)
        receipt['recovery_checkpoint_sha256']='0'*64
        with self.assertRaises(ValueError):qualify(raw(r),raw(e),receipt,BOOT,INV)
    def retained(self,work,forge=None,scenario='runtime-restart'):
        private=work/'integration';private.mkdir(mode=0o700);r,e,receipt=self.fixture()
        if scenario=='runtime-crash':
            for v in [r,e]:v.update(runtime_exit_signal='SIGKILL',runtime_exit_status=-9)
            receipt['runtime_crash_tested']=True
        def write(parent,name,data):
            path=parent/name;path.write_bytes(data);path.chmod(0o600);return sha(data)
        baseline=dict(schema_version=2,boot_id=BOOT,engine_id_sha256=fingerprint('primary-engine'),inventories={name:dict(count=1,sha256='e'*64) for name in ['images','containers','volumes','networks']})
        write(private,'primary-docker-before.json',raw(baseline))
        for phase,key,image in [('before','primary_docker_before_recovery_sha256',r['original_image_id']),('after','primary_docker_after_recovery_sha256',r['original_image_id']),('after-rebuild','primary_docker_after_rebuild_sha256',r['post_recovery_image_id'])]:
            sample=dict(baseline,checked_absent_image=image,child_image_absent=not(forge=='image' and phase=='after'))
            r[key]=write(private,'primary-docker-recovery-'+phase+'.json',raw(sample))
        markers=['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']
        if forge=='build':markers.remove('G8_REPEAT_BUILD_PASS')
        out=('\n'.join(markers)+'\nG8_BUILD_IMAGE_ID='+r['post_recovery_image_id']+'\n').encode()
        r['post_recovery_stdout_sha256']=write(private,'post-recovery-workload.stdout',out)
        r['post_recovery_stderr_sha256']=write(private,'post-recovery-workload.stderr',b'CACHED\nexit code: 37\n')
        if forge=='engine':
            for key in ['before','after','after_build','completed_first_build']:r[key]['engine_id']='primary-engine'
        r['runtime_exit_checkpoint_sha256']=write(work,'runtime-exit-checkpoint.json',raw(e));receipt['recovery_checkpoint_sha256']=write(work,'recovery-checkpoint.json',raw(r));write(work,'integration-receipt.json',raw(receipt))
    def test_private_lifecycle_binds_build_streams_and_exact_primary_image_absence(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.retained(work);r=inspect_fixture_recovery(work,BOOT,INV);self.assertTrue(r['post_recovery_build_observed']);self.assertIs(r['cleanup_qualified'],False)
    def test_private_crash_lifecycle_requalifies_only_under_caller_selected_scope(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.retained(work,scenario='runtime-crash')
            r=inspect_fixture_recovery(work,BOOT,INV,'runtime-crash')
            self.assertEqual(r['scope'],'receipt-bound-private-runtime-crash-and-child-build-continuity')
            self.assertIs(r['cleanup_qualified'],False)
            with self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV)
    def test_matching_recomputed_hashes_cannot_mask_primary_engine_image_or_missing_build(self):
        for mode in ['image','engine','build']:
            with tempfile.TemporaryDirectory() as d:
                work=Path(d);self.retained(work,mode)
                with self.subTest(mode=mode),self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV)

    def retained_active(self,work,forge=None):
        self.retained(work,scenario='runtime-crash');private=work/'integration'
        r=json.loads((work/'recovery-checkpoint.json').read_bytes());e=json.loads((work/'runtime-exit-checkpoint.json').read_bytes());life=json.loads((work/'integration-receipt.json').read_bytes())
        def write(parent,name,value):
            data=value if type(value) is bytes else raw(value);(parent/name).write_bytes(data);return sha(data)
        r.update(schema_version=3,crash_during_build=True);life['runtime_crash_during_build_tested']=True
        for key in ['before','after']:r[key].pop('saved_image_sha256')
        for phase in ['before','after']:
            name='primary-docker-recovery-'+phase+'.json';sample=json.loads((private/name).read_bytes());sample.update(checked_absent_image=None,child_image_absent=False)
            r['primary_docker_'+phase+'_recovery_sha256']=write(private,name,sample)
        sample=json.loads((private/'primary-docker-recovery-after-rebuild.json').read_bytes());sample.update(checked_absent_image=r['original_image_id'],child_image_absent=True)
        r['primary_docker_completed_first_sha256']=write(private,'primary-docker-recovery-completed-first.json',sample)
        out=(private/'post-recovery-workload.stdout').read_bytes().replace(r['post_recovery_image_id'].encode(),r['original_image_id'].encode())
        prefix=b'#9 0.01 G8_OVERLAP_RUN_BEGIN\n';err=prefix+b'#9 45.01 G8_OVERLAP_RUN_END\nCACHED\nexit code: 37\n'
        if forge=='missing-completion':err=prefix+b'CACHED\nexit code: 37\n'
        if forge=='original-image':out=out.replace(r['original_image_id'].encode(),('sha256:'+'0'*64).encode())
        if forge=='missing-first-build':out=out.replace(b'G8_REPEAT_BUILD_PASS\n',b'')
        workload=dict(boot_id=BOOT,invocation_id=INV,workload_stdout_sha256=write(private,'workload.stdout',out),workload_stderr_sha256=write(private,'workload.stderr',err))
        life['workload_checkpoint_sha256']=write(work,'workload-checkpoint.json',workload)
        e['observed_monotonic_ns']=150;r['runtime_exit_checkpoint_sha256']=write(work,'runtime-exit-checkpoint.json',e)
        frames={}
        for phase,t,pid in [('before-kill',100,r['runtime_pid_before']),('after-kill',200,r['runtime_pid_before']),('after-adoption',300,r['runtime_pid_after'])]:
            snapshot=prefix
            if phase=='after-adoption':
                if forge=='early-end':snapshot+=b'#9 45.01 G8_OVERLAP_RUN_END\n'
                if forge=='forged-prefix':snapshot+=b'synthetic-forged-line\n'
                if forge=='pid':pid=r['runtime_pid_before']
                if forge=='deadline':t=46_000_000_100
                if forge=='clock':t=175
            name='active-run-'+phase+'.stderr'
            frames[phase]=dict(observed_monotonic_ns=t,runtime_pid=pid,markers=['BEGIN'],stderr_snapshot=name,stderr_sha256=write(private,name,snapshot))
        if forge=='missing-stage':frames.pop('after-kill')
        if forge=='numeric-bool':frames['after-adoption']['runtime_pid']=True
        active=dict(version=1,boot_id=BOOT,invocation_id=INV,stages=frames,cleanup_qualified=False)
        if forge=='active-boot':active['boot_id']=CHILD
        if forge=='active-cleanup':active['cleanup_qualified']=True
        if forge=='wrong-snapshot':frames['after-adoption']['stderr_snapshot']='active-run-before-kill.stderr'
        active_raw=raw(active)
        if forge=='duplicate-version':active_raw=b'{"version":1,'+active_raw[1:]
        r['active_run_checkpoint_sha256']=write(work,'active-run-checkpoint.json',active_raw)
        if forge=='mask-active':life['runtime_crash_during_build_tested']=False
        if forge=='schema':r['schema_version']=2
        if forge=='first-archive':r['original_image_sha256_after_rebuild']='0'*64
        life['recovery_checkpoint_sha256']=write(work,'recovery-checkpoint.json',r);write(work,'integration-receipt.json',life)
        for p in [*work.glob('*.json'),*private.iterdir()]:p.chmod(0o600)

    def test_active_run_window_original_completion_and_second_build_require_explicit_scope(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.retained_active(work)
            result=inspect_fixture_recovery(work,BOOT,INV,'runtime-crash-during-build')
            self.assertIs(result['active_run_window_observed'],True);self.assertIs(result['cleanup_qualified'],False)
            for scenario in ['runtime-restart','runtime-crash']:
                with self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV,scenario)

    def test_recomputed_active_receipts_cannot_mask_finished_unbound_reordered_or_replaced_run(self):
        for mode in ['early-end','forged-prefix','pid','deadline','clock','missing-stage','numeric-bool','active-boot','active-cleanup','wrong-snapshot','duplicate-version','mask-active','schema','first-archive','missing-completion','original-image','missing-first-build']:
            with tempfile.TemporaryDirectory() as d:
                work=Path(d);self.retained_active(work,mode)
                with self.subTest(mode=mode),self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV,'runtime-crash-during-build')

if __name__=='__main__':unittest.main()
