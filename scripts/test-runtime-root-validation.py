#!/usr/bin/env python3
import json
import importlib.util
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

    def test_rejects_bundle_and_ancestor_under_broad_absolute_allowance(self):
        (self.bundle / ".multikernel-create.json").write_text("SYNTHETIC_PRIVATE_INTENT")
        for root in (self.bundle, self.temp):
            with self.subTest(root=root.name):
                self.check(str(root), False, self.temp)
        self.assertEqual((self.bundle / ".multikernel-create.json").read_text(), "SYNTHETIC_PRIVATE_INTENT")

    def test_rejects_private_runtime_roots_relative_and_absolute(self):
        runtime = self.bundle / ".multikernel"
        (runtime / "nested").mkdir(parents=True)
        token = runtime / "token"
        token.write_text("SYNTHETIC_PRIVATE_TOKEN")
        for root in (".multikernel", ".multikernel/nested", str(runtime), str(runtime / "nested")):
            with self.subTest(root=root):
                self.check(root, False, self.temp)
        self.assertEqual(token.read_text(), "SYNTHETIC_PRIVATE_TOKEN")

    def test_accepts_image_roots_beside_private_control_state(self):
        (self.bundle / ".multikernel").mkdir()
        (self.bundle / ".multikernel-image").mkdir()
        self.check("rootfs", True)
        self.check(str(self.bundle / "rootfs"), True, self.temp)
        self.check(".multikernel-image", True)

    def test_inherited_bundle_overlap_uses_held_inode_after_rename(self):
        descriptor = os.open(self.bundle, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
        self.addCleanup(os.close, descriptor)
        moved = self.temp / "held-bundle"
        self.bundle.rename(moved)
        self.bundle.mkdir()
        (moved / ".multikernel").mkdir()
        config = self.temp / "descriptor-overlap.json"
        for root in (moved, self.temp, moved / ".multikernel"):
            with self.subTest(root=root.name):
                config.write_text(json.dumps({"root": {"path": str(root)}}))
                result = subprocess.run(
                    [str(SCRIPT), f"/proc/self/fd/{descriptor}", str(config), "--json"],
                    pass_fds=(descriptor,), capture_output=True, text=True,
                    env={**os.environ, "MK_ALLOWED_ABSOLUTE_ROOTS": str(self.temp)},
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, "")

    def test_overlap_validation_releases_held_descriptors(self):
        spec = importlib.util.spec_from_file_location("root_policy_fixture", SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        runtime = self.bundle / ".multikernel"
        runtime.mkdir()
        before = len(os.listdir("/proc/self/fd"))
        for _ in range(20):
            module.resolve(self.bundle, "rootfs", [self.temp])
            for root in (str(self.bundle), str(runtime)):
                with self.assertRaises(ValueError):
                    module.resolve(self.bundle, root, [self.temp])
        self.assertEqual(len(os.listdir("/proc/self/fd")), before)

    def test_builder_rejects_overlap_before_bootstrap_or_output(self):
        staged = self.temp / "builder-generation"
        staged.mkdir(mode=0o700)
        for source in SCRIPT.parent.glob("*.py"):
            destination = staged / source.name
            shutil.copyfile(source, destination)
            destination.chmod(0o755)
        builder = staged / "build-runtime-container-initramfs.sh"
        shutil.copyfile(SCRIPT.with_name(builder.name), builder)
        builder.chmod(0o755)
        runtime = self.bundle / ".multikernel"
        runtime.mkdir()
        token = runtime / "token"
        token.write_text("SYNTHETIC_PRIVATE_TOKEN")
        output = self.temp / "output" / "initramfs.cpio.gz"
        output.parent.mkdir()
        config = {"ociVersion": "1.1.0", "process": {"cwd": "/", "args": ["/bin/true"], "user": {"uid": 0, "gid": 0}},
                  "root": {"path": str(self.bundle)}, "linux": {"namespaces": [{"type": "network", "path": "/run/netns/test"}]}}
        (self.bundle / "config.json").write_text(json.dumps(config))
        (self.bundle / "config.json").chmod(0o600)
        result = subprocess.run(
            [str(builder), str(self.bundle), str(output)], capture_output=True, text=True,
            env={**os.environ, "MK_ALLOWED_ABSOLUTE_ROOTS": str(self.temp), "MK_TASK_IDENTITY": "synthetic-task",
                 "MK_STORAGE_PORT": "4061", "MK_KERNEL_MANIFEST": "/nonexistent/private-manifest"},
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("OCI source includes the host bundle control directory", result.stderr)
        self.assertNotIn("SYNTHETIC_PRIVATE_TOKEN", result.stdout + result.stderr)
        self.assertEqual(list(output.parent.iterdir()), [])
        self.assertEqual(token.read_text(), "SYNTHETIC_PRIVATE_TOKEN")

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
