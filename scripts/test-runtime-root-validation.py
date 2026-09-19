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

    def test_rejects_noncanonical_relative_roots(self):
        for value in ("./rootfs", "rootfs/", "rootfs//nested"):
            with self.subTest(value=value):
                self.check(value, False)

    def test_rejects_relative_traversal(self):
        self.check("../outside", False)

    def test_rejects_symlink_component(self):
        outside = self.temp / "outside"
        outside.mkdir()
        os.symlink(outside, self.bundle / "linked")
        self.check("linked", False)

    def test_rejects_missing_and_non_directory_roots(self):
        (self.bundle / "file").write_text("not a root", encoding="utf-8")
        self.check("file", False)
        self.check("missing", False)

    def test_absolute_root_must_be_under_allowlist(self):
        allowed = self.temp / "storage"
        allowed.mkdir()
        root = allowed / "root"
        root.mkdir()
        self.check(str(root), True, allowed)
        self.check(str(self.bundle / "rootfs"), False, allowed)
        self.check(str(root) + "/", False, allowed)

    def test_inherited_bundle_anchor_survives_public_path_replacement(self):
        descriptor = os.open(self.bundle, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
        self.addCleanup(os.close, descriptor)
        moved = self.temp / "held-bundle"
        self.bundle.rename(moved)
        self.bundle.mkdir()
        (self.bundle / "rootfs").mkdir()
        (self.bundle / "rootfs" / "replacement").write_text("replacement")
        config = self.temp / "descriptor-config.json"
        config.write_text(json.dumps({"root": {"path": "rootfs"}}))
        inherited = f"/proc/self/fd/{descriptor}"
        result = subprocess.run(
            [str(SCRIPT), inherited, str(config), "--json"],
            text=True,
            capture_output=True,
            pass_fds=(descriptor,),
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        observed = json.loads(result.stdout)
        expected = (moved / "rootfs").stat()
        self.assertEqual(observed, {
            "device": expected.st_dev,
            "inode": expected.st_ino,
            "path": inherited + "/rootfs",
        })
        self.assertFalse((Path(observed["path"]) / "replacement").exists())

        handshake = (
            'exec {source_fd}<"$1"; '
            'read -r device inode < <(stat -Lc "%d %i" "/proc/self/fd/$source_fd"); '
            'test "$device" = "$2" && test "$inode" = "$3"'
        )
        accepted = subprocess.run(
            ["bash", "-c", handshake, "bash", observed["path"], str(observed["device"]), str(observed["inode"])],
            pass_fds=(descriptor,),
            check=False,
        )
        self.assertEqual(accepted.returncode, 0)
        original_root = moved / "validated-root"
        (moved / "rootfs").rename(original_root)
        (moved / "rootfs").mkdir()
        rejected = subprocess.run(
            ["bash", "-c", handshake, "bash", observed["path"], str(observed["device"]), str(observed["inode"])],
            pass_fds=(descriptor,),
            check=False,
        )
        self.assertNotEqual(rejected.returncode, 0)


if __name__ == "__main__":
    unittest.main()
