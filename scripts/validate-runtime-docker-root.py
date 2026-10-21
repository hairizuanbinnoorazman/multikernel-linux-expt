#!/usr/bin/env python3
"""Bind an approved Docker profile to a held-root scan and safe copied inputs."""
import hashlib
import bisect
import json
import os
from pathlib import Path, PurePosixPath
import stat
import sys

LIMIT = 64 << 20

class ApprovalError(Exception):
    pass


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ApprovalError("duplicate metadata field")
        result[key] = value
    return result


def read_metadata(path):
    fd = os.open(path, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid()
                or before.st_nlink != 1 or before.st_mode & 0o022
                or not 0 < before.st_size <= LIMIT):
            raise ApprovalError("unsafe generated metadata")
        with os.fdopen(os.dup(fd), "rb") as stream:
            data = stream.read(LIMIT + 1)
        after = os.fstat(fd)
        identity = lambda v: (v.st_dev, v.st_ino, v.st_mode, v.st_uid, v.st_gid,
                              v.st_nlink, v.st_size, v.st_mtime_ns, v.st_ctime_ns)
        if len(data) != before.st_size or identity(before) != identity(after):
            raise ApprovalError("generated metadata changed during read")
        return data
    finally:
        os.close(fd)


def validate(bootstrap, root_bytes, plan):
    profile = bootstrap.get("docker_guest")
    if not isinstance(profile, dict) or profile.get("kind") != "docker-rootless-offline-v1":
        raise ApprovalError("unsupported Docker approval")
    digest = hashlib.sha256(root_bytes).hexdigest()
    if digest != profile.get("rootfs_manifest_sha256"):
        raise ApprovalError("held source root differs from approved image")
    root = json.loads(root_bytes, object_pairs_hook=strict_object)
    if not isinstance(root, dict):
        raise ApprovalError("malformed source manifest")
    entries = root.get("entries")
    if root.get("schema_version") != 1 or not isinstance(entries, list) or len(entries) > 131072:
        raise ApprovalError("malformed source manifest")
    paths = {}
    for entry in entries:
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str) or entry["path"] in paths:
            raise ApprovalError("malformed source entries")
        path = entry["path"]
        if (path.startswith("/") or "\0" in path or len(path) > 4096
                or str(PurePosixPath(path)) != path or ".." in PurePosixPath(path).parts
                or entry.get("type") not in ("directory", "regular", "symlink")):
            raise ApprovalError("noncanonical source entry")
        paths[entry["path"]] = entry
    aliases = sorted(path for path, entry in paths.items()
                     if entry["type"] == "symlink" or entry.get("hardlink") is not None)
    if not isinstance(plan, dict):
        raise ApprovalError("malformed copied-input plan")
    binds = plan.get("readonly_binds")
    if plan.get("schema_version") != 1 or not isinstance(binds, list) or len(binds) > 256:
        raise ApprovalError("malformed copied-input plan")
    for item in binds:
        destination = item.get("destination") if isinstance(item, dict) else None
        if (not isinstance(destination, str) or not destination.startswith("/")
                or destination.startswith("//") or "\0" in destination
                or len(destination) > 4096 or str(PurePosixPath(destination)) != destination
                or ".." in PurePosixPath(destination).parts):
            raise ApprovalError("noncanonical copied-input destination")
        context = destination == "/work" or destination.startswith("/work/")
        if not context and destination not in ("/etc/hosts", "/etc/hostname", "/etc/resolv.conf"):
            raise ApprovalError("copied input can replace a trusted Docker dependency")
        parts = PurePosixPath(destination).parts[1:]
        for index in range(1, len(parts) + 1):
            entry = paths.get("/".join(parts[:index]))
            if entry is not None and (entry.get("type") == "symlink"
                    or entry.get("hardlink") is not None
                    or index < len(parts) and entry.get("type") != "directory"):
                raise ApprovalError("copied input traverses an image alias")
        prefix = "/".join(parts) + "/"
        index = bisect.bisect_left(aliases, prefix)
        if index < len(aliases) and aliases[index].startswith(prefix):
            raise ApprovalError("copied-input tree contains an image alias")
    return {"schema_version": 1, "source_bound": True,
            "rootfs_manifest_sha256": digest, "copied_input_count": len(binds)}


def main():
    if len(sys.argv) != 4:
        raise ApprovalError("bootstrap, held-root manifest and bind plan required")
    bootstrap = json.loads(read_metadata(sys.argv[1]), object_pairs_hook=strict_object)
    root = read_metadata(sys.argv[2])
    plan = json.loads(read_metadata(sys.argv[3]), object_pairs_hook=strict_object)
    print(json.dumps(validate(bootstrap, root, plan), sort_keys=True))

if __name__ == "__main__":
    try:
        main()
    except (ApprovalError, OSError, ValueError, TypeError, AttributeError) as error:
        # Do not reflect host source paths, destinations or malformed payloads.
        message = str(error) if isinstance(error, ApprovalError) else "invalid generated metadata"
        print("Docker root approval rejected: " + message, file=sys.stderr)
        sys.exit(1)
