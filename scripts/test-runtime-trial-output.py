#!/usr/bin/env python3
"""Actual attach expressions and kernel file caps; no Docker workload is run."""
import ast,json,os,pathlib,resource,subprocess,sys,tempfile,unittest
import runtime_trial_output as subject
import runtime_deployment_logging as deployment
from unittest.mock import patch

ROOT=pathlib.Path(__file__).resolve().parent

def attach_expressions():
    rows=[]
    for filename in ['test-runtime-docker-profile-live.py','runtime_docker_concurrency.py']:
        tree=ast.parse((ROOT/filename).read_text())
        for node in ast.walk(tree):
            if isinstance(node,ast.Call) and isinstance(node.func,ast.Attribute) and isinstance(node.func.value,ast.Name) and node.func.value.id=='subprocess' and node.func.attr in ['Popen','run'] and node.args and any(isinstance(x,ast.Constant) and x.value=='--attach' for x in ast.walk(node.args[0])):
                rows.append((filename,node.lineno,node.args[0]))
    return rows

class TrialOutput(unittest.TestCase):
    def test_all_four_live_attach_expressions_have_kernel_file_caps(self):
        rows=attach_expressions();self.assertEqual(len(rows),4)
        parent=resource.getrlimit(resource.RLIMIT_FSIZE)
        for filename,_,expression in rows:
            for fd in [1,2]:
                with self.subTest(source=filename,fd=fd),tempfile.TemporaryDirectory() as directory:
                    root=pathlib.Path(directory);stdout=root/'stdout';stderr=root/'stderr'
                    code='import os,sys\np=b"x"*(1<<20)\nn=0\ntry:\n while n<len(p): n+=os.write('+str(fd)+',p[n:])\nexcept OSError: sys.exit(27)\n'
                    cli=[sys.executable,'-c',code]
                    argv=eval(compile(ast.Expression(expression),'<live-attach-command>','eval'),dict(cli=cli,name='synthetic',attach_command=subject.attach_command))
                    with stdout.open('wb') as out,stderr.open('wb') as err:r=subprocess.run(argv,stdout=out,stderr=err,timeout=5)
                    self.assertNotEqual(r.returncode,0)
                    selected=stdout if fd==1 else stderr
                    self.assertEqual(selected.stat().st_size,subject.ATTACH_LIMIT)
                    with self.assertRaises(ValueError):subject.read_output(selected)
        self.assertEqual(resource.getrlimit(resource.RLIMIT_FSIZE),parent)

    def test_normal_output_and_child_only_resource_limits(self):
        before=resource.getrlimit(resource.RLIMIT_FSIZE)
        core_before=resource.getrlimit(resource.RLIMIT_CORE)
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);out=root/'out';err=root/'err'
            command=subject.attach_command([sys.executable,'-c','import resource,json;print(json.dumps(dict(file=resource.getrlimit(resource.RLIMIT_FSIZE),core=resource.getrlimit(resource.RLIMIT_CORE))))'])
            with out.open('wb') as stdout,err.open('wb') as stderr:r=subprocess.run(command,stdout=stdout,stderr=stderr,timeout=5)
            self.assertEqual(r.returncode,0);self.assertEqual(err.read_bytes(),b'')
            self.assertEqual(json.loads(subject.read_output(out)),dict(file=[subject.ATTACH_LIMIT]*2,core=[0,0]))
            out.write_bytes(b'x'*(subject.ATTACH_LIMIT-1));self.assertEqual(len(subject.read_output(out)),subject.ATTACH_LIMIT-1)
            out.write_bytes(b'x'*subject.ATTACH_LIMIT)
            with self.assertRaises(ValueError):subject.read_output(out)
            out.write_bytes(b'x'*(subject.ATTACH_LIMIT+1))
            with self.assertRaises(ValueError):subject.read_output(out)
        self.assertEqual(resource.getrlimit(resource.RLIMIT_FSIZE),before)
        self.assertEqual(resource.getrlimit(resource.RLIMIT_CORE),core_before)

    def test_reader_refuses_aliases_and_special_files_without_blocking(self):
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);file=root/'data';file.write_bytes(b'synthetic')
            link=root/'link';link.symlink_to(file.name)
            with self.assertRaises(OSError):subject.read_output(link)
            fifo=root/'fifo';os.mkfifo(fifo)
            with self.assertRaises(ValueError):subject.read_output(fifo)
        for argv in [[],[''],None,'/bin/true',[b'/bin/true'],['/bin/true','\x00']]:
            with self.assertRaises(ValueError):subject.attach_command(argv)

    def test_actual_private_daemon_config_and_create_flags_agree(self):
        tree=ast.parse((ROOT/'test-runtime-docker-profile-live.py').read_text());private=pathlib.Path('/synthetic/private')
        config_calls=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='dc' for t in n.targets)]
        self.assertEqual(len(config_calls),1)
        with tempfile.TemporaryDirectory() as directory:
            work=pathlib.Path(directory);private=work/'integration';private.mkdir(mode=0o700)
            source=work/'source/scripts';source.mkdir(parents=True,mode=0o700)
            for name in ['merge-runtime-docker-config.py','runtime_docker_logging_policy.py']:
                (source/name).write_bytes((ROOT/name).read_bytes());(source/name).chmod(0o500)
            (work/'build.json').write_text(json.dumps(dict(source_commit='a'*40)))
            captured=[];actual=deployment.capture
            def command(argv,*args):
                captured.append(argv)
                if '/usr/bin/dockerd' in argv:
                    self.assertEqual(argv[-4:],['/usr/bin/dockerd','--validate','--config-file',str(private/'docker.json')])
                    # Installed-parser behavior has its separate GCE proof.
                    return subprocess.CompletedProcess(argv,0,b'configuration OK\n',b'')
                return actual(argv,*args)
            with patch.object(deployment,'capture',command):
                path=eval(compile(ast.Expression(config_calls[0]),'<private-daemon-config>','eval'),dict(prepare_deployment_logging=deployment.prepare,work=work))
            self.assertEqual(len(captured),2)
            self.assertEqual(captured[0][-1],'--enable-bounded-logging')
            config=json.loads(path.read_text());expected=subject.logging_config()
            self.assertEqual({key:config[key] for key in expected},expected)
            self.assertEqual(config['default-runtime'],'runc')
            self.assertEqual(path.stat().st_mode&0o777,0o600)
        assignments=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='create_argv' for t in n.targets)]
        self.assertEqual(len(assignments),2)
        expressions=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='created' for t in n.targets)]
        self.assertEqual(len(expressions),1)
        self.assertIsInstance(expressions[0].args[0],ast.Name)
        self.assertEqual(expressions[0].args[0].id,'create_argv')
        for inherited in [False,True]:
            namespace=dict(cli=['/usr/bin/docker','--host','unix://'+str(private/'docker.sock')],private=private,workload=['/bin/true'],logging_arguments=subject.logging_arguments,child_log_pressure=inherited,inherited_create_argv=deployment.inherited_create_argv)
            argv=eval(compile(ast.Expression(assignments[0]),'<explicit-create-command>','eval'),namespace)
            if inherited:
                argv=eval(compile(ast.Expression(assignments[1]),'<inherited-create-command>','eval'),namespace)
                self.assertEqual(argv,deployment.inherited_create_argv(private,['/bin/true']))
                self.assertFalse(any(arg.startswith(('--log-driver','--log-opt')) for arg in argv))
            else:
                explicit=argv
        argv=explicit
        index=argv.index('--log-driver');kind=argv[index+1]
        options=dict(argv[i+1].split('=',1) for i,value in enumerate(argv) if value=='--log-opt')
        subject.require_logging(dict(Type=kind,Config=options))
        for value in [None,{},dict(Type='json-file',Config=options),dict(Type=kind,Config={})]:
            with self.assertRaises(ValueError):subject.require_logging(value)

if __name__=='__main__':unittest.main()
