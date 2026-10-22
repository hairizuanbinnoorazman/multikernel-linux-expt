#!/usr/bin/env python3
"""Approval, metadata, no-overwrite and xattr controls for Docker import data."""
import hashlib
import io
import os
from pathlib import Path
import runpy
import tarfile
import tempfile
import unittest

module = runpy.run_path(str(Path(__file__).with_name('build-runtime-docker-image-archive.py')))


class ArchiveTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.root = self.base / 'root'
        self.root.mkdir()
        (self.root / 'bin').mkdir()
        self.code = self.root / 'bin/tool'
        self.code.write_bytes(b'approved executable data, never executed\n')
        self.code.chmod(0o4755)
        os.link(self.code, self.root / 'bin/tool-link')
        (self.root / 'alias').symlink_to('/bin/tool')
        self.output = self.base / 'image.tar'
        self.approval = self.digest()

    def digest(self):
        entries, _ = module['rootfs']['scan'](self.root)
        return hashlib.sha256(module['rootfs']['manifest'](entries)).hexdigest()

    def test_approved_metadata_determinism_and_no_overwrite(self):
        receipt = module['build'](self.root, self.approval, self.output)
        data = self.output.read_bytes()
        self.assertEqual(receipt['source_manifest_sha256'], self.approval)
        self.assertEqual(receipt['archive_sha256'], hashlib.sha256(data).hexdigest())
        self.assertFalse(receipt['xattrs_emitted'])
        with tarfile.open(fileobj=io.BytesIO(data)) as archive:
            entries = archive.getmembers()
            self.assertTrue(all(e.mtime == 0 and not e.pax_headers for e in entries))
            self.assertEqual(archive.getmember('bin/tool').mode, 0o4755)
            self.assertEqual(archive.getmember('alias').linkname, '/bin/tool')
            self.assertTrue(archive.getmember('bin/tool-link').islnk())
            self.assertEqual(archive.getmember('bin/tool-link').linkname, 'bin/tool')
        second = self.base / 'second.tar'
        self.assertEqual(module['build'](self.root, self.approval, second), receipt)
        self.assertEqual(second.read_bytes(), data)
        with self.assertRaisesRegex(module['rootfs']['PublicationError'], 'refusing to overwrite'):
            module['build'](self.root, self.approval, self.output)
        self.assertEqual(self.output.read_bytes(), data)
        self.assertEqual(self.digest(), self.approval)

    def test_changed_source_refused_before_output(self):
        self.code.write_bytes(b'changed executable data\n')
        with self.assertRaisesRegex(ValueError, 'differs from operator approval'):
            module['build'](self.root, self.approval, self.output)
        self.assertFalse(self.output.exists())

    def test_unknown_xattr_is_refused_not_silently_normalized(self):
        os.setxattr(self.code, 'user.unexpected', b'not approved')
        with self.assertRaisesRegex(module['rootfs']['RootFSError'], 'unsupported xattrs'):
            module['build'](self.root, self.approval, self.output)
        self.assertFalse(self.output.exists())


if __name__ == '__main__':
    unittest.main()
