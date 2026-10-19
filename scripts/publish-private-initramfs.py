#!/usr/bin/env python3
"""Publish a streamed bootstrap archive privately without replacing a destination."""
from pathlib import Path
import os
import stat
import sys

# This script is called with -I: only its trusted adjacent publication helper
# is added, never cwd or environment-controlled Python search paths.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from runtime_safe_publish import atomic_write


def open_parent(path):
    absolute = os.path.abspath(path)
    if '..' in Path(path).parts:
        raise ValueError('parent traversal is not allowed')
    parent, name = os.path.split(absolute)
    if not name or name in {'.', '..'}:
        raise ValueError('an output filename is required')
    current = os.open('/', os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for component in Path(parent).parts[1:]:
            following = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW, dir_fd=current)
            os.close(current)
            current = following
            info = os.fstat(current)
            sticky_root = info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX)
            if info.st_uid not in {0, os.geteuid()} or (info.st_mode & 0o022 and not sticky_root):
                raise ValueError('output parent ownership or permissions are unsafe')
        return current, name
    except BaseException:
        os.close(current)
        raise


def publish(path, source):
    parent, name = open_parent(path)
    try:
        # Every publication and cleanup uses the held parent inode. Renaming
        # ancestors cannot redirect either the temporary file or final name.
        destination = Path(f'/proc/self/fd/{parent}') / name
        def write(stream):
            while chunk := source.read(1 << 20):
                stream.write(chunk)
        atomic_write(destination, write)
    finally:
        os.close(parent)


if __name__ == '__main__':
    try:
        if len(sys.argv) != 2:
            raise ValueError('usage: publish-private-initramfs.py OUTPUT')
        publish(sys.argv[1], sys.stdin.buffer)
    except Exception:
        print('private initramfs publication failed; existing output was preserved', file=sys.stderr)
        sys.exit(1)
