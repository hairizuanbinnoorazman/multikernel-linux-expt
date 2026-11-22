#!/usr/bin/env python3

import json
import os
import stat
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("merge-runtime-docker-config.py")
RUNTIME = "io.containerd.multikernel.v2"


class DockerConfigTests(unittest.TestCase):
    def run_merge(self, source, *options):
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(directory))
        input_path, output_path = directory / "daemon.json", directory / "candidate.json"
        if source is not None:
            input_path.write_text(source)
        result = subprocess.run([str(SCRIPT), str(input_path), str(output_path), *options],
                                text=True, capture_output=True)
        return result, output_path

    def test_merge_preserves_settings_and_default_runtime(self):
        source = {"default-runtime": "runc", "log-level": "warn", "runtimes": {"kata": {}}}
        result, output = self.run_merge(json.dumps(source))
        self.assertEqual(result.returncode, 0, result.stderr)
        value = json.loads(output.read_text())
        self.assertEqual(value["default-runtime"], "runc")
        self.assertEqual(value["log-level"], "warn")
        self.assertEqual(value["runtimes"]["kata"], {})
        self.assertEqual(value["runtimes"][RUNTIME], {"runtimeType": RUNTIME})

    def test_install_remove_round_trip_and_missing_input(self):
        installed, output = self.run_merge(None)
        self.assertEqual(installed.returncode, 0, installed.stderr)
        removed, final = self.run_merge(output.read_text(), "--remove")
        self.assertEqual(removed.returncode, 0, removed.stderr)
        self.assertEqual(json.loads(final.read_text()), {})

    def test_live_restore_is_explicit_idempotent_and_preserves_settings(self):
        source = {"default-runtime": "runc", "log-level": "warn"}
        result, output = self.run_merge(json.dumps(source), "--enable-live-restore")
        self.assertEqual(result.returncode, 0, result.stderr)
        value = json.loads(output.read_text())
        self.assertIs(value["live-restore"], True)
        self.assertEqual(value["default-runtime"], "runc")
        self.assertEqual(value["log-level"], "warn")
        self.assertEqual(value["runtimes"][RUNTIME], {"runtimeType": RUNTIME})

        repeated, repeated_output = self.run_merge(output.read_text(), "--enable-live-restore")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(json.loads(repeated_output.read_text()), value)

    def test_live_restore_refuses_conflicting_existing_policy(self):
        for existing in (False, "true", 1, {}):
            result, output = self.run_merge(
                json.dumps({"live-restore": existing}), "--enable-live-restore")
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(output.exists())

    def test_live_restore_and_remove_are_mutually_exclusive(self):
        result, output = self.run_merge(
            "{}", "--enable-live-restore", "--remove")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(output.exists())

    def test_bounded_logging_preserves_settings_and_is_idempotent(self):
        source = {"default-runtime": "runc", "debug": False,
                  "log-driver": "local", "log-opts": {"max-size": "1m"}}
        result, output = self.run_merge(json.dumps(source), "--enable-bounded-logging",
                                        "--enable-live-restore")
        self.assertEqual(result.returncode, 0, result.stderr)
        value = json.loads(output.read_text())
        self.assertEqual(value["default-runtime"], "runc")
        self.assertIs(value["debug"], False)
        self.assertIs(value["live-restore"], True)
        self.assertEqual(value["log-opts"], {
            "max-size": "1m", "max-file": "2", "compress": "false",
            "mode": "non-blocking", "max-buffer-size": "1m"})
        repeated, final = self.run_merge(output.read_text(), "--enable-bounded-logging")
        self.assertEqual(repeated.returncode, 0, repeated.stderr)
        self.assertEqual(json.loads(final.read_text()), value)
        removed, final = self.run_merge(output.read_text(), "--remove")
        self.assertEqual(removed.returncode, 0, removed.stderr)
        expected = dict(value)
        expected.pop("runtimes")
        self.assertEqual(json.loads(final.read_text()), expected)

    def test_bounded_logging_rejects_conflicts_before_creating_output(self):
        cases = [{"log-driver": driver} for driver in (None, "json-file", 1, {})]
        cases += [{"log-opts": options} for options in (
            None, [], "secret-marker", {"max-file": 2}, {"mode": "blocking"},
            {"env": "secret-marker"}, {"max-size": "2m"})]
        for source in cases:
            with self.subTest(source=source):
                result, output = self.run_merge(json.dumps(source), "--enable-bounded-logging")
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output.exists())
                self.assertNotIn("secret-marker", result.stderr)
        result, output = self.run_merge("{}", "--remove", "--enable-bounded-logging")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(output.exists())

    def test_registration_preserves_existing_logging_policy(self):
        source = {"log-driver": "json-file", "log-opts": {"labels": "application"}}
        result, output = self.run_merge(json.dumps(source))
        self.assertEqual(result.returncode, 0, result.stderr)
        value = json.loads(output.read_text())
        for key in source:
            self.assertEqual(value[key], source[key])

    def test_candidate_is_private_under_permissive_umask_and_never_replaces(self):
        old = os.umask(0)
        try:
            result, output = self.run_merge("{}", "--enable-bounded-logging")
        finally:
            os.umask(old)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
        original = output.read_bytes()
        for candidate in (output, output.parent / "link.json"):
            if candidate != output:
                candidate.symlink_to(output)
            result = subprocess.run([str(SCRIPT), str(output.parent / "daemon.json"),
                                     str(candidate), "--enable-bounded-logging"],
                                    text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(output.read_bytes(), original)
        self.assertTrue((output.parent / "link.json").is_symlink())

    def test_rejects_duplicate_keys_and_conflicting_entry(self):
        for source in (
            '{"runtimes":{},"runtimes":{}}',
            json.dumps({"runtimes": {RUNTIME: {"runtimeType": "other"}}}),
            json.dumps({"runtimes": []}),
        ):
            result, output = self.run_merge(source)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
