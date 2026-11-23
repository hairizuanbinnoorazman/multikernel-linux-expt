#!/usr/bin/env python3
"""Strict archived recovery proof and bounded live-path orchestration controls."""
import copy,importlib.util,json,os,subprocess,tempfile,threading,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_child_output_recovery as recovery
import runtime_child_output_accounting as output
import runtime_trial_agent_profile as agent_profile

spec=importlib.util.spec_from_file_location('finite_output_fixture',Path(__file__).with_name('test-runtime-child-output-accounting.py'))
fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
BOOT,INV=fixture.BOOT,fixture.INV
encoded=fixture.encoded
sha=fixture.sha


class RecoveryTests(unittest.TestCase):
    def make(self,work):
        fixture.OutputTests().make(work);p=work/'integration'
        def write(name,value):
            data=value if type(value) is bytes else encoded(value)
            (p/name).write_bytes(data);(p/name).chmod(0o600)
        build=json.loads((work/'build.json').read_text());container=(p/'create.stdout').read_text().strip()
        before,after=fixture.frames(container)
        after['tail']['records'][-1]['final_output_accounting']['recovery_drain']=dict(stdout_offset=0,stderr_offset=0,stdout_in_flight_bytes=0,stderr_in_flight_bytes=0)
        for label,stdout,stderr,status in [('child-recovery-before',encoded(before),b'',0),('child-recovery-probe',b'\0'*8192,output.STDERR,37),('child-recovery-after-0',encoded(after),b'',0),('child-recovery-rebuild',(p/'workload.stdout').read_bytes(),(p/'workload.stderr').read_bytes(),0),('child-recovery-old-image',(p/'log-saved-image.stdout').read_bytes().split()[0]+b'  /home/rootless/.local/tmp/g8-build-before-shim-recovery/image.tar\n',b'',0)]:
            for suffix,data in [('stdout',stdout),('stderr',stderr),('exit.json',dict(returncode=status))]:write(label+'.'+suffix,data)
        binding=json.loads((p/'child-output-after-worker.json').read_text());binding.update(worker_pid=1235,worker_start_ticks=5679);write('child-output-after-worker.json',binding)
        target=after['tail']['records'][-1]
        write('child-recovery-held.json',dict(version=1,scope='held-undelivered-child-exec',boot_id=BOOT,invocation_id=INV,source_commit=build['source_commit'],container_id=container,generation=before['generation'],bundle_identity=before['bundle_identity'],process=dict(id=target['id'],pid=target['pid'],status=2,stdout_offset=0,stderr_offset=0)))
        write('child-recovery-fault.json',dict(version=1,scope='live-held-child-worker-fault',boot_id=BOOT,invocation_id=INV,source_commit=build['source_commit'],container_id=container,worker_pid=1234,worker_start_ticks=5678,supervisor_pid=1233,supervisor_start_ticks=5600,signal='SIGKILL',pidfd_exit_observed=True,source_bundle_parent_and_cgroup_bound=True,supervisor_resumed=True,historical_receipt_authority=False))
        write('child-recovery-replacement.json',dict(version=1,scope='live-replacement-worker-parent',worker_pid=1235,worker_start_ticks=5679,supervisor_pid=1233,supervisor_start_ticks=5600,same_source_bundle_and_cgroup=True))
        write('child-recovery-primary.json',json.loads((p/'log-primary.json').read_text()))
        view=output.qualify_recovered_pair(encoded(before),encoded(after),container,b'\0'*8192,output.STDERR,37)
        view.update(worker_replacement_qualified=True,post_recovery_child_build_matrix_qualified=True,original_child_archive_preserved=True,post_recovery_primary_separation_qualified=True)
        checkpoint=dict(version=1,scope='held-child-output-recovery-checkpoint',boot_id=BOOT,invocation_id=INV,source_commit=build['source_commit'],after_attempt=0,view=view,artifact_sha256={})
        write('child-output-recovery.json',checkpoint)
        receipt=json.loads((work/'integration-receipt.json').read_text());receipt.update(child_output_recovery_tested=True,child_output_accounting_tested=False,diagnostic_exhaustion_tested=False)
        (work/'integration-receipt.json').write_bytes(encoded(receipt));self.rehash(work)
        return work

    def rehash(self,work):
        p=work/'integration';cp=p/'child-output-recovery.json';value=json.loads(cp.read_text())
        value['artifact_sha256']={name:sha((p/name).read_bytes()) for name in recovery.artifact_names(value['after_attempt'])}
        cp.write_bytes(encoded(value));receipt=json.loads((work/'integration-receipt.json').read_text());receipt['child_output_recovery_sha256']=sha(cp.read_bytes());(work/'integration-receipt.json').write_bytes(encoded(receipt))

    def inspect(self,work):
        with patch.object(agent_profile,'ORIGINAL_PROFILE',work/'original-profile.json'):
            return recovery.inspect_fixture(work,BOOT,INV)

    def test_complete_checkpoint_binds_replacement_and_post_fault_build(self):
        with tempfile.TemporaryDirectory() as raw:
            work=self.make(Path(raw))
            with patch.object(recovery,'WorkerFault',side_effect=AssertionError('archived proof instantiated live fault authority')):
                view=self.inspect(work)
            self.assertTrue(view['live_worker_replacement_bound']);self.assertTrue(view['stopped_guest_drain_observed'])
            self.assertTrue(view['post_recovery_child_build_matrix_qualified']);self.assertTrue(view['original_child_archive_preserved'])
            self.assertFalse(view['full_g7_g8_qualified'])

    def test_recomputed_receipt_attacks_refuse(self):
        modes=['missing-claim','ordinary-scope','instrumented-scope','boot','invocation','source','same-worker','older-worker','worker-boolean','wrong-worker-binary','bundle','no-origin','advanced-origin','loss','wrong-probe-bytes','wrong-probe-exit','held-other-exec','held-advanced-offset','fault-old-epoch','fault-no-exit','fault-not-resumed','fault-pid-one','fault-other-worker','replacement-other-parent','replacement-other-worker','replacement-other-start','post-build-matrix','post-build-exit','old-archive-changed','primary-image-leak','primary-wrong-image','view-false-promotion','extra-checkpoint','extra-fault']
        for mode in modes:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                work=self.make(Path(raw));p=work/'integration'
                def change(name,fn):
                    path=p/name;value=json.loads(path.read_text());fn(value);path.write_bytes(encoded(value))
                if mode=='missing-claim':
                    r=json.loads((work/'integration-receipt.json').read_text());r['child_output_recovery_tested']=False;(work/'integration-receipt.json').write_bytes(encoded(r))
                elif mode in ['ordinary-scope','instrumented-scope']:
                    r=json.loads((work/'integration-receipt.json').read_text());r['child_output_accounting_tested' if mode=='ordinary-scope' else 'diagnostic_exhaustion_tested']=True;(work/'integration-receipt.json').write_bytes(encoded(r))
                elif mode in ['boot','invocation','source']:change('child-output-recovery.json',lambda v:v.update({{'boot':'boot_id','invocation':'invocation_id','source':'source_commit'}[mode]:'d'*40}))
                elif mode in ['same-worker','older-worker','worker-boolean','wrong-worker-binary','bundle']:
                    def mutate(v):
                        if mode=='same-worker':v['worker_pid']=1234
                        elif mode=='older-worker':v['worker_start_ticks']=1
                        elif mode=='worker-boolean':v['worker_pid']=True
                        elif mode=='wrong-worker-binary':v['executable_sha256']='d'*64
                        else:v['bundle_identity']['inode']+=1
                    change('child-output-after-worker.json',mutate)
                elif mode in ['no-origin','advanced-origin','loss']:
                    def mutate(v):
                        r=v['tail']['records'][-1]
                        if mode=='no-origin':r['final_output_accounting'].pop('recovery_drain')
                        elif mode=='advanced-origin':r['final_output_accounting']['recovery_drain']['stdout_offset']=1
                        else:r['output_disposition']['stdout']['replay_exposure_bytes']=1
                    change('child-recovery-after-0.stdout',mutate)
                elif mode=='wrong-probe-bytes':(p/'child-recovery-probe.stdout').write_bytes(b'\0'*8191)
                elif mode=='wrong-probe-exit':change('child-recovery-probe.exit.json',lambda v:v.update(returncode=0))
                elif mode.startswith('held-'):
                    def mutate(v):v['process']['id' if mode=='held-other-exec' else 'stdout_offset']='f'*64 if mode=='held-other-exec' else 1
                    change('child-recovery-held.json',mutate)
                elif mode.startswith('fault-'):
                    changes={'fault-old-epoch':dict(invocation_id='d'*32),'fault-no-exit':dict(pidfd_exit_observed=False),'fault-not-resumed':dict(supervisor_resumed=False),'fault-pid-one':dict(supervisor_pid=1),'fault-other-worker':dict(worker_pid=9999)}
                    change('child-recovery-fault.json',lambda v:v.update(changes[mode]))
                elif mode.startswith('replacement-'):
                    changes={'replacement-other-parent':dict(supervisor_pid=9999),'replacement-other-worker':dict(worker_pid=9999),'replacement-other-start':dict(worker_start_ticks=9999)}
                    change('child-recovery-replacement.json',lambda v:v.update(changes[mode]))
                elif mode=='post-build-matrix':(p/'child-recovery-rebuild.stdout').write_bytes((p/'child-recovery-rebuild.stdout').read_bytes().replace(b'G8_FAILED_BUILD_REJECTED',b'G8_FAILED_BUILD_UNQUALIFIED'))
                elif mode=='post-build-exit':change('child-recovery-rebuild.exit.json',lambda v:v.update(returncode=23))
                elif mode=='old-archive-changed':(p/'child-recovery-old-image.stdout').write_bytes(b'0'*64+b'  /home/rootless/.local/tmp/g8-build-before-shim-recovery/image.tar\n')
                elif mode=='primary-image-leak':change('child-recovery-primary.json',lambda v:v.update(child_image_absent=False))
                elif mode=='primary-wrong-image':change('child-recovery-primary.json',lambda v:v.update(checked_absent_image='sha256:'+'0'*64))
                elif mode=='view-false-promotion':change('child-output-recovery.json',lambda v:v['view'].update(full_g7_g8_qualified=True))
                elif mode=='extra-checkpoint':change('child-output-recovery.json',lambda v:v.update(payload='private'))
                elif mode=='extra-fault':change('child-recovery-fault.json',lambda v:v.update(payload='private'))
                self.rehash(work)
                with self.assertRaises(ValueError):self.inspect(work)

    def test_old_scope_refuses_before_read_only_worker_observation(self):
        with tempfile.TemporaryDirectory() as raw:
            work=self.make(Path(raw));authority=unittest.mock.Mock()
            authority._scope.side_effect=ValueError('historical scope refused')
            with patch.object(recovery,'WorkerFault',return_value=authority),patch.object(recovery,'worker_binding',side_effect=AssertionError('target lookup before scope refusal')):
                with self.assertRaises(ValueError):recovery.qualify([],work/'integration',BOOT,INV,'unused')

    def test_undelivered_exec_refuses_advanced_or_aliased_counters(self):
        base=dict(id='e'*64,pid=901,status=2,stdout_offset=0,stderr_offset=0)
        self.assertEqual(recovery.undelivered(base),base)
        for changes in [dict(id='init'),dict(pid=True),dict(pid=1<<31),dict(status=3),dict(status=2.0),dict(stdout_offset=1),dict(stderr_offset=True),dict(exit_event_queued=1),dict(final_output_accounting={'state':'unavailable'})]:
            with self.subTest(changes=changes),self.assertRaises(ValueError):recovery.undelivered({**base,**changes})

    def live_fixture(self,mode):
        with tempfile.TemporaryDirectory() as raw:
            work=self.make(Path(raw));p=work/'integration';container=(p/'create.stdout').read_text().strip()
            snapshots={name:(p/name).read_bytes() for name in recovery.artifact_names(0)}
            bindings={phase:json.loads(snapshots['child-output-'+phase+'-worker.json']) for phase in ['before','after']}
            held=json.loads(snapshots['child-recovery-held.json'])['process'];replacement=json.loads(snapshots['child-recovery-replacement.json'])
            for name in recovery.artifact_names(0):
                if not name.startswith('child-output-'):(p/name).unlink()
            (p/'child-output-recovery.json').unlink();release=threading.Event();started=threading.Event();clock=[0];labels=[];reads=[0];kills=[]
            class Guard:
                binding={k:v for k,v in bindings['before'].items() if k!='phase'}
                supervisor_pid=1233;supervisor_start=5600
                def _scope(self):pass
                def __enter__(self):return self
                def pause(self):pass
                def kill_worker(self):
                    kills.append(True);release.set();return dict(signal='SIGKILL',pidfd_exit_observed=True,source_bundle_parent_and_cgroup_bound=True,supervisor_resumed=True,historical_receipt_authority=False)
                def observe_replacement(self,_):return replacement
                def __exit__(self,*_):release.set()
            def saved(*_):
                reads[0]+=1
                if reads[0]==1:return {'':{}}
                self.assertTrue(started.wait(2))
                if mode=='readiness-no-exec':return {'':{}}
                return {'':{},held['id']:held}
            def capture(argv,private,label,success,deadline):
                labels.append(label)
                if label=='child-recovery-probe':
                    started.set()
                    if mode=='probe-error':raise ValueError('selected probe collection refusal')
                    self.assertTrue(release.wait(2))
                    if mode=='late-probe':clock[0]=deadline
                source='child-recovery-after-0' if label.startswith('child-recovery-after-') else label
                stdout=snapshots[source+'.stdout'];stderr=snapshots[source+'.stderr'];exit_data=snapshots[source+'.exit.json']
                if label.startswith('child-recovery-after-'):
                    if mode=='frozen-no-tail':stdout=snapshots['child-recovery-before.stdout']
                    if mode=='late-after-capture':clock[0]=deadline
                for suffix,data in [('stdout',stdout),('stderr',stderr),('exit.json',exit_data)]:output.write(private,label+'.'+suffix,data)
                status=json.loads(exit_data)['returncode']
                if success:self.assertEqual(status,0)
                return subprocess.CompletedProcess(argv,status,stdout,stderr)
            primary=json.loads(snapshots['child-recovery-primary.json'])
            with patch.object(recovery,'WorkerFault',return_value=Guard()),patch.object(recovery,'worker_binding',side_effect=[(Path('/fixture'),bindings['before']),(Path('/fixture'),bindings['after'])]),patch.object(recovery,'saved_processes',side_effect=saved),patch.object(recovery,'capture',side_effect=capture),patch.object(recovery.time,'sleep'),patch.object(recovery.time,'monotonic_ns',side_effect=lambda:clock[0]),patch('runtime_primary_docker.observe',return_value=primary):
                if mode=='success':recovery.qualify([],p,BOOT,INV,'fixture')
                else:
                    with self.assertRaises(ValueError):recovery.qualify([],p,BOOT,INV,'fixture')
            if mode=='success':
                self.rehash(work);self.assertTrue(self.inspect(work)['post_recovery_child_build_matrix_qualified'])
            else:self.assertFalse((p/'child-output-recovery.json').exists())
            if mode=='frozen-no-tail':self.assertEqual(len([label for label in labels if label.startswith('child-recovery-after-')]),25)
            if mode=='readiness-no-exec':self.assertEqual(reads[0],102)
            if mode in ['readiness-no-exec','probe-error']:self.assertFalse(kills)

    def test_live_orchestration_and_saved_checkpoint_requalify(self):self.live_fixture('success')

    def test_live_deadlines_capture_failure_and_frozen_clock_poll_caps_refuse(self):
        for mode in ['late-probe','late-after-capture','frozen-no-tail','readiness-no-exec','probe-error']:
            with self.subTest(mode=mode):self.live_fixture(mode)


if __name__=='__main__':unittest.main()
