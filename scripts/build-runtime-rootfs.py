#!/usr/bin/env python3
"""Build and verify a deterministic Linux initramfs from a directory tree.

The emitted newc archive deliberately normalizes mtimes and inode numbers while
preserving file type, mode, uid/gid, hardlink relationships, symlink targets,
and regular-file bytes. File types and metadata that the runtime cannot safely
reproduce are rejected instead of being silently discarded.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import posixpath
import stat
from dataclasses import dataclass
from pathlib import Path
from typing import BinaryIO

from runtime_safe_publish import PublicationError, atomic_write, file_identity, remove_if_identity


class RootFSError(Exception):
    pass


OVERLAY_OPAQUE_XATTRS = {"trusted.overlay.opaque", "user.overlay.opaque"}


@dataclass(frozen=True)
class Entry:
    path: str
    mode: int
    uid: int
    gid: int
    size: int
    kind: str
    digest: str | None = None
    target: str | None = None
    hardlink: str | None = None


def _safe_target(path: str, target: str) -> bool:
    if target.startswith("/"):
        return True
    depth = len([part for part in posixpath.dirname(path).split("/") if part not in ("", ".")])
    for part in target.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if depth == 0:
                return False
            depth -= 1
        else:
            depth += 1
    return True


def _identity(value: os.stat_result) -> tuple[int, ...]:
    return (
        value.st_dev,
        value.st_ino,
        value.st_mode,
        value.st_uid,
        value.st_gid,
        value.st_size,
        value.st_mtime_ns,
        value.st_ctime_ns,
    )


def _read_stable(path: Path, before: os.stat_result) -> bytes:
    with path.open("rb", buffering=0) as stream:
        data = stream.read()
        after_fd = os.fstat(stream.fileno())
    after_path = path.stat(follow_symlinks=False)
    if _identity(before) != _identity(after_fd) or _identity(before) != _identity(after_path):
        raise RootFSError(f"input mutated while reading: {path}")
    return data


def _entry_stat(path: Path, relative: str) -> os.stat_result:
    # The synthetic root is a procfs magic link to an already-held descriptor.
    # Following that one link observes the held directory, not a public path.
    return path.stat(follow_symlinks=relative == ".")


def _revalidate_tree(root: Path, raw: list[tuple[str, Path, os.stat_result]]) -> None:
    """Reject membership or identity changes anywhere after the initial scan."""
    expected = {relative: _identity(info) for relative, _, info in raw}
    observed: dict[str, tuple[int, ...]] = {}
    stack = [(".", root)]
    try:
        while stack:
            relative, current = stack.pop()
            info = _entry_stat(current, relative)
            observed[relative] = _identity(info)
            if stat.S_ISDIR(info.st_mode):
                children = sorted(os.scandir(current), key=lambda item: os.fsencode(item.name), reverse=True)
                for child in children:
                    child_relative = child.name if relative == "." else relative + "/" + child.name
                    stack.append((child_relative, Path(child.path)))
    except OSError as error:
        raise RootFSError(f"input tree mutated during final validation: {error}") from error
    if expected.keys() != observed.keys():
        raise RootFSError("input tree membership mutated during build")
    for relative, identity in expected.items():
        if observed[relative] != identity:
            raise RootFSError(f"input mutated during build: {relative}")


def scan(root: Path) -> tuple[list[Entry], dict[str, bytes]]:
    try:
        root_fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    except OSError as error:
        raise RootFSError(f"cannot open root directory without following symlinks: {root}: {error}") from error
    try:
        return _scan_held(Path(f"/proc/self/fd/{root_fd}"))
    finally:
        os.close(root_fd)


def _scan_held(root: Path) -> tuple[list[Entry], dict[str, bytes]]:
    raw: list[tuple[str, Path, os.stat_result]] = []
    stack = [(".", root)]
    while stack:
        relative, current = stack.pop()
        info = _entry_stat(current, relative)
        raw.append((relative, current, info))
        if stat.S_ISDIR(info.st_mode):
            try:
                children = sorted(os.scandir(current), key=lambda item: os.fsencode(item.name), reverse=True)
            except OSError as error:
                raise RootFSError(f"cannot scan {current}: {error}") from error
            for child in children:
                if child.name in (".", "..") or "/" in child.name or "\x00" in child.name:
                    raise RootFSError(f"unsafe path component below {relative!r}: {child.name!r}")
                child_relative = child.name if relative == "." else relative + "/" + child.name
                stack.append((child_relative, Path(child.path)))

    raw.sort(key=lambda item: os.fsencode(item[0]))
    hardlink_groups: dict[tuple[int, int], list[str]] = {}
    for relative, _, info in raw:
        if stat.S_ISREG(info.st_mode) and info.st_nlink > 1:
            hardlink_groups.setdefault((info.st_dev, info.st_ino), []).append(relative)
    hardlink_ids = {
        key: "h" + hashlib.sha256("\0".join(paths).encode()).hexdigest()[:16]
        for key, paths in hardlink_groups.items()
    }
    # A link outside the admitted root would make the manifest an incomplete
    # description of the inode's ownership and content identity.
    for key, paths in hardlink_groups.items():
        observed = next(info.st_nlink for _, _, info in raw if (info.st_dev, info.st_ino) == key)
        if observed != len(paths):
            raise RootFSError(
                f"hardlink group crosses the root boundary: {paths[0]} has {observed} links, {len(paths)} admitted"
            )

    entries: list[Entry] = []
    contents: dict[str, bytes] = {}
    for relative, path, info in raw:
        try:
            xattrs = os.listxattr(path, follow_symlinks=relative == ".")
        except OSError as error:
            raise RootFSError(f"cannot inspect xattrs for {relative}: {error}") from error
        unsupported_xattrs = set(xattrs) - OVERLAY_OPAQUE_XATTRS
        if unsupported_xattrs:
            raise RootFSError(f"unsupported xattrs on {relative}: {','.join(sorted(unsupported_xattrs))}")
        for name in set(xattrs) & OVERLAY_OPAQUE_XATTRS:
            try:
                value = os.getxattr(path, name, follow_symlinks=relative == ".")
            except OSError as error:
                raise RootFSError(f"cannot inspect overlay opacity on {relative}: {error}") from error
            if not stat.S_ISDIR(info.st_mode) or value not in (b"y", b"x"):
                raise RootFSError(f"malformed overlay opacity metadata on {relative}")
        common = dict(path=relative, mode=info.st_mode, uid=info.st_uid, gid=info.st_gid, size=0)
        if stat.S_ISDIR(info.st_mode):
            entry = Entry(kind="directory", **common)
        elif stat.S_ISREG(info.st_mode):
            data = _read_stable(path, info)
            contents[relative] = data
            key = (info.st_dev, info.st_ino)
            entry = Entry(
                kind="regular",
                size=len(data),
                digest=hashlib.sha256(data).hexdigest(),
                hardlink=hardlink_ids.get(key),
                **{key: value for key, value in common.items() if key != "size"},
            )
        elif stat.S_ISLNK(info.st_mode):
            target = os.readlink(path)
            # readlink has no descriptor form in pathlib; bind the observed
            # target to the lstat identity on both sides of the read.
            after = path.stat(follow_symlinks=False)
            if (info.st_dev, info.st_ino, info.st_mode, info.st_uid, info.st_gid, info.st_mtime_ns, info.st_ctime_ns) != (
                after.st_dev, after.st_ino, after.st_mode, after.st_uid, after.st_gid, after.st_mtime_ns, after.st_ctime_ns
            ):
                raise RootFSError(f"input symlink mutated while reading: {relative}")
            if not _safe_target(relative, target):
                raise RootFSError(f"escaping symlink: {relative} -> {target}")
            encoded = os.fsencode(target)
            contents[relative] = encoded
            entry = Entry(kind="symlink", size=len(encoded), target=target, **{key: value for key, value in common.items() if key != "size"})
        else:
            raise RootFSError(f"unsupported file type at {relative}: mode {info.st_mode:#o}")
        entries.append(entry)
    _revalidate_tree(root, raw)
    return entries, contents


def manifest(entries: list[Entry]) -> bytes:
    body = {
        "schema_version": 1,
        "normalization": {
            "archive": "cpio-newc",
            "mtime": 0,
            "inode_assignment": "lexical-path-with-hardlink-groups",
            "xattrs": "rejected-except-realized-overlay-opacity",
            "overlay_opacity": "materialized-view-normalized",
            "sparse_extents": "normalized-to-regular-bytes",
            "device_nodes": "rejected",
        },
        "entries": [
            {key: value for key, value in {
                "path": item.path,
                "type": item.kind,
                "mode": item.mode & 0o177777,
                "uid": item.uid,
                "gid": item.gid,
                "size": item.size,
                "sha256": item.digest,
                "target": item.target,
                "hardlink": item.hardlink,
            }.items() if value is not None}
            for item in entries
        ],
    }
    return (json.dumps(body, sort_keys=True, separators=(",", ":")) + "\n").encode()


def _pad(stream: BinaryIO, count: int) -> None:
    stream.write(b"\0" * ((-count) % 4))


def write_newc(stream: BinaryIO, entries: list[Entry], contents: dict[str, bytes]) -> None:
    inode_by_link: dict[str, int] = {}
    next_inode = 1
    for entry in entries:
        if entry.hardlink and entry.hardlink in inode_by_link:
            continue
        if entry.hardlink:
            inode_by_link[entry.hardlink] = next_inode
        next_inode += 1
    link_counts: dict[str, int] = {}
    for entry in entries:
        if entry.hardlink:
            link_counts[entry.hardlink] = link_counts.get(entry.hardlink, 0) + 1
    next_inode = 1
    assigned: dict[str, int] = {}
    emitted_link_data: set[str] = set()

    def emit(name: str, mode: int, uid: int, gid: int, nlink: int, ino: int, data: bytes) -> None:
        encoded_name = os.fsencode(name) + b"\0"
        fields = (ino, mode, uid, gid, nlink, 0, len(data), 0, 0, 0, 0, len(encoded_name), 0)
        stream.write(b"070701" + b"".join(f"{field:08x}".encode() for field in fields))
        stream.write(encoded_name)
        _pad(stream, 110 + len(encoded_name))
        stream.write(data)
        _pad(stream, len(data))

    for entry in entries:
        if entry.hardlink:
            if entry.hardlink not in assigned:
                assigned[entry.hardlink] = next_inode
                next_inode += 1
            ino = assigned[entry.hardlink]
            nlink = link_counts[entry.hardlink]
            data = b"" if entry.hardlink in emitted_link_data else contents[entry.path]
            emitted_link_data.add(entry.hardlink)
        else:
            ino, nlink, data = next_inode, 2 if entry.kind == "directory" else 1, contents.get(entry.path, b"")
            next_inode += 1
        emit(entry.path, entry.mode, entry.uid, entry.gid, nlink, ino, data)
    emit("TRAILER!!!", 0, 0, 0, 1, next_inode, b"")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("root", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--manifest-only", action="store_true")
    parser.add_argument("--max-bytes", type=int, default=0)
    parser.add_argument("--max-inodes", type=int, default=0)
    parser.add_argument("--min-free-bytes", type=int, default=0)
    args = parser.parse_args()
    entries, contents = scan(args.root)
    payload_bytes = sum(item.size for item in entries if item.kind == "regular")
    if args.max_bytes and payload_bytes > args.max_bytes:
        raise RootFSError(f"payload bytes {payload_bytes} exceed limit {args.max_bytes}")
    if args.max_inodes and len(entries) > args.max_inodes:
        raise RootFSError(f"entry count {len(entries)} exceeds limit {args.max_inodes}")
    encoded_manifest = manifest(entries)
    if args.manifest_only:
        atomic_write(args.manifest, lambda stream: stream.write(encoded_manifest))
        print(json.dumps({
            "manifest_sha256": hashlib.sha256(encoded_manifest).hexdigest(),
            "entries": len(entries),
            "payload_bytes": payload_bytes,
        }, sort_keys=True))
        return 0
    available = os.statvfs(args.output.parent).f_bavail * os.statvfs(args.output.parent).f_frsize
    worst_case_archive = payload_bytes + len(entries) * 512 + 1024
    if available - worst_case_archive < args.min_free_bytes:
        raise RootFSError(
            f"high-water refusal: available {available}, worst-case archive {worst_case_archive}, "
            f"required reserve {args.min_free_bytes}"
        )
    # All admission checks precede both outputs. A refused build therefore does
    # not leave a plausible manifest without its corresponding archive.
    def archive(stream: BinaryIO) -> None:
        with gzip.GzipFile(filename="", mode="wb", fileobj=stream, compresslevel=9, mtime=0) as compressed:
            write_newc(compressed, entries, contents)

    archive_identity = atomic_write(args.output, archive)
    try:
        atomic_write(args.manifest, lambda stream: stream.write(encoded_manifest))
    except BaseException:
        remove_if_identity(args.output, archive_identity)
        raise
    print(json.dumps({
        "manifest_sha256": hashlib.sha256(encoded_manifest).hexdigest(),
        "initramfs_sha256": hashlib.sha256(args.output.read_bytes()).hexdigest(),
        "entries": len(entries),
        "payload_bytes": payload_bytes,
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (PublicationError, RootFSError) as error:
        raise SystemExit(f"rootfs validation failed: {error}")
