#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest


SCRIPT = Path(__file__).with_name("build-runtime-storage.py")


class StorageBuildTests(unittest.TestCase):
    def setUp(self):
        if not shutil.which("mke2fs") or not shutil.which("e2fsck"):
            self.skipTest("e2fsprogs unavailable")
        self.temp = Path(tempfile.mkdtemp(prefix="mk-storage-test-"))
        self.addCleanup(shutil.rmtree, self.temp)
        self.root = self.temp / "root"
        (self.root / "etc").mkdir(parents=True)
        (self.root / "etc" / "value").write_text("one\n")
        os.link(self.root / "etc" / "value", self.root / "etc" / "value-hardlink")
        (self.root / "etc" / "value-symlink").symlink_to("value")

    def build(self, name, **overrides):
        output = self.temp / f"{name}.ext4"
        metadata = self.temp / f"{name}.json"
        values = {
            "image_id": "source-manifest-abcd", "uuid": "11111111-2222-4333-8444-555555555555",
            "size": 64 << 20, "inodes": 4096, "port": 4061, "min_free_bytes": 0,
        }
        values.update(overrides)
        command = [str(SCRIPT), str(self.root), str(output), str(metadata)]
        for key, value in values.items():
            command += ["--" + key.replace("_", "-"), str(value)]
        result = subprocess.run(command, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        return result, output, metadata

    def test_reproducible_fully_allocated_and_changed_input_control(self):
        source = self.root / "etc" / "value"
        source_before_build = source.stat()
        first, image_a, metadata_a = self.build("a")
        source_after_build = source.stat()
        self.assertEqual(source_before_build.st_mtime_ns, source_after_build.st_mtime_ns)
        self.assertEqual(source_before_build.st_ctime_ns, source_after_build.st_ctime_ns)
        source_before = source.stat()
        os.utime(
            source,
            ns=(source_before.st_atime_ns + 10_000_000_000, source_before.st_mtime_ns),
        )
        # Cross a wall-clock second so this detects e2fsprogs silently treating
        # a zero fake-time value as "now" and source atime leaking into ext4.
        time.sleep(1.1)
        second, image_b, metadata_b = self.build("b")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(image_a.read_bytes(), image_b.read_bytes())
        record_a = json.loads(metadata_a.read_text())
        record_b = json.loads(metadata_b.read_text())
        self.assertEqual(record_a["determinism"]["fake_time"], 1)
        self.assertEqual(record_a["determinism"]["source_metadata_time"], 1)
        self.assertEqual(record_a["sha256"], hashlib.sha256(image_a.read_bytes()).hexdigest())
        self.assertEqual(record_a["sha256"], record_b["sha256"])
        self.assertGreaterEqual(image_a.stat().st_blocks * 512, image_a.stat().st_size)
        (self.root / "etc" / "value").write_text("two\n")
        third, _, metadata_c = self.build("c")
        self.assertEqual(third.returncode, 0, third.stderr)
        self.assertNotEqual(record_a["sha256"], json.loads(metadata_c.read_text())["sha256"])

    def test_logical_path_is_recorded_without_redirecting_output(self):
        logical = Path("/var/lib/multikernel/rootfs/task-test/root.ext4")
        result, output, metadata = self.build("logical", logical_path=logical)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(output.is_file())
        self.assertEqual(json.loads(metadata.read_text())["path"], str(logical))
        self.assertEqual(json.loads(result.stdout)["path"], str(logical))

    def test_high_water_and_bad_identity_leave_no_artifacts(self):
        for name, overrides, message in (
            ("high", {"min_free_bytes": 16 << 40}, "high-water refusal"),
            ("uuid", {"uuid": "bad"}, "malformed"),
            ("size", {"size": 4096}, "outside the supported bounds"),
            ("negative-reserve", {"min_free_bytes": -1}, "outside the supported bounds"),
        ):
            result, output, metadata = self.build(name, **overrides)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(message, result.stderr)
            self.assertFalse(output.exists())
            self.assertFalse(metadata.exists())

    def test_inode_exhaustion_is_bounded_and_cleans_partial_artifacts(self):
        crowded = self.root / "crowded"
        crowded.mkdir()
        for index in range(256):
            (crowded / f"entry-{index:04d}").write_text("x")
        result, output, metadata = self.build("inode-exhaustion", inodes=128)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("mke2fs failed", result.stderr)
        self.assertFalse(output.exists())
        self.assertFalse(metadata.exists())
        self.assertEqual(list(self.temp.glob(".root-staging.*")), [])

    def test_block_exhaustion_is_bounded_and_cleans_partial_artifacts(self):
        payload = self.root / "allocated-payload"
        with payload.open("wb") as stream:
            chunk = b"multikernel-block-exhaustion\n" * 32768
            remaining = 63 << 20
            while remaining:
                written = min(remaining, len(chunk))
                stream.write(chunk[:written])
                remaining -= written
        result, output, metadata = self.build("block-exhaustion", size=64 << 20)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("mke2fs failed", result.stderr)
        self.assertFalse(output.exists())
        self.assertFalse(metadata.exists())
        self.assertEqual(list(self.temp.glob(".root-staging.*")), [])

    def test_metadata_normalization_failure_cleans_partial_artifacts(self):
        result, output, metadata = self.build("debugfs", debugfs="/bin/false")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("metadata normalization failed", result.stderr)
        self.assertFalse(output.exists())
        self.assertFalse(metadata.exists())
        self.assertEqual(list(self.temp.glob(".root-staging.*")), [])
        self.assertEqual(list(self.temp.glob(".debugfs-normalize.*")), [])


if __name__ == "__main__":
    unittest.main()
