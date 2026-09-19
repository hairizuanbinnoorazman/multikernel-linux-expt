#!/usr/bin/env python3
"""Focused pre-allocation image/kernel architecture binding tests."""

import copy
import hashlib
import importlib.util
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
from unittest import mock


VALIDATOR = pathlib.Path(__file__).with_name("validate-runtime-image.py")


def load_validator():
    spec = importlib.util.spec_from_file_location("runtime_image_validator", VALIDATOR)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


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

        config_path.write_text(json.dumps(config), encoding="utf-8")
        validator = load_validator()
        real_open = validator.os.open
        moved_bin = root / "bin-held"
        outside = directory / "outside-bin"
        outside.mkdir()
        (outside / "program").write_bytes(
            b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little")
        )
        (outside / "program").chmod(0o755)
        replaced_child = False

        def replace_child_component(path, flags, *args, **kwargs):
            nonlocal replaced_child
            if os.fspath(path) == "program" and not replaced_child:
                replaced_child = True
                (root / "bin").rename(moved_bin)
                (root / "bin").symlink_to(outside, target_is_directory=True)
            return real_open(path, flags, *args, **kwargs)

        with mock.patch.object(validator.os, "open", side_effect=replace_child_component):
            child_result = validator.validate(root, config_path, bootstrap_path)
        original_digest = hashlib.sha256((moved_bin / "program").read_bytes()).hexdigest()
        if child_result["entrypoint_sha256"] != original_digest:
            raise AssertionError("entrypoint traversal escaped a held child descriptor")
        if (root / "bin" / "program").read_bytes()[18:20] != (183).to_bytes(2, "little"):
            raise AssertionError("child-path replacement was modified")
        (root / "bin").unlink()
        moved_bin.rename(root / "bin")

        validator = load_validator()
        original_validate = validator._validate_held
        moved = directory / "held-root"

        def replace_public_root(held_root, held_config, held_bootstrap):
            root.rename(moved)
            (root / "bin").mkdir(parents=True)
            (root / "bin" / "program").write_bytes(
                b"\x7fELF\x02\x01" + b"\0" * 12 + (183).to_bytes(2, "little")
            )
            (root / "bin" / "program").chmod(0o755)
            return original_validate(held_root, held_config, held_bootstrap)

        config_path.write_text(json.dumps(config), encoding="utf-8")
        with mock.patch.object(validator, "_validate_held", side_effect=replace_public_root):
            held_result = validator.validate(root, config_path, bootstrap_path)
        expected = hashlib.sha256((moved / "bin" / "program").read_bytes()).hexdigest()
        if held_result["entrypoint_sha256"] != expected:
            raise AssertionError("entrypoint validation escaped the held root")
        if (root / "bin" / "program").read_bytes()[18:20] != (183).to_bytes(2, "little"):
            raise AssertionError("public replacement was modified")
    print("runtime image architecture binding: PASS (ELF, interpreter, wrong-arch, escape, held root/child races)")


if __name__ == "__main__":
    main()
