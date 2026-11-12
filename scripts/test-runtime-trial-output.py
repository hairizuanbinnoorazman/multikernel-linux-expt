#!/usr/bin/env python3
"""Actual attach expressions and kernel file caps; no Docker workload is run."""
import ast,json,os,pathlib,resource,subprocess,sys,tempfile,unittest
import runtime_trial_output as subject

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
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);out=root/'out';err=root/'err'
            command=subject.attach_command([sys.executable,'-c','import resource,json;print(json.dumps(resource.getrlimit(resource.RLIMIT_FSIZE)))'])
            with out.open('wb') as stdout,err.open('wb') as stderr:r=subprocess.run(command,stdout=stdout,stderr=stderr,timeout=5)
            self.assertEqual(r.returncode,0);self.assertEqual(err.read_bytes(),b'')
            self.assertEqual(json.loads(subject.read_output(out)),[subject.ATTACH_LIMIT]*2)
            out.write_bytes(b'x'*(subject.ATTACH_LIMIT-1));self.assertEqual(len(subject.read_output(out)),subject.ATTACH_LIMIT-1)
            out.write_bytes(b'x'*subject.ATTACH_LIMIT)
            with self.assertRaises(ValueError):subject.read_output(out)
            out.write_bytes(b'x'*(subject.ATTACH_LIMIT+1))
            with self.assertRaises(ValueError):subject.read_output(out)
        self.assertEqual(resource.getrlimit(resource.RLIMIT_FSIZE),before)

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
        config_calls=[n for n in ast.walk(tree) if isinstance(n,ast.Call) and isinstance(n.func,ast.Attribute) and isinstance(n.func.value,ast.Name) and n.func.value.id=='dc' and n.func.attr=='write_text']
        self.assertEqual(len(config_calls),1)
        namespace=dict(json=json,private=private,sock=private/'containerd.sock',dsock=private/'docker.sock',logging_config=subject.logging_config)
        config=json.loads(eval(compile(ast.Expression(config_calls[0].args[0]),'<private-daemon-config>','eval'),namespace))
        expected=subject.logging_config()
        self.assertEqual({key:config[key] for key in expected},expected)
        create=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='created' for t in n.targets)]
        self.assertEqual(len(create),1)
        argv=eval(compile(ast.Expression(create[0].args[0]),'<private-create-command>','eval'),dict(cli=['/synthetic/docker'],private=private,workload=['/bin/true'],logging_arguments=subject.logging_arguments))
        index=argv.index('--log-driver');kind=argv[index+1]
        options=dict(argv[i+1].split('=',1) for i,value in enumerate(argv) if value=='--log-opt')
        subject.require_logging(dict(Type=kind,Config=options))
        for value in [None,{},dict(Type='json-file',Config=options),dict(Type=kind,Config={})]:
            with self.assertRaises(ValueError):subject.require_logging(value)

if __name__=='__main__':unittest.main()
