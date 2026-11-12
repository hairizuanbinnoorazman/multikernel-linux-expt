#!/usr/bin/env python3
"""Opt-in isolated native Docker logging trial; no multikernel child is started.

Run only inside a delegated oneshot with PrivateNetwork/PrivateMounts and an
exclusive RuntimeDirectory. An outer authority must pin source, boot, primary
preservation, tool identities and reserves. Keep all artifacts root-private.
"""
import argparse,hashlib,io,json,os
from pathlib import Path
import re,stat,subprocess as sp,tarfile,time
from runtime_primary_docker import bounded_command,ENV
from runtime_docker_log_rotation import qualify_rotation
from runtime_trial_output import logging_arguments,logging_config,require_logging


def execute(work,run,boot):
    if (os.geteuid()!=0 or Path('/proc/sys/kernel/random/boot_id').read_text().strip()!=boot or
            not re.fullmatch('[a-f0-9]{32}',os.environ.get('INVOCATION_ID','')) or
            not work.is_absolute() or not run.is_absolute() or run.parent!=Path('/run') or
            not re.fullmatch('g8ll-[a-f0-9]{8}',run.name) or work.exists()):
        raise ValueError('invalid logging trial authority')
    for p in [work.parent,*work.parent.parents,run]:
        info=p.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid!=0 or info.st_mode&0o022:
            raise ValueError('unprotected logging trial directory')
    cgroup=Path('/proc/self/cgroup').read_text().splitlines()
    if len(cgroup)!=1 or not re.fullmatch(r'0::/system.slice/g8-log-live-[a-f0-9]{8}\.service',cgroup[0]):
        raise ValueError('logging trial requires its own delegated unit')
    if any(os.readlink('/proc/self/ns/'+ns)==os.readlink('/proc/1/ns/'+ns) for ns in ['net','mnt']):
        raise ValueError('logging trial namespaces are not private')
    os.umask(0o077);work.mkdir(mode=0o700);data=work/'data';deadline=time.monotonic()+90
    scratch=work/'tmp';scratch.mkdir(mode=0o700)
    private_env=dict(ENV,TMPDIR=str(scratch),DOCKER_TMPDIR=str(scratch))
    for p in [work,run]:
        space=os.statvfs(p)
        if space.f_bavail*space.f_frsize<(1<<30)+(64<<20):raise ValueError('logging trial reserve is unavailable')
    def record(name,value):
        with (work/name).open('x') as f:json.dump(value,f,sort_keys=True);f.write('\n');f.flush();os.fsync(f.fileno())
    def command(argv):
        return bounded_command(['/usr/bin/env','TMPDIR='+str(scratch),'DOCKER_TMPDIR='+str(scratch),*argv],timeout=min(15,deadline-time.monotonic()),limit=65536)
    # No external registry, host mounts, child kernel or daemon credentials.
    busy=Path('/usr/bin/busybox');payload=busy.read_bytes()
    if not 0<len(payload)<8<<20:raise ValueError('logging fixture binary exceeds bound')
    with tarfile.open(work/'image.tar','w') as archive:
        entry=tarfile.TarInfo('bin/busybox');entry.mode=0o755;entry.size=len(payload);archive.addfile(entry,io.BytesIO(payload))
    record('input.json',dict(busybox_sha256=hashlib.sha256(payload).hexdigest(),bytes=len(payload)))
    csock=run/'c.sock';dsock=run/'d.sock'
    config=work/'containerd.toml'
    config.write_text('version = 2\nroot = '+json.dumps(str(work/'containerd-root'))+'\nstate = '+json.dumps(str(run/'containerd-state'))+'\ndisabled_plugins = ["io.containerd.grpc.v1.cri", "io.containerd.cri.v1.images", "io.containerd.cri.v1.runtime", "io.containerd.nri.v1.nri"]\n[grpc]\naddress = '+json.dumps(str(csock))+'\n')
    dc=work/'docker.json'
    dc.write_text(json.dumps(dict(logging_config(),**{'data-root':str(data),'exec-root':str(run/'docker-exec'),'pidfile':str(run/'docker.pid'),'hosts':['unix://'+str(dsock)],'containerd':str(csock),'storage-driver':'vfs','features':{'containerd-snapshotter':False},'bridge':'none','iptables':False,'ip6tables':False,'ip-forward':False,'ip-masq':False,'exec-opts':['native.cgroupdriver=cgroupfs']})))
    processes=[];logs=[]
    def start(argv,name,socket):
        stream=(work/name).open('xb');logs.append(stream)
        proc=sp.Popen(['/usr/bin/prlimit','--fsize=16777216:16777216','--core=0:0','--',*argv],stdin=sp.DEVNULL,stdout=stream,stderr=sp.STDOUT,env=private_env);processes.append(proc)
        while not socket.exists():
            if proc.poll() is not None or time.monotonic()>=deadline:raise ValueError('private logging daemon unavailable')
            time.sleep(.05)
        return proc
    try:
        start(['/usr/bin/containerd','--config',str(config)],'containerd.log',csock)
        start(['/usr/bin/dockerd','--config-file',str(dc)],'docker.log',dsock)
        cli=['/usr/bin/docker','--host','unix://'+str(dsock)]
        while True:
            try:info=json.loads(command(cli+['info','--format','{{json .}}']));break
            except RuntimeError:
                if any(p.poll() is not None for p in processes) or time.monotonic()>=deadline:raise
                time.sleep(.05)
        if info['DockerRootDir']!=str(data) or info['LoggingDriver']!='local':raise ValueError('private logging daemon identity differs')
        record('docker-info.json',info)
        image=command(cli+['import',str(work/'image.tar'),'g8-log-fixture:native']).decode().strip()
        if not re.fullmatch('sha256:[a-f0-9]{64}',image):raise ValueError('private logging image identity differs')
        observations={}
        # Separate default-policy and explicit-policy containers. Throttling
        # lets the lossy nonblocking logger rotate instead of dropping a burst.
        script='i=0; while [ "$i" -lt 4096 ]; do /bin/busybox printf "%4095s\\n" x; i=$((i+1)); if [ $((i%64)) -eq 0 ]; then /bin/busybox sleep 0.01; fi; done'
        for case,flags in [('default',[]),('explicit',logging_arguments())]:
            name='g8-log-'+case
            created=command(cli+['create',*flags,'--name',name,'--runtime','runc','--network','none','--read-only','--user','65534:65534','--cap-drop','ALL','--security-opt','no-new-privileges=true','--security-opt','apparmor=unconfined','--pids-limit','16','--memory','64m','--cgroup-parent',cgroup[0][3:]+'/workloads',image,'/bin/busybox','sh','-ec',script]).decode().strip()
            if not re.fullmatch('[a-f0-9]{64}',created):raise ValueError('private logging container identity differs')
            before=json.loads(command(cli+['inspect',name]))[0];require_logging(before['HostConfig'].get('LogConfig'))
            command(cli+['start',name])
            if command(cli+['wait',name])!=b'0\n':raise ValueError('private logging container failed')
            item=json.loads(command(cli+['inspect',name]))[0];record(case+'-inspect.json',item)
            observations[case]=qualify_rotation(item,data)
        record('observations.json',observations)
    finally:
        clean=True
        for proc in reversed(processes):
            if proc.poll() is None:
                proc.terminate()
                try:proc.wait(timeout=10)
                except sp.TimeoutExpired:proc.kill();proc.wait(timeout=2)
            clean &= proc.returncode==0
        for stream in logs:stream.close()
        if not clean:raise ValueError('private logging daemon did not stop cleanly')
    # Recheck after daemon shutdown, retaining raw logs only inside owned data.
    for case in ['default','explicit']:
        if qualify_rotation(json.loads((work/(case+'-inspect.json')).read_text()),data)!=observations[case]:raise ValueError('local logging sample changed after shutdown')
    if list(scratch.iterdir()):raise ValueError('private logging scratch remains')
    record('result.json',dict(native_private_docker_rotation_qualified=True,default_and_explicit_policy_verified=True,two_rotated_samples_within_observed_bound=True,private_daemons_stopped=True,child_kernel_started=False,full_g7_g8_qualified=False))


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--work',type=Path,required=True);parser.add_argument('--run-dir',type=Path,required=True);parser.add_argument('--boot-id',required=True);args=parser.parse_args()
    try:execute(args.work,args.run_dir,args.boot_id)
    except Exception:
        print('private Docker logging trial refused; inspect private evidence',file=__import__('sys').stderr);raise SystemExit(1)
