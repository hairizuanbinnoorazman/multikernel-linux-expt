#!/usr/bin/env python3
"""Bind an OCI entrypoint's architecture to the approved child-kernel manifest."""

from __future__ import annotations

import argparse
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


def resolve_in_root(root: Path, name: str) -> Path:
    if not name.startswith("/") or "\0" in name:
        raise ImageError("OCI argv[0] must be an absolute path")
    pending = list(PurePosixPath(name).parts[1:])
    resolved: list[str] = []
    links = 0
    while pending:
        component = pending.pop(0)
        if component in ("", "."):
            continue
        if component == "..":
            if not resolved:
                raise ImageError("entrypoint traversal escapes the OCI root")
            resolved.pop()
            continue
        candidate = root.joinpath(*resolved, component)
        info = candidate.lstat()
        if stat.S_ISLNK(info.st_mode):
            links += 1
            if links > 40:
                raise ImageError("entrypoint has too many symlinks")
            target = os.readlink(candidate)
            target_parts = list(PurePosixPath(target).parts)
            if target.startswith("/"):
                resolved = []
                target_parts = target_parts[1:]
            pending = target_parts + pending
        else:
            resolved.append(component)
    candidate = root.joinpath(*resolved)
    info = candidate.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o111 == 0:
        raise ImageError("resolved entrypoint is not an executable regular file")
    return candidate


def stable_bytes(path: Path) -> bytes:
    before = path.stat(follow_symlinks=False)
    with path.open("rb", buffering=0) as stream:
        data = stream.read()
        after_fd = os.fstat(stream.fileno())
    after = path.stat(follow_symlinks=False)
    identity = lambda item: (item.st_dev, item.st_ino, item.st_mode, item.st_uid, item.st_gid, item.st_size, item.st_mtime_ns, item.st_ctime_ns)
    if identity(before) != identity(after_fd) or identity(before) != identity(after):
        raise ImageError("entrypoint mutated during architecture validation")
    return data


def executable(root: Path, name: str, depth: int = 0) -> tuple[Path, bytes]:
    if depth > 4:
        raise ImageError("entrypoint interpreter chain is too deep")
    path = resolve_in_root(root, name)
    data = stable_bytes(path)
    if data.startswith(b"#!"):
        line = data.splitlines()[0][2:].strip().split()
        if not line:
            raise ImageError("entrypoint has an empty shebang")
        return executable(root, os.fsdecode(line[0]), depth + 1)
    return path, data


def validate(root: Path, config_path: Path, bootstrap_path: Path) -> dict:
    try:
        root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    except OSError as error:
        raise ImageError(f"cannot open OCI root without following symlinks: {root}: {error}") from error
    try:
        return _validate_held(Path(f"/proc/self/fd/{root_fd}"), config_path, bootstrap_path)
    finally:
        os.close(root_fd)


def _validate_held(root: Path, config_path: Path, bootstrap_path: Path) -> dict:
    config = json.loads(config_path.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    bootstrap = json.loads(bootstrap_path.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    if bootstrap.get("architecture") != "amd64":
        raise ImageError("selected kernel is not amd64")
    try:
        entrypoint = config["process"]["args"][0]
    except (KeyError, IndexError, TypeError) as error:
        raise ImageError("OCI process entrypoint is missing") from error
    resolved, data = executable(root, entrypoint)
    if len(data) < 20 or data[:5] != b"\x7fELF\x02" or data[5] != 1 or int.from_bytes(data[18:20], "little") != 62:
        raise ImageError("OCI entrypoint/interpreter is not a little-endian x86-64 ELF image")
    return {
        "architecture": "amd64",
        "entrypoint": entrypoint,
        "resolved_entrypoint": "/" + str(resolved.relative_to(root)),
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
