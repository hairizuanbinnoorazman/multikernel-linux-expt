"""Derive a disposable approved bootstrap for the exact committed staged agent."""
import copy
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess as sp


def sha(path):
    fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC|os.O_NONBLOCK)
    with os.fdopen(fd,'rb') as file:
        info=os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.geteuid() or info.st_nlink!=1 or info.st_mode&0o022 or not 0<info.st_size<=128<<20:
            raise ValueError('unsafe trial profile artifact')
        digest=hashlib.sha256();total=0
        while chunk:=file.read(1<<20):
            total+=len(chunk)
            if total>info.st_size:raise ValueError('trial profile artifact grew')
            digest.update(chunk)
        after=os.fstat(file.fileno());named=os.stat(path,follow_symlinks=False)
        fields=['st_dev','st_ino','st_uid','st_nlink','st_mode','st_size','st_mtime_ns','st_ctime_ns']
        if total!=info.st_size or any(getattr(info,k)!=getattr(value,k) for value in [after,named] for k in fields):raise ValueError('trial profile artifact changed')
        return digest.hexdigest()


def prepare(work, original, approval):
    work=Path(work);original=Path(original);approval=Path(approval)
    private=work/'integration'
    if approval!=private/'manifests/gce-mk2.json' or approval.exists():
        raise ValueError('trial approval must be a fresh fixed private path')
    source=work/'source';build=json.loads((work/'build.json').read_text())
    agent=work/'bin/mk-agent';agent_sha=sha(agent)
    if agent_sha!=build['binaries']['mk-agent']:
        raise ValueError('staged agent differs from committed binary pin')
    # The original approval remains untouched. Both validations run with the
    # full normal policy: no parent, digest, modinfo or archive-identity bypass.
    validate=['/usr/bin/python3','-I',str(source/'scripts/validate-runtime-bootstrap.py')]
    result=sp.run(validate+[str(original),'gce-mk2'],capture_output=True,timeout=45)
    (private/'original-agent-profile-validation.stdout').write_bytes(result.stdout)
    (private/'original-agent-profile-validation.stderr').write_bytes(result.stderr)
    if result.returncode or result.stderr:raise ValueError('original approved bootstrap refused')
    profile=json.loads(original.read_text());derived=copy.deepcopy(profile)
    archive=private/'current-agent.cpio.gz'
    env={**os.environ,'BUSYBOX':'/bin/busybox'};env.pop('MK_AGENT_TOKEN_FILE',None)
    command=['/bin/bash',str(source/'scripts/build-agent-initramfs.sh'),str(agent),str(archive),profile['transport']['module']['path'],profile['relay']['path']]
    result=sp.run(command,capture_output=True,timeout=90,env=env)
    (private/'current-agent-bootstrap-build.stdout').write_bytes(result.stdout)
    (private/'current-agent-bootstrap-build.stderr').write_bytes(result.stderr)
    if result.returncode:raise ValueError('current-agent bootstrap build refused')
    derived['agent']={'path':str(agent),'sha256':agent_sha}
    derived['initramfs']={'path':str(archive),'sha256':sha(archive)}
    with approval.open('x') as file:os.fchmod(file.fileno(),0o600);json.dump(derived,file,sort_keys=True);file.write('\n')
    result=sp.run(validate+[str(approval),'gce-mk2'],capture_output=True,timeout=45)
    (private/'current-agent-profile-validation.stdout').write_bytes(result.stdout)
    (private/'current-agent-profile-validation.stderr').write_bytes(result.stderr)
    if result.returncode or result.stderr:raise ValueError('current-agent approved bootstrap refused')
    receipt=dict(version=1,source_commit=build['source_commit'],original_profile_sha256=sha(original),derived_profile_sha256=sha(approval),agent_sha256=agent_sha,initramfs_sha256=sha(archive),full_original_and_derived_validation=True,installed_approval_changed=False,credential_injection_requested=False)
    (private/'current-agent-profile-receipt.json').write_text(json.dumps(receipt,sort_keys=True)+'\n')
    return derived
