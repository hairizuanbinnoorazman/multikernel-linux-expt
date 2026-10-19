#!/usr/bin/env python3
"""Exercise actual token-bearing builders and no-replace private publication."""
import argparse
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import tempfile

ROOT=Path(__file__).resolve().parent.parent
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--old-root',type=Path)
parser.add_argument('--reader-uid',type=int)
args=parser.parse_args()
for name in ['busybox','cpio']:
    assert shutil.which(name),name+' is required'
if args.reader_uid is not None: assert os.geteuid()==0 and args.reader_uid>0

with tempfile.TemporaryDirectory(prefix='g7-private-bootstrap-') as temporary:
    directory=Path(temporary);directory.chmod(0o755)
    private=directory/'private';private.mkdir(mode=0o700)
    public=directory/'output';public.mkdir(mode=0o755)
    credential=private/'token';secret=secrets.token_hex(32).encode()+b'\n'
    credential.write_bytes(secret);credential.chmod(0o600)
    component=private/'component';component.write_bytes(b'synthetic-component\n');component.chmod(0o700)
    env=dict(os.environ,MK_AGENT_TOKEN_FILE=str(credential),BUSYBOX=shutil.which('busybox'))
    result={'actor_uid':os.geteuid(),'builders':{}}
    def build(root,kind,path,environment=None):
        script=root/'scripts'/('build-agent-initramfs.sh' if kind=='agent' else 'build-security-authority-initramfs.sh')
        argv=['bash',str(script),str(component)]
        if kind=='authority': argv += [str(component),str(component),'nonroot',str(path)]
        else: argv += [str(path),str(component),str(component)]
        return subprocess.run(argv,env=environment or env,capture_output=True,timeout=15)
    def reader(path):
        if args.reader_uid is None:return None
        command=['setpriv','--reuid='+str(args.reader_uid),'--regid='+str(args.reader_uid),'--clear-groups',sys.executable,'-I','-c',
                 'import hashlib,pathlib,sys; print(hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest())',str(path)]
        return subprocess.run(command,capture_output=True,timeout=5)
    for kind in ['agent','authority']:
        row={};output=public/(kind+'.cpio.gz')
        completed=build(ROOT,kind,output);assert completed.returncode==0,completed.stderr.decode()
        assert secret in gzip.decompress(output.read_bytes())
        assert output.stat().st_mode&0o777==0o600
        assert secret.strip() not in completed.stdout+completed.stderr
        row['contains_exact_synthetic_credential']=True
        row['mode']='0600'
        probe=reader(output)
        if probe is not None:
            assert probe.returncode!=0 and b'PermissionError' in probe.stderr
            row['unauthorized_uid_read_refused']=True
        for case in ['existing-file','symlink','hardlink','fifo','parent-symlink','unsafe-parent']:
            target=public/(kind+'-'+case);victim=private/(kind+'-'+case+'-victim');victim.write_bytes(b'SYNTHETIC_PRESERVE')
            if case=='existing-file': target.write_bytes(b'SYNTHETIC_PRESERVE')
            elif case=='symlink': target.symlink_to(victim)
            elif case=='hardlink': os.link(victim,target)
            elif case=='fifo': os.mkfifo(target,0o600)
            elif case=='parent-symlink':
                target.symlink_to(public,target_is_directory=True);target=target/'never-published.cpio.gz'
            else:
                target.mkdir(mode=0o777);target.chmod(0o777);target=target/'never-published.cpio.gz'
            original=target.lstat() if case in ['existing-file','symlink','hardlink','fifo'] else None
            failed=build(ROOT,kind,target);assert failed.returncode!=0,(kind,case)
            assert victim.read_bytes()==b'SYNTHETIC_PRESERVE'
            if original:
                current=target.lstat();assert (current.st_dev,current.st_ino,current.st_mode)==(original.st_dev,original.st_ino,original.st_mode)
                if case=='existing-file':assert target.read_bytes()==b'SYNTHETIC_PRESERVE'
            else: assert not target.exists()
            row[case+'_refused_without_replacement']=True
        broken_bin=private/(kind+'-broken-bin');broken_bin.mkdir(mode=0o700)
        broken_cpio=broken_bin/'cpio'
        broken_cpio.write_text('#!/bin/sh\ncat >/dev/null\nprintf synthetic-partial-cpio\nexit 17\n');broken_cpio.chmod(0o700)
        broken_env=dict(env,PATH=str(broken_bin)+':'+env.get('PATH','/usr/bin:/bin'))
        broken_output=public/(kind+'-producer-failed.cpio.gz')
        broken=build(ROOT,kind,broken_output,broken_env)
        assert broken.returncode!=0 and not broken_output.exists()
        row['failed_producer_leaves_no_output']=True
        if args.old_root:
            old_output=public/(kind+'-old.cpio.gz')
            old=build(args.old_root.resolve(),kind,old_output);assert old.returncode==0,old.stderr.decode()
            assert old_output.stat().st_mode&0o777==0o644 and secret in gzip.decompress(old_output.read_bytes())
            row['old_archive_contains_credential_mode']='0644'
            probe=reader(old_output)
            if probe is not None:
                assert probe.returncode==0 and probe.stdout.strip().decode()==hashlib.sha256(old_output.read_bytes()).hexdigest()
                row['old_unauthorized_uid_read_succeeded']=True
            victim=private/(kind+'-old-overwritten');victim.write_bytes(b'SYNTHETIC_VICTIM')
            alias=public/(kind+'-old-alias');alias.symlink_to(victim)
            old=build(args.old_root.resolve(),kind,alias);assert old.returncode==0
            assert secret in gzip.decompress(victim.read_bytes())
            row['old_builder_overwrote_symlink_target']=True
        result['builders'][kind]=row
    spec=importlib.util.spec_from_file_location('private_archive_publisher',ROOT/'scripts/publish-private-initramfs.py')
    publisher=importlib.util.module_from_spec(spec);spec.loader.exec_module(publisher)
    anchored=directory/'anchored';anchored.mkdir(mode=0o700)
    original=publisher.atomic_write
    def swap_parent(destination,writer):
        anchored.rename(directory/'held-parent')
        anchored.mkdir(mode=0o700)
        (anchored/'archive').write_bytes(b'REPLACEMENT_PARENT_CANARY')
        return original(destination,writer)
    publisher.atomic_write=swap_parent
    publisher.publish(str(anchored/'archive'),io.BytesIO(b'SYNTHETIC_ARCHIVE'))
    assert (anchored/'archive').read_bytes()==b'REPLACEMENT_PARENT_CANARY'
    assert (directory/'held-parent/archive').read_bytes()==b'SYNTHETIC_ARCHIVE'
    assert (directory/'held-parent/archive').stat().st_mode&0o777==0o600
    result['held_parent_rename_preserves_replacement_canary']=True
    publisher.atomic_write=original
    class BrokenInput:
        def read(self,size):raise OSError('synthetic input failure')
    failed=directory/'failed';failed.mkdir(mode=0o700)
    try:publisher.publish(str(failed/'archive'),BrokenInput())
    except OSError:pass
    else:raise AssertionError('broken input published an archive')
    assert not list(failed.iterdir())
    result['input_failure_leaves_no_partial_archive']=True
    if args.reader_uid is not None:result['unauthorized_reader_uid']=args.reader_uid
    print(json.dumps(result,indent=2))
print('G7_PRIVATE_BOOTSTRAP_ARCHIVE_PROOF_PASS')
