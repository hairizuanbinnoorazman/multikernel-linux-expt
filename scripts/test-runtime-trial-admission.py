#!/usr/bin/env python3
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from runtime_trial_admission import observe, require, IMAGE_BYTES, RESERVE_BYTES


class TrialAdmissionTests(unittest.TestCase):
    def sample(self, free_bytes, free_inodes=100000, count=1):
        with tempfile.TemporaryDirectory() as directory:
            capacity = SimpleNamespace(f_bavail=free_bytes, f_frsize=1, f_favail=free_inodes)
            with patch('runtime_trial_admission.os.fstatvfs', return_value=capacity):
                return observe(directory, count)

    def test_floor_boundaries_and_concurrent_images(self):
        for count in (1, 2):
            floor = count * IMAGE_BYTES + RESERVE_BYTES
            report = self.sample(floor, count=count)
            require(report)
            self.assertTrue(report['minimum_capacity_available'])
            self.assertFalse(report['capacity_reserved'])
            self.assertFalse(report['source_staging_accounted'])
            self.assertFalse(report['full_builder_admission_claimed'])
            self.assertFalse(report['cleanup_qualified'])
            report = self.sample(floor-1, count=count)
            self.assertEqual(report['refusal_codes'], ['MINIMUM_FREE_BYTES'])
            with self.assertRaises(RuntimeError): require(report)
            report = self.sample(floor, 1024+count*4-1, count)
            self.assertEqual(report['refusal_codes'], ['MINIMUM_FREE_INODES'])
            with self.assertRaises(RuntimeError): require(report)

    def test_real_descriptor_observation_does_not_write_to_storage(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);sentinel=root/'retained';sentinel.write_bytes(b'retained-private-image-marker')
            before=sentinel.stat();report=observe(root);after=sentinel.stat()
            self.assertEqual(before,after)
            self.assertEqual(list(root.iterdir()),[sentinel])
            self.assertNotIn('retained-private-image-marker',json.dumps(report))
            self.assertNotIn(directory,json.dumps(report))
            self.assertEqual(report['directory_identity']['inode'],root.stat().st_ino)

    def test_symlinks_invalid_counts_and_relative_paths_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);(root/'actual').mkdir();(root/'link').symlink_to(root/'actual',target_is_directory=True)
            (root/'actual'/'child').mkdir()
            for path in (root/'link',root/'link'/'child'):
                with self.assertRaises(OSError): observe(path)
        for count in (True,0,3,-1,1.0):
            with self.assertRaises(ValueError):observe('/tmp',count)
        for path in ('relative','/tmp/../tmp'):
            with self.assertRaises(ValueError):observe(path)

    def test_guard_precedes_mutating_live_trial_operations(self):
        root=Path(__file__).resolve().parent
        for name, mutations in {
            'test-runtime-vsock-module-trial.py':["run(['/sbin/rmmod'", "trial.mkdir("],
            'test-runtime-docker-profile-live.py':["private.mkdir(","run(['/bin/mount'", "m=start("],
        }.items():
            source=(root/name).read_text();guard=source.index('require_trial_capacity(capacity)')
            for mutation in mutations:self.assertLess(guard,source.index(mutation),name+':'+mutation)


if __name__=='__main__':unittest.main()
