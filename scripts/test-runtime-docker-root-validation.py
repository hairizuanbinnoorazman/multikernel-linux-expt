#!/usr/bin/env python3
"""Image approval and copied-input isolation regressions, without privileged work."""
import copy
import hashlib
import json
import os
from pathlib import Path
import runpy
import tempfile
import unittest

module = runpy.run_path(str(Path(__file__).with_name('validate-runtime-docker-root.py')))
validate = module['validate']
Error = module['ApprovalError']

class DockerRootTests(unittest.TestCase):
    def setUp(self):
        self.entries = [{'path':'.','type':'directory'}, {'path':'etc','type':'directory'}, {'path':'work','type':'directory'}]
    def check(self, entries=None, binds=(), accepted=True, wrong=False):
        raw = json.dumps({'schema_version':1,'entries':self.entries if entries is None else entries}).encode()
        info = {'docker_guest':{'kind':'docker-rootless-offline-v1','rootfs_manifest_sha256':'0'*64 if wrong else hashlib.sha256(raw).hexdigest()}}
        plan = {'schema_version':1,'readonly_binds':[{'destination':dest,'source':'synthetic-host-source'} for dest in binds]}
        if accepted:
            result=validate(info,raw,plan)
            self.assertTrue(result['source_bound']);self.assertEqual(result['copied_input_count'],len(binds))
        else:
            with self.assertRaises(Error): validate(info,raw,plan)
    def test_approved_image_and_context(self):
        self.check(binds=['/work/context','/etc/hosts','/etc/hostname','/etc/resolv.conf'])
    def test_other_image_refused(self):self.check(wrong=True,accepted=False)
    def test_critical_destinations_refused(self):
        for dest in ['/','/usr','/usr/bin/newuidmap','/etc/subuid','/etc/docker','/run','/home/rootless','/work/../usr','/work//x','//work','/workload']:
            with self.subTest(dest=dest):self.check(binds=[dest],accepted=False)
    def test_aliases_cannot_turn_context_into_privileged_input(self):
        for path,kind in [('work','symlink'),('work/project','symlink'),('etc/hosts','symlink')]:
            entries=[e for e in self.entries if e['path']!=path]+[{'path':path,'type':kind,'target':'/usr'}]
            self.check(entries,binds=['/work/project' if path.startswith('work') else '/etc/hosts'],accepted=False)
    def test_nested_aliases_and_hardlinks_refused(self):
        for entry in [{'path':'work/project/a','type':'symlink','target':'/usr'}, {'path':'work/project/a','type':'regular','hardlink':'shared-with-helper'}]:
            self.check(self.entries+[entry],binds=['/work'],accepted=False)
    def test_unrelated_image_symlinks_allowed(self):
        self.check(self.entries+[{'path':'bin/sh','type':'symlink','target':'/bin/busybox'}],binds=['/work/context'])
    def test_metadata_reader_refuses_fifo_link_and_writes(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory=Path(temporary);path=directory/'metadata'
            path.write_text('{}');path.chmod(0o600)
            self.assertEqual(module['read_metadata'](path),b'{}')
            alias=directory/'alias';alias.symlink_to(path)
            with self.assertRaises(OSError):module['read_metadata'](alias)
            fifo=directory/'fifo';os.mkfifo(fifo)
            with self.assertRaises(Error):module['read_metadata'](fifo)
            link=directory/'link';os.link(path,link)
            with self.assertRaises(Error):module['read_metadata'](path)
            link.unlink();path.chmod(0o666)
            with self.assertRaises(Error):module['read_metadata'](path)

if __name__=='__main__':unittest.main()
