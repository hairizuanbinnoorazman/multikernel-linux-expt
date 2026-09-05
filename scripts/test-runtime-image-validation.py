#!/usr/bin/env python3
"""Focused pre-allocation image/kernel architecture binding tests."""

import copy
import json
import pathlib
import shutil
import subprocess
import sys
import tempfile


VALIDATOR = pathlib.Path(__file__).with_name("validate-runtime-image.py")


def main():
    with tempfile.TemporaryDirectory(prefix="mk-image-validation-") as temporary:
        directory = pathlib.Path(temporary)
        root = directory / "root"
        (root / "bin").mkdir(parents=True)
        shutil.copyfile(pathlib.Path(sys.executable).resolve(), root / "bin" / "program")
        (root / "bin" / "program").chmod(0o755)
        config = {"process": {"args": ["/bin/program"]}}
        bootstrap = {
            "architecture": "amd64",
            "manifest_sha256": "1" * 64,
            "kernel_release": "test-release",
            "required_config": ["CONFIG_64BIT=y"],
            "oci_features": ["argv"],
        }
        config_path, bootstrap_path = directory / "config.json", directory / "bootstrap.json"
        config_path.write_text(json.dumps(config), encoding="utf-8")
        bootstrap_path.write_text(json.dumps(bootstrap), encoding="utf-8")

        def run(candidate=config):
            config_path.write_text(json.dumps(candidate), encoding="utf-8")
            return subprocess.run([str(VALIDATOR), str(root), str(config_path), str(bootstrap_path)], text=True, capture_output=True)

        result = run()
        if result.returncode != 0:
            raise AssertionError(result.stderr)
        observed = json.loads(result.stdout)
        if observed["architecture"] != "amd64" or observed["kernel_manifest_sha256"] != "1" * 64:
            raise AssertionError(observed)

        (root / "bin" / "wrong").write_bytes(b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little"))
        wrong = copy.deepcopy(config)
        wrong["process"]["args"][0] = "/bin/wrong"
        if run(wrong).returncode == 0:
            raise AssertionError("AArch64 entrypoint accepted")

        (root / "bin" / "script").write_text("#!/bin/program\n", encoding="utf-8")
        (root / "bin" / "script").chmod(0o755)
        script = copy.deepcopy(config)
        script["process"]["args"][0] = "/bin/script"
        if run(script).returncode != 0:
            raise AssertionError("valid x86-64 shebang interpreter rejected")

        (root / "escape").symlink_to("../../outside")
        escaped = copy.deepcopy(config)
        escaped["process"]["args"][0] = "/escape"
        if run(escaped).returncode == 0:
            raise AssertionError("escaping entrypoint symlink accepted")
    print("runtime image architecture binding: PASS (ELF, interpreter, wrong-arch, escape)")


if __name__ == "__main__":
    main()
