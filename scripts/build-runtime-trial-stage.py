#!/usr/bin/env python3
"""Build a protected disposable-trial packet from one committed source snapshot."""
import argparse
import ast
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess as sp
import tarfile

CONTAINERD_SHA = 'dbc252c7f4e3534dcbe674552b87b20194967e7b6069e66de61632dcdd3acc83'
ROOTS = ('scripts', 'guest', 'deploy', 'runtime', 'tools')
PACKAGES = {'mkruntimed':'./cmd/mkruntimed', 'mknetd':'./cmd/mknetd',
            'containerd-shim-multikernel-v2':'./cmd/containerd-shim-multikernel-v2'}


def sha(path):
    with path.open('rb') as file:
        return hashlib.file_digest(file, 'sha256').hexdigest()


def protected_parent(path):
    if not path.is_absolute() or '..' in path.parts:
        raise ValueError('stage output must be an absolute new path')
    for parent in [path.parent, *path.parent.parents]:
        info = parent.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid not in (0, os.geteuid()) or
                (info.st_mode & 0o022 and not (info.st_uid == 0 and info.st_mode & stat.S_ISVTX))):
            raise ValueError('stage output has an unprotected parent')


def copy_pinned(source, target, digest):
    fd = os.open(source, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW | os.O_CLOEXEC)
    with os.fdopen(fd, 'rb') as file:
        info = os.fstat(file.fileno())
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 128 << 20:
            raise ValueError('input artifact is not a bounded regular file')
        with target.open('xb') as output:
            total = 0
            while chunk := file.read(1 << 20):
                total += len(chunk)
                if total > 128 << 20:
                    raise ValueError('input artifact exceeds its bound')
                output.write(chunk)
    target.chmod(0o500)
    if sha(target) != digest:
        raise ValueError('input artifact differs from its committed pin')


def build(args):
    source_root = args.source_root.resolve()
    revision = sp.check_output(['git','rev-parse','--verify','--end-of-options',args.revision+'^{commit}'],cwd=source_root,text=True).strip()
    output = args.output
    protected_parent(output)
    output.mkdir(mode=0o700)  # Never replace or resume an existing generation.
    (output/'bin').mkdir(mode=0o700)
    (output/'source').mkdir(mode=0o700)
    archive = output/'source.tar'
    with archive.open('xb') as file:
        sp.run(['git','-c','tar.umask=0022','archive',revision,*ROOTS],cwd=source_root,stdout=file,check=True)
    archive.chmod(0o600)
    with tarfile.open(archive) as snapshot:
        if snapshot.pax_headers.get('comment') != revision:
            raise ValueError('source archive lacks the selected commit identity')
        for member in snapshot.getmembers():
            path = Path(member.name)
            if (path.is_absolute() or '..' in path.parts or not path.parts or
                    path.parts[0] not in ROOTS or not (member.isdir() or member.isfile())):
                raise ValueError('unsupported committed source member')
            target = output/'source'/path
            if member.isdir():
                target.mkdir(mode=0o755,parents=True,exist_ok=True)
            else:
                target.parent.mkdir(mode=0o755,parents=True,exist_ok=True)
                with snapshot.extractfile(member) as src, target.open('xb') as dst:
                    while chunk := src.read(1 << 20):
                        dst.write(chunk)
                target.chmod(0o755 if member.mode & 0o111 else 0o644)
    tree = output/'source'
    assignments = ast.parse((tree/'scripts/runtime_trial_helper.py').read_text()).body
    pins = [ast.literal_eval(node.value) for node in assignments if isinstance(node,ast.Assign)
            and any(isinstance(name,ast.Name) and name.id=='PINS' for name in node.targets)]
    if len(pins)!=1 or args.helper_variant not in pins[0] or not re.fullmatch('[0-9a-f]{64}',pins[0][args.helper_variant]):
        raise ValueError('selected helper lacks one committed pin')
    relay_pins = [ast.literal_eval(node.value) for node in assignments if isinstance(node,ast.Assign)
                  and any(isinstance(name,ast.Name) and name.id=='RELAY_PIN' for name in node.targets)]
    if len(relay_pins)!=1 or not isinstance(relay_pins[0],str) or not re.fullmatch('[0-9a-f]{64}',relay_pins[0]):
        raise ValueError('relay lacks one committed pin')
    copy_pinned(args.containerd,output/'bin/containerd',CONTAINERD_SHA)
    copy_pinned(args.helper,output/'bin/mkvsock-nbd',pins[0][args.helper_variant])
    copy_pinned(args.relay,output/'bin/mkvsock-relay',relay_pins[0])
    env = {**os.environ,'CGO_ENABLED':'0','GOOS':'linux','GOARCH':'amd64','GOAMD64':'v1','GOWORK':'off','GOFLAGS':''}
    for name,package in PACKAGES.items():
        sp.run(['go','build','-mod=readonly','-trimpath','-buildvcs=false',
                '-ldflags=-X github.com/hairizuan/multikernel-linux-expt/runtime/internal/buildinfo.Revision='+revision[:7],
                '-o',str(output/'bin'/name),package],cwd=tree/'runtime',env=env,check=True)
        (output/'bin'/name).chmod(0o500)
    report = dict(schema_version=1,source_commit=revision,source_archive_sha256=sha(archive),
                  helper_variant=args.helper_variant,helper_sha256=pins[0][args.helper_variant],
                  relay_sha256=relay_pins[0],relay_interface='server-peer',
                  binaries={path.name:sha(path) for path in (output/'bin').iterdir()},
                  go_version=sp.check_output(['go','version'],cwd=tree/'runtime',env=env,text=True).strip(),
                  build_info={name:sp.check_output(['go','version','-m',str(output/'bin'/name)],env=env,text=True) for name in PACKAGES},
                  built_from_committed_snapshot=True,installed=False,full_trial_started=False)
    (output/'build.json').write_text(json.dumps(report,indent=2)+'\n')
    (output/'build.json').chmod(0o600)
    # Normalize packet ownership; privileged extraction must retain these modes.
    def packet_owner(info):
        info.uid=info.gid=0;info.uname=info.gname='root'
        return info
    with tarfile.open(output/'stage.tar.gz','w:gz') as packet:
        for name in ['source.tar','build.json','bin']:
            packet.add(output/name,arcname=name,filter=packet_owner)
    (output/'stage.tar.gz').chmod(0o600)
    print(json.dumps(dict(source_commit=revision,packet_sha256=sha(output/'stage.tar.gz'),installed=False,full_trial_started=False)))


if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-root',type=Path,default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--revision',default='HEAD')
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--containerd',type=Path,required=True)
    parser.add_argument('--helper',type=Path,required=True)
    parser.add_argument('--relay',type=Path,required=True)
    parser.add_argument('--helper-variant',choices=('qualified','server-queue-candidate'),default='qualified')
    build(parser.parse_args())
