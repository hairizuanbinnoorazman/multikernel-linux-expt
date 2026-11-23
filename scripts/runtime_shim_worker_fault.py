"""Exact disposable-trial shim fault; no historical receipt grants authority.

Only the current workload unit's main process may bind these pidfds. The bound
supervisor is temporarily stopped so its worker can exit before replacement.
Context exit resumes only previously bound processes, including after refusal.
There is deliberately no numeric-PID signal fallback or reusable CLI entry.
"""
import hashlib,os,re,select,signal,stat,time
from pathlib import Path
from runtime_bounded_command import bounded_command
from runtime_child_output_accounting import observe_worker
from runtime_diagnostic_bundle_checkpoint import equal
from runtime_service_policy import check_fixture_scope
from runtime_trial_shim_variant import require_production


def process_state(pid):
    raw=(Path('/proc')/str(pid)/'stat').read_bytes()
    if len(raw)>4096:raise ValueError('worker process metadata exceeds bound')
    fields=raw[raw.rfind(b')')+1:].split()
    if len(fields)<20 or fields[0] in [b'Z',b'X']:raise ValueError('worker process is not live')
    return dict(state=fields[0].decode('ascii'),parent=int(fields[1]),start=int(fields[19]))


class WorkerFault:
    def __init__(self,private,build,boot,invocation,unit,deadline):
        self.private,self.build,self.boot,self.invocation,self.unit=private,build,boot,invocation,unit
        if type(deadline) is not int:raise ValueError('invalid worker fault deadline')
        self.deadline=deadline
        self.worker_fd=self.supervisor_fd=None
        self.worker_stopped=self.supervisor_stopped=False
        self.bound=False

    def _scope(self):
        # Reject a historical boot/epoch before reading any target PID or
        # opening a pidfd. Recheck this live scope before each fault signal.
        if (Path('/proc/sys/kernel/random/boot_id').read_text().strip()!=self.boot or
            type(self.invocation) is not str or not re.fullmatch('[a-f0-9]{32}',self.invocation) or
            os.environ.get('INVOCATION_ID')!=self.invocation or
            type(self.unit) is not str or not re.fullmatch('g8-host-docker-[a-z0-9-]+[.]service',self.unit)):
            raise ValueError('worker fault epoch or unit differs')
        if time.monotonic_ns()>=self.deadline:raise ValueError('worker fault deadline expired')
        result=bounded_command(['systemctl','show',self.unit,'-p','MainPID','-p','InvocationID','-p','KillMode','-p','PrivateMounts'],min(self.deadline,time.monotonic_ns()+5_000_000_000))
        if result.returncode or result.stderr:raise ValueError('worker fault unit observation failed')
        pairs=[line.split('=',1) for line in result.stdout.decode('ascii').splitlines()]
        if any(len(pair)!=2 for pair in pairs) or len(pairs)!=len(dict(pairs)):raise ValueError('worker fault unit properties differ')
        check_fixture_scope(dict(pairs),os.getpid(),self.invocation)
        cgroup=Path('/proc/self/cgroup').read_bytes()
        if cgroup!=('0::/system.slice/'+self.unit+'\n').encode():raise ValueError('worker fault caller cgroup differs')
        return cgroup

    def _supervisor(self,pid):
        current=process_state(pid);proc=Path('/proc')/str(pid)
        executable=self.private.parent/'bin/containerd-shim-multikernel-v2'
        if os.readlink(proc/'exe')!=str(executable):raise ValueError('worker supervisor executable differs')
        with (proc/'exe').open('rb') as file:
            before=os.fstat(file.fileno())
            if not stat.S_ISREG(before.st_mode) or before.st_uid!=os.geteuid() or not 0<before.st_size<=128<<20:raise ValueError('unsafe worker supervisor executable')
            actual=hashlib.file_digest(file,'sha256').hexdigest()
            for after in [os.fstat(file.fileno()),executable.stat()]:
                if any(getattr(before,k)!=getattr(after,k) for k in ['st_dev','st_ino','st_size','st_mtime_ns','st_ctime_ns']):raise ValueError('worker supervisor executable changed')
        if actual!=self.build['binaries']['containerd-shim-multikernel-v2']:raise ValueError('worker supervisor source differs')
        cwd=(proc/'cwd').stat();identity=self.bundle.lstat()
        if (cwd.st_dev,cwd.st_ino,cwd.st_uid)!=(identity.st_dev,identity.st_ino,identity.st_uid):raise ValueError('worker supervisor bundle differs')
        return current

    def _live(self):
        if self._scope()!=self.cgroup:raise ValueError('worker fault cgroup changed')
        bundle,binding=observe_worker(self.private,self.build,self.boot)
        equal(binding,self.binding)
        if bundle!=self.bundle:raise ValueError('worker fault bundle changed')
        worker=process_state(self.worker_pid);supervisor=self._supervisor(self.supervisor_pid)
        if worker['parent']!=self.supervisor_pid or supervisor['start']!=self.supervisor_start:raise ValueError('worker fault parent or incarnation changed')
        for pid,fd in [(self.worker_pid,self.worker_fd),(self.supervisor_pid,self.supervisor_fd)]:
            if (Path('/proc')/str(pid)/'cgroup').read_bytes()!=self.cgroup:raise ValueError('worker fault target cgroup differs')
            signal.pidfd_send_signal(fd,0)
        if time.monotonic_ns()>=self.deadline:raise ValueError('worker fault binding observation was late')
        return worker,supervisor

    def __enter__(self):
        try:
            self.cgroup=self._scope()
            require_production(self.build)
            self.bundle,self.binding=observe_worker(self.private,self.build,self.boot)
            self.worker_pid=self.binding['worker_pid']
            worker=process_state(self.worker_pid);self.supervisor_pid=worker['parent']
            if any(type(pid) is not int or pid<=1 or pid==os.getpid() for pid in [self.worker_pid,self.supervisor_pid]) or self.worker_pid==self.supervisor_pid:raise ValueError('worker fault process roles differ')
            self.supervisor_start=self._supervisor(self.supervisor_pid)['start']
            self.worker_fd=os.pidfd_open(self.worker_pid,0)
            self.supervisor_fd=os.pidfd_open(self.supervisor_pid,0)
            worker,supervisor=self._live()
            if worker['state'] not in ['R','S','D','I'] or supervisor['state'] not in ['R','S','D','I']:raise ValueError('worker fault target already stopped')
            self.bound=True
            return self
        except BaseException:
            self.close()
            raise

    def _stopped(self,pid):
        deadline=min(self.deadline,time.monotonic_ns()+2_000_000_000)
        for _ in range(201):
            if time.monotonic_ns()>=deadline:break
            if process_state(pid)['state']=='T':return
            time.sleep(.01)
        raise ValueError('worker fault stop not observed within deadline')

    def pause(self):
        if not self.bound or self.worker_stopped or self.supervisor_stopped:raise ValueError('worker fault pause state differs')
        self._live()
        # Mark attempted stops before sending: finally must resume even if a
        # signal or the following metadata observation returns an error.
        self.supervisor_stopped=True
        signal.pidfd_send_signal(self.supervisor_fd,signal.SIGSTOP)
        self._stopped(self.supervisor_pid)
        self._live()
        self.worker_stopped=True
        signal.pidfd_send_signal(self.worker_fd,signal.SIGSTOP)
        self._stopped(self.worker_pid)
        worker,supervisor=self._live()
        if worker['state']!='T' or supervisor['state']!='T':raise ValueError('worker fault processes are not held')

    def kill_worker(self):
        if not self.bound or not self.worker_stopped or not self.supervisor_stopped:raise ValueError('worker fault kill requires both held processes')
        worker,supervisor=self._live()
        if worker['state']!='T' or supervisor['state']!='T':raise ValueError('worker fault hold changed')
        remaining=self.deadline-time.monotonic_ns()
        if remaining<=0:raise ValueError('worker fault deadline expired before kill')
        signal.pidfd_send_signal(self.worker_fd,signal.SIGKILL)
        poll=select.poll();poll.register(self.worker_fd,select.POLLIN)
        if not poll.poll(max(1,min(2000,remaining//1_000_000))):raise ValueError('worker fault exit not observed')
        if time.monotonic_ns()>=self.deadline:raise ValueError('worker fault exit observation was late')
        self.worker_stopped=False
        # This already-bound pidfd cannot name a replacement or a reused PID.
        signal.pidfd_send_signal(self.supervisor_fd,signal.SIGCONT)
        self.supervisor_stopped=False
        self.bound=False
        return dict(signal='SIGKILL',pidfd_exit_observed=True,source_bundle_parent_and_cgroup_bound=True,supervisor_resumed=True,historical_receipt_authority=False)

    def close(self):
        errors=[]
        for name in ['worker','supervisor']:
            fd=getattr(self,name+'_fd')
            if fd is not None:
                try:
                    if getattr(self,name+'_stopped'):signal.pidfd_send_signal(fd,signal.SIGCONT)
                except ProcessLookupError:pass
                except BaseException as error:errors.append(error)
                finally:
                    try:os.close(fd)
                    except OSError as error:errors.append(error)
                    setattr(self,name+'_fd',None);setattr(self,name+'_stopped',False)
        self.bound=False
        if errors:raise ValueError('worker fault bound-process resume failed') from None

    def __exit__(self,*_):self.close()
