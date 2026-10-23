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

    def test_foreign_or_unbounded_instance_identity_refused_before_io(self):
        for name in ['../../other', '40\ninput', 'mk-foreign', 'mk-'+'a'*1000]:
            with self.assertRaises(ValueError):ConsoleCapture(name, '/not-used')


if __name__ == '__main__':unittest.main()
