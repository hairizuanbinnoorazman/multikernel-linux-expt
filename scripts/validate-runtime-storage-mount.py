#!/usr/bin/env python3
"""Fail closed unless the runtime storage path is the approved mounted disk."""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import re
import stat
import sys


MOUNT_ESCAPES = {"\\040": " ", "\\011": "\t", "\\012": "\n", "\\134": "\\"}
UUID = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
TOKEN = re.compile(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}")


def decode_mount(value: str) -> str:
    for encoded, decoded in MOUNT_ESCAPES.items():
        value = value.replace(encoded, decoded)
    return value


def mount_entries(text: str):
    result = []
    for line in text.splitlines():
        left, separator, right = line.partition(" - ")
        left_fields = left.split()
        right_fields = right.split()
        if not separator or len(left_fields) < 6 or len(right_fields) < 3:
            raise ValueError("malformed mountinfo record")
        result.append({
            "device": left_fields[2],
            "mountpoint": decode_mount(left_fields[4]),
            "options": set(left_fields[5].split(",")),
            "type": right_fields[0],
            "source": decode_mount(right_fields[1]),
        })
    return result


def canonical_absolute(value: str, name: str) -> Path:
    path = Path(value)
    if not path.is_absolute() or os.path.normpath(value) != value or len(value) > 4096:
        raise ValueError(f"{name} must be a canonical absolute path")
    return path


def safe_directory(path: Path):
    current = Path("/")
    for component in path.parts[1:]:
        current /= component
        info = current.lstat()
        if (not stat.S_ISDIR(info.st_mode) or current.is_symlink() or
                info.st_uid != 0 or info.st_mode & 0o022):
            raise ValueError(f"unsafe storage mount directory: {current}")


def same_device_link(path: Path, device: Path, description: str):
    info = path.lstat()
    if not stat.S_ISLNK(info.st_mode) or info.st_uid != 0:
        raise ValueError(f"{description} must be a root-owned symlink")
    if path.resolve(strict=True) != device:
        raise ValueError(f"{description} resolves to a different device")


def udev_serial(text: str) -> str:
    values = [line.removeprefix("E:ID_SERIAL_SHORT=") for line in text.splitlines()
              if line.startswith("E:ID_SERIAL_SHORT=")]
    if len(values) != 1 or not TOKEN.fullmatch(values[0]):
        raise ValueError("udev storage serial is missing or malformed")
    return values[0]


def validate(args):
    mountpoint = canonical_absolute(args.mountpoint, "mountpoint")
    device_link = canonical_absolute(args.device, "device")
    if not TOKEN.fullmatch(args.serial) or not TOKEN.fullmatch(args.label):
        raise ValueError("storage serial or label is malformed")
    if not UUID.fullmatch(args.uuid):
        raise ValueError("storage UUID is malformed")
    if args.bytes <= 0 or args.bytes % 512:
        raise ValueError("storage byte size must be a positive sector multiple")

    link_info = device_link.lstat()
    if not stat.S_ISLNK(link_info.st_mode) or link_info.st_uid != 0:
        raise ValueError("storage by-id path must be a root-owned symlink")
    device = device_link.resolve(strict=True)
    device_info = device.stat()
    if not stat.S_ISBLK(device_info.st_mode) or device_info.st_uid != 0:
        raise ValueError("storage by-id target must be a root-owned block device")
    major, minor = os.major(device_info.st_rdev), os.minor(device_info.st_rdev)
    identity = f"{major}:{minor}"
    sys_block = Path("/sys/dev/block") / identity
    if (sys_block / "partition").exists():
        raise ValueError("runtime storage must be a whole disk")
    sectors = int((sys_block / "size").read_text(encoding="ascii").strip())
    if sectors * 512 != args.bytes:
        raise ValueError("runtime storage byte size mismatch")
    udev_path = Path("/run/udev/data") / f"b{identity}"
    udev_info = udev_path.lstat()
    if (not stat.S_ISREG(udev_info.st_mode) or udev_info.st_uid != 0 or
            udev_info.st_mode & 0o022 or not 0 < udev_info.st_size <= 1 << 20):
        raise ValueError("udev storage identity record is unsafe")
    if udev_serial(udev_path.read_text(encoding="utf-8")) != args.serial:
        raise ValueError("runtime storage serial mismatch")

    same_device_link(Path("/dev/disk/by-label") / args.label, device, "label link")
    same_device_link(Path("/dev/disk/by-uuid") / args.uuid, device, "UUID link")
    safe_directory(mountpoint)
    mount_info = Path("/proc/self/mountinfo").read_text(encoding="utf-8")
    entries = mount_entries(mount_info)
    selected = [item for item in entries if item["mountpoint"] == os.fspath(mountpoint)]
    if len(selected) != 1:
        raise ValueError("runtime storage path is not one distinct mountpoint")
    entry = selected[0]
    if entry["device"] != identity or entry["type"] != "ext4" or "rw" not in entry["options"]:
        raise ValueError("runtime storage mount device, type, or mode mismatch")
    if sum(item["device"] == identity for item in entries) != 1:
        raise ValueError("runtime storage device is mounted more than once")
    if mountpoint.stat().st_dev != device_info.st_rdev or Path("/").stat().st_dev == device_info.st_rdev:
        raise ValueError("runtime storage mount identity aliases the wrong filesystem")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mountpoint", required=True)
    parser.add_argument("--device", required=True)
    parser.add_argument("--serial", required=True)
    parser.add_argument("--bytes", required=True, type=int)
    parser.add_argument("--label", required=True)
    parser.add_argument("--uuid", required=True)
    args = parser.parse_args()
    try:
        validate(args)
    except (OSError, ValueError) as error:
        print(f"runtime storage mount rejected: {error}", file=sys.stderr)
        return 1
    print("RUNTIME_STORAGE_MOUNT_VALID")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
