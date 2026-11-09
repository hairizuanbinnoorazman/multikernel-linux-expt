#!/usr/bin/env python3
"""Exercise one trusted compiled inspection CLI with owned synthetic files only."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile

FIELDS = {'version','component','scope','status','code','contents_observed','cleanup_authorized','admission_guaranteed','recovery_complete'}
MARKER = '.multikernel-create.json'


def snapshot(root):
    result = {}
    for path in [root, *root.iterdir()]:
        info = path.lstat()
        result[path.name] = (info.st_dev, info.st_ino, info.st_uid, info.st_mode, info.st_size,
            hashlib.sha256(path.read_bytes()).hexdigest() if stat.S_ISREG(info.st_mode) else None)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    os.umask(0o077)
    captured = 0
    with tempfile.TemporaryDirectory(prefix='intent-cli-') as temporary:
        root = Path(temporary)
        for kind in ['absent','file','fifo','symlink','quarantine','inventory-bound']:
            bundle = root/kind
            bundle.mkdir(mode=0o700)
            path = bundle/MARKER
            if kind == 'file':
                path.write_bytes(b'SYNTHETIC_PRIVATE_MARKER_DATA')
            elif kind == 'fifo':
                os.mkfifo(path,0o600)
            elif kind == 'symlink':
                path.symlink_to('/SYNTHETIC_PRIVATE_TARGET')
            elif kind == 'quarantine':
                prefix = hashlib.sha256(MARKER.encode()).hexdigest()[:16]
                (bundle/('.mklinux-remove-'+prefix+'-unknown')).write_bytes(b'SYNTHETIC_PRIVATE')
            elif kind == 'inventory-bound':
                for index in range(257):
                    (bundle/str(index)).write_bytes(b'')
            before = snapshot(bundle)
            command = [str(binary),'--inspect-create-intent','--bundle',str(bundle)]
            result = subprocess.run(command,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=3,env={'PATH':'/usr/bin:/bin'})
            assert not result.stderr and len(result.stdout)<=1024 and result.stdout.count(b'\n')==1
            report = json.loads(result.stdout)
            assert set(report)==FIELDS and report['version']==1
            assert report['component']=='containerd-shim-multikernel-v2' and report['scope']=='create-intent-marker'
            for field in ['contents_observed','cleanup_authorized','admission_guaranteed','recovery_complete']:
                assert report[field] is False
            expected = ('absent','CREATE_INTENT_NOT_OBSERVED',0) if kind=='absent' else ('inconclusive','CREATE_INTENT_UNAVAILABLE',1) if kind=='inventory-bound' else ('pending','CREATE_INTENT_PENDING',0)
            assert (report['status'],report['code'],result.returncode)==expected
            assert b'SYNTHETIC_PRIVATE' not in result.stdout and str(bundle).encode() not in result.stdout
            assert snapshot(bundle)==before
            captured += 1
        for options in [['--bundle','relative/SYNTHETIC_PRIVATE'],['--bundle',str(root/'file'),'--bundle','/SYNTHETIC_PRIVATE']]:
            result = subprocess.run([str(binary),'--inspect-create-intent',*options],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=3,env={'PATH':'/usr/bin:/bin'})
            report = json.loads(result.stdout)
            assert result.returncode==2 and not result.stderr and set(report)==FIELDS and report['code']=='CREATE_INTENT_ARGUMENTS_INVALID' and b'SYNTHETIC_PRIVATE' not in result.stdout
            captured += 1
        before = snapshot(root/'file')
        for transport in ['closed-reader','full']:
            reader,writer = os.pipe()
            try:
                flags = fcntl.fcntl(writer,fcntl.F_GETFL)
                if transport=='closed-reader':
                    os.close(reader)
                    reader = None
                else:
                    os.set_blocking(writer,False)
                    while True:
                        try:
                            os.write(writer,b'x'*4096)
                        except BlockingIOError:
                            break
                    os.set_blocking(writer,True)
                command = [str(binary),'--inspect-create-intent','--bundle',str(root/'file')]
                child = subprocess.Popen(command,stdin=subprocess.DEVNULL,stdout=writer,stderr=subprocess.PIPE,env={'PATH':'/usr/bin:/bin'})
                try:
                    _,stderr = child.communicate(timeout=3)
                except BaseException:
                    child.kill()
                    child.communicate(timeout=3)
                    raise
                assert child.returncode==1 and not stderr and fcntl.fcntl(writer,fcntl.F_GETFL)==flags
            finally:
                if reader is not None:
                    os.close(reader)
                os.close(writer)
        assert snapshot(root/'file')==before
    print(json.dumps(dict(scope='actual-compiled-create-intent-inspection-no-child',binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),captured_cases=captured,real_stdout_fault_cases=2,no_raw_private_data_observed=True,marker_contents_and_identities_preserved=True,child_kernel_started=False,full_g7_g8_qualified=False),sort_keys=True))


if __name__=='__main__':
    main()
