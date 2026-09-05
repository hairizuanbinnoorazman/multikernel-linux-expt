#!/usr/bin/env python3
import gzip
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("build-runtime-rootfs.py")


class RootFSBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.temp)
        self.root = self.temp / "root"
        self.root.mkdir()

    def build(self, suffix):
        output = self.temp / f"{suffix}.cpio.gz"
        manifest = self.temp / f"{suffix}.json"
        result = subprocess.run([str(SCRIPT), str(self.root), str(output), str(manifest)], text=True, capture_output=True)
        return result, output, manifest

    def test_reproducible_and_changed_input_control(self):
        (self.root / "etc").mkdir()
        source = self.root / "etc" / "value"
        source.write_text("one\n")
        os.chmod(source, 0o640)
        os.link(source, self.root / "etc" / "linked")
        os.symlink("etc/value", self.root / "value")
        first, archive_a, manifest_a = self.build("a")
        second, archive_b, manifest_b = self.build("b")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(archive_a.read_bytes(), archive_b.read_bytes())
        self.assertEqual(manifest_a.read_bytes(), manifest_b.read_bytes())
        data = json.loads(manifest_a.read_text())
        values = {entry["path"]: entry for entry in data["entries"]}
        self.assertEqual(values["etc/value"]["mode"] & 0o777, 0o640)
        self.assertEqual(values["etc/value"]["hardlink"], values["etc/linked"]["hardlink"])
        source.write_text("two\n")
        third, archive_c, manifest_c = self.build("c")
        self.assertEqual(third.returncode, 0, third.stderr)
        self.assertNotEqual(hashlib.sha256(archive_a.read_bytes()).digest(), hashlib.sha256(archive_c.read_bytes()).digest())
        self.assertNotEqual(manifest_a.read_bytes(), manifest_c.read_bytes())

    def test_archive_has_normalized_mtime(self):
        (self.root / "file").write_text("content")
        result, archive, _ = self.build("normalized")
        self.assertEqual(result.returncode, 0, result.stderr)
        raw = gzip.decompress(archive.read_bytes())
        self.assertEqual(raw[:6], b"070701")
        self.assertEqual(int(raw[46:54], 16), 0)

    def test_archive_extracts_hardlinks_symlinks_and_modes(self):
        (self.root / "dir").mkdir()
        original = self.root / "dir" / "original"
        original.write_text("payload")
        os.chmod(original, 0o751)
        os.link(original, self.root / "dir" / "linked")
        os.symlink("original", self.root / "dir" / "symbolic")
        result, archive, _ = self.build("extract")
        self.assertEqual(result.returncode, 0, result.stderr)
        destination = self.temp / "extracted"
        destination.mkdir()
        extraction = subprocess.run(
            ["cpio", "--extract", "--quiet"], cwd=destination,
            input=gzip.decompress(archive.read_bytes()), capture_output=True,
        )
        self.assertEqual(extraction.returncode, 0, extraction.stderr)
        extracted = destination / "dir" / "original"
        linked = destination / "dir" / "linked"
        self.assertEqual(extracted.read_text(), "payload")
        self.assertEqual(extracted.stat().st_ino, linked.stat().st_ino)
        self.assertEqual(stat.S_IMODE(extracted.stat().st_mode), 0o751)
        self.assertEqual(os.readlink(destination / "dir" / "symbolic"), "original")

    def test_rejects_escaping_symlink(self):
        (self.root / "dir").mkdir()
        os.symlink("../../outside", self.root / "dir" / "escape")
        result, _, _ = self.build("escape")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("escaping symlink", result.stderr)

    def test_rejects_fifo(self):
        os.mkfifo(self.root / "fifo")
        result, _, _ = self.build("fifo")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported file type", result.stderr)

    def test_capacity_and_inode_limits_refuse_before_output(self):
        (self.root / "one").write_bytes(b"12345")
        output = self.temp / "limited.cpio.gz"
        manifest = self.temp / "limited.json"
        for option, value, message in (
            ("--max-bytes", "4", "payload bytes"),
            ("--max-inodes", "1", "entry count"),
            ("--min-free-bytes", str(1 << 62), "high-water refusal"),
        ):
            output.unlink(missing_ok=True)
            manifest.unlink(missing_ok=True)
            result = subprocess.run(
                [str(SCRIPT), str(self.root), str(output), str(manifest), option, value],
                text=True, capture_output=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(message, result.stderr)
            self.assertFalse(output.exists())

    @unittest.skipUnless(hasattr(os, "setxattr"), "xattrs unavailable")
    def test_rejects_xattrs(self):
        target = self.root / "file"
        target.write_text("content")
        try:
            os.setxattr(target, "user.multikernel-test", b"value")
        except OSError as error:
            self.skipTest(f"filesystem xattrs unavailable: {error}")
        result, _, _ = self.build("xattr")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported xattrs", result.stderr)


if __name__ == "__main__":
    unittest.main()
