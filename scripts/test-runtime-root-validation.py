#!/usr/bin/env python3
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("validate-runtime-root.py")


class RootValidationTests(unittest.TestCase):
    def setUp(self):
        self.temp = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.temp)
        self.bundle = self.temp / "bundle"
        self.bundle.mkdir()
        (self.bundle / "rootfs").mkdir()

    def check(self, root, accepted, allowed=None):
        config = self.bundle / "validated.json"
        config.write_text(json.dumps({"root": {"path": root}}))
        environment = os.environ.copy()
        if allowed is not None:
            environment["MK_ALLOWED_ABSOLUTE_ROOTS"] = str(allowed)
        result = subprocess.run([str(SCRIPT), str(self.bundle), str(config)], env=environment, text=True, capture_output=True)
        self.assertEqual(result.returncode == 0, accepted, result.stderr)

    def test_accepts_canonical_relative_root(self):
        self.check("rootfs", True)

    def test_rejects_relative_traversal(self):
        self.check("../outside", False)

    def test_rejects_symlink_component(self):
        outside = self.temp / "outside"
        outside.mkdir()
        os.symlink(outside, self.bundle / "linked")
        self.check("linked", False)

    def test_absolute_root_must_be_under_allowlist(self):
        allowed = self.temp / "storage"
        allowed.mkdir()
        root = allowed / "root"
        root.mkdir()
        self.check(str(root), True, allowed)
        self.check(str(self.bundle / "rootfs"), False, allowed)


if __name__ == "__main__":
    unittest.main()
