#!/usr/bin/env python3
import ast,json,os,pathlib,subprocess,sys,tempfile,time,unittest
from runtime_concurrency_command import capture,LIMIT
from runtime_bounded_command import CommandOutputLimit


def live_function(name,namespace):
    tree=ast.parse(pathlib.Path(__file__).with_name('runtime_docker_concurrency.py').read_text())
    outer=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='qualify')
    node=next(n for n in outer.body if isinstance(n,ast.FunctionDef) and n.name==name)
    exec(compile(ast.Module(body=[node],type_ignores=[]),'<actual-concurrent-'+name+'>','exec'),namespace)
    return namespace[name]


class ConcurrentCommandTests(unittest.TestCase):
    def invoke(self,root):
        return live_function('invoke',dict(time=time,capture_command=capture,private=root))

    def test_actual_invocation_refuses_both_stream_floods_before_writer_finishes(self):
        for fd in [1,2]:
            with self.subTest(fd=fd),tempfile.TemporaryDirectory() as raw:
                root=pathlib.Path(raw);done=root/'writer-completed'
                code='import os,pathlib\np=b"x"*(1<<20)\nn=0\nwhile n<len(p):n+=os.write('+str(fd)+',p[n:])\npathlib.Path('+repr(str(done))+').touch()'
                with self.assertRaises(CommandOutputLimit) as error:self.invoke(root)([sys.executable,'-c',code],'flood')
                self.assertFalse(done.exists());self.assertEqual(error.exception.stdout,b'');self.assertEqual(error.exception.stderr,b'')
                selected=root/('flood.stdout' if fd==1 else 'flood.stderr')
                self.assertEqual(selected.stat().st_size,LIMIT)
                self.assertTrue(json.loads((root/'flood-failure.json').read_text())['output_refused'])
                self.assertFalse((root/'flood.exit.json').exists())

    def test_existing_acceptance_boundary_and_nonzero_controls_are_preserved(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw);invoke=self.invoke(root)
            result=invoke([sys.executable,'-c','import os;os.write(1,b"x"*131071);os.write(2,b"y"*131071)'],'valid')
            self.assertEqual(result.returncode,0);self.assertEqual(len(result.stdout),LIMIT);self.assertEqual(len(result.stderr),LIMIT)
            result=invoke([sys.executable,'-c','import sys;print("private cause",file=sys.stderr);sys.exit(7)'],'negative',False)
            self.assertEqual(result.returncode,7)
            with self.assertRaisesRegex(ValueError,'^concurrent trial command failed; private evidence retained$'):
                invoke([sys.executable,'-c','import sys;print("private cause",file=sys.stderr);sys.exit(7)'],'failure')
            self.assertEqual((root/'failure.stderr').read_bytes(),b'private cause\n')

    def test_exact_cap_refuses_instead_of_publishing_exit_status(self):
        for fd in [1,2]:
            with self.subTest(fd=fd),tempfile.TemporaryDirectory() as raw:
                root=pathlib.Path(raw)
                with self.assertRaises(CommandOutputLimit):self.invoke(root)([sys.executable,'-c','import os;os.write('+str(fd)+',b"x"*131072)'],'boundary')
                self.assertFalse((root/'boundary.exit.json').exists())

    def test_expired_and_eof_deadlines_do_not_execute_or_grant_new_wait_budget(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw);done=root/'unexpected'
            with self.assertRaises(subprocess.TimeoutExpired) as error:
                self.invoke(root)([sys.executable,'-c','from pathlib import Path;Path('+repr(str(done))+').touch()'],'expired',timeout=0)
            self.assertNotIn(str(done),str(error.exception));self.assertFalse(done.exists())
            start=time.monotonic()
            with self.assertRaises(subprocess.TimeoutExpired):
                self.invoke(root)([sys.executable,'-c','import os,time;os.close(1);os.close(2);time.sleep(5)'],'eof',timeout=.15)
            self.assertLess(time.monotonic()-start,1.5)
            self.assertTrue(json.loads((root/'eof-timeout.json').read_text())['timed_out'])

    def test_private_existing_or_aliased_evidence_refuses_before_any_command(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw);invoke=self.invoke(root);previous=os.umask(0)
            try:invoke([sys.executable,'-c','print("controlled")'],'owned')
            finally:os.umask(previous)
            self.assertTrue(all(p.stat().st_mode&0o777==0o600 for p in root.iterdir()))
            before={p.name:p.read_bytes() for p in root.iterdir()};marker=root/'unexpected'
            code='from pathlib import Path;Path('+repr(str(marker))+').touch()'
            with self.assertRaisesRegex(ValueError,'^concurrent command evidence already exists$'):invoke([sys.executable,'-c',code],'owned')
            self.assertFalse(marker.exists());self.assertEqual(before,{p.name:p.read_bytes() for p in root.iterdir()})
            (root/'alias.stdout').symlink_to(root/'owned.stdout')
            with self.assertRaises(ValueError):invoke([sys.executable,'-c',code],'alias')
            self.assertFalse(marker.exists());self.assertFalse((root/'alias.stderr').exists())

    def test_actual_failure_diagnostics_keep_original_bounded_refusals(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw);invoke=self.invoke(root)
            ns=dict(invoke=invoke,cli=[sys.executable,'-c','import os;os.write(2,b"x"*(1<<20))'],cases=['a','b'],names=['one','two'],subprocess=subprocess,CommandOutputLimit=CommandOutputLimit)
            live_function('diagnose_failure',ns)()
            for case in ['a','b']:
                self.assertEqual((root/('failure-diagnostic-'+case+'.stderr')).stat().st_size,LIMIT)
                self.assertTrue((root/('failure-diagnostic-'+case+'-failure.json')).is_file())
                self.assertFalse((root/('failure-diagnostic-'+case+'.exit.json')).exists())

if __name__=='__main__':unittest.main()
