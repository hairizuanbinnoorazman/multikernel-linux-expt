#!/usr/bin/env python3
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest
from types import SimpleNamespace
from unittest import mock


SCRIPT = Path(__file__).with_name("build-runtime-storage.py")


def load_builder():
    spec = importlib.util.spec_from_file_location("runtime_storage_builder", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


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

    def arguments(self, name):
        return SimpleNamespace(
            root=self.root,
            output=self.temp / f"{name}.ext4",
            metadata=self.temp / f"{name}.json",
            logical_path=None,
            image_id="source-manifest-abcd",
            uuid="11111111-2222-4333-8444-555555555555",
            size=64 << 20,
            inodes=4096,
            port=4061,
            min_free_bytes=0,
            mke2fs="/usr/sbin/mke2fs",
            e2fsck="/usr/sbin/e2fsck",
            debugfs="/usr/sbin/debugfs",
        )

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

    def test_raced_output_and_metadata_collisions_preserve_replacements(self):
        builder = load_builder()

        arguments = self.arguments("raced-image")
        original_publish = builder.publish_existing

        def collide_image(source, destination, expected):
            destination.write_bytes(b"replacement image")
            return original_publish(source, destination, expected)

        with mock.patch.object(builder, "publish_existing", side_effect=collide_image):
            with self.assertRaisesRegex(builder.PublicationError, "refusing to overwrite"):
                builder.build(arguments)
        self.assertEqual(arguments.output.read_bytes(), b"replacement image")
        self.assertFalse(arguments.metadata.exists())

        arguments = self.arguments("raced-metadata")
        original_json = builder.atomic_json

        def collide_metadata(path, value):
            path.write_bytes(b"replacement metadata")
            return original_json(path, value)

        with mock.patch.object(builder, "atomic_json", side_effect=collide_metadata):
            with self.assertRaisesRegex(builder.PublicationError, "refusing to overwrite"):
                builder.build(arguments)
        self.assertFalse(arguments.output.exists())
        self.assertEqual(arguments.metadata.read_bytes(), b"replacement metadata")

    def test_staging_consumers_use_held_directory_and_preserve_public_replacement(self):
        builder = load_builder()
        arguments = self.arguments("raced-staging")
        original_normalize = builder.normalize_tree_times
        moved = self.temp / "held-staging"

        def replace_staging(staged_root):
            candidates = list(self.temp.glob(".root-staging.*"))
            self.assertEqual(len(candidates), 1)
            candidates[0].rename(moved)
            candidates[0].mkdir(mode=0o700)
            (candidates[0] / "replacement-marker").write_text("preserved")
            original_normalize(staged_root)

        with mock.patch.object(builder, "normalize_tree_times", side_effect=replace_staging):
            with self.assertRaisesRegex(builder.StorageBuildError, "staging directory identity changed"):
                builder.build(arguments)
        self.assertFalse(arguments.output.exists())
        self.assertFalse(arguments.metadata.exists())
        replacements = list(self.temp.glob(".root-staging.*"))
        self.assertEqual(len(replacements), 1)
        self.assertEqual((replacements[0] / "replacement-marker").read_text(), "preserved")
        self.assertTrue((moved / "root/etc/value").is_file())

    def test_source_copy_uses_held_root_after_public_path_replacement(self):
        builder = load_builder()
        arguments = self.arguments("raced-source")
        original_build = builder._build_held
        moved = self.temp / "held-source"

        def replace_public_root(build_arguments, held_root, root_fd):
            self.root.rename(moved)
            (self.root / "etc").mkdir(parents=True)
            (self.root / "etc" / "value").write_text("replacement\n")
            return original_build(build_arguments, held_root, root_fd)

        with mock.patch.object(builder, "_build_held", side_effect=replace_public_root):
            builder.build(arguments)
        inspection = subprocess.run(
            [arguments.debugfs, "-R", "cat /etc/value", str(arguments.output)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        self.assertEqual(inspection.returncode, 0, inspection.stderr)
        self.assertEqual(inspection.stdout, "one\n")
        self.assertEqual((self.root / "etc" / "value").read_text(), "replacement\n")

    def test_source_copy_accepts_an_exact_inherited_root_descriptor(self):
        builder = load_builder()
        arguments = self.arguments("inherited-source")
        descriptor = os.open(self.root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
        self.addCleanup(os.close, descriptor)
        arguments.root = Path(f"/proc/self/fd/{descriptor}")
        moved = self.temp / "inherited-source-held"
        self.root.rename(moved)
        (self.root / "etc").mkdir(parents=True)
        (self.root / "etc" / "value").write_text("replacement\n")
        builder.build(arguments)
        inspection = subprocess.run(
            [arguments.debugfs, "-R", "cat /etc/value", str(arguments.output)],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        self.assertEqual(inspection.returncode, 0, inspection.stderr)
        self.assertEqual(inspection.stdout, "one\n")
        self.assertEqual((self.root / "etc" / "value").read_text(), "replacement\n")


if __name__ == "__main__":
    unittest.main()
