#!/usr/bin/env python3
import json
import os
import pathlib
import stat
import subprocess
import tempfile
import time
import unittest
from unittest import mock
from runtime_native_record_fault import HiddenNativeRecord


class RecordFaultTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tools = tempfile.TemporaryDirectory()
        cls.binary = pathlib.Path(cls.tools.name) / 'server'
        source = cls.binary.with_suffix('.c')
        source.write_text('''#include <unistd.h>
#include <stdlib.h>
#include <fcntl.h>
int main(void) { int fd=open(getenv("TEST_IMAGE"),O_RDONLY); if(fd<0||dup2(fd,3)<0)return 2; for(;;)pause(); }
''')
        subprocess.run(['cc', '-O2', '-Wall', '-Wextra', '-Werror', str(source), '-o', str(cls.binary)], check=True)

    @classmethod
    def tearDownClass(cls):
        cls.tools.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = pathlib.Path(self.temp.name)
        self.image = self.directory / 'backing'
        self.image.write_bytes(b'private backing fixture')
        self.export = dict(sandbox_id='box', sandbox_generation='a'*32, export_generation='b'*32,
                           state='ACTIVE', path=str(self.image), port=4061, image_id='fixture', peer_cid=40,
                           image_identity=dict(device=self.image.stat().st_dev, inode=self.image.stat().st_ino))
        self.process = subprocess.Popen([str(self.binary), 'server', '/proc/self/fd/3', '4061', 'fixture', 'b'*32, '40'],
                                        env={'TEST_IMAGE':str(self.image)})
        self.addCleanup(self.stop)
        deadline = time.monotonic()+3
        while not pathlib.Path('/proc', str(self.process.pid), 'fd/3').exists():
            self.assertIsNone(self.process.poll())
            self.assertLess(time.monotonic(), deadline)
            time.sleep(.01)
        proc = pathlib.Path('/proc') / str(self.process.pid)
        data = (proc/'stat').read_bytes()
        self.record = dict(version=4, pid=self.process.pid, start_time=int(data[data.rindex(b')')+2:].split()[19]),
                           path=str(self.image), port=4061, image_id='fixture', export_generation='b'*32,
                           peer_cid=40, image_device=self.image.stat().st_dev, image_inode=self.image.stat().st_ino,
                           binary_device=self.binary.stat().st_dev, binary_inode=self.binary.stat().st_ino)
        self.name = self.directory / ('box-'+'a'*12+'.json')
        self.data = json.dumps(self.record).encode()
        self.name.write_bytes(self.data)
        self.name.chmod(0o600)

    def stop(self):
        if self.process.poll() is None:
            self.process.terminate()
        self.process.wait(timeout=3)

    def fault(self):
        return HiddenNativeRecord(self.directory, self.export, self.binary)

    def test_exact_inode_and_bytes_restored_after_exception(self):
        inode=self.name.stat().st_ino
        with self.assertRaisesRegex(RuntimeError, 'injected client failure'):
            with self.fault() as held:
                self.assertFalse(self.name.exists())
                held.check_hidden()
                self.assertEqual(held.proof()['record_inode'],inode)
                self.assertIsNone(self.process.poll())
                raise RuntimeError('injected client failure')
        self.assertEqual(self.name.read_bytes(),self.data)
        self.assertEqual(self.name.stat().st_ino,inode)
        self.assertEqual(sorted(p.name for p in self.directory.iterdir()), ['backing',self.name.name])

    def test_failed_directory_sync_restores_successful_rename(self):
        inode=self.name.stat().st_ino
        with mock.patch('runtime_native_record_fault.os.fsync',side_effect=OSError('injected sync failure')):
            with self.assertRaises(OSError):
                with self.fault():self.fail('sync failure ignored')
        self.assertEqual(self.name.read_bytes(),self.data)
        self.assertEqual(self.name.stat().st_ino,inode)
        self.assertEqual(len(list(self.directory.iterdir())),2)

    def test_unpaired_fifo_preserved_and_original_restored_on_exception(self):
        inode=self.name.stat().st_ino
        with self.assertRaisesRegex(RuntimeError,'injected doctor failure'):
            with HiddenNativeRecord(self.directory,self.export,self.binary,placeholder='fifo') as held:
                self.assertTrue(stat.S_ISFIFO(self.name.lstat().st_mode))
                held.check_hidden()
                self.assertEqual(held.proof()['placeholder'],'fifo')
                self.assertTrue(held.proof()['no_fifo_writer_opened'])
                raise RuntimeError('injected doctor failure')
        self.assertEqual(self.name.read_bytes(),self.data)
        self.assertEqual(self.name.stat().st_ino,inode)
        self.assertEqual(len(list(self.directory.iterdir())),2)

    def test_changed_fifo_placeholder_is_preserved(self):
        with self.assertRaisesRegex(ValueError,'changed FIFO placeholder'):
            with HiddenNativeRecord(self.directory,self.export,self.binary,placeholder='fifo') as held:
                self.name.unlink()
                self.name.write_bytes(b'conflicting replacement')
        self.assertEqual(self.name.read_bytes(),b'conflicting replacement')
        self.assertEqual((self.directory/held.held).read_bytes(),self.data)

    def test_occupied_restore_target_is_preserved_with_held_evidence(self):
        with self.assertRaises(OSError):
            with self.fault() as held:
                self.name.write_bytes(b'conflicting replacement')
                self.name.chmod(0o600)
        self.assertEqual(self.name.read_bytes(),b'conflicting replacement')
        self.assertEqual((self.directory/held.held).read_bytes(),self.data)
        self.assertIsNone(self.process.poll())

    def test_modified_held_record_is_not_restored(self):
        with self.assertRaisesRegex(ValueError,'modified held record'):
            with self.fault() as held:
                (self.directory/held.held).write_bytes(b'modified private record')
        self.assertFalse(self.name.exists())
        self.assertEqual((self.directory/held.held).read_bytes(),b'modified private record')

    def test_conflicting_incarnation_and_lease_refuse_before_rename(self):
        for key,value in [('peer_cid',41),('start_time',self.record['start_time']+1),('binary_inode',self.record['binary_inode']+1),('image_inode',self.record['image_inode']+1),('pid',True)]:
            r={**self.record,key:value};raw=json.dumps(r).encode();self.name.write_bytes(raw)
            with self.subTest(key=key),self.assertRaises((ValueError,OSError)):
                with self.fault():self.fail('conflicting identity entered fault')
            self.assertEqual(self.name.read_bytes(),raw)
            self.assertEqual(len(list(self.directory.iterdir())),2)
            self.assertIsNone(self.process.poll())

    def test_hardlink_symlink_and_nonprivate_directory_refuse(self):
        os.link(self.name,self.directory/'alias')
        with self.assertRaises(ValueError):
            with self.fault():self.fail('hardlink accepted')
        (self.directory/'alias').unlink()
        self.name.rename(self.directory/'actual')
        self.name.symlink_to(self.directory/'actual')
        with self.assertRaises(OSError):
            with self.fault():self.fail('symlink accepted')
        self.name.unlink();(self.directory/'actual').rename(self.name)
        self.directory.chmod(0o755)
        with self.assertRaises(ValueError):
            with self.fault():self.fail('public directory accepted')
        self.assertEqual(self.name.read_bytes(),self.data)

if __name__=='__main__':unittest.main()
