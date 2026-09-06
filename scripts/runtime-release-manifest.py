#!/usr/bin/env python3
"""Create a deterministic, self-verifying runtime binary manifest."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys


COMPONENTS = (
    "containerd-shim-multikernel-v2",
    "mk-agent",
    "mk-agentctl",
    "mk-cni",
    "mk-host-check",
    "mknetd",
    "mkruntimed",
)
IDENTITY = re.compile(
    r"^(?P<name>[a-z0-9][a-z0-9.-]*) version=(?P<version>[^\s]+) "
    r"revision=(?P<revision>[0-9a-f]{40})\n$"
)


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def inspect(path: Path, expected_version: str, expected_revision: str) -> dict:
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode):
        raise ValueError(f"{path}: component must be a regular non-symlink file")
    if info.st_mode & 0o111 == 0:
        raise ValueError(f"{path}: component is not executable")
    completed = subprocess.run(
        [os.fspath(path), "--version"],
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=10,
    )
    if completed.returncode != 0 or completed.stderr:
        raise ValueError(
            f"{path}: version command failed rc={completed.returncode} "
            f"stderr={completed.stderr!r}"
        )
    match = IDENTITY.fullmatch(completed.stdout)
    if not match or match.group("name") != path.name:
        raise ValueError(f"{path}: malformed component identity {completed.stdout!r}")
    if match.group("version") != expected_version:
        raise ValueError(f"{path}: version does not match {expected_version!r}")
    if match.group("revision") != expected_revision:
        raise ValueError(f"{path}: revision does not match {expected_revision!r}")
    return {
        "name": path.name,
        "version": match.group("version"),
        "revision": match.group("revision"),
        "size_bytes": info.st_size,
        "sha256": digest(path),
    }


def create(binary_dir: Path, expected_version: str, expected_revision: str) -> dict:
    if not re.fullmatch(r"[0-9A-Za-z][0-9A-Za-z.+_-]{0,127}", expected_version):
        raise ValueError("expected version has an unsafe or empty value")
    if not re.fullmatch(r"[0-9a-f]{40}", expected_revision):
        raise ValueError("expected revision must be a lowercase 40-hex commit")
    return {
        "schema_version": 1,
        "runtime": "io.containerd.multikernel.v2",
        "version": expected_version,
        "revision": expected_revision,
        "components": [
            inspect(binary_dir / name, expected_version, expected_revision)
            for name in COMPONENTS
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("binary_dir", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--expected-version", required=True)
    parser.add_argument("--expected-revision", required=True)
    arguments = parser.parse_args()
    try:
        manifest = create(
            arguments.binary_dir,
            arguments.expected_version,
            arguments.expected_revision,
        )
        payload = (json.dumps(manifest, indent=2, sort_keys=True) + "\n").encode()
        descriptor = os.open(
            arguments.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o644
        )
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"runtime release manifest: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
