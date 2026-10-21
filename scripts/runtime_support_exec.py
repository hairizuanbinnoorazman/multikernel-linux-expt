#!/usr/bin/env python3
"""Execute preflighted builder sources without implicit imports or bytecode."""

import os
from pathlib import Path
import stat
import sys
import types

LIMIT = 4 << 20
HELPERS = frozenset((
    "validate-runtime-oci.py", "validate-runtime-bootstrap.py",
    "validate-runtime-root.py", "validate-runtime-image.py", "build-runtime-rootfs.py",
    "build-runtime-storage.py", "materialize-runtime-binds.py",
    "runtime-storage-identity.py", "verify-runtime-rootfs.py", "validate-runtime-docker-root.py",
))


class SupportError(Exception):
    pass


def source_identity(info):
    return (info.st_dev, info.st_ino, info.st_mode, info.st_uid, info.st_gid,
            info.st_nlink, info.st_size, info.st_mtime_ns, info.st_ctime_ns)


def read_source(directory, name):
    descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK,
                         dir_fd=directory)
    try:
        before = os.fstat(descriptor)
        if (not stat.S_ISREG(before.st_mode) or before.st_uid not in (0, os.geteuid())
                or before.st_mode & 0o022 or before.st_nlink != 1
                or not 0 < before.st_size <= LIMIT):
            raise SupportError("unsafe support source")
        with os.fdopen(os.dup(descriptor), "rb") as stream:
            source = stream.read(LIMIT + 1)
        if len(source) != before.st_size or source_identity(os.fstat(descriptor)) != source_identity(before):
            raise SupportError("support source changed during read")
        return source
    finally:
        os.close(descriptor)


def execute_module(name, path, source):
    module = types.ModuleType(name)
    module.__file__ = str(path)
    module.__package__ = None
    sys.modules[name] = module
    # Compile the held source bytes explicitly. Do not read or create a sibling
    # .pyc, add the generation to sys.path, or invoke an import loader for it.
    exec(compile(source, str(path), "exec"), module.__dict__)
    return module


def main():
    if not sys.flags.isolated or not sys.flags.no_site or len(sys.argv) < 2:
        raise SupportError("isolated no-site launch and helper name are required")
    helper = sys.argv[1]
    if helper not in HELPERS:
        raise SupportError("unsupported helper")
    root = Path(__file__).resolve().parent
    directory = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        # Acquire both before executing either, so a bad helper cannot trigger
        # support code execution merely by requesting the runner.
        publication = read_source(directory, "runtime_safe_publish.py")
        selected = read_source(directory, helper)
    finally:
        os.close(directory)
    sys.dont_write_bytecode = True
    execute_module("runtime_safe_publish", root / "runtime_safe_publish.py", publication)
    sys.argv = [str(root / helper), *sys.argv[2:]]
    execute_module("__main__", root / helper, selected)


if __name__ == "__main__":
    try:
        main()
    except (SupportError, OSError) as error:
        print("builder support execution rejected: " + type(error).__name__, file=sys.stderr)
        sys.exit(1)
