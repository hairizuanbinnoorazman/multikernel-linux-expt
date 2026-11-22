#!/usr/bin/env python3
"""Synthetic archive controls bind exhaustion to the complete child matrix."""
import hashlib,importlib.util,json,os,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_trial_agent_profile as profile
from runtime_diagnostic_exhaustion_worker import checkpoint,inspect_fixture_exhaustion,LIMIT,FRAME_BYTES,PHASES,sha

spec=importlib.util.spec_from_file_location('logging_fixture',Path(__file__).with_name('test-runtime-child-log-checkpoint.py'))
fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
BOOT,INV=fixture.BOOT,fixture.INV


class CheckpointTests(unittest.TestCase):
    def make(self,work,mode=None):
        fixture.ChildLogTests().make(work,'matrix' if mode=='child-matrix' else 'writer-short' if mode=='child-writer' else None)
        private=work/'integration'
        def write(path,value):
            path.write_bytes(value if isinstance(value,bytes) else json.dumps(value).encode());path.chmod(0o600)
        build=json.loads((work/'build.json').read_text())
        build.update(shim_variant='diagnostic-exhaustion-test-worker',instrumented_test_executable=True)
        build['binaries']['containerd-shim-multikernel-v2']='c'*64;write(work/'build.json',build)
        container=(private/'create.stdout').read_text().strip()
        frames=LIMIT//FRAME_BYTES
        preload=dict(version=2,scope='diagnostic-exhaustion-test-worker',source_revision='a'*7,worker_pid=41,worker_start_ticks=123,boot_id=BOOT,bundle_device=1,bundle_inode=2,bundle_uid=os.geteuid(),limit_bytes=LIMIT,reserved_bytes=frames*FRAME_BYTES,synthetic_preload_attempts=frames+bool(LIMIT%FRAME_BYTES),synthetic_preload_admitted=1,allowance_closed_before_production_entry=True,instrumented_test_executable=True)
        for phase in PHASES:
            raw=json.dumps(preload).encode();write(private/('exhaustion-'+phase+'-preload.json'),raw)
            snapshot=dict(version=1,scope='instrumented-worker-live-binding',phase=phase,boot_id=BOOT,source_commit='a'*40,container_id=container,worker_pid=41,worker_start_ticks=123,bundle_device=1,bundle_inode=2,bundle_uid=os.geteuid(),executable_sha256='c'*64,preload_sha256=sha(raw))
            write(private/('exhaustion-'+phase+'-worker.json'),snapshot)
        checkpoint(private,build,BOOT,INV)
        cp=private/'diagnostic-exhaustion-checkpoint.json'
        value=json.loads(cp.read_text())
        if mode in ['preload-open','preload-short','preload-bool','preload-start','preload-source']:
            p=private/'exhaustion-after-start-preload.json';v=json.loads(p.read_text())
            key,new={'preload-open':('allowance_closed_before_production_entry',False),'preload-short':('reserved_bytes',1),'preload-bool':('worker_pid',True),'preload-start':('worker_start_ticks',124),'preload-source':('source_revision','b'*7)}[mode]
            v[key]=new;write(p,v)
            s=private/'exhaustion-after-start-worker.json';v=json.loads(s.read_text());v['preload_sha256']=sha(p.read_bytes());write(s,v)
        if mode in ['snapshot-start','snapshot-binary','snapshot-extra']:
            p=private/'exhaustion-before-release-worker.json';v=json.loads(p.read_text())
            key,new={'snapshot-start':('worker_start_ticks',124),'snapshot-binary':('executable_sha256','d'*64),'snapshot-extra':('unexpected',True)}[mode];v[key]=new;write(p,v)
        for name in value['artifact_sha256']:value['artifact_sha256'][name]=sha((private/name).read_bytes())
        if mode=='checkpoint-source':value['source_commit']='b'*40
        if mode=='checkpoint-boot':value['boot_id']='wrong'
        if mode=='checkpoint-epoch':value['invocation_id']='b'*32
        if mode=='checkpoint-binary':value['binary_sha256']='d'*64
        if mode=='checkpoint-child':value['child_log_pressure_sha256']='d'*64
        if mode=='checkpoint-extra':value['extra']=True
        write(cp,value)
        receipt=json.loads((work/'integration-receipt.json').read_text())
        receipt['diagnostic_exhaustion_tested']=mode!='missing-claim'
        receipt['diagnostic_exhaustion_sha256']=sha(cp.read_bytes());write(work/'integration-receipt.json',receipt)
        if mode=='production-variant':build.update(shim_variant='production',instrumented_test_executable=False);write(work/'build.json',build)

    def inspect(self,work):
        with patch.object(profile,'ORIGINAL_PROFILE',work/'original-profile.json'):
            return inspect_fixture_exhaustion(work,BOOT,INV)

    def test_complete_binding_and_child_matrix(self):
        with tempfile.TemporaryDirectory() as raw:
            work=Path(raw);self.make(work)
            self.assertTrue(self.inspect(work)['complete_child_build_matrix_bound'])

    def test_recomputed_hashes_do_not_repair_semantic_refusals(self):
        modes=['child-matrix','child-writer','preload-open','preload-short','preload-bool','preload-start','preload-source','snapshot-start','snapshot-binary','snapshot-extra','checkpoint-source','checkpoint-boot','checkpoint-epoch','checkpoint-binary','checkpoint-child','checkpoint-extra','missing-claim','production-variant']
        for mode in modes:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                work=Path(raw);self.make(work,mode)
                with self.assertRaises(ValueError):self.inspect(work)
        print('RECOMPUTED_EXHAUSTION_CHECKPOINT_18_SEMANTIC_REFUSALS_PASS')


if __name__=='__main__':unittest.main()
