"""Detached, current-agent child builds and metadata-only host log rotation."""
import hashlib,json,re,subprocess,time
from pathlib import Path
from runtime_trial_command import capture
from runtime_docker_log_rotation import qualify_rotation
from runtime_primary_docker import observe,unchanged
from runtime_trial_agent_profile import inspect_current_agent

WRITER_BYTES=16 << 20
# Match the existing concurrent trial's cold child-start admission window.
# Ordinary execs keep their separate 30-second command deadline.
START_BUDGET=180
BUILD_ROOT='/home/rootless/.local/tmp/g8-log-pressure'
WRITER_SCRIPT='''/bin/busybox dd if=/dev/zero bs=65536 count=256 2>"$p/writer-stderr"
writer_status=$?; printf '%s\\n' "$writer_status" >"$p/writer-status"
test "$writer_status" -eq 0 || exit "$writer_status"
printf '16777216\\n' >"$p/writer-bytes"
'''
WORKLOAD=['/bin/sh','-c', '''umask 077
release=0; trap 'release=1' USR1
p=/home/rootless/.local/tmp/g8-log-pressure
mkdir -m 700 "$p" || exit 1
/bin/sh /work/input/probe.sh >"$p/stdout" 2>"$p/stderr"
status=$?; printf '%s\n' "$status" >"$p/status"
test "$status" -eq 0 || exit "$status"
'''+WRITER_SCRIPT+'''
: >"$p/done"
while [ "$release" -eq 0 ]; do sleep 1 || :; done
exit 0
''']


def digest(path):return hashlib.sha256(path.read_bytes()).hexdigest()


def wait_for_writer(probe,deadline):
    """Share one poll/collection/completion budget; never accept a late success."""
    for attempt in range(301):
        now=time.monotonic_ns()
        if now>=deadline:break
        ready=probe(attempt,min(deadline,now+30_000_000_000))
        remaining=deadline-time.monotonic_ns()
        if remaining<=0:break
        if ready.returncode==0:return
        time.sleep(min(1,remaining/1_000_000_000))
    raise ValueError('child log writer did not complete within deadline')


def qualify(cli,private,boot,invocation):
    # Imports stay lazy: the concurrency checkpoint imports its own fixture.
    from runtime_concurrency_checkpoint import matrix
    name='g8-materialization'
    def command(args,label,success=True,timeout=30):
        return capture(cli+args,private,label,success,time.monotonic_ns()+int(timeout*1e9))
    def execute(args,label,success=True):return command(['exec','--user','1000:1000',name,*args],label,success)
    agent=digest(private/'current-agent-profile-receipt.json')
    inspect_current_agent(private.parent,agent)
    command(['start',name],'log-start',timeout=START_BUDGET)
    wait_for_writer(lambda attempt,deadline:capture(
        cli+['exec','--user','1000:1000',name,'/bin/sh','-c','test -f '+BUILD_ROOT+'/done'],
        private,'log-ready-'+str(attempt),False,deadline),time.monotonic_ns()+300_000_000_000)
    assert execute(['/bin/cat',BUILD_ROOT+'/status'],'log-status').stdout==b'0\n'
    assert execute(['/bin/cat',BUILD_ROOT+'/writer-status'],'log-writer-status').stdout==b'0\n'
    assert execute(['/bin/cat',BUILD_ROOT+'/writer-bytes'],'log-writer-bytes').stdout==str(WRITER_BYTES).encode()+b'\n'
    execute(['/bin/cat',BUILD_ROOT+'/writer-stderr'],'log-writer-diagnostic')
    stdout=execute(['/bin/cat',BUILD_ROOT+'/stdout'],'log-build-out').stdout
    stderr=execute(['/bin/cat',BUILD_ROOT+'/stderr'],'log-build-err').stdout
    images=re.findall(rb'^G8_BUILD_IMAGE_ID=(sha256:[a-f0-9]{64})$',stdout,re.M);assert len(images)==1
    image=images[0].decode();matrix(stdout,stderr,image,True)
    for suffix,data in [('stdout',stdout),('stderr',stderr)]:
        with (private/('workload.'+suffix)).open('xb') as file:file.write(data)
    saved=execute(['/usr/bin/sha256sum','/home/rootless/.local/tmp/g8-build/image.tar'],'log-saved-image').stdout.split()[0]
    assert re.fullmatch(rb'[a-f0-9]{64}',saved) and saved in stdout
    engine=json.loads(execute(['/usr/local/bin/docker','info','--format','{{json .}}'],'log-engine').stdout)
    profile=json.loads((private/'manifests/gce-mk2.json').read_text())
    assert engine['KernelVersion']==profile['kernel_release'] and engine['ServerVersion']=='29.9.0' and 'name=rootless' in engine['SecurityOptions']
    baseline=json.loads((private/'primary-docker-before.json').read_text());primary=observe(image);unchanged(baseline,primary);assert primary['child_image_absent']
    (private/'log-primary.json').write_text(json.dumps(primary,sort_keys=True)+'\n')
    command(['kill','--signal=USR1',name],'log-release')
    assert command(['wait',name],'log-wait',timeout=60).stdout==b'0\n'
    item=json.loads(command(['inspect',name],'log-inspect').stdout)[0]
    assert item['Config']['Cmd']==WORKLOAD
    rotation=qualify_rotation(item,private/'docker-data')
    # A separate metadata observation while the stopped container is retained.
    assert qualify_rotation(item,private/'docker-data')==rotation
    result=dict(version=3,boot_id=boot,invocation_id=invocation,writer_bytes=WRITER_BYTES,writer_exit_status=0,writer_completed_bytes=WRITER_BYTES,
                detached_start=True,deployment_config_receipt_sha256=digest(private/'docker-deployment-config.json'),
                create_argv_sha256=digest(private/'docker-create-argv.json'),create_logging_policy_source='daemon-default',agent_profile_receipt_sha256=agent,build_image_id=image,
                saved_image_sha256=saved.decode(),rotation=rotation,log_primary_sha256=digest(private/'log-primary.json'),
                command_stdout_sha256={name:digest(private/(name+'.stdout')) for name in ['log-status','log-writer-status','log-writer-bytes','log-writer-diagnostic','log-build-out','log-build-err','log-saved-image','log-engine','log-wait','log-inspect']},
                workload_stdout_sha256=digest(private/'workload.stdout'),workload_stderr_sha256=digest(private/'workload.stderr'),
                log_contents_read=False,loss_accounting_qualified=False,cleanup_qualified=False)
    (private/'child-log-pressure.json').write_text(json.dumps(result,sort_keys=True)+'\n')
    return subprocess.CompletedProcess(cli,0)
