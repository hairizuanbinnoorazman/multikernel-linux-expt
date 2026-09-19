#!/usr/bin/env python3
"""Linux no-replace publication with exact-identity rollback."""

from __future__ import annotations

import ctypes
import errno
import os
from pathlib import Path
import re
import shutil
import stat
import tempfile
from typing import BinaryIO, Callable


class PublicationError(Exception):
    pass


Identity = tuple[int, int]
PROC_FD = re.compile(r"/proc/self/fd/([0-9]+)")


def open_directory_nofollow(path: Path) -> int:
    text = os.fspath(path)
    inherited = PROC_FD.fullmatch(text)
    if inherited:
        descriptor = os.dup(int(inherited.group(1)))
    else:
        descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW)
    info = os.fstat(descriptor)
    if not stat.S_ISDIR(info.st_mode):
        os.close(descriptor)
        raise NotADirectoryError(errno.ENOTDIR, "descriptor is not a directory", text)
    return descriptor


def _rename_noreplace(source: Path, destination: Path) -> None:
    libc = ctypes.CDLL(None, use_errno=True)
    renameat2 = libc.renameat2
    renameat2.argtypes = (ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint)
    renameat2.restype = ctypes.c_int
    if renameat2(-100, os.fsencode(source), -100, os.fsencode(destination), 1) != 0:
        value = ctypes.get_errno()
        raise OSError(value, os.strerror(value), destination)


def file_identity(path: Path) -> Identity:
    value = path.stat(follow_symlinks=False)
    return value.st_dev, value.st_ino


def descriptor_identity(descriptor: int) -> Identity:
    value = os.fstat(descriptor)
    return value.st_dev, value.st_ino


def sync_directory(path: Path) -> None:
    directory = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(directory)
    finally:
        os.close(directory)


def remove_if_identity(path: Path, expected: Identity) -> bool:
    try:
        if file_identity(path) != expected:
            return False
    except FileNotFoundError:
        return False
    quarantine = path.with_name(f".{path.name}.rollback-{expected[0]:x}-{expected[1]:x}")
    try:
        _rename_noreplace(path, quarantine)
    except FileNotFoundError:
        return False
    if file_identity(quarantine) != expected:
        try:
            _rename_noreplace(quarantine, path)
        except OSError:
            pass
        raise PublicationError("output identity changed during rollback quarantine")
    quarantine.unlink()
    sync_directory(path.parent)
    return True


def remove_directory_if_identity(path: Path, expected: Identity) -> bool:
    try:
        value = path.stat(follow_symlinks=False)
        if (value.st_dev, value.st_ino) != expected or not stat.S_ISDIR(value.st_mode):
            return False
    except FileNotFoundError:
        return False
    quarantine = path.with_name(f".{path.name}.rollback-{expected[0]:x}-{expected[1]:x}")
    try:
        _rename_noreplace(path, quarantine)
    except FileNotFoundError:
        return False
    value = quarantine.stat(follow_symlinks=False)
    if (value.st_dev, value.st_ino) != expected or not stat.S_ISDIR(value.st_mode):
        try:
            _rename_noreplace(quarantine, path)
        except OSError:
            pass
        raise PublicationError("directory identity changed during cleanup quarantine")
    shutil.rmtree(quarantine)
    sync_directory(path.parent)
    return True


def publish_existing(source: Path, destination: Path, expected: Identity) -> Identity:
    if file_identity(source) != expected:
        raise PublicationError("temporary output identity changed before publication")
    published = False
    try:
        try:
            os.link(source, destination, follow_symlinks=False)
        except FileExistsError as error:
            raise PublicationError(f"refusing to overwrite existing output: {destination}") from error
        published = True
        if file_identity(destination) != expected:
            raise PublicationError("published output identity changed before verification")
        if not remove_if_identity(source, expected):
            try:
                file_identity(source)
            except FileNotFoundError:
                pass
            else:
                raise PublicationError("temporary output identity changed before cleanup")
        sync_directory(destination.parent)
        return expected
    except BaseException:
        if published:
            remove_if_identity(destination, expected)
        remove_if_identity(source, expected)
        raise


def atomic_write(path: Path, write: Callable[[BinaryIO], object]) -> Identity:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix="." + path.name + ".", dir=path.parent)
    temporary_path = Path(temporary)
    expected: Identity | None = None
    try:
        with os.fdopen(descriptor, "wb") as stream:
            write(stream)
            stream.flush()
            os.fchmod(stream.fileno(), 0o600)
            os.fsync(stream.fileno())
            expected = descriptor_identity(stream.fileno())
        return publish_existing(temporary_path, path, expected)
    except BaseException:
        if expected is not None:
            remove_if_identity(temporary_path, expected)
        raise
