#!/usr/bin/env python3
"""Real miniature Go build: committed snapshot, hostile ambient flags and modes."""
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess as sp
import tarfile
import tempfile
import unittest

ROOT=Path(__file__).resolve().parent.parent
BUILDER=ROOT/'scripts/build-runtime-trial-stage.py'

class TrialStageTests(unittest.TestCase):
    def invoke(self, source, output, containerd, helper, **kw):
        return sp.run(['python3',str(BUILDER),'--source-root',str(source),'--output',str(output),
                       '--containerd',str(containerd),'--helper',str(helper),'--relay',str(helper.parent/'relay')],stdout=sp.PIPE,stderr=sp.PIPE,timeout=180,**kw)

    def test_missing_relay_refused_before_output_creation(self):
        with tempfile.TemporaryDirectory() as directory:
            output=Path(directory)/'stage'
            result=sp.run(['python3',str(BUILDER),'--output',str(output),
                           '--containerd','/absent','--helper','/absent'],stdout=sp.PIPE,stderr=sp.PIPE,timeout=5)
            self.assertEqual(result.returncode,2)
            self.assertIn(b'--relay',result.stderr)
            self.assertFalse(output.exists())

    def test_existing_generation_and_unsafe_parent_refused_without_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);existing=root/'existing';existing.mkdir();sentinel=existing/'retained';sentinel.write_bytes(b'unchanged')
            self.assertNotEqual(self.invoke(ROOT,existing,root/'absent',root/'absent').returncode,0)
            self.assertEqual(sentinel.read_bytes(),b'unchanged');self.assertEqual(list(existing.iterdir()),[sentinel])
            (root/'link').symlink_to(existing,target_is_directory=True)
            self.assertNotEqual(self.invoke(ROOT,root/'link'/'new',root/'absent',root/'absent').returncode,0)
            self.assertFalse((existing/'new').exists())
            unsafe=root/'unsafe';unsafe.mkdir();unsafe.chmod(0o777)
            self.assertNotEqual(self.invoke(ROOT,unsafe/'new',root/'absent',root/'absent').returncode,0)
            self.assertFalse((unsafe/'new').exists())

    def test_committed_source_built_and_packet_has_protected_modes(self):
        containerd=os.environ.get('MK_CONTAINERD')
        if not containerd:self.skipTest('MK_CONTAINERD qualified artifact required for real packet build')
        with tempfile.TemporaryDirectory() as directory:
            base=Path(directory);repo=base/'repo';repo.mkdir()
            for name in ['scripts','guest','deploy','runtime','tools']:(repo/name).mkdir();(repo/name/'.keep').write_bytes(b'')
            (repo/'runtime/go.mod').write_text('module trialfixture\n\ngo 1.26.9\n')
            names=['mkruntimed','mknetd','containerd-shim-multikernel-v2']
            for name in names:
                target=repo/'runtime/cmd'/name;target.mkdir(parents=True)
                (target/'main.go').write_text('package main\nimport "fmt"\nfunc main(){fmt.Println("committed")}\n')
            helper=base/'helper';helper.write_bytes(b'opaque committed pin fixture, never executed')
            digest=hashlib.sha256(helper.read_bytes()).hexdigest()
            relay=base/'relay';relay.write_bytes(b'opaque committed CID relay fixture, never executed')
            relay_digest=hashlib.sha256(relay.read_bytes()).hexdigest()
            (repo/'scripts/runtime_trial_helper.py').write_text('PINS = '+repr({'qualified':digest})+'\nRELAY_PIN = '+repr(relay_digest)+'\n')
            def git(*args):return sp.check_output(['git',*args],cwd=repo,stderr=sp.PIPE,text=True).strip()
            git('init','-q');git('config','tar.umask','0000');git('add','.')
            git('-c','user.name=Fixture','-c','user.email=fixture@invalid','commit','-qm','committed snapshot')
            commit=git('rev-parse','HEAD')
            for name in names:(repo/'runtime/cmd'/name/'main.go').write_text('package main\nfunc main(){panic("dirty worktree used")}\n')
            (repo/'scripts/runtime_trial_helper.py').write_text('raise RuntimeError("dirty support executed")\n')
            output=base/'stage'
            prior=os.umask(0o002)
            try:
                result=self.invoke(repo,output,containerd,helper,env={**os.environ,'GOWORK':str(base/'absent.work'),'GOFLAGS':'-overlay='+str(base/'absent-overlay')})
            finally:os.umask(prior)
            self.assertEqual(result.returncode,0,result.stderr.decode())
            report=json.loads((output/'build.json').read_text());self.assertEqual(report['source_commit'],commit)
            self.assertTrue(report['built_from_committed_snapshot']);self.assertFalse(report['installed']);self.assertFalse(report['full_trial_started'])
            self.assertEqual(report['relay_sha256'],relay_digest)
            self.assertEqual(report['relay_interface'],'server-peer')
            for name in names:self.assertEqual(sp.check_output([str(output/'bin'/name)]),b'committed\n')
            for name in ['', 'bin', 'source']:self.assertEqual(stat.S_IMODE((output/name).stat().st_mode),0o700)
            for path in (output/'bin').iterdir():self.assertEqual(stat.S_IMODE(path.stat().st_mode),0o500)
            with tarfile.open(output/'stage.tar.gz') as packet:
                for member in packet.getmembers():
                    self.assertEqual((member.uid,member.gid),(0,0))
                    self.assertFalse(member.mode&0o022)
                    if member.name=='bin':self.assertEqual(member.mode,0o700)
                    if member.name.startswith('bin/'):self.assertEqual(member.mode,0o500)
            with tarfile.open(output/'source.tar') as source_packet:
                for member in source_packet.getmembers():
                    self.assertEqual((member.uid,member.gid),(0,0))
                    self.assertFalse(member.mode&0o022,member.name)
                    if member.isdir():self.assertEqual(member.mode,0o755)
            self.assertEqual((output/'bin/mkvsock-nbd').read_bytes(),helper.read_bytes())
            self.assertEqual((output/'bin/mkvsock-relay').read_bytes(),relay.read_bytes())
            print('REAL_COMMITTED_GO_SNAPSHOT_DIRTY_WORKTREE_AMBIENT_FLAG_ISOLATION_PROTECTED_PACKET_PASS')
            original=helper.read_bytes();helper.write_bytes(original+b'corruption')
            wrong=base/'wrong-pin'
            refused=self.invoke(repo,wrong,containerd,helper)
            self.assertNotEqual(refused.returncode,0)
            self.assertIn(b'input artifact differs from its committed pin',refused.stderr)
            self.assertFalse((wrong/'stage.tar.gz').exists());self.assertFalse((wrong/'build.json').exists())
            helper.write_bytes(original)
            original_relay=relay.read_bytes();relay.write_bytes(original_relay+b'corruption')
            wrong=base/'wrong-relay-pin'
            refused=self.invoke(repo,wrong,containerd,helper)
            self.assertNotEqual(refused.returncode,0)
            self.assertIn(b'input artifact differs from its committed pin',refused.stderr)
            self.assertFalse((wrong/'stage.tar.gz').exists());self.assertFalse((wrong/'build.json').exists())
            relay.write_bytes(original_relay)
            print('COMMITTED_RELAY_PIN_COPIED_CORRUPT_RELAY_REFUSED_UNPUBLISHED_PASS')
            outside=base/'outside-secret';outside.write_bytes(b'synthetic private source boundary')
            (repo/'tools/outside').symlink_to(outside)
            git('add','tools/outside');git('-c','user.name=Fixture','-c','user.email=fixture@invalid','commit','-qm','unsafe source link')
            linked=base/'source-link'
            refused=self.invoke(repo,linked,containerd,helper)
            self.assertNotEqual(refused.returncode,0)
            self.assertIn(b'unsupported committed source member',refused.stderr)
            self.assertFalse((linked/'stage.tar.gz').exists());self.assertFalse((linked/'build.json').exists())
            self.assertEqual(outside.read_bytes(),b'synthetic private source boundary')
            print('COMMITTED_HELPER_PIN_MISMATCH_AND_SOURCE_SYMLINK_REFUSE_UNPUBLISHED_PASS')


if __name__=='__main__':unittest.main()
