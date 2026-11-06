#!/usr/bin/env python3
import copy,hashlib,json,os
from pathlib import Path
import tempfile,unittest
from unittest import mock
import runtime_stage_binding as binding

class StageBindingTests(unittest.TestCase):
    def record(self):
        return dict(version=1,capture_boot='6349e148-e008-4fa9-a36c-83549cfe2501',volume='/srv/multikernel-storage',uuid='11111111-1111-4111-8111-111111111111',backing='/srv/multikernel-storage/g8-stage-fixture',alias='/opt/mkruntime/g8-host-fixture',backing_inode=100,underlay_inode=200,alias_parent_inode=300)
    def test_schema_refuses_legacy_numeric_alias_private_extra_and_unsafe_paths(self):
        value=self.record();self.assertEqual(binding.validate_record(value),value)
        for key,item in [('version',True),('version',1.0),('backing_inode',True),('uuid','private-canary'),('alias','/etc'),('backing','/srv/multikernel-storage/../other'),('private','secret')]:
            changed=dict(value,**{key:item})
            with self.subTest(key=key),self.assertRaisesRegex(ValueError,'^stage binding identity or state is unqualified$'):binding.validate_record(changed)
        with self.assertRaises(ValueError):binding.validate_record(dict(value,backing=value['alias']))
    def test_capture_refuses_unqualified_topology_before_private_namespace(self):
        value=self.record()
        info=type('Info',(),dict(st_dev=1,st_ino=100))()
        with mock.patch.object(binding,'directory',return_value=info),mock.patch.object(Path,'stat',return_value=info),mock.patch.object(Path,'read_text',return_value=value['capture_boot']),mock.patch.object(binding,'topology',side_effect=ValueError('refused')),mock.patch.object(binding,'underlay_inode',side_effect=AssertionError('namespace reached')):
            with self.assertRaises(ValueError):binding.capture(value['volume'],value['uuid'],value['backing'],value['alias'])
    def test_idempotence_does_not_issue_mount_syscalls(self):
        for bound in [False,True]:
            with mock.patch.object(binding,'topology',return_value=bound),mock.patch.object(binding.libc,'mount',side_effect=AssertionError('mount reached')),mock.patch.object(binding.libc,'umount2',side_effect=AssertionError('umount reached')):
                self.assertFalse(binding.change(self.record(),bound))
    def test_publish_never_overwrites_and_load_refuses_digest_alias_or_ambiguity(self):
        if os.geteuid()!=0:self.skipTest('root-owned private record control requires root')
        with tempfile.TemporaryDirectory() as raw:
            # Test the private record operations with a protected parent shim;
            # actual path ownership and namespace/mount predicates are GCE tests.
            parent=Path(raw);path=parent/'binding.json';value=self.record()
            with mock.patch.object(binding,'directory'):
                binding.publish(path,value);data=path.read_bytes();pin=hashlib.sha256(data).hexdigest()
                self.assertEqual(binding.load(path,pin),value)
                with self.assertRaises(FileExistsError):binding.publish(path,value)
                self.assertEqual(path.read_bytes(),data)
                with self.assertRaises(ValueError):binding.load(path,'0'*64)
                alias=parent/'alias';alias.symlink_to(path)
                with self.assertRaises(OSError):binding.load(alias,pin)
                path.write_bytes(b'{"version":1,'+data[1:]);pin=hashlib.sha256(path.read_bytes()).hexdigest()
                with self.assertRaises(ValueError):binding.load(path,pin)

if __name__=='__main__':unittest.main()
