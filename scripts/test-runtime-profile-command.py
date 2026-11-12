#!/usr/bin/env python3
import ast,pathlib,subprocess,sys,tempfile,time,unittest
from runtime_trial_command import capture
from runtime_bounded_command import CommandOutputLimit

SOURCE=pathlib.Path(__file__).with_name('test-runtime-docker-profile-live.py')


def expression(name):
    tree=ast.parse(SOURCE.read_text())
    rows=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and isinstance(n.value,ast.Call) and any(isinstance(t,ast.Name) and t.id==name for t in n.targets)]
    assert len(rows)==1
    return rows[0]


def invoke(name,private,code,deadline=None):
    ns=dict(cli=[sys.executable,'-c',code],private=private,time=time,capture_owned_command=capture,
            probe_script='controlled',fault_deadline=deadline if deadline is not None else time.monotonic()+5)
    return eval(compile(ast.Expression(expression(name)),'<actual-profile-'+name+'>','eval'),ns)


class ProfileCommandTests(unittest.TestCase):
    def test_actual_rebuild_and_hidden_probe_refuse_floods_before_writer_completion(self):
        for name in ['rebuilt','probe']:
            for fd in [1,2]:
                with self.subTest(name=name,fd=fd),tempfile.TemporaryDirectory() as raw:
                    root=pathlib.Path(raw);done=root/'finished'
                    code='import os,pathlib\np=b"x"*(1<<20)\nn=0\nwhile n<len(p):n+=os.write('+str(fd)+',p[n:])\npathlib.Path('+repr(str(done))+').touch()'
                    with self.assertRaises(CommandOutputLimit) as error:invoke(name,root,code)
                    self.assertFalse(done.exists());self.assertEqual(error.exception.stdout,b'');self.assertEqual(error.exception.stderr,b'');self.assertIsNone(error.exception.__context__)
                    label='post-recovery-workload' if name=='rebuilt' else 'record-hidden-child'
                    self.assertEqual((root/(label+('.stdout' if fd==1 else '.stderr'))).stat().st_size,131071)
                    self.assertFalse((root/(label+'.exit.json')).exists())

    def test_actual_rebuild_preserves_accepted_streams_and_fixed_failure(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw)
            result=invoke('rebuilt',root,'import os;os.write(1,b"x"*131071);os.write(2,b"y"*131071)')
            self.assertEqual(result.returncode,0);self.assertEqual(result.stdout,(root/'post-recovery-workload.stdout').read_bytes());self.assertEqual(len(result.stderr),131071)
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw)
            with self.assertRaisesRegex(ValueError,'^trial command failed; private evidence retained$'):invoke('rebuilt',root,'import sys;print("private guest cause",file=sys.stderr);sys.exit(7)')
            self.assertEqual((root/'post-recovery-workload.stderr').read_bytes(),b'private guest cause\n')

    def test_actual_hidden_probe_preserves_stricter_sum_and_exact_marker_guards(self):
        tree=ast.parse(SOURCE.read_text());statement=next(n for n in ast.walk(tree) if isinstance(n,ast.Assign) and isinstance(n.value,ast.Call) and any(isinstance(t,ast.Name) and t.id=='probe' for t in n.targets))
        body=next(value for n in ast.walk(tree) for _,value in ast.iter_fields(n) if isinstance(value,list) and statement in value)
        index=body.index(statement);guards=[]
        for n in body[index+1:]:
            if isinstance(n,ast.Assert):guards.append(n)
            else:break
        self.assertEqual(len(guards),2)
        compiled=compile(ast.Module(body=guards,type_ignores=[]),'<actual-hidden-probe-guards>','exec')
        marker=b'G8_NATIVE_RECORD_HIDDEN_CHILD_DOCKER_PASS\n'
        for payload,err,valid in [(marker,b'',True),(marker+b'extra',b'',False),(marker,b'x'*(65536-len(marker)),False)]:
            with tempfile.TemporaryDirectory() as raw:
                root=pathlib.Path(raw);result=invoke('probe',root,'import os;os.write(1,'+repr(payload)+');os.write(2,'+repr(err)+')')
                if valid:exec(compiled,dict(probe=result))
                else:
                    with self.assertRaises(AssertionError):exec(compiled,dict(probe=result))

    def test_hidden_probe_uses_the_original_fault_deadline_without_execution(self):
        with tempfile.TemporaryDirectory() as raw:
            root=pathlib.Path(raw);marker=root/'must-not-exist'
            with self.assertRaises(subprocess.TimeoutExpired) as error:invoke('probe',root,'from pathlib import Path;Path('+repr(str(marker))+').touch()',time.monotonic()-1)
            self.assertFalse(marker.exists());self.assertNotIn(str(marker),str(error.exception));self.assertIsNone(error.exception.__context__)

if __name__=='__main__':unittest.main()
