#!/usr/bin/env python3
"""Materialize admitted read-only OCI bind directories into a private root.

The original host paths are never projected into the child.  Each source is
manifested before and after an archive-semantic copy, and the copied tree must
have the same normalized manifest before it can be published.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path, PurePosixPath
import stat
import subprocess
import sys


class MaterializationError(Exception):
    pass


def load_rootfs_builder():
    path = Path(__file__).with_name("build-runtime-rootfs.py")
    spec = importlib.util.spec_from_file_location("runtime_rootfs_builder_for_binds", path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    assert spec.loader is not None
    previous = sys.dont_write_bytecode
    sys.dont_write_bytecode = True
    try:
        spec.loader.exec_module(module)
    finally:
        sys.dont_write_bytecode = previous
    return module


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise MaterializationError(f"duplicate bind-plan key {key!r}")
        result[key] = value
    return result


def real_directory(path: Path, name: str) -> Path:
    if not path.is_absolute() or Path(os.path.normpath(path)) != path:
        raise MaterializationError(f"{name} must be absolute and canonical")
    current = Path("/")
    for component in path.parts[1:]:
        current /= component
        try:
            info = current.lstat()
        except OSError as error:
            raise MaterializationError(f"{name} is unavailable: {error}") from error
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
            raise MaterializationError(f"{name} must contain only real directories")
    return path


def manifest_bytes(builder, root: Path) -> tuple[bytes, int, int]:
    entries, _ = builder.scan(root)
    payload_bytes = sum(item.size for item in entries if item.kind == "regular")
    return builder.manifest(entries), payload_bytes, len(entries)


def exclusive_write(path: Path, data: bytes) -> None:
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600)
    try:
        with os.fdopen(descriptor, "wb", closefd=False) as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
    finally:
        os.close(descriptor)


def destination_directory(root: Path, destination: str) -> Path:
    pure = PurePosixPath(destination)
    if not destination.startswith("/") or pure.as_posix() != destination or destination == "/" or ".." in pure.parts:
        raise MaterializationError("bind destination is not absolute and canonical")
    current = root
    parts = pure.parts[1:]
    for component in parts[:-1]:
        current /= component
        try:
            info = current.lstat()
        except FileNotFoundError:
            current.mkdir(mode=0o755)
            info = current.lstat()
        if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
            raise MaterializationError(f"bind destination parent is not a real directory: {destination}")
    target = current / parts[-1]
    try:
        target.lstat()
    except FileNotFoundError:
        target.mkdir(mode=0o755)
    else:
        raise MaterializationError(f"bind destination already exists in the OCI root: {destination}")
    return target


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("plan", type=Path)
    parser.add_argument("target_root", type=Path)
    parser.add_argument("result", type=Path)
    parser.add_argument("--max-bytes", type=int, default=1 << 30)
    parser.add_argument("--max-inodes", type=int, default=131072)
    args = parser.parse_args()
    if args.max_bytes < 0 or args.max_inodes < 1:
        raise MaterializationError("read-only bind limits are invalid")
    builder = load_rootfs_builder()
    plan = json.loads(args.plan.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    if set(plan) != {"schema_version", "readonly_binds"} or plan["schema_version"] != 1 or not isinstance(plan["readonly_binds"], list):
        raise MaterializationError("invalid read-only bind plan")
    if len(plan["readonly_binds"]) > 8:
        raise MaterializationError("at most eight read-only bind inputs are supported")
    root = real_directory(args.target_root, "materialization root")
    records = []
    destinations = []
    total_bytes = 0
    total_inodes = 0
    for index, item in enumerate(plan["readonly_binds"]):
        if not isinstance(item, dict) or set(item) != {"destination", "type", "source", "options"}:
            raise MaterializationError(f"invalid read-only bind record {index}")
        if (item["type"] != "bind" or item["options"] != ["bind", "ro", "nodev", "nosuid", "noexec"] or
                not isinstance(item["source"], str) or not isinstance(item["destination"], str)):
            raise MaterializationError(f"read-only bind record {index} differs from the enforced contract")
        destination = item["destination"]
        if destination == "/" or any(destination == protected or destination.startswith(protected + "/")
                                     for protected in ("/dev", "/proc", "/run", "/sys")):
            raise MaterializationError(f"read-only bind destination {destination!r} overlaps a runtime-owned path")
        if any(destination == prior or destination.startswith(prior + "/") or prior.startswith(destination + "/")
               for prior in destinations):
            raise MaterializationError("read-only bind destinations overlap")
        destinations.append(destination)
        source = real_directory(Path(item["source"]), f"read-only bind source {index}")
        before, payload_bytes, inodes = manifest_bytes(builder, source)
        total_bytes += payload_bytes
        total_inodes += inodes
        if total_bytes > args.max_bytes or total_inodes > args.max_inodes:
            raise MaterializationError("read-only bind inputs exceed the aggregate byte or inode limit")
        target = destination_directory(root, destination)
        completed = subprocess.run(
            ["/bin/cp", "--archive", "--reflink=never", "--", str(source) + "/.", str(target)],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
        )
        if completed.returncode != 0:
            raise MaterializationError(f"copy read-only bind {index} failed")
        after, _, _ = manifest_bytes(builder, source)
        copied, _, _ = manifest_bytes(builder, target)
        if before != after:
            raise MaterializationError(f"read-only bind source {index} mutated during materialization")
        if before != copied:
            raise MaterializationError(f"read-only bind copy {index} differs from its admitted source")
        records.append({
            "destination": item["destination"],
            "source": item["source"],
            "manifest": json.loads(before),
            "manifest_sha256": hashlib.sha256(before).hexdigest(),
            "ownership": "numeric-uid-gid-preserved",
            "propagation": "none-materialized-copy",
            "guest_policy": "bind-remount-ro-nodev-nosuid-noexec",
        })
    encoded = (json.dumps({"schema_version": 1, "readonly_binds": records}, sort_keys=True, separators=(",", ":")) + "\n").encode()
    if len(encoded) > 128 << 20:
        raise MaterializationError("read-only bind manifest set exceeds its retained limit")
    exclusive_write(args.result, encoded)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError, MaterializationError) as error:
        raise SystemExit(f"read-only bind materialization failed: {error}")
