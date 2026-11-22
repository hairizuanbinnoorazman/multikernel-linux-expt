#!/usr/bin/env python3
"""Bind a real preload receipt from the actual held instrumented executable."""
import copy,hashlib,json,os,re,select,shutil,subprocess,tempfile,unittest
from pathlib import Path
from runtime_diagnostic_exhaustion_worker import capture_worker,inspect_binding

ROOT=Path(__file__).resolve().parent.parent
BOOT=Path('/proc/sys/kernel/random/boot_id').read_text().strip()


class BindingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build_directory=tempfile.TemporaryDirectory(prefix='mk-binding-executable-')
        selected=os.environ.get('MK_DIAGNOSTIC_WORKER_TEST_BINARY')
        cls.source=os.environ.get('MK_DIAGNOSTIC_WORKER_TEST_SOURCE','a'*40)
        if not re.fullmatch('[a-f0-9]{40}',cls.source):raise ValueError('invalid native test source')
        if selected:
            cls.binary=Path(selected)
            if not cls.binary.is_absolute() or cls.binary.is_symlink():raise ValueError('invalid native test executable')
        else:
            cls.binary=Path(cls.build_directory.name)/'worker'
            env={**os.environ,'CGO_ENABLED':'0','GOWORK':'off','GOFLAGS':''}
            subprocess.run(['go','test','-c','-mod=readonly','-tags=diagnostic_exhaustion_worker','-trimpath','-buildvcs=false','-ldflags=-X github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo.Revision='+cls.source[:7],'-o',str(cls.binary),'./cmd/containerd-shim-multikernel-v2'],cwd=ROOT/'runtime',env=env,check=True,timeout=180,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        label=subprocess.run([str(cls.binary),'--version'],capture_output=True,check=True,timeout=10)
        if not re.fullmatch(rb'containerd-shim-multikernel-v2 version=\S+ revision='+cls.source[:7].encode()+b'\n',label.stdout) or label.stderr:raise ValueError('native executable source label differs')

    @classmethod
    def tearDownClass(cls):cls.build_directory.cleanup()

    def exercise(self,exited=False):
        with tempfile.TemporaryDirectory(prefix='mk-worker-binding-') as directory:
            work=Path(directory);private=work/'integration';private.mkdir(mode=0o700)
            binary=work/'bin/containerd-shim-multikernel-v2';binary.parent.mkdir(mode=0o700)
            shutil.copyfile(self.binary,binary);binary.chmod(0o500)
            container='b'*64
            bundle=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container
            bundle.mkdir(parents=True,mode=0o700)
            def write(path,value):
                path.write_bytes(value if isinstance(value,bytes) else json.dumps(value).encode());path.chmod(0o600)
            write(private/'create.stdout',(container+'\n').encode())
            build=dict(source_commit=self.source,shim_variant='diagnostic-exhaustion-test-worker',instrumented_test_executable=True,binaries={'containerd-shim-multikernel-v2':hashlib.sha256(binary.read_bytes()).hexdigest()})
            with (bundle/'preload.stderr').open('xb') as err:
                os.fchmod(err.fileno(),0o600)
                process=subprocess.Popen([str(binary),'--diagnostic-exhaustion-binding-hold'],cwd=bundle,stdout=subprocess.PIPE,stderr=err)
                try:
                    ready,_,_=select.select([process.stdout],[],[],10)
                    self.assertTrue(ready,'instrumented producer did not become ready')
                    self.assertEqual(process.stdout.readline(),b'DIAGNOSTIC_EXHAUSTION_BINDING_READY\n')
                    path=bundle/'diagnostic-exhaustion.fixture.json';original=path.read_bytes();receipt=json.loads(original)
                    self.assertEqual(receipt['worker_pid'],process.pid)
                    marker=bundle/'.multikernel-worker.pid';write(marker,(str(process.pid)+'\n').encode())
                    if exited:
                        process.terminate();process.wait(timeout=5)
                        with self.assertRaises((ValueError,OSError)):capture_worker(private,build,BOOT,'after-start')
                        return
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
                        with self.subTest(value=bad),self.assertRaises((ValueError,OSError)):capture_worker(private,build,BOOT,'after-start')
                    write(path,original)
                    first=capture_worker(private,build,BOOT,'after-start')
                    second=capture_worker(private,build,BOOT,'before-release')
                    self.assertEqual({**first,'phase':'before-release'},second)
                    self.assertTrue(inspect_binding(private,build,BOOT)['worker_unchanged_through_child_matrix'])
                    altered=copy.deepcopy(first);altered['worker_start_ticks']+=1
                    write(private/'exhaustion-after-start-worker.json',altered)
                    with self.assertRaises(ValueError):inspect_binding(private,build,BOOT)
                    self.assertLessEqual((bundle/'preload.stderr').stat().st_size,1<<20)
                    print('REAL_INSTRUMENTED_PRELOAD_LIVE_INCARNATION_BUNDLE_EXECUTABLE_AND_REFUSALS_PASS')
                    print('actual_preload_receipt=true actual_child_execution_qualified=false')
                finally:
                    if process.poll() is None:process.terminate()
                    process.wait(timeout=5);process.stdout.close()

    def test_real_held_executable_and_recomputed_record_refusals(self):self.exercise()
    def test_exited_producer_never_grants_live_authority(self):self.exercise(exited=True)


if __name__=='__main__':unittest.main()
