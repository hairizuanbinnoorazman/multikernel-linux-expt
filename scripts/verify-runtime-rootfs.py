#!/usr/bin/env python3
"""Verify a deterministic newc/gzip archive against its canonical manifest."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import stat
from pathlib import Path


class VerificationError(Exception):
    pass


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise VerificationError(f"duplicate manifest key {key!r}")
        result[key] = value
    return result


def reject_constant(value: str):
    raise VerificationError(f"invalid JSON numeric constant {value}")


def align(value: int) -> int:
    return (value + 3) & ~3


def parse_newc(data: bytes) -> list[dict]:
    offset = 0
    entries: list[dict] = []
    while True:
        if offset + 110 > len(data) or data[offset:offset + 6] != b"070701":
            raise VerificationError(f"invalid newc header at offset {offset}")
        try:
            fields = [int(data[offset + 6 + index * 8:offset + 14 + index * 8], 16) for index in range(13)]
        except ValueError as error:
            raise VerificationError(f"invalid newc field at offset {offset}") from error
        ino, mode, uid, gid, nlink, mtime, size = fields[:7]
        namesize = fields[11]
        if namesize < 2:
            raise VerificationError("invalid newc name length")
        name_start = offset + 110
        name_end = name_start + namesize
        if name_end > len(data) or data[name_end - 1] != 0:
            raise VerificationError("unterminated newc name")
        name = os.fsdecode(data[name_start:name_end - 1])
        payload_start = align(name_end)
        payload_end = payload_start + size
        if payload_end > len(data):
            raise VerificationError(f"truncated newc payload for {name!r}")
        payload = data[payload_start:payload_end]
        offset = align(payload_end)
        if name == "TRAILER!!!":
            if size != 0 or any(data[offset:]):
                raise VerificationError("invalid data after newc trailer")
            break
        if name.startswith("/") or name == "" or any(part == ".." for part in name.split("/")):
            raise VerificationError(f"unsafe newc path {name!r}")
        entries.append({
            "path": name, "mode": mode, "uid": uid, "gid": gid,
            "nlink": nlink, "mtime": mtime, "size": size, "ino": ino,
            "data": payload,
        })
    if len({entry["path"] for entry in entries}) != len(entries):
        raise VerificationError("duplicate newc path")
    return entries


def verify(archive_path: Path, manifest_path: Path) -> dict:
    encoded = archive_path.read_bytes()
    if len(encoded) < 10 or encoded[:2] != b"\x1f\x8b" or encoded[4:8] != b"\0\0\0\0" or encoded[3] & 0x08:
        raise VerificationError("gzip header is not deterministic")
    try:
        unpacked = gzip.decompress(encoded)
    except (OSError, EOFError) as error:
        raise VerificationError(f"invalid gzip stream: {error}") from error
    actual = parse_newc(unpacked)
    try:
        manifest_bytes = manifest_path.read_bytes()
        manifest = json.loads(
            manifest_bytes,
            object_pairs_hook=strict_object,
            parse_constant=reject_constant,
        )
    except (OSError, json.JSONDecodeError, UnicodeDecodeError) as error:
        raise VerificationError(f"invalid manifest: {error}") from error
    if not isinstance(manifest, dict) or set(manifest) != {"schema_version", "normalization", "entries"}:
        raise VerificationError("manifest fields differ from schema")
    if manifest.get("schema_version") != 1 or isinstance(manifest.get("schema_version"), bool) or not isinstance(manifest.get("entries"), list):
        raise VerificationError("unsupported manifest schema")
    required_normalization = {
        "archive": "cpio-newc",
        "mtime": 0,
        "inode_assignment": "lexical-path-with-hardlink-groups",
        "xattrs": "rejected",
        "sparse_extents": "normalized-to-regular-bytes",
        "device_nodes": "rejected",
    }
    if manifest.get("normalization") != required_normalization:
        raise VerificationError("manifest normalization policy differs from builder contract")
    expected = manifest["entries"]
    if not all(isinstance(entry, dict) for entry in expected):
        raise VerificationError("manifest entries must be objects")
    if [entry.get("path") for entry in expected] != [entry["path"] for entry in actual]:
        raise VerificationError("archive path order/content differs from manifest")

    data_by_inode: dict[int, bytes] = {}
    for entry in actual:
        if entry["data"]:
            prior = data_by_inode.setdefault(entry["ino"], entry["data"])
            if prior != entry["data"]:
                raise VerificationError("one inode contains conflicting hardlink data")
    groups: dict[int, list[str]] = {}
    for entry in actual:
        groups.setdefault(entry["ino"], []).append(entry["path"])

    hardlinks: dict[str, tuple[int, set[str]]] = {}
    for wanted, observed in zip(expected, actual, strict=True):
        kind = wanted.get("type")
        allowed = {"path", "type", "mode", "uid", "gid", "size"}
        allowed |= {"sha256"} if kind == "regular" else {"target"} if kind == "symlink" else set()
        if kind == "regular":
            allowed.add("hardlink")
        if set(wanted) - allowed or not {"path", "type", "mode", "uid", "gid", "size"} <= set(wanted):
            raise VerificationError(f"invalid manifest fields for {observed['path']}")
        mode_kind = stat.S_IFMT(observed["mode"])
        required_kind = {"directory": stat.S_IFDIR, "regular": stat.S_IFREG, "symlink": stat.S_IFLNK}.get(kind)
        if required_kind is None or mode_kind != required_kind:
            raise VerificationError(f"type mismatch for {observed['path']}")
        for field in ("mode", "uid", "gid"):
            if wanted.get(field) != observed[field]:
                raise VerificationError(f"{field} mismatch for {observed['path']}")
        if observed["mtime"] != 0:
            raise VerificationError(f"nonzero mtime for {observed['path']}")
        content = data_by_inode.get(observed["ino"], observed["data"])
        logical_size = len(content) if kind == "regular" else observed["size"]
        if wanted.get("size") != logical_size:
            raise VerificationError(f"size mismatch for {observed['path']}")
        if kind == "regular" and hashlib.sha256(content).hexdigest() != wanted.get("sha256"):
            raise VerificationError(f"content digest mismatch for {observed['path']}")
        if kind == "symlink" and os.fsdecode(observed["data"]) != wanted.get("target"):
            raise VerificationError(f"symlink target mismatch for {observed['path']}")
        link = wanted.get("hardlink")
        if link:
            if not isinstance(link, str) or not link:
                raise VerificationError(f"invalid hardlink identity for {observed['path']}")
            hardlinks.setdefault(link, (observed["ino"], set()))[1].add(observed["path"])
        elif observed["nlink"] != (2 if kind == "directory" else 1):
            raise VerificationError(f"unexpected link count for {observed['path']}")
    for link, (inode, paths) in hardlinks.items():
        if set(groups[inode]) != paths or len(paths) != actual[[entry["ino"] for entry in actual].index(inode)]["nlink"]:
            raise VerificationError(f"hardlink group mismatch for {link}")
    return {
        "archive_sha256": hashlib.sha256(encoded).hexdigest(),
        "manifest_sha256": hashlib.sha256(manifest_bytes).hexdigest(),
        "entries": len(actual),
        "uncompressed_bytes": len(unpacked),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("archive", type=Path)
    parser.add_argument("manifest", type=Path)
    arguments = parser.parse_args()
    try:
        print(json.dumps(verify(arguments.archive, arguments.manifest), sort_keys=True))
    except (OSError, VerificationError) as error:
        print(f"rootfs verification failed: {error}", file=os.sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
