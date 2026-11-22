"""Generate and requalify explicit deployment logging in a disposable host daemon."""
import hashlib,json,os,re,time
from pathlib import Path
from runtime_diagnostic_bundle_checkpoint import decode,equal,read_fixture_artifact
from runtime_docker_logging_policy import logging_config
from runtime_trial_agent_profile import sha
from runtime_trial_command import capture


def base_config(private):
    return {'default-runtime':'runc','data-root':'/var/lib/docker',
            'exec-root':str(private/'docker-exec'),'pidfile':str(private/'docker.pid'),
            'hosts':['unix://'+str(private/'docker.sock')],'containerd':str(private/'containerd.sock'),
            'bridge':'none','iptables':False,'ip6tables':False,'ip-forward':False,'ip-masq':False,
            'features':{'containerd-snapshotter':True}}



def inherited_create_argv(private,workload):
    return ['/usr/bin/docker','--host','unix://'+str(private/'docker.sock'),'create',
            '--name','g8-materialization','--runtime','io.containerd.multikernel.v2','--network','none',
            '--user','1000:1000','--cap-drop','ALL','--security-opt','no-new-privileges=true',
            '--security-opt','apparmor=unconfined','--security-opt','seccomp=unconfined',
            '--sysctl','net.ipv4.ip_unprivileged_port_start=1024','--sysctl','net.ipv4.ping_group_range=1 0',
            '--device-cgroup-rule','a *:* rwm','--mount',
            'type=bind,source='+str(private/'input')+',target=/work/input,readonly','g8-approved:fixture',*workload]


def prepare(work):
    work=Path(work);private=work/'integration';source=work/'source/scripts'
    with (private/'docker-base.json').open('x') as stream:
        os.fchmod(stream.fileno(),0o600)
        json.dump(base_config(private),stream,sort_keys=True);stream.write('\n')
        stream.flush();os.fsync(stream.fileno())
    commands=[('docker-config-merge',['/usr/bin/python3',str(source/'merge-runtime-docker-config.py'),
                str(private/'docker-base.json'),str(private/'docker.json'),'--enable-bounded-logging']),
              ('docker-config-validate',['/usr/bin/dockerd','--validate','--config-file',str(private/'docker.json')])]
    for label,argv in commands:
        capture(['/usr/bin/env','-i','PATH=/usr/bin:/bin','LANG=C','LC_ALL=C','PYTHONDONTWRITEBYTECODE=1',*argv],
                private,label,True,time.monotonic_ns()+30_000_000_000)
    equal(json.loads((private/'docker.json').read_text()),dict(base_config(private),**logging_config(),
        runtimes={'io.containerd.multikernel.v2':{'runtimeType':'io.containerd.multikernel.v2'}}))
    if (private/'docker.json').stat().st_mode&0o777!=0o600:raise ValueError('deployment candidate is not private')
    receipt=dict(version=1,source_commit=json.loads((work/'build.json').read_text())['source_commit'],
                 input_sha256=sha(private/'docker-base.json'),candidate_sha256=sha(private/'docker.json'),
                 merger_sha256=sha(source/'merge-runtime-docker-config.py'),policy_module_sha256=sha(source/'runtime_docker_logging_policy.py'),
                 explicit_bounded_logging=True,default_runtime_preserved=True,installed_parser_validated=True)
    with (private/'docker-deployment-config.json').open('x') as stream:
        os.fchmod(stream.fileno(),0o600);json.dump(receipt,stream,sort_keys=True);stream.write('\n')
        stream.flush();os.fsync(stream.fileno())
    return private/'docker.json'


def inspect_defaults(work,expected_receipt_sha256,expected_create_argv_sha256,workload):
    """Require actual merger outputs and an inherited-policy create command."""
    work=Path(work);private=work/'integration'
    read=lambda name,empty=False:read_fixture_artifact(private,name,1<<20,allow_empty=empty)
    try:
        receipt_data=read('docker-deployment-config.json')
        if hashlib.sha256(receipt_data).hexdigest()!=expected_receipt_sha256:raise ValueError('receipt differs')
        receipt=decode(receipt_data,1<<20)
        build=decode(read_fixture_artifact(work,'build.json',1<<20),1<<20)
        source=work/'source/scripts';trusted=Path(__file__).parent
        pins={name:hashlib.sha256((trusted/name).read_bytes()).hexdigest() for name in ['merge-runtime-docker-config.py','runtime_docker_logging_policy.py']}
        if {name:sha(source/name) for name in pins}!=pins:raise ValueError('deployment source differs')
        equal(receipt,dict(version=1,source_commit=build['source_commit'],input_sha256=hashlib.sha256(read('docker-base.json')).hexdigest(),
            candidate_sha256=hashlib.sha256(read('docker.json')).hexdigest(),merger_sha256=pins['merge-runtime-docker-config.py'],
            policy_module_sha256=pins['runtime_docker_logging_policy.py'],explicit_bounded_logging=True,default_runtime_preserved=True,installed_parser_validated=True))
        equal(decode(read('docker-base.json'),1<<20),base_config(private))
        equal(decode(read('docker.json'),1<<20),dict(base_config(private),**logging_config(),
            runtimes={'io.containerd.multikernel.v2':{'runtimeType':'io.containerd.multikernel.v2'}}))
        if (private/'docker.json').stat().st_mode&0o777!=0o600:raise ValueError('candidate mode differs')
        for label in ['docker-config-merge','docker-config-validate']:
            equal(decode(read(label+'.exit.json'),65536),dict(returncode=0))
            read(label+'.stdout',True);read(label+'.stderr',True)
        info=decode(read('docker-info.json'),1<<20)
        if info.get('LoggingDriver')!='local' or info.get('DefaultRuntime')!='runc' or info.get('DockerRootDir')!='/var/lib/docker':raise ValueError('running daemon defaults differ')
        if not isinstance(info.get('Runtimes'),dict) or 'io.containerd.multikernel.v2' not in info['Runtimes']:raise ValueError('running registration differs')
        argv_data=read('docker-create-argv.json')
        if hashlib.sha256(argv_data).hexdigest()!=expected_create_argv_sha256:raise ValueError('create command differs')
        argv=decode(argv_data,1<<20)
        equal(argv,inherited_create_argv(private,workload))
        created=read('create.stdout').decode().strip()
        item=decode(read('log-inspect.stdout'),1<<20)
        if not re.fullmatch('[a-f0-9]{64}',created) or type(item) is not list or len(item)!=1 or item[0].get('Id')!=created:raise ValueError('created container differs')
    except (KeyError,IndexError,TypeError,ValueError,OSError,UnicodeError):
        raise ValueError('deployment logging default provenance refused') from None
    return True
