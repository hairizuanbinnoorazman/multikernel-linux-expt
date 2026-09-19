#!/usr/bin/env python3
"""Materialize admitted read-only OCI bind inputs into a private root.

The original host paths are never projected into the child.  Each source is
manifested before and after an archive-semantic copy, and the copied object must
have the same normalized manifest before it can be published.
"""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path, PurePosixPath
import shutil
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


def open_real_source(path: Path, name: str) -> tuple[int, bool]:
    if not path.is_absolute() or Path(os.path.normpath(path)) != path or path == Path("/"):
        raise MaterializationError(f"{name} must be absolute and canonical")
    descriptor = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for index, component in enumerate(path.parts[1:]):
            final = index == len(path.parts[1:]) - 1
            flags = os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW
            if not final:
                flags |= os.O_DIRECTORY
            try:
                following = os.open(component, flags, dir_fd=descriptor)
            except OSError as error:
                raise MaterializationError(
                    f"{name} must not traverse a symlink, non-directory, or unavailable component: {error}"
                ) from error
            os.close(descriptor)
            descriptor = following
        try:
            info = os.fstat(descriptor)
        except OSError as error:
            raise MaterializationError(f"{name} is unavailable: {error}") from error
        if stat.S_ISDIR(info.st_mode):
            return descriptor, True
        if stat.S_ISREG(info.st_mode) and info.st_nlink == 1:
            return descriptor, False
        raise MaterializationError(f"{name} must be a real directory or private single-link regular file")
    except BaseException:
        os.close(descriptor)
        raise


def manifest_descriptor(builder, descriptor: int, is_directory: bool) -> tuple[bytes, int, int]:
    held = Path(f"/proc/self/fd/{descriptor}")
    if is_directory:
        entries, _ = builder._scan_held(held)
    else:
        before = os.fstat(descriptor)
        if not stat.S_ISREG(before.st_mode) or before.st_nlink != 1:
            raise MaterializationError("read-only bind regular file must have exactly one link")
        try:
            xattrs = os.listxattr(descriptor)
        except OSError as error:
            raise MaterializationError(f"cannot inspect bind-file xattrs: {error}") from error
        if xattrs:
            raise MaterializationError("read-only bind regular file has unsupported xattrs")
        data = bytearray()
        offset = 0
        while True:
            chunk = os.pread(descriptor, 4 << 20, offset)
            if not chunk:
                break
            data.extend(chunk)
            offset += len(chunk)
        after = os.fstat(descriptor)
        if builder._identity(before) != builder._identity(after) or after.st_nlink != 1:
            raise MaterializationError("read-only bind regular file mutated during manifesting")
        entries = [builder.Entry(
            path=".", mode=before.st_mode, uid=before.st_uid, gid=before.st_gid,
            size=len(data), kind="regular", digest=hashlib.sha256(data).hexdigest(),
        )]
    payload_bytes = sum(item.size for item in entries if item.kind == "regular")
    return builder.manifest(entries), payload_bytes, len(entries)


def manifest_bytes(builder, root: Path) -> tuple[bytes, int, int]:
    info = root.stat(follow_symlinks=False)
    if stat.S_ISDIR(info.st_mode):
        entries, _ = builder.scan(root)
    elif stat.S_ISREG(info.st_mode):
        if info.st_nlink != 1:
            raise MaterializationError("read-only bind regular file must have exactly one link")
        try:
            xattrs = os.listxattr(root, follow_symlinks=False)
        except OSError as error:
            raise MaterializationError(f"cannot inspect bind-file xattrs: {error}") from error
        if xattrs:
            raise MaterializationError("read-only bind regular file has unsupported xattrs")
        data = builder._read_stable(root, info)
        entries = [builder.Entry(
            path=".", mode=info.st_mode, uid=info.st_uid, gid=info.st_gid,
            size=len(data), kind="regular", digest=hashlib.sha256(data).hexdigest(),
        )]
        after = root.stat(follow_symlinks=False)
        if builder._identity(info) != builder._identity(after) or after.st_nlink != 1:
            raise MaterializationError("read-only bind regular file mutated during manifesting")
    else:
        raise MaterializationError("read-only bind source changed to an unsupported type")
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


def destination_path(root: Path, destination: str, source_is_directory: bool) -> Path:
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
        info = target.lstat()
    except FileNotFoundError:
        if source_is_directory:
            target.mkdir(mode=0o755)
        else:
            descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600)
            os.close(descriptor)
    else:
        if stat.S_ISLNK(info.st_mode) or (source_is_directory and not stat.S_ISDIR(info.st_mode)) or (
                not source_is_directory and not stat.S_ISREG(info.st_mode)):
            raise MaterializationError(f"existing bind destination type differs from its source: {destination}")
        # This is the private staging copy, not the caller-owned OCI snapshot.
        # Replacing its verified object reproduces the hiding semantics of a
        # bind mount without exposing the host source inside the child.
        if source_is_directory:
            shutil.rmtree(target)
            target.mkdir(mode=0o755)
        else:
            target.unlink()
            descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600)
            os.close(descriptor)
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
    root_fd, root_is_directory = open_real_source(args.target_root, "materialization root")
    try:
        if not root_is_directory:
            raise MaterializationError("materialization root must be a real directory")
        root = Path(f"/proc/self/fd/{root_fd}")
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
            source_fd, source_is_directory = open_real_source(
                Path(item["source"]), f"read-only bind source {index}"
            )
            try:
                before, payload_bytes, inodes = manifest_descriptor(builder, source_fd, source_is_directory)
                total_bytes += payload_bytes
                total_inodes += inodes
                if total_bytes > args.max_bytes or total_inodes > args.max_inodes:
                    raise MaterializationError("read-only bind inputs exceed the aggregate byte or inode limit")
                target = destination_path(root, destination, source_is_directory)
                held_source = f"/proc/self/fd/{source_fd}"
                copy_source = held_source + "/." if source_is_directory else held_source
                copy_options = ["--archive"]
                if not source_is_directory:
                    copy_options.append("--dereference")
                completed = subprocess.run(
                    ["/bin/cp", *copy_options, "--reflink=never", "--", copy_source, str(target)],
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False,
                    pass_fds=(source_fd, root_fd),
                )
                if completed.returncode != 0:
                    raise MaterializationError(f"copy read-only bind {index} failed")
                after, _, _ = manifest_descriptor(builder, source_fd, source_is_directory)
                copied, _, _ = manifest_bytes(builder, target)
                if before != after:
                    raise MaterializationError(f"read-only bind source {index} mutated during materialization")
                if before != copied:
                    raise MaterializationError(f"read-only bind copy {index} differs from its admitted source")
            finally:
                os.close(source_fd)
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
    finally:
        os.close(root_fd)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError, MaterializationError) as error:
        raise SystemExit(f"read-only bind materialization failed: {error}")
