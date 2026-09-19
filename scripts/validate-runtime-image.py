#!/usr/bin/env python3
"""Bind an OCI entrypoint's architecture to the approved child-kernel manifest."""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import stat
from pathlib import Path, PurePosixPath


class ImageError(Exception):
    pass


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ImageError(f"duplicate JSON field {key!r}")
        result[key] = value
    return result


def _readlink_descriptor(descriptor: int) -> str:
    libc = ctypes.CDLL(None, use_errno=True)
    readlinkat = libc.readlinkat
    readlinkat.argtypes = (ctypes.c_int, ctypes.c_char_p, ctypes.c_void_p, ctypes.c_size_t)
    readlinkat.restype = ctypes.c_ssize_t
    size = 256
    while size <= 1 << 20:
        buffer = ctypes.create_string_buffer(size)
        length = readlinkat(descriptor, b"", buffer, size)
        if length < 0:
            value = ctypes.get_errno()
            raise OSError(value, os.strerror(value))
        if length < size:
            return os.fsdecode(buffer.raw[:length])
        size *= 2
    raise ImageError("entrypoint symlink target is too large")


def resolve_in_root(root_fd: int, name: str) -> tuple[int, str]:
    if not name.startswith("/") or "\0" in name:
        raise ImageError("OCI argv[0] must be an absolute path")
    pending = list(PurePosixPath(name).parts[1:])
    resolved: list[str] = []
    directories = [os.dup(root_fd)]
    links = 0
    try:
        while pending:
            component = pending.pop(0)
            if component in ("", "."):
                continue
            if component == "..":
                if not resolved:
                    raise ImageError("entrypoint traversal escapes the OCI root")
                resolved.pop()
                os.close(directories.pop())
                continue
            candidate = os.open(
                component,
                os.O_PATH | os.O_NOFOLLOW | os.O_CLOEXEC,
                dir_fd=directories[-1],
            )
            info = os.fstat(candidate)
            if stat.S_ISLNK(info.st_mode):
                links += 1
                if links > 40:
                    os.close(candidate)
                    raise ImageError("entrypoint has too many symlinks")
                target = _readlink_descriptor(candidate)
                os.close(candidate)
                target_parts = list(PurePosixPath(target).parts)
                if target.startswith("/"):
                    while len(directories) > 1:
                        os.close(directories.pop())
                    resolved = []
                    target_parts = target_parts[1:]
                pending = target_parts + pending
                continue
            resolved.append(component)
            if pending:
                if not stat.S_ISDIR(info.st_mode):
                    os.close(candidate)
                    raise ImageError("entrypoint path contains a non-directory component")
                directories.append(candidate)
                continue
            if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o111 == 0:
                os.close(candidate)
                raise ImageError("resolved entrypoint is not an executable regular file")
            exact = os.open(f"/proc/self/fd/{candidate}", os.O_RDONLY | os.O_CLOEXEC)
            exact_info = os.fstat(exact)
            os.close(candidate)
            if (exact_info.st_dev, exact_info.st_ino) != (info.st_dev, info.st_ino):
                os.close(exact)
                raise ImageError("entrypoint identity changed while opening")
            return exact, "/".join(resolved)
        raise ImageError("resolved entrypoint is not an executable regular file")
    finally:
        for directory in reversed(directories):
            os.close(directory)


def stable_bytes(descriptor: int) -> bytes:
    before = os.fstat(descriptor)
    data = bytearray()
    offset = 0
    while True:
        chunk = os.pread(descriptor, 4 << 20, offset)
        if not chunk:
            break
        data.extend(chunk)
        offset += len(chunk)
    after = os.fstat(descriptor)
    identity = lambda item: (item.st_dev, item.st_ino, item.st_mode, item.st_uid, item.st_gid, item.st_size, item.st_mtime_ns, item.st_ctime_ns)
    if identity(before) != identity(after):
        raise ImageError("entrypoint mutated during architecture validation")
    return bytes(data)


def executable(root_fd: int, name: str, depth: int = 0) -> tuple[str, bytes]:
    if depth > 4:
        raise ImageError("entrypoint interpreter chain is too deep")
    descriptor, resolved = resolve_in_root(root_fd, name)
    try:
        data = stable_bytes(descriptor)
    finally:
        os.close(descriptor)
    if data.startswith(b"#!"):
        line = data.splitlines()[0][2:].strip().split()
        if not line:
            raise ImageError("entrypoint has an empty shebang")
        return executable(root_fd, os.fsdecode(line[0]), depth + 1)
    return resolved, data


def validate(root: Path, config_path: Path, bootstrap_path: Path) -> dict:
    try:
        root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    except OSError as error:
        raise ImageError(f"cannot open OCI root without following symlinks: {root}: {error}") from error
    try:
        return _validate_held(root_fd, config_path, bootstrap_path)
    finally:
        os.close(root_fd)


def _validate_held(root_fd: int, config_path: Path, bootstrap_path: Path) -> dict:
    config = json.loads(config_path.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    bootstrap = json.loads(bootstrap_path.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    if bootstrap.get("architecture") != "amd64":
        raise ImageError("selected kernel is not amd64")
    try:
        entrypoint = config["process"]["args"][0]
    except (KeyError, IndexError, TypeError) as error:
        raise ImageError("OCI process entrypoint is missing") from error
    resolved, data = executable(root_fd, entrypoint)
    if len(data) < 20 or data[:5] != b"\x7fELF\x02" or data[5] != 1 or int.from_bytes(data[18:20], "little") != 62:
        raise ImageError("OCI entrypoint/interpreter is not a little-endian x86-64 ELF image")
    return {
        "architecture": "amd64",
        "entrypoint": entrypoint,
        "resolved_entrypoint": "/" + resolved,
        "entrypoint_sha256": hashlib.sha256(data).hexdigest(),
        "kernel_manifest_sha256": bootstrap["manifest_sha256"],
        "kernel_release": bootstrap["kernel_release"],
        "required_kernel_config": bootstrap["required_config"],
        "supported_oci_features": bootstrap["oci_features"],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("config", type=Path)
    parser.add_argument("bootstrap", type=Path)
    arguments = parser.parse_args()
    try:
        print(json.dumps(validate(arguments.root, arguments.config, arguments.bootstrap), sort_keys=True))
    except (OSError, ImageError, json.JSONDecodeError) as error:
        print(f"image architecture validation failed: {error}", file=os.sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
