#!/usr/bin/env python3
"""Real live-process binding controls; synthetic preload records are explicit."""
import copy,hashlib,json,os,shutil,subprocess,tempfile,unittest
from pathlib import Path
from runtime_diagnostic_exhaustion_worker import capture_worker,inspect_binding,process_start,validate_preload,LIMIT,FRAME_BYTES

BOOT=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
SOURCE='a'*40


class BindingTests(unittest.TestCase):
    def test_live_native_process_and_recomputed_record_refusals(self):
        self.exercise('/bin/sleep')

    def test_multicall_sleep_preserves_dispatch_name(self):
        self.exercise('/bin/busybox')

    def exercise(self,program):
        with tempfile.TemporaryDirectory(prefix='mk-worker-binding-') as directory:
            work=Path(directory);private=work/'integration';private.mkdir(mode=0o700)
            binary=work/'bin/containerd-shim-multikernel-v2';binary.parent.mkdir(mode=0o700)
            shutil.copyfile(program,binary);binary.chmod(0o500)
            container='b'*64
            bundle=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container
            bundle.mkdir(parents=True,mode=0o700)
            def write(path,value):
                path.write_bytes(value if isinstance(value,bytes) else json.dumps(value).encode());path.chmod(0o600)
            write(private/'create.stdout',(container+'\n').encode())
            build=dict(source_commit=SOURCE,shim_variant='diagnostic-exhaustion-test-worker',instrumented_test_executable=True,binaries={'containerd-shim-multikernel-v2':hashlib.sha256(binary.read_bytes()).hexdigest()})
            process=subprocess.Popen(['sleep','60'],executable=str(binary),cwd=bundle)
            try:
                info=bundle.stat();frames=LIMIT//FRAME_BYTES
                receipt=dict(version=2,scope='diagnostic-exhaustion-test-worker',source_revision=SOURCE[:7],worker_pid=process.pid,worker_start_ticks=process_start(process.pid),boot_id=BOOT,bundle_device=info.st_dev,bundle_inode=info.st_ino,bundle_uid=os.geteuid(),limit_bytes=LIMIT,reserved_bytes=frames*FRAME_BYTES,synthetic_preload_attempts=frames+bool(LIMIT%FRAME_BYTES),synthetic_preload_admitted=1,allowance_closed_before_production_entry=True,instrumented_test_executable=True)
                path=bundle/'diagnostic-exhaustion.fixture.json'
                marker=bundle/'.multikernel-worker.pid';write(marker,(str(process.pid)+'\n').encode())
                cases=[]
                for key in receipt:
                    mutated=copy.deepcopy(receipt)
                    if isinstance(mutated[key],bool):mutated[key]=False
                    elif isinstance(mutated[key],int):mutated[key]=True
                    else:mutated[key]='wrong'
                    cases.append(mutated)
                for key in ['worker_start_ticks','bundle_inode','bundle_device','worker_pid','reserved_bytes','synthetic_preload_attempts']:
                    mutated=copy.deepcopy(receipt);mutated[key]+=1;cases.append(mutated)
                for bad in cases:
                    write(path,bad)
                    with self.subTest(value=bad),self.assertRaises((ValueError,OSError)):
                        capture_worker(private,build,BOOT,'after-start')
                write(path,receipt)
                first=capture_worker(private,build,BOOT,'after-start')
                second=capture_worker(private,build,BOOT,'before-release')
                self.assertEqual({**first,'phase':'before-release'},second)
                self.assertTrue(inspect_binding(private,build,BOOT)['worker_unchanged_through_child_matrix'])
                # Forging a new digest cannot repair a mismatched incarnation.
                altered=copy.deepcopy(first);altered['worker_start_ticks']+=1
                write(private/'exhaustion-after-start-worker.json',altered)
                with self.assertRaises(ValueError):inspect_binding(private,build,BOOT)
                print('NATIVE_LIVE_PROCESS_INCARNATION_BUNDLE_EXECUTABLE_AND_PRELOAD_REFUSALS_PASS')
                print('synthetic_preload_records=true actual_child_execution_qualified=false')
            finally:
                process.terminate();process.wait(timeout=5)


if __name__=='__main__':unittest.main()
