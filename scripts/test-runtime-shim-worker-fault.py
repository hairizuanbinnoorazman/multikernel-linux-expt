#!/usr/bin/env python3
"""Owned-process pidfd controls; native helpers are not a child-kernel proof."""
import contextlib,hashlib,json,os,signal,shutil,subprocess,sys,tempfile,time,unittest
from pathlib import Path
from unittest import mock
import runtime_shim_worker_fault as fault

BOOT=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
INV='a'*32
UNIT='g8-host-docker-native-worker-fault.service'


@contextlib.contextmanager
def owned_helpers():
    with tempfile.TemporaryDirectory(prefix='mk-worker-fault-') as temporary:
        root=Path(temporary);os.chmod(root,0o700)
        private=root/'integration';private.mkdir(mode=0o700)
        (root/'bin').mkdir(mode=0o700)
        executable=root/'bin/containerd-shim-multikernel-v2'
        shutil.copyfile(sys.executable,executable);executable.chmod(0o500)
        container='b'*64
        (private/'create.stdout').write_text(container+'\n');(private/'create.stdout').chmod(0o600)
        bundle=private/'containerd-state/io.containerd.runtime.v2.task/moby'/container
        bundle.mkdir(parents=True,mode=0o700)
        program="""import os,pathlib,subprocess,sys,time
os.umask(0o077)
child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)'])
pathlib.Path('.multikernel-worker.pid').write_text(str(child.pid)+'\\n')
status=child.wait()
pathlib.Path('child-status').write_text(str(status))
time.sleep(60)
"""
        supervisor=subprocess.Popen([str(executable),'-c',program],cwd=bundle,start_new_session=True,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        canary=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)'],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            deadline=time.monotonic()+5
            while not (bundle/'.multikernel-worker.pid').exists() and time.monotonic()<deadline:
                if supervisor.poll() is not None:raise ValueError('native supervisor fixture exited')
                time.sleep(.01)
            assert (bundle/'.multikernel-worker.pid').exists()
            build=dict(source_commit='c'*40,binaries={'containerd-shim-multikernel-v2':hashlib.sha256(executable.read_bytes()).hexdigest()},shim_variant='production',instrumented_test_executable=False)
            yield private,build,bundle,supervisor,canary
        finally:
            if supervisor.poll() is None:
                os.killpg(supervisor.pid,signal.SIGCONT)
                os.killpg(supervisor.pid,signal.SIGKILL)
            supervisor.wait(timeout=3)
            if canary.poll() is None:canary.kill()
            canary.wait(timeout=3)


class WorkerFaultTests(unittest.TestCase):
    def make(self,private,build,**kw):
        return fault.WorkerFault(private,build,kw.get('boot',BOOT),kw.get('invocation',INV),kw.get('unit',UNIT),kw.get('deadline',time.monotonic_ns()+30_000_000_000))

    def test_epoch_and_unit_refuse_before_target_observation_or_pidfd(self):
        for mode in ['boot','environment','invocation','unit','expired','wrong-main','wrong-unit-epoch','wrong-kill-mode','shared-mount','wrong-cgroup']:
            options={};env=INV
            if mode=='boot':options['boot']='00000000-0000-0000-0000-000000000000'
            if mode=='environment':env='d'*32
            if mode=='invocation':options['invocation']=True
            if mode=='unit':options['unit']='docker.service'
            if mode=='expired':options['deadline']=0
            properties=dict(MainPID=str(os.getpid()),InvocationID=INV,KillMode='process',PrivateMounts='yes')
            if mode=='wrong-main':properties['MainPID']='1'
            if mode=='wrong-unit-epoch':properties['InvocationID']='d'*32
            if mode=='wrong-kill-mode':properties['KillMode']='control-group'
            if mode=='shared-mount':properties['PrivateMounts']='no'
            result=subprocess.CompletedProcess([],0,('\n'.join(k+'='+v for k,v in properties.items())+'\n').encode(),b'')
            with self.subTest(mode=mode),mock.patch.dict(os.environ,INVOCATION_ID=env),mock.patch.object(fault,'bounded_command',return_value=result),mock.patch.object(fault,'observe_worker',side_effect=AssertionError('target PID lookup before scope refusal')),mock.patch.object(fault.os,'pidfd_open',side_effect=AssertionError('pidfd before scope refusal')):
                with self.assertRaises(ValueError):
                    with self.make(Path('/unused'),{},**options):pass

    def test_native_bound_kill_resumes_supervisor_and_preserves_canary(self):
        with owned_helpers() as (private,build,bundle,supervisor,canary):
            group=Path('/proc/self/cgroup').read_bytes()
            # Only systemd admission is a fixture here. All PID, source, cwd,
            # parent, process-state and pidfd operations below are real.
            with mock.patch.object(fault.WorkerFault,'_scope',return_value=group):
                with self.make(private,build) as guard:
                    self.assertEqual(guard.supervisor_pid,supervisor.pid)
                    guard.pause();self.assertEqual(fault.process_state(supervisor.pid)['state'],'T')
                    self.assertEqual(fault.process_state(guard.worker_pid)['state'],'T')
                    proof=guard.kill_worker();self.assertTrue(proof['pidfd_exit_observed']);self.assertTrue(proof['supervisor_resumed'])
                    with self.assertRaises(ValueError):guard.kill_worker()
                deadline=time.monotonic()+3
                while not (bundle/'child-status').exists() and time.monotonic()<deadline:time.sleep(.01)
                self.assertEqual((bundle/'child-status').read_text(),'-9')
                self.assertIsNone(canary.poll());self.assertIsNone(supervisor.poll())
                self.assertIsNone(guard.worker_fd);self.assertIsNone(guard.supervisor_fd)

    def test_native_exception_after_pause_resumes_both_bound_processes(self):
        with owned_helpers() as (private,build,bundle,supervisor,canary):
            with mock.patch.object(fault.WorkerFault,'_scope',return_value=Path('/proc/self/cgroup').read_bytes()):
                guard=self.make(private,build)
                with self.assertRaisesRegex(RuntimeError,'selected refusal'):
                    with guard:
                        worker=guard.worker_pid;guard.pause();raise RuntimeError('selected refusal')
                deadline=time.monotonic()+2
                while time.monotonic()<deadline and any(fault.process_state(p)['state']=='T' for p in [worker,supervisor.pid]):time.sleep(.01)
                self.assertNotEqual(fault.process_state(worker)['state'],'T');self.assertNotEqual(fault.process_state(supervisor.pid)['state'],'T')
                self.assertIsNone(canary.poll())

    def test_changed_source_and_marker_refuse_without_mutating_signals(self):
        for mode in ['source-before-bind','source-after-bind','marker-after-bind']:
            with self.subTest(mode=mode),owned_helpers() as (private,build,bundle,supervisor,canary):
                with mock.patch.object(fault.WorkerFault,'_scope',return_value=Path('/proc/self/cgroup').read_bytes()),mock.patch.object(fault.signal,'pidfd_send_signal',wraps=signal.pidfd_send_signal) as send:
                    if mode=='source-before-bind':build['binaries']['containerd-shim-multikernel-v2']='0'*64
                    with self.assertRaises(ValueError):
                        with self.make(private,build) as guard:
                            if mode=='source-after-bind':build['binaries']['containerd-shim-multikernel-v2']='0'*64
                            if mode=='marker-after-bind':(bundle/'.multikernel-worker.pid').write_text(str(supervisor.pid)+'\n')
                            guard.pause()
                    self.assertFalse(any(call.args[1]!=0 for call in send.call_args_list))
                    self.assertIsNone(canary.poll());self.assertNotEqual(fault.process_state(supervisor.pid)['state'],'T')

    def test_pause_deadline_refusal_resumes_attempted_stop(self):
        with owned_helpers() as (private,build,bundle,supervisor,canary):
            with mock.patch.object(fault.WorkerFault,'_scope',return_value=Path('/proc/self/cgroup').read_bytes()):
                guard=self.make(private,build)
                with self.assertRaises(ValueError):
                    with guard:
                        with mock.patch.object(guard,'_stopped',side_effect=ValueError('stop observation expired')):guard.pause()
                deadline=time.monotonic()+2
                while time.monotonic()<deadline and fault.process_state(supervisor.pid)['state']=='T':time.sleep(.01)
                self.assertNotEqual(fault.process_state(supervisor.pid)['state'],'T');self.assertIsNone(canary.poll())

    def test_failed_or_late_exit_observation_still_resumes_supervisor(self):
        for mode in ['timeout','late']:
            with self.subTest(mode=mode),owned_helpers() as (private,build,bundle,supervisor,canary):
                with mock.patch.object(fault.WorkerFault,'_scope',return_value=Path('/proc/self/cgroup').read_bytes()):
                    guard=self.make(private,build);real_poll=fault.select.poll()
                    class ExitObservation:
                        def register(self,*args):real_poll.register(*args)
                        def poll(self,*args):
                            observed=real_poll.poll(*args)
                            if mode=='timeout':return []
                            guard.deadline=0
                            return observed
                    with self.assertRaises(ValueError):
                        with guard:
                            guard.pause()
                            with mock.patch.object(fault.select,'poll',return_value=ExitObservation()):guard.kill_worker()
                    deadline=time.monotonic()+3
                    while not (bundle/'child-status').exists() and time.monotonic()<deadline:time.sleep(.01)
                    self.assertEqual((bundle/'child-status').read_text(),'-9');self.assertIsNone(canary.poll())
                    self.assertNotEqual(fault.process_state(supervisor.pid)['state'],'T')

    def test_resume_error_does_not_skip_other_bound_process_or_close(self):
        guard=self.make(Path('/unused'),{})
        guard.worker_fd,guard.supervisor_fd=10,11
        guard.worker_stopped=guard.supervisor_stopped=True
        def send(fd,_):
            if fd==10:raise PermissionError('selected worker resume refusal')
        with mock.patch.object(fault.signal,'pidfd_send_signal',side_effect=send) as signals,mock.patch.object(fault.os,'close') as close:
            with self.assertRaisesRegex(ValueError,'resume failed'):guard.close()
            self.assertEqual(signals.call_args_list,[mock.call(10,signal.SIGCONT),mock.call(11,signal.SIGCONT)])
            self.assertEqual(close.call_args_list,[mock.call(10),mock.call(11)])
            self.assertIsNone(guard.worker_fd);self.assertIsNone(guard.supervisor_fd)


if __name__=='__main__':unittest.main()
