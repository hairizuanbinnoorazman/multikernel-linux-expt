#!/usr/bin/env python3
"""Negative cgroup and authority controls for the read-only live Docker audit."""
import hashlib
import ast
import copy
import importlib.util
from pathlib import Path
import os
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('live_audit',Path(__file__).with_name('audit-runtime-docker-trial.py'))
audit=importlib.util.module_from_spec(spec);spec.loader.exec_module(audit)

class AuditTests(unittest.TestCase):
    def test_actual_audit_refuses_exact_cap_before_parsing_guest_markers(self):
        tree=ast.parse(Path(audit.__file__).read_text())
        body=next(n.body for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='audit')
        start=next(i for i,n in enumerate(body) if isinstance(n,ast.Assign) and ast.unparse(n).startswith("stdout, stderr = (raw(private, 'workload.stdout'"))
        end=next(i for i,n in enumerate(body[start:],start) if isinstance(n,ast.If) and ast.unparse(n.test)=="scenario == 'pressure'")
        fragment=compile(ast.Module(body=body[start:end],type_ignores=[]),'<actual-audit-stream-bound>','exec')
        for length in [131071,131072,131073]:
            for stream in ['workload.stdout','workload.stderr']:
                env=dict(raw=lambda parent,name,limit:b'x'*length if name==stream else b'',private=Path('/synthetic'))
                with self.subTest(length=length,stream=stream):
                    if length>=131072:
                        with self.assertRaisesRegex(ValueError,'^attached workload reached its acceptance limit$'):exec(fragment,env)
                    else:exec(fragment,env)

    def test_actual_audit_rechecks_daemon_and_container_log_policy(self):
        from runtime_trial_output import logging_config
        # Execute the real audit's inspect/authority block, without granting
        # this fixture any live host, process or root authority.
        tree=ast.parse(Path(audit.__file__).read_text())
        body=next(n.body for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='audit')
        start=next(i for i,n in enumerate(body) if isinstance(n,ast.Assign) and ast.unparse(n).startswith("item = read(private, 'inspect.json')"))
        end=next(i for i,n in enumerate(body[start:],start) if isinstance(n,ast.Assign) and ast.unparse(n).startswith('nested ='))
        fragment=compile(ast.Module(body=body[start:end],type_ignores=[]),'<actual-audit-authority>','exec')
        item=dict(State=dict(Status='exited',ExitCode=0),Config=dict(User='1000:1000'),HostConfig=dict(Runtime='io.containerd.multikernel.v2',Privileged=False,CapDrop=['ALL'],LogConfig=dict(Type='local',Config=logging_config()['log-opts'])))
        original={'docker.json':dict(logging_config(),hosts=['unix:///synthetic/docker.sock']), 'docker-info.json':dict(LoggingDriver='local'), 'inspect.json':[item]}
        cases=[('valid',None),('daemon-driver',lambda v:v['docker.json'].update({'log-driver':'json-file'})),('daemon-options',lambda v:v['docker.json']['log-opts'].pop('max-size')),('observed-driver',lambda v:v['docker-info.json'].update(LoggingDriver='json-file')),('container-driver',lambda v:v['inspect.json'][0]['HostConfig']['LogConfig'].update(Type='json-file')),('container-options',lambda v:v['inspect.json'][0]['HostConfig']['LogConfig']['Config'].update({'max-file':'20'})),('missing-container-policy',lambda v:v['inspect.json'][0]['HostConfig'].pop('LogConfig'))]
        for name,mutate in cases:
            records=copy.deepcopy(original)
            if mutate:mutate(records)
            env=dict(read=lambda parent,name:records[name],private=Path('/synthetic'),qualify_trial_logging=audit.qualify_trial_logging)
            with self.subTest(case=name):
                if mutate:
                    with self.assertRaises(ValueError):exec(fragment,env)
                else:exec(fragment,env)

    def test_empty_retained_leaf_is_distinct_from_absence(self):
        with tempfile.TemporaryDirectory() as raw:
            directory=Path(raw);(directory/'cgroup.events').write_bytes(b'populated 0\nfrozen 0\n');(directory/'cgroup.procs').write_bytes(b'')
            audit.empty_cgroup(directory)
            with self.assertRaises(FileNotFoundError):audit.empty_cgroup(directory/'missing')

    def test_population_pid_descendants_aliases_and_replacement_refuse(self):
        with tempfile.TemporaryDirectory() as raw:
            d=Path(raw);events=d/'cgroup.events';procs=d/'cgroup.procs'
            for payload, pid in [(b'populated 1\n',b''),(b'populated 0\n',b'123\n'),(b'populated 0\npopulated 0\n',b''),(b'frozen 0\n',b'')]:
                events.write_bytes(payload);procs.write_bytes(pid)
                with self.assertRaises(ValueError):audit.empty_cgroup(d)
            events.write_bytes(b'populated 0\n');procs.write_bytes(b'');(d/'child').mkdir()
            with self.assertRaises(ValueError):audit.empty_cgroup(d)
            (d/'child').rmdir();procs.unlink();procs.symlink_to(events)
            with self.assertRaises(OSError):audit.empty_cgroup(d)
            procs.unlink();procs.write_bytes(b'');d.chmod(0o777)
            with self.assertRaises(ValueError):audit.empty_cgroup(d)
            d.chmod(0o700)

    def test_primary_alias_under_writable_parent_refuses(self):
        if os.geteuid()!=0:self.skipTest('root-owned primary alias control requires root')
        # Use a root-owned protected parent; /tmp's writable sticky parent is
        # intentionally not accepted for primary deployment aliases.
        with tempfile.TemporaryDirectory() as raw:
            d=Path(raw);f=d/'generation';f.write_bytes(b'approved');alias=d/'active';alias.symlink_to(f)
            with self.assertRaisesRegex(ValueError,'^unprotected primary watched path$'):audit.primary_digest(alias)

    def test_digest_preserves_empty_file_and_refuses_alias_shared_or_large_input(self):
        if os.geteuid()!=0:self.skipTest('root-owned audit input control requires root')
        with tempfile.TemporaryDirectory() as raw:
            d=Path(raw);f=d/'input';f.write_bytes(b'')
            self.assertEqual(audit.digest(f),hashlib.sha256(b'').hexdigest())
            (d/'alias').symlink_to(f)
            with self.assertRaises(OSError):audit.digest(d/'alias')
            os.link(f,d/'shared')
            with self.assertRaises(ValueError):audit.digest(f)
            (d/'shared').unlink();f.write_bytes(b'private-canary')
            with self.assertRaisesRegex(ValueError,'^unsafe audit input$'):audit.digest(f,1)

if __name__=='__main__':unittest.main()
