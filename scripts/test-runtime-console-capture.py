#!/usr/bin/env python3
import os
import threading
import unittest
from runtime_console_capture import ConsoleCapture, filtered_events, read_bounded


class ConsoleCaptureTests(unittest.TestCase):
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
        self.assertIn(b'EXT4-fs error: invalid bitmap', result)
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
            b'MK_STORAGE_BOOTSTRAP_READY sandbox=owned\n'
            b'MK_STORAGE_BOOTSTRAP_FAIL reason=mount\n'
            b'MK_AGENT_START sandbox=owned token=[REDACTED]\n'
            b'MK_AGENT_BOOT_CONFIG_MISSING\n'
            b'MK_AGENT_INSECURE_BOOT_CREDENTIAL\n'))
        self.assertNotIn(b'private-value', result)


if __name__ == '__main__':unittest.main()
