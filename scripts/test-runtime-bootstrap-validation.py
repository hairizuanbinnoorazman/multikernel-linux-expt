#!/usr/bin/env python3
"""Focused tests for approved agent/relay/transport bootstrap resolution."""

import copy
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile


REPO = pathlib.Path(__file__).resolve().parent.parent
VALIDATOR = REPO / "scripts/validate-runtime-bootstrap.py"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(path):
    return subprocess.run(
        [str(VALIDATOR), str(path), "test", "--required-uid", str(os.getuid()), "--skip-modinfo", "--skip-parent-safety"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def main():
    with tempfile.TemporaryDirectory(prefix="mk-bootstrap-") as temporary:
        directory = pathlib.Path(temporary)
        executable = pathlib.Path(sys.executable).resolve()
        artifacts = {}
        for name in ("vmlinux", "mk-agent", "mkvsock-relay"):
            target = directory / name
            shutil.copyfile(executable, target)
            target.chmod(0o755)
            artifacts[name] = {"path": str(target), "sha256": digest(target)}
        initramfs = directory / "initramfs"
        module = directory / "mk_transport.ko"
        initramfs.write_bytes(b"initramfs")
        module.write_bytes(b"module")
        initramfs.chmod(0o644)
        module.chmod(0o644)
        manifest = {
            "schema_version": 1,
            "name": "test",
            "architecture": "amd64",
            "kernel_release": "test-release",
            "compatibility": {"multikernel_revision": "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7", "kerf_version": "v0.2.0", "kerf_revision": "8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec"},
            "kernel": artifacts["vmlinux"],
            "initramfs": {"path": str(initramfs), "sha256": digest(initramfs)},
            "agent": artifacts["mk-agent"],
            "relay": artifacts["mkvsock-relay"],
            "transport": {"module": {"path": str(module), "sha256": digest(module)}, "module_name": "mk_transport", "socket_option": 9, "transport_id": 1, "primary_role": "server", "child_role": "client"},
            "protocol": {"min": 1, "max": 1},
            "required_config": ["CONFIG_MULTIKERNEL=y"],
            "oci_features": ["argv"],
        }
        path = directory / "test.json"
        path.write_text(json.dumps(manifest), encoding="utf-8")
        path.chmod(0o600)
        result = validate(path)
        if result.returncode != 0:
            raise AssertionError(result.stderr)
        resolved = json.loads(result.stdout)
        if resolved["relay"] != str(directory / "mkvsock-relay") or resolved["module"] != str(module):
            raise AssertionError(resolved)

        for name, mutate in (
            ("direction", lambda value: value["transport"].update(child_role="server")),
            ("digest", lambda value: value["relay"].update(sha256="0" * 64)),
            ("unknown", lambda value: value.update(unknown=True)),
        ):
            candidate = copy.deepcopy(manifest)
            mutate(candidate)
            path.write_text(json.dumps(candidate), encoding="utf-8")
            if validate(path).returncode == 0:
                raise AssertionError(f"{name} mismatch accepted")

        path.write_text(json.dumps(manifest), encoding="utf-8")
        path.chmod(0o622)
        if validate(path).returncode == 0:
            raise AssertionError("writable manifest accepted")
    print("runtime bootstrap validation: PASS (approved hashes, ownership, transport ID and direction)")


if __name__ == "__main__":
    main()
