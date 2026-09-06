#!/usr/bin/env python3
import hashlib
import json
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
        first, image_a, metadata_a = self.build("a")
        # Cross a wall-clock second so this detects e2fsprogs silently treating
        # a zero fake-time value as "now".
        time.sleep(1.1)
        second, image_b, metadata_b = self.build("b")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(image_a.read_bytes(), image_b.read_bytes())
        record_a = json.loads(metadata_a.read_text())
        record_b = json.loads(metadata_b.read_text())
        self.assertEqual(record_a["determinism"]["fake_time"], 1)
        self.assertEqual(record_a["sha256"], hashlib.sha256(image_a.read_bytes()).hexdigest())
        self.assertEqual(record_a["sha256"], record_b["sha256"])
        self.assertGreaterEqual(image_a.stat().st_blocks * 512, image_a.stat().st_size)
        (self.root / "etc" / "value").write_text("two\n")
        third, _, metadata_c = self.build("c")
        self.assertEqual(third.returncode, 0, third.stderr)
        self.assertNotEqual(record_a["sha256"], json.loads(metadata_c.read_text())["sha256"])

    def test_high_water_and_bad_identity_leave_no_artifacts(self):
        for name, overrides, message in (
            ("high", {"min_free_bytes": 1 << 62}, "high-water refusal"),
            ("uuid", {"uuid": "bad"}, "malformed"),
            ("size", {"size": 4096}, "outside the supported bounds"),
        ):
            result, output, metadata = self.build(name, **overrides)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(message, result.stderr)
            self.assertFalse(output.exists())
            self.assertFalse(metadata.exists())


if __name__ == "__main__":
    unittest.main()
