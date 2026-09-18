#!/usr/bin/env python3
import gzip
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("build-runtime-rootfs.py")
VERIFIER = Path(__file__).with_name("verify-runtime-rootfs.py")


def load_builder():
    spec = importlib.util.spec_from_file_location("runtime_rootfs_builder", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    previous = sys.dont_write_bytecode
    sys.dont_write_bytecode = True
    try:
        spec.loader.exec_module(module)
    finally:
        sys.dont_write_bytecode = previous
    return module


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
        verification = subprocess.run([str(VERIFIER), str(archive_a), str(manifest_a)], text=True, capture_output=True)
        self.assertEqual(verification.returncode, 0, verification.stderr)
        verified = json.loads(verification.stdout)
        self.assertEqual(verified["archive_sha256"], hashlib.sha256(archive_a.read_bytes()).hexdigest())

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

    def test_rejects_mutation_after_file_read(self):
        source = self.root / "value"
        source.write_text("before\n")
        builder = load_builder()
        original = builder._read_stable

        def mutate_after_read(path, before):
            data = original(path, before)
            if path.name == "value":
                path.write_text("after\n")
            return data

        with mock.patch.object(builder, "_read_stable", side_effect=mutate_after_read):
            with self.assertRaisesRegex(builder.RootFSError, "input mutated during build"):
                builder.scan(self.root)

    def test_rejects_fifo(self):
        os.mkfifo(self.root / "fifo")
        result, _, _ = self.build("fifo")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported file type", result.stderr)

    def test_rejects_socket_and_external_hardlink(self):
        sock = socket.socket(socket.AF_UNIX)
        self.addCleanup(sock.close)
        try:
            sock.bind(str(self.root / "socket"))
        except PermissionError as error:
            print(f"socket rejection subcase skipped: {error}")
        else:
            result, _, _ = self.build("socket")
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("unsupported file type", result.stderr)
            os.unlink(self.root / "socket")
        inside = self.root / "inside"
        inside.write_text("linked")
        os.link(inside, self.temp / "outside")
        result, _, _ = self.build("external-hardlink")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("hardlink group crosses the root boundary", result.stderr)

    def test_sparse_metadata_identity_and_large_tree_are_deterministic(self):
        sparse = self.root / "sparse"
        with sparse.open("wb") as stream:
            stream.seek((1 << 20) - 1)
            stream.write(b"x")
        os.chmod(sparse, 0o604)
        for index in range(1000):
            (self.root / f"entry-{index:04d}").write_text(str(index))
        result, archive, manifest = self.build("large-sparse")
        self.assertEqual(result.returncode, 0, result.stderr)
        entry = {item["path"]: item for item in json.loads(manifest.read_text())["entries"]}["sparse"]
        self.assertEqual(entry["size"], 1 << 20)
        self.assertEqual(entry["uid"], os.getuid())
        self.assertEqual(entry["gid"], os.getgid())
        self.assertEqual(entry["mode"] & 0o777, 0o604)
        self.assertEqual(json.loads(subprocess.check_output([str(VERIFIER), str(archive), str(manifest)]))["entries"], 1002)

    def test_mtime_and_sparse_allocation_are_normalized(self):
        target = self.root / "content"
        payload = b"prefix" + (b"\0" * (1 << 20)) + b"suffix"
        with target.open("wb") as stream:
            stream.write(b"prefix")
            stream.seek((1 << 20) + len(b"prefix"))
            stream.write(b"suffix")
        os.utime(target, ns=(1_000_000_000, 1_000_000_000))
        first, archive_a, manifest_a = self.build("sparse-form")
        self.assertEqual(first.returncode, 0, first.stderr)
        # Replace sparse extents with allocated zero bytes and choose unrelated
        # timestamps. The admitted bytes and OCI-visible metadata are equal.
        target.write_bytes(payload)
        os.utime(target, ns=(9_000_000_000, 9_000_000_000))
        second, archive_b, manifest_b = self.build("dense-form")
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(archive_a.read_bytes(), archive_b.read_bytes())
        self.assertEqual(manifest_a.read_bytes(), manifest_b.read_bytes())

    def test_verifier_rejects_archive_and_manifest_corruption(self):
        (self.root / "value").write_text("content")
        result, archive, manifest = self.build("verified")
        self.assertEqual(result.returncode, 0, result.stderr)
        corrupt_archive = self.temp / "corrupt.cpio.gz"
        data = bytearray(archive.read_bytes())
        data[len(data) // 2] ^= 0x40
        corrupt_archive.write_bytes(data)
        verification = subprocess.run([str(VERIFIER), str(corrupt_archive), str(manifest)], text=True, capture_output=True)
        self.assertNotEqual(verification.returncode, 0)
        corrupt_manifest = self.temp / "corrupt.json"
        value = json.loads(manifest.read_text())
        value["entries"][-1]["uid"] += 1
        corrupt_manifest.write_text(json.dumps(value))
        verification = subprocess.run([str(VERIFIER), str(archive), str(corrupt_manifest)], text=True, capture_output=True)
        self.assertNotEqual(verification.returncode, 0)

        duplicate_manifest = self.temp / "duplicate.json"
        original = manifest.read_text()
        duplicate_manifest.write_text(original.replace('"schema_version":1', '"schema_version":1,"schema_version":1', 1))
        verification = subprocess.run([str(VERIFIER), str(archive), str(duplicate_manifest)], text=True, capture_output=True)
        self.assertNotEqual(verification.returncode, 0)
        self.assertIn("duplicate manifest key", verification.stderr)

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
            self.assertFalse(manifest.exists())

    def test_output_publication_is_no_replace_and_rolls_back_its_peer(self):
        (self.root / "value").write_text("content")
        output = self.temp / "exclusive.cpio.gz"
        manifest = self.temp / "exclusive.json"

        output.write_bytes(b"existing archive")
        manifest.write_bytes(b"existing manifest")
        result = subprocess.run([str(SCRIPT), str(self.root), str(output), str(manifest)], text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("refusing to overwrite existing output", result.stderr)
        self.assertEqual(output.read_bytes(), b"existing archive")
        self.assertEqual(manifest.read_bytes(), b"existing manifest")

        output.unlink()
        result = subprocess.run([str(SCRIPT), str(self.root), str(output), str(manifest)], text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("refusing to overwrite existing output", result.stderr)
        self.assertFalse(output.exists())
        self.assertEqual(manifest.read_bytes(), b"existing manifest")

        manifest.unlink()
        output.write_bytes(b"existing archive")
        result = subprocess.run([str(SCRIPT), str(self.root), str(output), str(manifest)], text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(manifest.exists())
        self.assertEqual(output.read_bytes(), b"existing archive")

    def test_identity_rollback_preserves_replacement(self):
        builder = load_builder()
        output = self.temp / "owned"
        output.write_bytes(b"owned")
        expected = builder._file_identity(output)
        original = self.temp / "owned-original"
        output.rename(original)
        output.write_bytes(b"replacement")
        self.assertFalse(builder._remove_if_identity(output, expected))
        self.assertEqual(output.read_bytes(), b"replacement")
        self.assertEqual(original.read_bytes(), b"owned")

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

    @unittest.skipUnless(hasattr(os, "setxattr"), "xattrs unavailable")
    def test_materializes_overlay_opaque_directory_metadata(self):
        target = self.root / "opaque"
        target.mkdir()
        (target / "visible").write_text("materialized\n")
        try:
            os.setxattr(target, "user.overlay.opaque", b"y")
        except OSError as error:
            self.skipTest(f"overlay opacity xattr unavailable: {error}")
        result, archive, manifest = self.build("opaque")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(manifest.read_text())
        self.assertEqual(data["normalization"]["overlay_opacity"], "materialized-view-normalized")
        self.assertTrue(archive.exists())

    @unittest.skipUnless(hasattr(os, "setxattr"), "xattrs unavailable")
    def test_rejects_malformed_overlay_opacity(self):
        for name, directory, value in (
            ("wrong-type", False, b"y"),
            ("wrong-value", True, b"not-opaque"),
        ):
            with self.subTest(name=name):
                target = self.root / name
                target.mkdir() if directory else target.write_text("content")
                try:
                    os.setxattr(target, "user.overlay.opaque", value)
                except OSError as error:
                    self.skipTest(f"overlay xattrs unavailable: {error}")
                result, _, _ = self.build(name)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("malformed overlay opacity", result.stderr)

    def test_rejects_whiteout_or_device_node(self):
        target = self.root / "whiteout"
        try:
            os.mknod(target, stat.S_IFCHR | 0o600, os.makedev(0, 0))
        except (PermissionError, OSError) as error:
            self.skipTest(f"device node creation unavailable: {error}")
        result, _, _ = self.build("whiteout")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported file type", result.stderr)


if __name__ == "__main__":
    unittest.main()
