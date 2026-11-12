#!/usr/bin/env python3
import os
import threading
import unittest
import tempfile
import pathlib
import json
import hashlib
import time
import fcntl
import termios
import array
import stat
import types
from unittest import mock
from runtime_console_capture import ConsoleCapture, filtered_events, read_bounded, MARKERS, CATEGORIES, FILTERED_FORMAT


class ConsoleCaptureTests(unittest.TestCase):
    def test_teardown_connection_survives_instance_removal_after_hold_release(self):
        a,aw=os.pipe();b,bw=os.pipe()
        real_open,real_write=os.open,os.write
        selections=[];opened=[]
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);instance=root/'instance';instance.mkdir()
            (instance/'id').write_text('40\n')
            class Fixture(ConsoleCapture):
                def _await_instance(self, path):return True
                def _connect(self, path, expected=None):return super()._connect(instance,expected)
            def open_device(path,flags,*args):
                if path=='/dev/mktty':
                    fd=[a,b][len(opened)];opened.append(fd);return fd
                return real_open(path,flags,*args)
            def write(fd,data):
                if fd in [a,b]:selections.append(data);return len(data)
                return real_write(fd,data)
            try:
                device=types.SimpleNamespace(st_mode=stat.S_IFCHR,st_uid=0,st_rdev=os.makedev(10,200))
                with mock.patch('runtime_console_capture.os.open',side_effect=open_device), mock.patch('runtime_console_capture.os.write',side_effect=write), mock.patch('runtime_console_capture.os.fstat',return_value=device):
                    capture=Fixture('mk-'+'a'*32+'-'+'b'*16,root);capture.start()
                    self.assertTrue(capture.attached.wait(timeout=1))
                    # The held workload still pins its instance at this point.
                    capture.begin_teardown()
                    (instance/'id').unlink();instance.rmdir()
                    # Docker can remove sysfs at task exit, before docker rm.
                    late=b'MK_STORAGE_NBD_DISCONNECT_PASS\n';real_write(bw,late)
                    end=time.monotonic()+1
                    while time.monotonic()<end:
                        pending=array.array('i',[0]);fcntl.ioctl(b,termios.FIONREAD,pending,True)
                        if pending[0]==0:break
                        time.sleep(.001)
                    else:self.fail('late output was not consumed')
                    capture.begin_teardown()  # Idempotent, no new selection.
                    capture.finish()
                self.assertEqual(selections,[b'40\n',b'40\n'])
                self.assertIn(late,(root/'console-events.log').read_bytes())
                self.assertTrue(json.loads((root/'console-capture.json').read_text())['teardown_attached'])
            finally:
                for fd in [a,aw,b,bw]:
                    try:os.close(fd)
                    except OSError:pass

    def run_windows(self, initial, final, finish_early=False, replace=False):
        a, aw = os.pipe(); b, bw = os.pipe()
        os.write(aw, initial); os.close(aw)
        os.write(bw, final)
        identity = (35, 7, 40)
        connections = []
        class Fake(ConsoleCapture):
            def _await_instance(self, instance):return True
            def _connect(self, instance, expected=None):
                connections.append(expected)
                if len(connections)==1:return a, identity
                if replace:raise ValueError('replaced instance')
                self_test.assertEqual(expected, identity)
                return b, identity
        self_test = self
        try:
            with tempfile.TemporaryDirectory() as directory, mock.patch('runtime_console_capture.LIMIT',256), mock.patch('runtime_console_capture.TEARDOWN_LIMIT',128):
                capture=Fake('mk-'+'a'*32+'-'+'b'*16, directory);capture.start()
                self.assertTrue(capture.attached.wait(timeout=1))
                if len(initial)>128:
                    end=time.monotonic()+1
                    while not capture.record['phases'] and time.monotonic()<end:time.sleep(.001)
                    self.assertTrue(capture.record['phases'])
                if not finish_early:
                    if replace:
                        with self.assertRaises(RuntimeError):capture.begin_teardown()
                    else:capture.begin_teardown()
                # Wait until the finite teardown payload has been consumed.
                if not replace and not finish_early:
                    end=time.monotonic()+1
                    while time.monotonic()<end:
                        pending=array.array('i',[0])
                        try:fcntl.ioctl(b,termios.FIONREAD,pending,True)
                        except OSError:break  # A capped window closes its FD.
                        if pending[0]==0:break
                        time.sleep(.001)
                    else:self.fail('teardown pipe was not consumed within its bound')
                capture.finish()
                root=pathlib.Path(directory);metadata=json.loads((root/'console-capture.json').read_text())
                raw=(root/'console-private.log').read_bytes();events=(root/'console-events.log').read_bytes()
                self.assertEqual(metadata['sha256'],hashlib.sha256(raw).hexdigest())
                self.assertEqual(metadata['filtered_sha256'],hashlib.sha256(events).hexdigest())
                self.assertLessEqual(len(raw),256)
                self.assertFalse(metadata['complete_stream'])
                self.assertEqual(metadata['filtered_format'],FILTERED_FORMAT)
                for name in ['console-private.log','console-events.log','console-capture.json']:self.assertEqual((root/name).stat().st_mode&0o777,0o600)
                return metadata, raw, events
        finally:
            for fd in [a,b,bw]:
                try:os.close(fd)
                except OSError:pass

    def test_teardown_reservation_survives_initial_flood_without_larger_limit(self):
        late=b'MK_STORAGE_ROOT_QUIESCE_PASS device=nbd0\nMK_STORAGE_NBD_DISCONNECT_PASS\n'
        metadata,raw,events=self.run_windows(b'nbd token=PRIVATE\n'+b'x'*200,late)
        self.assertEqual([p['name'] for p in metadata['phases']],['startup','teardown'])
        self.assertEqual(metadata['phases'][0]['reason'],'output-limit')
        self.assertTrue(metadata['truncated'])
        self.assertTrue(metadata['teardown_attached'])
        self.assertIn(b'MK_STORAGE_ROOT_QUIESCE_PASS',events)
        self.assertIn(b'MK_STORAGE_NBD_DISCONNECT_PASS',events)
        self.assertNotIn(b'PRIVATE',events)
        self.assertLessEqual(len(raw),128+len(late))

    def test_teardown_flood_and_early_finish_remain_bounded_and_explicit(self):
        metadata,_,_=self.run_windows(b'x'*200,b'y'*200)
        self.assertEqual(metadata['phases'][-1]['reason'],'output-limit')
        self.assertEqual(metadata['bytes'],256)
        metadata,_,_=self.run_windows(b'nbd start\n',b'not read',finish_early=True)
        self.assertFalse(metadata['teardown_attached'])
        self.assertEqual(metadata['reason'],'stopped-before-teardown')

    def test_replacement_refuses_teardown_and_never_opens_device(self):
        metadata,_,_=self.run_windows(b'x'*200,b'foreign',replace=True)
        self.assertEqual(metadata['reason'],'capture-error')
        self.assertFalse(metadata['teardown_attached'])
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);(root/'id').write_text('41\n')
            st=root.stat();capture=ConsoleCapture('mk-'+'a'*32+'-'+'b'*16,directory)
            with mock.patch('runtime_console_capture.os.open') as opened:
                with self.assertRaises(ValueError):capture._connect(root,(st.st_dev,st.st_ino,40))
                opened.assert_not_called()

    def test_reads_to_eof_and_bounds_pressure(self):
        for payload, limit, expected, reason in [(b'kernel-event', 64, b'kernel-event', 'eof'),
                                                  (b'x'*32, 16, b'x'*16, 'output-limit')]:
            left, right = os.pipe()
            try:
                os.write(right, payload); os.close(right); right = None
                self.assertEqual(read_bounded(left, threading.Event(), limit=limit), (expected, reason))
            finally:
                os.close(left)
                if right is not None:os.close(right)

    def test_stopped_or_invalid_reader_is_bounded(self):
        stop = threading.Event();stop.set()
        self.assertEqual(read_bounded(-1, stop), (b'', 'stopped'))
        left, right = os.pipe()
        try:
            self.assertEqual(read_bounded(left, threading.Event(), timeout=.01), (b'', 'deadline'))
        finally:os.close(left);os.close(right)
        left, right = os.pipe();os.close(right)
        try:
            self.assertEqual(read_bounded(left, threading.Event(), timeout=.01, empty_is_idle=True), (b'', 'deadline'))
        finally:os.close(left)

    def test_exports_only_bounded_filtered_redacted_events(self):
        secret = 'a'*64
        data = ('unrelated environment line\nEXT4-fs error: invalid bitmap\n'
                'nbd token='+secret+'\nnbd Authorization: Bearer private-value\n'
                'mkvsock '+('z'*2000)+'\n').encode()
        result = filtered_events(data)
        self.assertNotIn(secret.encode(), result)
        self.assertNotIn(b'private-value', result)
        self.assertNotIn(b'environment line', result)
        self.assertIn(b'MK_CONSOLE_EXT4\n', result)
        self.assertNotIn(b'invalid bitmap', result)
        self.assertTrue(all(len(line)<=1024 for line in result.splitlines()))
        self.assertLessEqual(len(filtered_events(b'nbd error\n'*10000)), 65536)
        filtered_events(('nbd '+('\u2603'*2000)+'\n').encode()*100).decode()

    def test_foreign_or_unbounded_instance_identity_refused_before_io(self):
        for name in ['../../other', '40\ninput', 'mk-foreign', 'mk-'+'a'*1000]:
            with self.assertRaises(ValueError):ConsoleCapture(name, '/not-used')

    def test_bootstrap_progress_survives_export_without_general_agent_logs(self):
        result = filtered_events(
            b'MK_STORAGE_BOOTSTRAP_READY sandbox=owned\n'
            b'MK_STORAGE_BOOTSTRAP_FAIL reason=mount\n'
            b'MK_AGENT_START sandbox=owned token=private-value\n'
            b'MK_AGENT_BOOT_CONFIG_MISSING\n'
            b'MK_AGENT_INSECURE_BOOT_CREDENTIAL\n'
            b'MK_AGENT_STARTUP environment=private-value\n'
            b'MK_AGENT_REQUEST payload=private-value\n'
            b'prefix MK_AGENT_START payload=private-value\n')
        self.assertEqual(result, (
            b'MK_STORAGE_BOOTSTRAP_READY\n'
            b'MK_STORAGE_BOOTSTRAP_FAIL\n'
            b'MK_AGENT_START\n'
            b'MK_AGENT_BOOT_CONFIG_MISSING\n'
            b'MK_AGENT_INSECURE_BOOT_CREDENTIAL\n'))
        self.assertNotIn(b'private-value', result)

    def test_unlabelled_json_binary_and_control_payloads_never_leave_projection(self):
        payloads=[b'SYNTHETIC_UNLABELLED_PRIVATE',b'{"credential":"synthetic-value"}',
                  b'https://user:synthetic-value@example.invalid/path',b'\x1b[31mSYNTHETIC_CONTROL\x1b[0m',
                  bytes(range(256)),b'c3ludGhldGljLXByaXZhdGU=']
        allowed=set(MARKERS)|{code for _,code in CATEGORIES}
        for prefix in [b'nbd ',b'EXT4-fs error ',b'mkvsock ',b'out of memory ',b'I/O error ',* [marker+b' ' for marker in MARKERS]]:
            for payload in payloads:
                with self.subTest(prefix=prefix,payload_length=len(payload)):
                    projected=filtered_events(prefix+payload+b'\n')
                    self.assertTrue(projected)
                    self.assertTrue(set(projected.splitlines())<=allowed)
                    self.assertNotIn(b'synthetic-value',projected)
                    self.assertNotIn(b'SYNTHETIC_',projected)
    def test_complete_line_boundaries_and_whole_code_suffix_remain_explicit(self):
        self.assertEqual(filtered_events(b'MK_AGENT_START private'),b'')
        self.assertEqual(filtered_events(b'MK_AGENT_STARTUP private\n'),b'')
        self.assertEqual(filtered_events(b'prefix MK_AGENT_START private\n'),b'')
        self.assertEqual(filtered_events(b'MK_AGENT_START\r\n'),b'MK_AGENT_START\n')
        allowed=set(MARKERS)|{code for _,code in CATEGORIES}
        projected=filtered_events(b'ext4 nbd mkvsock oom I/O error SYNTHETIC\n'*10000)
        self.assertLessEqual(len(projected),65536)
        self.assertTrue(projected.endswith(b'\n'))
        self.assertTrue(set(projected.splitlines())<=allowed)
    def test_pipe_capture_preserves_private_raw_but_exports_only_codes_under_umask_zero(self):
        prior=os.umask(0)
        try:
            metadata,raw,events=self.run_windows(b'nbd SYNTHETIC_UNLABELLED_PRIVATE\n'+b'x'*200,b'MK_STORAGE_NBD_DISCONNECT_PASS private=synthetic\n')
        finally:os.umask(prior)
        self.assertIn(b'SYNTHETIC_UNLABELLED_PRIVATE',raw)
        self.assertNotIn(b'SYNTHETIC_',events)
        self.assertNotIn(b'private=',events)
        self.assertTrue(set(events.splitlines())<=(set(MARKERS)|{code for _,code in CATEGORIES}))

    def test_existing_regular_or_symlinked_metadata_is_preserved(self):
        for symlink in [False,True]:
            with self.subTest(symlink=symlink),tempfile.TemporaryDirectory() as directory:
                root=pathlib.Path(directory);retained=root/'retained-private-proof'
                retained.write_bytes(b'SYNTHETIC_RETAINED_METADATA');inode=retained.stat().st_ino
                metadata=root/'console-capture.json'
                if symlink:metadata.symlink_to(retained.name)
                else:metadata.write_bytes(b'SYNTHETIC_OLD_METADATA')
                old=metadata.read_bytes();identity=metadata.lstat()
                capture=ConsoleCapture('mk-'+'a'*32+'-'+'b'*16,root)
                with mock.patch.object(capture,'_await_instance',return_value=False),mock.patch.object(capture,'_connect') as connect:
                    with self.assertRaises(FileExistsError):capture._capture()
                connect.assert_not_called()
                self.assertEqual(metadata.lstat().st_ino,identity.st_ino);self.assertEqual(metadata.read_bytes(),old)
                self.assertEqual(retained.stat().st_ino,inode);self.assertEqual(retained.read_bytes(),b'SYNTHETIC_RETAINED_METADATA')


if __name__ == '__main__':unittest.main()
