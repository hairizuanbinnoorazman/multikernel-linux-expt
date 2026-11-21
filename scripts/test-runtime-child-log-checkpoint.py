#!/usr/bin/env python3
"""Recomputed receipt attacks against detached child log-pressure acceptance."""
import copy,hashlib,importlib.util,json,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_trial_agent_profile as agent_profile
from runtime_child_log_pressure import WORKLOAD,WRITER_BYTES
from runtime_child_log_checkpoint import inspect_fixture_logging,view

spec=importlib.util.spec_from_file_location('concurrent_fixture',Path(__file__).with_name('test-runtime-concurrency-checkpoint.py'))
fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
BOOT,INV=fixture.BOOT,fixture.INV
sha=lambda b:hashlib.sha256(b).hexdigest()


class ChildLogTests(unittest.TestCase):
    def make(self,work,mode=None):
        fixture.ConcurrentCheckpointTests().fixture(work)
        p=work/'integration'
        def write(name,value):
            data=value if type(value) is bytes else json.dumps(value).encode()
            (p/name).write_bytes(data);(p/name).chmod(0o600);return sha(data)
        item=json.loads((p/'owner-map-a.stdout').read_text())[0]
        item['Config']['Cmd']=copy.deepcopy(WORKLOAD);item['State']=dict(Status='exited',ExitCode=0)
        image='sha256:'+'e'*64
        stdout=(p/'a-workload.stdout').read_bytes();stderr=(p/'a-workload.stderr').read_bytes()
        if mode=='matrix':stdout=stdout.replace(b'G8_FAILED_BUILD_REJECTED',b'G8_FAILED_BUILD_UNQUALIFIED')
        if mode=='unbounded-policy':item['HostConfig']['LogConfig']['Config']={}
        if mode=='wrong-runtime':item['HostConfig']['Runtime']='runc'
        if mode=='privileged':item['HostConfig']['Privileged']=True
        if mode=='writer-changed':item['Config']['Cmd']=['/bin/sh','-c','echo fake']
        if mode=='failed-exit':item['State']['ExitCode']=1
        if mode=='boolean-exit':item['State']['ExitCode']=False
        captures={'log-writer-status':b'0\n','log-writer-bytes':str(WRITER_BYTES).encode()+b'\n','log-writer-diagnostic':b'256+0 records in\n256+0 records out\n','log-status':b'0\n','log-build-out':stdout,'log-build-err':stderr,'log-saved-image':('c'*64+' /home/rootless/.local/tmp/g8-build/image.tar\n').encode(),'log-engine':(p/'before-a-engine.stdout').read_bytes(),'log-wait':b'0\n','log-inspect':json.dumps([item]).encode()}
        if mode=='writer-status-record':captures['log-writer-status']=b'1\n'
        if mode=='writer-byte-record':captures['log-writer-bytes']=b'1\n'
        if mode=='writer-byte-float-record':captures['log-writer-bytes']=b'16777216.0\n'
        if mode=='status':captures['log-status']=b'1\n'
        if mode=='wait':captures['log-wait']=b'1\n'
        if mode=='engine':captures['log-engine']=json.dumps(dict(ID='other',KernelVersion='test-kernel',ServerVersion='29.9.0',SecurityOptions=[])).encode()
        commands={}
        for label,data in captures.items():
            commands[label]=write(label+'.stdout',data);write(label+'.stderr',b'');write(label+'.exit.json',dict(returncode=1 if mode=='command-exit' and label=='log-build-out' else 0))
        primary=json.loads((p/'concurrency-primary-two-built-a.json').read_text())
        if mode=='image-leak':primary['child_image_absent']=False
        if mode=='wrong-image':primary['checked_absent_image']='sha256:'+'0'*64
        r=dict(version=2,boot_id=BOOT,invocation_id=INV,writer_bytes=WRITER_BYTES,writer_exit_status=0,writer_completed_bytes=WRITER_BYTES,detached_start=True,
               agent_profile_receipt_sha256=sha((p/'current-agent-profile-receipt.json').read_bytes()),build_image_id=image,saved_image_sha256='c'*64,
               rotation=dict(rotated_file_count=2,observed_bytes=1_004_116,conservative_sample_bound=2_131_072,log_contents_read=False),
               log_primary_sha256=write('log-primary.json',primary),command_stdout_sha256=commands,
               workload_stdout_sha256=write('workload.stdout',stdout),workload_stderr_sha256=write('workload.stderr',stderr),
               log_contents_read=False,loss_accounting_qualified=False,cleanup_qualified=False)
        if mode=='legacy-writer':r['version']=1
        if mode=='writer-failed':r['writer_exit_status']=1
        if mode=='writer-boolean-status':r['writer_exit_status']=False
        if mode=='writer-short':r['writer_completed_bytes']=1
        if mode=='writer-float-count':r['writer_completed_bytes']=float(WRITER_BYTES)
        if mode=='small-writer':r['writer_bytes']=1024
        if mode=='attached':r['detached_start']=False
        if mode=='oversize':r['rotation']['observed_bytes']=2_131_073
        if mode=='no-rotation':r['rotation']['rotated_file_count']=1
        if mode=='tiny':r['rotation']['observed_bytes']=1000
        if mode=='extra-rotation-field':r['rotation']['extra']=True
        if mode=='float-count':r['rotation']['rotated_file_count']=2.0
        if mode=='contents-read':r['rotation']['log_contents_read']=True
        if mode=='loss-claim':r['loss_accounting_qualified']=True
        if mode=='cleanup-claim':r['cleanup_qualified']=True
        if mode=='stale-agent':r['agent_profile_receipt_sha256']='0'*64
        if mode=='wrong-boot':r['boot_id']='0'*8+'-0000-4000-8000-'+'0'*12
        if mode=='missing-command':r['command_stdout_sha256'].pop('log-wait')
        if mode=='archive':r['saved_image_sha256']='0'*64
        receipt=json.loads((work/'integration-receipt.json').read_text())
        for key in ['storage_pressure_tested','concurrent_guest_builds_tested','runtime_restart_tested','runtime_crash_tested','engine_observation_tested','aggregate_docker_health_tested']:receipt[key]=False
        receipt['child_log_pressure_tested']=mode!='missing-claim'
        if mode=='mixed-scope':receipt['storage_pressure_tested']=True
        receipt['child_log_pressure_sha256']=write('child-log-pressure.json',r)
        (work/'integration-receipt.json').write_text(json.dumps(receipt));(work/'integration-receipt.json').chmod(0o600)

    def inspect(self,work):
        with patch.object(agent_profile,'ORIGINAL_PROFILE',work/'original-profile.json'):
            return inspect_fixture_logging(work,BOOT,INV)

    def test_complete_detached_build_and_bounded_rotation_accepts(self):
        with tempfile.TemporaryDirectory() as raw:
            work=Path(raw);self.make(work);self.assertEqual(self.inspect(work),view(BOOT,INV))

    def test_recomputed_receipts_refuse_incomplete_builds_scope_policy_and_rotation(self):
        for mode in ['legacy-writer','writer-failed','writer-boolean-status','writer-short','writer-float-count','writer-status-record','writer-byte-record','writer-byte-float-record','matrix','unbounded-policy','wrong-runtime','privileged','writer-changed','failed-exit','boolean-exit','status','wait','engine','command-exit','image-leak','wrong-image','small-writer','attached','oversize','no-rotation','tiny','extra-rotation-field','float-count','contents-read','loss-claim','cleanup-claim','stale-agent','wrong-boot','missing-command','archive','missing-claim','mixed-scope']:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                work=Path(raw);self.make(work,mode)
                with self.assertRaises(ValueError):self.inspect(work)

if __name__=='__main__':unittest.main()
