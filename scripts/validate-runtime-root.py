#!/usr/bin/env python3
"""Resolve an OCI root path without traversal or symlinked path components."""

import argparse
import json
import os
from pathlib import Path, PurePosixPath
import stat


def beneath(path, anchor):
    try:
        path.relative_to(anchor)
        return True
    except ValueError:
        return False


def reject_symlink_components(path, anchor):
    relative = path.relative_to(anchor)
    current = anchor
    for part in relative.parts:
        current = current / part
        info = current.lstat()
        if stat.S_ISLNK(info.st_mode):
            raise ValueError(f"root path contains symlink component: {current}")


def resolve(bundle, configured, allowed_absolute):
    bundle = bundle.resolve(strict=True)
    raw = PurePosixPath(configured)
    if configured == "" or "\x00" in configured:
        raise ValueError("root.path is empty or contains NUL")
    if raw.as_posix() != configured or any(part in (".", "..") for part in raw.parts):
        raise ValueError("root.path contains traversal or non-canonical components")
    if raw.is_absolute():
        candidate = Path(configured)
        anchors = [Path(item).resolve(strict=True) for item in allowed_absolute]
        matching = [anchor for anchor in anchors if beneath(candidate, anchor)]
        if not matching:
            raise ValueError("absolute root.path is outside the configured container storage roots")
        anchor = max(matching, key=lambda item: len(item.parts))
    else:
        if not raw.parts or any(part == "" for part in raw.parts):
            raise ValueError("relative root.path contains traversal or non-canonical components")
        candidate, anchor = bundle.joinpath(*raw.parts), bundle
        if not beneath(candidate, bundle):
            raise ValueError("relative root.path escapes the bundle")
    reject_symlink_components(candidate, anchor)
    info = candidate.stat(follow_symlinks=False)
    if not stat.S_ISDIR(info.st_mode):
        raise ValueError("root.path is not a directory")
    return candidate


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("bundle", type=Path)
    parser.add_argument("config", type=Path)
    args = parser.parse_args()
    try:
        config = json.loads(args.config.read_text(encoding="utf-8"))
        allowed = [item for item in os.environ.get(
            "MK_ALLOWED_ABSOLUTE_ROOTS", "/var/lib/docker:/var/lib/containerd"
        ).split(":") if item]
        print(resolve(args.bundle, config["root"]["path"], allowed))
    except (OSError, KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise SystemExit(f"OCI root path rejected: {error}")


if __name__ == "__main__":
    main()
