#!/usr/bin/env python3

import json
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
