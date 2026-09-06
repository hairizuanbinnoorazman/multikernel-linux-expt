#!/usr/bin/env python3
"""Build a reproducible, fully allocated ext4 sandbox root and identity record."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import tempfile


IDENTITY = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")
UUID = re.compile(r"^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$")


class StorageBuildError(Exception):
    pass


def digest(path: Path) -> str:
    result = hashlib.sha256()
    with path.open("rb", buffering=0) as stream:
        for chunk in iter(lambda: stream.read(4 << 20), b""):
            result.update(chunk)
    return result.hexdigest()


def inspect_ext4(path: Path, filesystem_uuid: str, size: int, inodes: int) -> None:
    info = path.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_size != size or info.st_blocks * 512 < size:
        raise StorageBuildError("ext4 image is not a fully allocated regular file of the requested quota")
    with path.open("rb", buffering=0) as stream:
        stream.seek(1024)
        superblock = stream.read(1024)
    if len(superblock) != 1024 or int.from_bytes(superblock[0x38:0x3A], "little") != 0xEF53:
        raise StorageBuildError("generated image has no ext4 superblock")
    observed_uuid = "-".join((
        superblock[0x68:0x6C].hex(), superblock[0x6C:0x6E].hex(),
        superblock[0x6E:0x70].hex(), superblock[0x70:0x72].hex(),
        superblock[0x72:0x78].hex(),
    ))
    if observed_uuid != filesystem_uuid:
        raise StorageBuildError("generated ext4 UUID differs from requested identity")
    if int.from_bytes(superblock[0:4], "little") != inodes:
        raise StorageBuildError("generated ext4 inode quota differs from requested identity")
    if int.from_bytes(superblock[0x3A:0x3C], "little") & 1 == 0:
        raise StorageBuildError("generated ext4 filesystem is not clean")


def atomic_json(path: Path, value: dict) -> None:
    descriptor, temporary = tempfile.mkstemp(prefix="." + path.name + ".", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as stream:
            json.dump(value, stream, separators=(",", ":"), sort_keys=True)
            stream.write("\n")
            stream.flush()
            os.fsync(stream.fileno())
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    except BaseException:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass
        raise


def build(arguments) -> dict:
    root = arguments.root.resolve(strict=True)
    if not root.is_dir() or arguments.output.exists() or arguments.metadata.exists():
        raise StorageBuildError("root must be a directory and output paths must not already exist")
    if not IDENTITY.fullmatch(arguments.image_id) or not UUID.fullmatch(arguments.uuid):
        raise StorageBuildError("image ID or UUID is malformed")
    if (arguments.size < 64 << 20 or arguments.size > 16 << 30 or arguments.size % 4096 or
            arguments.inodes < 128 or arguments.inodes > 2_097_152 or arguments.port < 1024 or arguments.port > 65535):
        raise StorageBuildError("size, inode quota, or export port is outside the supported bounds")
    arguments.output.parent.mkdir(parents=True, exist_ok=True)
    arguments.metadata.parent.mkdir(parents=True, exist_ok=True)
    free = shutil.disk_usage(arguments.output.parent).free
    if free < arguments.size + arguments.min_free_bytes:
        raise StorageBuildError(f"storage high-water refusal: free={free} required={arguments.size + arguments.min_free_bytes}")

    descriptor, temporary_name = tempfile.mkstemp(prefix="." + arguments.output.name + ".", dir=arguments.output.parent)
    os.close(descriptor)
    temporary = Path(temporary_name)
    completed = False
    try:
        os.chmod(temporary, 0o600)
        with temporary.open("r+b", buffering=0) as stream:
            os.posix_fallocate(stream.fileno(), 0, arguments.size)
            stream.flush()
            os.fsync(stream.fileno())
        environment = {
            "PATH": "/usr/sbin:/usr/bin:/sbin:/bin",
            "LANG": "C", "LC_ALL": "C", "E2FSPROGS_FAKE_TIME": "0", "SOURCE_DATE_EPOCH": "0",
        }
        command = [
            arguments.mke2fs, "-q", "-F", "-t", "ext4", "-m", "0",
            "-E", f"nodiscard,lazy_itable_init=0,lazy_journal_init=0,hash_seed={arguments.uuid}",
            "-N", str(arguments.inodes), "-U", arguments.uuid, "-L", "mk-runtime",
            "-d", str(root), str(temporary),
        ]
        result = subprocess.run(command, env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False)
        if result.returncode:
            raise StorageBuildError(f"mke2fs failed: {result.stderr.strip()}")
        inspect_ext4(temporary, arguments.uuid, arguments.size, arguments.inodes)
        check = subprocess.run([arguments.e2fsck, "-fn", str(temporary)], env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        if check.returncode:
            raise StorageBuildError(f"offline filesystem validation failed with status {check.returncode}")
        image_digest = digest(temporary)
        check_digest = hashlib.sha256(check.stdout).hexdigest()
        os.replace(temporary, arguments.output)
        directory = os.open(arguments.output.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
        record = {
            "schema_version": 1, "path": str(arguments.output.resolve()), "image_id": arguments.image_id,
            "filesystem_uuid": arguments.uuid, "size_bytes": arguments.size, "quota_bytes": arguments.size,
            "inode_limit": arguments.inodes, "port": arguments.port, "sha256": image_digest,
            "offline_check_sha256": check_digest, "allocation": "posix_fallocate", "format": "ext4",
            "determinism": {"fake_time": 0, "hash_seed": arguments.uuid, "lazy_initialization": False},
        }
        atomic_json(arguments.metadata, record)
        completed = True
        return record
    finally:
        if not completed:
            temporary.unlink(missing_ok=True)
            arguments.output.unlink(missing_ok=True)
            arguments.metadata.unlink(missing_ok=True)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("metadata", type=Path)
    parser.add_argument("--image-id", required=True)
    parser.add_argument("--uuid", required=True)
    parser.add_argument("--size", type=int, default=1 << 30)
    parser.add_argument("--inodes", type=int, default=131072)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--min-free-bytes", type=int, default=1 << 30)
    parser.add_argument("--mke2fs", default="/usr/sbin/mke2fs")
    parser.add_argument("--e2fsck", default="/usr/sbin/e2fsck")
    arguments = parser.parse_args()
    try:
        print(json.dumps(build(arguments), separators=(",", ":"), sort_keys=True))
    except (OSError, StorageBuildError) as error:
        print(f"storage build failed: {error}", file=os.sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
