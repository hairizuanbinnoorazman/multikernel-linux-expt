#!/usr/bin/env python3
"""Prove that DAXFS validate rejects a corrupted inode in a copied image."""

import ctypes
import os
import struct
import sys

from kerf.daxfs.mkdaxfs import (
    FSCONFIG_CMD_CREATE,
    FSCONFIG_SET_FD,
    FSCONFIG_SET_STRING,
    SYS_FSCONFIG,
    SYS_FSOPEN,
    _allocate_dma_heap,
    _get_libc,
    _syscall,
)


FSCONFIG_SET_FLAG = 0
SUPERBLOCK_FORMAT = "<IIIIQQQQIIQQQIIQQII"


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} STATIC_DAXFS_IMAGE", file=sys.stderr)
        return 2

    with open(sys.argv[1], "rb") as image_file:
        image = bytearray(image_file.read())

    superblock = struct.unpack_from(SUPERBLOCK_FORMAT, image)
    base_offset = superblock[5]
    root_mode_offset = base_offset + 4
    original_mode = struct.unpack_from("<I", image, root_mode_offset)[0]
    struct.pack_into("<I", image, root_mode_offset, 0)
    print(
        f"corrupted copied root inode mode at 0x{root_mode_offset:x}: "
        f"0x{original_mode:x} -> 0"
    )

    dmabuf_fd, memory = _allocate_dma_heap(
        "/dev/dma_heap/system", len(image)
    )
    fs_fd = -1
    try:
        memory.write(image)
        memory.close()

        libc = _get_libc()
        fs_fd = _syscall(libc, SYS_FSOPEN, ctypes.c_char_p(b"daxfs"), 0)
        _syscall(
            libc,
            SYS_FSCONFIG,
            fs_fd,
            FSCONFIG_SET_FD,
            ctypes.c_char_p(b"dmabuf"),
            0,
            dmabuf_fd,
        )
        _syscall(
            libc,
            SYS_FSCONFIG,
            fs_fd,
            FSCONFIG_SET_STRING,
            ctypes.c_char_p(b"name"),
            ctypes.c_char_p(b"corruption-test"),
            0,
        )
        _syscall(
            libc,
            SYS_FSCONFIG,
            fs_fd,
            FSCONFIG_SET_FLAG,
            ctypes.c_char_p(b"validate"),
            0,
            0,
        )
        try:
            _syscall(libc, SYS_FSCONFIG, fs_fd, FSCONFIG_CMD_CREATE, 0, 0, 0)
        except OSError as error:
            print(
                "CORRUPTED_IMAGE_REJECTED "
                f"errno={error.errno} message={error.strerror}"
            )
            return 0

        print("CORRUPTED_IMAGE_ACCEPTED_UNEXPECTEDLY", file=sys.stderr)
        return 1
    finally:
        if fs_fd >= 0:
            os.close(fs_fd)
        os.close(dmabuf_fd)


if __name__ == "__main__":
    raise SystemExit(main())
